package localcache

import (
	"context"
	"fmt"
)

// Connector is the unified data source interface.
// Implementations connect to MySQL, PostgreSQL, Redis, Memcached, etc.
type Connector interface {
	// Name returns a readable identifier (e.g., "mysql", "redis").
	Name() string

	// ScanAll loads all rows from the named table/query into dest.
	// dest must be a pointer to a slice of structs (e.g., *[]User).
	ScanAll(ctx context.Context, table string, dest any) error

	// Ping checks connectivity.
	Ping(ctx context.Context) error

	// Close releases the connection.
	Close() error
}

// ConnectorFunc adapts a function to the Connector interface.
type ConnectorFunc struct {
	name string
	fn   func(ctx context.Context, table string, dest any) error
	ping func(ctx context.Context) error
}

func NewConnectorFunc(name string, fn func(ctx context.Context, table string, dest any) error) *ConnectorFunc {
	return &ConnectorFunc{name: name, fn: fn, ping: func(ctx context.Context) error { return nil }}
}

func (c *ConnectorFunc) Name() string                                  { return c.name }
func (c *ConnectorFunc) ScanAll(ctx context.Context, table string, dest any) error { return c.fn(ctx, table, dest) }
func (c *ConnectorFunc) Ping(ctx context.Context) error                 { return c.ping(ctx) }
func (c *ConnectorFunc) Close() error                                   { return nil }

// DataSource wraps a primary Connector with optional fallback chain.
// When the primary fails, each fallback is tried in order.
type DataSource struct {
	primary   Connector
	fallbacks []Connector
	errCount  int // consecutive failures since last success
}

// NewDataSource creates a DataSource with the given primary and fallbacks.
func NewDataSource(primary Connector, fallbacks ...Connector) *DataSource {
	return &DataSource{primary: primary, fallbacks: fallbacks}
}

// ScanAll attempts to load data from the primary, falling back through the chain.
// Returns the first successful result. If all sources fail, returns a combined error.
func (ds *DataSource) ScanAll(ctx context.Context, table string, dest any) error {
	firstErr := ds.primary.ScanAll(ctx, table, dest)
	if firstErr == nil {
		ds.errCount = 0
		return nil
	}

	ds.errCount++

	var fallbackErrs []error
	for i, fb := range ds.fallbacks {
		if err := fb.ScanAll(ctx, table, dest); err == nil {
			return nil
		} else {
			fallbackErrs = append(fallbackErrs, fmt.Errorf("fallback[%d](%s): %w", i, fb.Name(), err))
		}
	}

	return fmt.Errorf("datasource: primary(%s): %w; fallbacks: %v",
		ds.primary.Name(), firstErr, fallbackErrs)
}

// Degraded reports whether the DataSource is in degraded mode
// (primary failed, using fallback).
func (ds *DataSource) Degraded() bool {
	return ds.errCount > 0
}

// Primary returns the primary Connector.
func (ds *DataSource) Primary() Connector { return ds.primary }

// Fallbacks returns the fallback Connectors.
func (ds *DataSource) Fallbacks() []Connector { return ds.fallbacks }
