package localcache

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// View is a named, versioned cache layer that ties multiple Table's
// to a unified DataSource with coordinated refresh timing.
//
// All tables in a View share a single refresh epoch and version counter,
// ensuring cross-table temporal consistency. When any table fails to load,
// no tables are updated (all-or-nothing semantics).
type View struct {
	name    string
	tables  []*viewTable
	ds      *DataSource
	group   *Group
	version atomic.Int64
	epoch   atomic.Value // time.Time
	mu      sync.Mutex
}

type viewTable struct {
	name  string
	adder TableAdder
	query string
}

// NewView creates a new View with the given name and refresh interval.
func NewView(name string, interval time.Duration) *View {
	v := &View{
		name:  name,
		group: NewGroup(interval),
	}
	v.epoch.Store(time.Time{})
	return v
}

// Add registers a Table with a query. When the View refreshes, it calls
// Table.LoadFrom(ctx, query, scan) where scan uses the View's DataSource.
//
//	view := localcache.NewView("order_dashboard", 5*time.Minute)
//	view.SetDataSource(mysqlDS)
//	view.Add(users, "SELECT id, name, age FROM users")
func (v *View) Add(adder TableAdder, query string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.tables = append(v.tables, &viewTable{
		name:  adder.TableName(),
		adder: adder,
		query: query,
	})
}

// SetDataSource configures the DataSource used during refresh.
func (v *View) SetDataSource(ds *DataSource) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.ds = ds
}

// Refresh triggers an immediate coordinated refresh of all tables.
// Data is loaded through the DataSource with automatic fallback.
// All-or-nothing: if any table fails, no tables are updated.
func (v *View) Refresh(ctx context.Context) error {
	return v.group.ForceRefresh(ctx, func(ctx context.Context) error {
		return v.loadAll(ctx)
	})
}

// TryRefresh triggers a refresh only if the interval has elapsed.
func (v *View) TryRefresh(ctx context.Context) error {
	return v.group.Refresh(ctx, func(ctx context.Context) error {
		return v.loadAll(ctx)
	})
}

// loadAll refreshes all tables atomically.
func (v *View) loadAll(ctx context.Context) error {
	v.mu.Lock()
	ds := v.ds
	tables := v.tables
	v.mu.Unlock()

	if ds == nil {
		return fmt.Errorf("view %s: no DataSource configured", v.name)
	}

	for _, tbl := range tables {
		if err := tbl.adder.LoadFrom(ctx, tbl.query, ds.ScanAll); err != nil {
			return fmt.Errorf("view %s: table %s: %w", v.name, tbl.name, err)
		}
	}

	v.version.Add(1)
	v.epoch.Store(time.Now())
	return nil
}

// Version returns the current version counter (increments on each successful refresh).
func (v *View) Version() int64 { return v.version.Load() }

// Epoch returns the last successful refresh time.
func (v *View) Epoch() time.Time { return v.epoch.Load().(time.Time) }

// Name returns the view name.
func (v *View) Name() string { return v.name }

// Query returns a ViewQuery for the named table.
// Returns nil if the table is not registered in this View.
func (v *View) Query[K comparable, V any](name string) *ViewQuery[K, V] {
	v.mu.Lock()
	defer v.mu.Unlock()

	for _, tbl := range v.tables {
		if tbl.name == name {
			if cache, ok := tbl.adder.(*Table[K, V]); ok {
				return &ViewQuery[K, V]{
					Query: cache.Cache.Query(),
					stampFunc: func() VersionStamp {
						return VersionStamp{
							ViewVersion: v.Version(),
							Epoch:       v.Epoch(),
						}
					},
				}
			}
		}
	}
	return nil
}
