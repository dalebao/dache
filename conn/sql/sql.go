// Package sqlconn provides a database/sql-based Connector for MySQL, PostgreSQL, etc.
package sqlconn

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// Connector implements localcache.Connector using database/sql.
// Works with any registered driver (mysql, postgres, sqlite3, etc.).
type Connector struct {
	name   string
	db     *sql.DB
	mu     sync.RWMutex
	closed bool
}

// New creates a Connector from an existing *sql.DB.
//
//	db, _ := sql.Open("mysql", dsn)
//	conn := sqlconn.New("mysql", db)
func New(name string, db *sql.DB) *Connector {
	return &Connector{name: name, db: db}
}

// Open creates a Connector by opening a database connection.
// The driver must be registered before calling Open.
func Open(driverName, dataSourceName string) (*Connector, error) {
	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("sqlconn open %s: %w", driverName, err)
	}
	return New(driverName, db), nil
}

func (c *Connector) Name() string { return c.name }

// ScanAll executes "SELECT * FROM table" and scans results into dest.
// dest must be a pointer to a slice of structs, e.g. *[]User.
// Column names are matched to struct field names (case-insensitive) or "db" tags.
func (c *Connector) ScanAll(ctx context.Context, table string, dest any) error {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return fmt.Errorf("sqlconn %s: connector is closed", c.name)
	}
	c.mu.RUnlock()

	query := fmt.Sprintf("SELECT * FROM %s", table)
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("sqlconn %s: query %q: %w", c.name, query, err)
	}
	defer rows.Close()

	return scanRows(rows, dest, c.name)
}

func scanRows(rows *sql.Rows, dest any, connName string) error {
	destVal := reflect.ValueOf(dest)
	if destVal.Kind() != reflect.Ptr || destVal.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("sqlconn %s: dest must be pointer to slice, got %T", connName, dest)
	}

	sliceVal := destVal.Elem()
	elemType := sliceVal.Type().Elem()
	if elemType.Kind() == reflect.Ptr {
		elemType = elemType.Elem()
	}
	if elemType.Kind() != reflect.Struct {
		return fmt.Errorf("sqlconn %s: slice element must be struct, got %s", connName, elemType)
	}

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("sqlconn %s: columns: %w", connName, err)
	}
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		colTypes = nil // non-fatal
	}

	// Build column → field index mapping
	fieldMap := buildFieldMap(elemType, columns)

	for rows.Next() {
		elem := reflect.New(elemType).Elem()
		scanTargets := make([]any, len(columns))

		for i, col := range columns {
			fieldIdx, ok := fieldMap[strings.ToLower(col)]
			if !ok {
				var discard any
				scanTargets[i] = &discard
				continue
			}

			field := elem.Field(fieldIdx)
			if !field.CanAddr() || !field.CanSet() {
				var discard any
				scanTargets[i] = &discard
				continue
			}

			// Handle nullable types
			if colTypes != nil {
				scanTargets[i] = nullableTarget(field, colTypes[i])
			} else {
				scanTargets[i] = field.Addr().Interface()
			}
		}

		if err := rows.Scan(scanTargets...); err != nil {
			return fmt.Errorf("sqlconn %s: scan row: %w", connName, err)
		}

		sliceVal = reflect.Append(sliceVal, elem)
	}

	destVal.Elem().Set(sliceVal)
	return rows.Err()
}

func buildFieldMap(structType reflect.Type, columns []string) map[string]int {
	fieldMap := make(map[string]int, len(columns))

	for i := range structType.NumField() {
		f := structType.Field(i)
		if !f.IsExported() {
			continue
		}

		// Check "db" tag first
		if tag := f.Tag.Get("db"); tag != "" {
			fieldMap[strings.ToLower(tag)] = i
			continue
		}

		// Fall back to field name
		fieldMap[strings.ToLower(f.Name)] = i
	}

	return fieldMap
}

func nullableTarget(field reflect.Value, colType *sql.ColumnType) any {
	nullable, ok := colType.Nullable()
	if !ok || !nullable {
		return field.Addr().Interface()
	}

	// Wrap in Null type
	switch field.Kind() {
	case reflect.String:
		return &sql.NullString{}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &sql.NullInt64{}
	case reflect.Float32, reflect.Float64:
		return &sql.NullFloat64{}
	case reflect.Bool:
		return &sql.NullBool{}
	case reflect.Struct:
		if field.Type() == reflect.TypeOf(sql.NullTime{}) {
			return &sql.NullTime{}
		}
		return field.Addr().Interface()
	default:
		return field.Addr().Interface()
	}
}

func (c *Connector) Ping(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return fmt.Errorf("sqlconn %s: connector is closed", c.name)
	}
	return c.db.PingContext(ctx)
}

func (c *Connector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.db.Close()
}
