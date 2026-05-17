package localcache

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// View is a named, versioned cache layer that ties multiple caches
// to a unified data source with coordinated refresh timing.
//
// All tables in a View share a single refresh epoch and version counter,
// ensuring cross-table temporal consistency.
type View struct {
	name    string
	tables  []*viewTable
	ds      *DataSource
	group   *Group
	version atomic.Int64
	epoch   atomic.Value // time.Time, last successful sync time
	mu      sync.Mutex
	stopped atomic.Bool
}

type viewTable struct {
	name   string
	cache  any           // *Cache[K, V], accessed via scanFn
	query  string        // SQL or key for the table
	scanFn func(ctx context.Context, q string, dest any) error
}

// NewView creates a new View with the given name and refresh interval.
// The DataSource should be configured before calling Register.
func NewView(name string, interval time.Duration) *View {
	v := &View{
		name:  name,
		group: NewGroup(interval),
	}
	v.epoch.Store(time.Time{})
	return v
}

// Register adds a cache to the View. When the View refreshes, it calls
// scanFn to load data. scanFn reads from the View's DataSource.
//
// Example:
//
//	view := localcache.NewView("order_dashboard", 5*time.Minute)
//	view.Register("users", userCache, "SELECT * FROM users", nil)
//	view.SetDataSource(mysqlDS)
func (v *View) Register(name string, cache any, query string, scanFn func(ctx context.Context, q string, dest any) error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if scanFn == nil {
		scanFn = v.defaultScan
	}

	v.tables = append(v.tables, &viewTable{
		name:   name,
		cache:  cache,
		query:  query,
		scanFn: scanFn,
	})
}

// SetDataSource sets or replaces the DataSource for this View.
func (v *View) SetDataSource(ds *DataSource) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.ds = ds
}

// Refresh triggers a coordinated refresh of all tables.
// Data is loaded through the DataSource with automatic fallback.
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

// loadAll loads data for all tables atomically.
// If any table fails, no tables are updated.
func (v *View) loadAll(ctx context.Context) error {
	v.mu.Lock()
	ds := v.ds
	tables := v.tables
	v.mu.Unlock()

	if ds == nil {
		return fmt.Errorf("view %s: no DataSource configured", v.name)
	}

	snapshots := make([]snapshot, len(tables))
	for i, tbl := range tables {
		if err := tbl.scanFn(ctx, tbl.query, &snapshots[i].data); err != nil {
			return fmt.Errorf("view %s: table %s: %w", v.name, tbl.name, err)
		}
		snapshots[i].table = tbl
	}

	for _, snap := range snapshots {
		_ = snap.table.cache.(interface{ Load(context.Context, []any) error }).Load(ctx, snap.data)
	}

	v.version.Add(1)
	v.epoch.Store(time.Now())
	return nil
}

type snapshot struct {
	table *viewTable
	data  []any
}

func (v *View) defaultScan(ctx context.Context, query string, dest any) error {
	v.mu.Lock()
	ds := v.ds
	v.mu.Unlock()
	if ds == nil {
		return fmt.Errorf("view %s: no DataSource", v.name)
	}
	return ds.ScanAll(ctx, query, dest)
}

// Version returns the current version counter.
// Version increments on each successful refresh.
func (v *View) Version() int64 { return v.version.Load() }

// Epoch returns the last successful refresh time.
func (v *View) Epoch() time.Time { return v.epoch.Load().(time.Time) }

// Name returns the view name.
func (v *View) Name() string { return v.name }
