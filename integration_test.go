package localcache_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dalebao/dache"
	"github.com/dalebao/dache/conn/sql"
)

// mockConnector implements localcache.Connector for testing.
type mockConnector struct {
	name     string
	scanFn   func(ctx context.Context, table string, dest any) error
	pingFn   func(ctx context.Context) error
	closeFn  func() error
}

func (m *mockConnector) Name() string                                      { return m.name }
func (m *mockConnector) ScanAll(ctx context.Context, table string, dest any) error { return m.scanFn(ctx, table, dest) }
func (m *mockConnector) Ping(ctx context.Context) error                     { return m.pingFn(ctx) }
func (m *mockConnector) Close() error                                       { return m.closeFn() }

func TestDataSourcePrimarySuccess(t *testing.T) {
	ctx := context.Background()
	called := false

	primary := &mockConnector{
		name: "primary",
		scanFn: func(ctx context.Context, table string, dest any) error {
			called = true
			*(dest.(*[]string)) = []string{"data"}
			return nil
		},
		pingFn:  func(ctx context.Context) error { return nil },
		closeFn: func() error { return nil },
	}

	ds := localcache.NewDataSource(primary)
	var result []string
	err := ds.ScanAll(ctx, "test", &result)
	if err != nil {
		t.Fatalf("ScanAll failed: %v", err)
	}
	if !called {
		t.Fatal("primary ScanAll was not called")
	}
	if ds.Degraded() {
		t.Fatal("expected Degraded()=false after primary success")
	}
}

func TestDataSourceFallback(t *testing.T) {
	ctx := context.Background()
	primaryCalled := false
	fallbackCalled := false

	primary := &mockConnector{
		name: "primary",
		scanFn: func(ctx context.Context, table string, dest any) error {
			primaryCalled = true
			return fmt.Errorf("primary down")
		},
		pingFn:  func(ctx context.Context) error { return fmt.Errorf("ping failed") },
		closeFn: func() error { return nil },
	}

	fallback := &mockConnector{
		name: "fallback",
		scanFn: func(ctx context.Context, table string, dest any) error {
			fallbackCalled = true
			*(dest.(*[]string)) = []string{"cached"}
			return nil
		},
		pingFn:  func(ctx context.Context) error { return nil },
		closeFn: func() error { return nil },
	}

	ds := localcache.NewDataSource(primary, fallback)
	var result []string
	err := ds.ScanAll(ctx, "test", &result)
	if err != nil {
		t.Fatalf("ScanAll failed: %v", err)
	}
	if !primaryCalled {
		t.Fatal("primary should have been tried")
	}
	if !fallbackCalled {
		t.Fatal("fallback should have been tried")
	}
	if len(result) != 1 || result[0] != "cached" {
		t.Fatalf("expected cached result, got %v", result)
	}
	if ds.Degraded() {
		t.Fatal("expected Degraded()=false after successful fallback")
	}
}

func TestDataSourceAllSourcesFail(t *testing.T) {
	ctx := context.Background()

	primary := &mockConnector{
		name:   "primary",
		scanFn: func(ctx context.Context, table string, dest any) error { return fmt.Errorf("primary err") },
		pingFn: func(ctx context.Context) error { return nil },
		closeFn: func() error { return nil },
	}

	fallback := &mockConnector{
		name:   "fallback",
		scanFn: func(ctx context.Context, table string, dest any) error { return fmt.Errorf("fallback err") },
		pingFn: func(ctx context.Context) error { return nil },
		closeFn: func() error { return nil },
	}

	ds := localcache.NewDataSource(primary, fallback)
	var result []string
	err := ds.ScanAll(ctx, "test", &result)
	if err == nil {
		t.Fatal("expected error when all sources fail")
	}
	if !ds.Degraded() {
		t.Fatal("expected Degraded()=true after all sources fail")
	}
}

// mockTable implements localcache.TableAdder for testing.
type mockTable struct {
	name       string
	loadFn     func(ctx context.Context, query string, scan func(ctx context.Context, q string, dest any) error) error
}

func (m *mockTable) TableName() string { return m.name }
func (m *mockTable) LoadFrom(ctx context.Context, query string, scan func(ctx context.Context, q string, dest any) error) error {
	return m.loadFn(ctx, query, scan)
}

func TestViewRefresh(t *testing.T) {
	ctx := context.Background()
	view := localcache.NewView("test_view", 5*time.Minute)

	var usersLoaded, ordersLoaded bool

	users := &mockTable{
		name: "users",
		loadFn: func(ctx context.Context, query string, scan func(ctx context.Context, q string, dest any) error) error {
			usersLoaded = true
			if query != "SELECT * FROM users" {
				t.Fatalf("expected users query, got %s", query)
			}
			return nil
		},
	}

	orders := &mockTable{
		name: "orders",
		loadFn: func(ctx context.Context, query string, scan func(ctx context.Context, q string, dest any) error) error {
			ordersLoaded = true
			if query != "SELECT * FROM orders" {
				t.Fatalf("expected orders query, got %s", query)
			}
			return nil
		},
	}

	ds := localcache.NewDataSource(&mockConnector{
		name:   "mockdb",
		scanFn: func(ctx context.Context, table string, dest any) error { return nil },
		pingFn: func(ctx context.Context) error { return nil },
		closeFn: func() error { return nil },
	})

	view.SetDataSource(ds)
	view.Add(users, "SELECT * FROM users")
	view.Add(orders, "SELECT * FROM orders")

	if err := view.Refresh(ctx); err != nil {
		t.Fatalf("View.Refresh failed: %v", err)
	}

	if !usersLoaded || !ordersLoaded {
		t.Fatal("expected both tables to be loaded")
	}
	if view.Version() != 1 {
		t.Fatalf("expected Version()=1 after first refresh, got %d", view.Version())
	}
}

func TestViewAllOrNothing(t *testing.T) {
	ctx := context.Background()
	view := localcache.NewView("test", 5*time.Minute)

	users := &mockTable{
		name: "users",
		loadFn: func(ctx context.Context, query string, scan func(ctx context.Context, q string, dest any) error) error {
			return nil
		},
	}

	orders := &mockTable{
		name: "orders",
		loadFn: func(ctx context.Context, query string, scan func(ctx context.Context, q string, dest any) error) error {
			return fmt.Errorf("orders table failed")
		},
	}

	view.Add(users, "SELECT * FROM users")
	view.Add(orders, "SELECT * FROM orders")

	ds := localcache.NewDataSource(&mockConnector{
		name:   "mockdb",
		scanFn: func(ctx context.Context, table string, dest any) error { return nil },
		pingFn: func(ctx context.Context) error { return nil },
		closeFn: func() error { return nil },
	})
	view.SetDataSource(ds)

	err := view.Refresh(ctx)
	if err == nil {
		t.Fatal("expected error when one table fails")
	}
	if view.Version() != 0 {
		t.Fatalf("expected Version()=0 (no refresh due to error), got %d", view.Version())
	}
}

func TestViewTryRefreshHonorsInterval(t *testing.T) {
	ctx := context.Background()
	view := localcache.NewView("test", 1*time.Hour)
	callCount := 0

	view.Add(&mockTable{
		name: "t1",
		loadFn: func(ctx context.Context, query string, scan func(ctx context.Context, q string, dest any) error) error {
			callCount++
			return nil
		},
	}, "SELECT 1")

	ds := localcache.NewDataSource(&mockConnector{
		name:   "mock",
		scanFn: func(ctx context.Context, table string, dest any) error { return nil },
		pingFn: func(ctx context.Context) error { return nil },
		closeFn: func() error { return nil },
	})
	view.SetDataSource(ds)

	// Force first refresh
	if err := view.Refresh(ctx); err != nil {
		t.Fatalf("first Refresh failed: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 call after first refresh, got %d", callCount)
	}

	// TryRefresh should be no-op (interval not elapsed)
	if err := view.TryRefresh(ctx); err != nil {
		t.Fatalf("TryRefresh failed: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 call (no-op), got %d", callCount)
	}
}

func TestViewVersionStamping(t *testing.T) {
	ctx := context.Background()
	view := localcache.NewView("test", 5*time.Minute)
	ds := localcache.NewDataSource(&mockConnector{
		name:   "mock",
		scanFn: func(ctx context.Context, table string, dest any) error { return nil },
		pingFn: func(ctx context.Context) error { return nil },
		closeFn: func() error { return nil },
	})
	view.SetDataSource(ds)
	view.Add(&mockTable{name: "t", loadFn: func(ctx context.Context, q string, scan func(ctx context.Context, q2 string, dest any) error) error {
		return scan(ctx, q, &[]string{})
	}}, "SELECT 1")

	view.Refresh(ctx)
	v1 := view.Version()
	t.Logf("version after first refresh: %d", v1)

	view.Refresh(ctx)
	v2 := view.Version()
	t.Logf("version after second refresh: %d", v2)

	if v2 <= v1 {
		t.Fatal("expected version to increase after refresh")
	}
	if view.Epoch().IsZero() {
		t.Fatal("expected non-zero epoch after refresh")
	}
}

// TestSQLConnector verifies the SQL connector with an in-memory mock.
func TestSQLConnectorBasic(t *testing.T) {
	// Without a real database, we can at least verify the API compiles:

	conn := sqlconn.New("test", nil) // nil db will panic on use; we only test struct creation
	if conn.Name() != "test" {
		t.Fatalf("expected Name()=test, got %s", conn.Name())
	}
}

func TestConnectorFunc(t *testing.T) {
	ctx := context.Background()
	var callCount atomic.Int64

	conn := localcache.NewConnectorFunc("custom", func(ctx context.Context, table string, dest any) error {
		callCount.Add(1)
		*(dest.(*[]string)) = []string{"custom_data"}
		return nil
	})

	if conn.Name() != "custom" {
		t.Fatalf("expected Name=custom, got %s", conn.Name())
	}

	var result []string
	if err := conn.ScanAll(ctx, "test", &result); err != nil {
		t.Fatalf("ScanAll failed: %v", err)
	}
	if callCount.Load() != 1 {
		t.Fatalf("expected 1 call, got %d", callCount.Load())
	}
	if len(result) != 1 || result[0] != "custom_data" {
		t.Fatalf("expected [custom_data], got %v", result)
	}
}
