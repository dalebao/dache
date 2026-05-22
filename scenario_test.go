package localcache_test

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dalebao/dache"
)

// --- E-Commerce Domain Models ---

type Product struct {
	ID       int
	Name     string
	Category string
	Price    float64
	Stock    int
}

type Customer struct {
	ID     int
	Name   string
	Email  string
	Tier   string
	Region string
}

type Order struct {
	ID         int
	CustomerID int
	ProductID  int
	Quantity   int
	Total      float64
	Status     string
	CreatedAt  time.Time
}

func productKey(p Product) int   { return p.ID }
func customerKey(c Customer) int  { return c.ID }
func orderKey(o Order) int        { return o.ID }

var testProducts = []Product{
	{ID: 1, Name: "MacBook Pro", Category: "Electronics", Price: 2000, Stock: 10},
	{ID: 2, Name: "Wireless Mouse", Category: "Electronics", Price: 50, Stock: 100},
	{ID: 3, Name: "USB-C Hub", Category: "Electronics", Price: 80, Stock: 50},
	{ID: 4, Name: "Desk Chair", Category: "Furniture", Price: 500, Stock: 20},
	{ID: 5, Name: "Standing Desk", Category: "Furniture", Price: 800, Stock: 15},
	{ID: 6, Name: "Coffee Mug", Category: "Kitchen", Price: 25, Stock: 200},
}

var testCustomers = []Customer{
	{ID: 1, Name: "Alice Smith", Email: "alice@example.com", Tier: "Gold", Region: "NA"},
	{ID: 2, Name: "Bob Jones", Email: "bob@example.com", Tier: "Silver", Region: "EU"},
	{ID: 3, Name: "Charlie Brown", Email: "charlie@example.com", Tier: "Gold", Region: "EU"},
	{ID: 4, Name: "Diana Prince", Email: "diana@example.com", Tier: "Bronze", Region: "APAC"},
	{ID: 5, Name: "Eve Wilson", Email: "eve@example.com", Tier: "Silver", Region: "NA"},
	{ID: 6, Name: "Frank Miller", Email: "frank@example.com", Tier: "Gold", Region: "APAC"},
}

func generateTestOrders(baseTime time.Time) []Order {
	return []Order{
		{ID: 1, CustomerID: 1, ProductID: 1, Quantity: 1, Total: 2000, Status: "delivered", CreatedAt: baseTime},
		{ID: 2, CustomerID: 2, ProductID: 2, Quantity: 2, Total: 100, Status: "shipped", CreatedAt: baseTime.Add(2 * time.Hour)},
		{ID: 3, CustomerID: 1, ProductID: 3, Quantity: 3, Total: 240, Status: "paid", CreatedAt: baseTime.Add(-24 * time.Hour)},
		{ID: 4, CustomerID: 3, ProductID: 4, Quantity: 1, Total: 500, Status: "pending", CreatedAt: baseTime.Add(-48 * time.Hour)},
		{ID: 5, CustomerID: 5, ProductID: 5, Quantity: 2, Total: 1600, Status: "pending", CreatedAt: baseTime.Add(-2 * time.Hour)},
		{ID: 6, CustomerID: 2, ProductID: 6, Quantity: 10, Total: 250, Status: "delivered", CreatedAt: baseTime.Add(-72 * time.Hour)},
		{ID: 7, CustomerID: 4, ProductID: 1, Quantity: 1, Total: 2000, Status: "paid", CreatedAt: baseTime.Add(-1 * time.Hour)},
		{ID: 8, CustomerID: 6, ProductID: 2, Quantity: 5, Total: 250, Status: "cancelled", CreatedAt: baseTime.Add(-4 * time.Hour)},
	}
}

// --- Test 1: Product Inventory ---
// Exercises: multi-index cache, index planner optimization, multi-field WHERE,
// range queries on indexed and non-indexed fields, multi-field sort, OpIn.

func TestScenario_ProductInventory(t *testing.T) {
	ctx := context.Background()

	c := localcache.New[int, Product](
		localcache.WithKey(productKey),
		localcache.WithIndex[int, Product, string]("Category", func(p Product) string { return p.Category }),
		localcache.WithIndex[int, Product, float64]("Price", func(p Product) float64 { return p.Price }),
	)

	if err := c.Load(ctx, testProducts); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	t.Run("Category filter", func(t *testing.T) {
		results, err := c.Query().Where("Category", localcache.OpEQ, "Electronics").Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 3 {
			t.Fatalf("expected 3 Electronics, got %d", len(results))
		}
	})

	t.Run("Price range with sort", func(t *testing.T) {
		results, err := c.Query().
			Where("Price", localcache.OpGTE, 500.0).
			OrderBy("Price", localcache.Asc).
			Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 3 {
			t.Fatalf("expected 3 products >= $500, got %d", len(results))
		}
		if results[0].Price != 500 || results[2].Price != 2000 {
			t.Fatal("sort by Price ascending failed")
		}
	})

	t.Run("Index planner selects best index", func(t *testing.T) {
		results, err := c.Query().
			Where("Category", localcache.OpEQ, "Electronics").
			Where("Price", localcache.OpEQ, 50.0).
			Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 || results[0].Name != "Wireless Mouse" {
			t.Fatalf("expected 1 cheap Electronics, got %+v", results)
		}
	})

	t.Run("In operator on indexed field", func(t *testing.T) {
		results, err := c.Query().
			Where("Category", localcache.OpIn, []string{"Electronics", "Kitchen"}).
			Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 4 {
			t.Fatalf("expected 4 products in Electronics|Kitchen, got %d", len(results))
		}
	})

	t.Run("Range on non-indexed field (full scan)", func(t *testing.T) {
		results, err := c.Query().
			Where("Stock", localcache.OpGTE, 50).
			Where("Stock", localcache.OpLTE, 150).
			Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 2 {
			t.Fatalf("expected 2 products with stock 50-150, got %d", len(results))
		}
	})

	t.Run("Multi-field sort: Category asc, Price desc", func(t *testing.T) {
		results, err := c.Query().
			OrderBy("Category", localcache.Asc).
			OrderBy("Price", localcache.Desc).
			Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 6 {
			t.Fatal("expected 6 products")
		}
		if results[0].Category != "Electronics" || results[3].Category != "Furniture" || results[5].Category != "Kitchen" {
			t.Fatal("primary sort by Category failed")
		}
		if results[0].Name != "MacBook Pro" || results[2].Name != "Wireless Mouse" {
			t.Fatal("secondary sort by Price descending within Category failed")
		}
	})
}

// --- Test 2: Customer Segments ---
// Exercises: multi-index cache (Tier, Region), index planner with competing
// indexes, OpIn on non-primary lookup, multi-field sort with string fields,
// empty result set.

func TestScenario_CustomerSegments(t *testing.T) {
	ctx := context.Background()

	c := localcache.New[int, Customer](
		localcache.WithKey(customerKey),
		localcache.WithIndex[int, Customer, string]("Tier", func(c Customer) string { return c.Tier }),
		localcache.WithIndex[int, Customer, string]("Region", func(c Customer) string { return c.Region }),
	)

	if err := c.Load(ctx, testCustomers); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	t.Run("Single index lookup on Tier", func(t *testing.T) {
		results, err := c.Query().Where("Tier", localcache.OpEQ, "Gold").Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 3 {
			t.Fatalf("expected 3 Gold customers, got %d", len(results))
		}
	})

	t.Run("Multi-field with planner choosing best index", func(t *testing.T) {
		results, err := c.Query().
			Where("Tier", localcache.OpEQ, "Gold").
			Where("Region", localcache.OpEQ, "EU").
			Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 || results[0].Name != "Charlie Brown" {
			t.Fatalf("expected 1 Gold+EU customer, got %+v", results)
		}
	})

	t.Run("In operator on Tier", func(t *testing.T) {
		results, err := c.Query().
			Where("Tier", localcache.OpIn, []string{"Silver", "Bronze"}).
			Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 3 {
			t.Fatalf("expected 3 Silver|Bronze customers, got %d", len(results))
		}
	})

	t.Run("Multi-field sort: Tier asc, Name asc", func(t *testing.T) {
		results, err := c.Query().
			OrderBy("Tier", localcache.Asc).
			OrderBy("Name", localcache.Asc).
			Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 6 {
			t.Fatal("expected 6 customers")
		}
		if results[0].Tier != "Bronze" {
			t.Fatalf("expected Bronze first, got %s", results[0].Tier)
		}
		var goldNames []string
		for _, c := range results {
			if c.Tier == "Gold" {
				goldNames = append(goldNames, c.Name)
			}
		}
		if !sort.SliceIsSorted(goldNames, func(i, j int) bool { return goldNames[i] < goldNames[j] }) {
			t.Fatalf("Gold customers not sorted by name: %v", goldNames)
		}
	})

	t.Run("Empty result for non-existent value", func(t *testing.T) {
		results, err := c.Query().Where("Region", localcache.OpEQ, "Antarctica").Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 0 {
			t.Fatal("expected 0 results for non-existent region")
		}
	})
}

// --- Test 3: Order Dashboard ---
// Exercises: index on string (Status) and int (CustomerID), time.Time range
// query and sort via fallback comparison path, cross-cache data enrichment,
// empty cache edge case.

func TestScenario_OrderDashboard(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	orders := generateTestOrders(baseTime)

	oc := localcache.New[int, Order](
		localcache.WithKey(orderKey),
		localcache.WithIndex[int, Order, string]("Status", func(o Order) string { return o.Status }),
		localcache.WithIndex[int, Order, int]("CustomerID", func(o Order) int { return o.CustomerID }),
	)

	if err := oc.Load(ctx, orders); err != nil {
		t.Fatalf("Load orders failed: %v", err)
	}

	t.Run("Status filter via index", func(t *testing.T) {
		pending, err := oc.Query().Where("Status", localcache.OpEQ, "pending").Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(pending) != 2 {
			t.Fatalf("expected 2 pending orders, got %d", len(pending))
		}
	})

	t.Run("Customer order history sorted by total descending", func(t *testing.T) {
		customerOrders, err := oc.Query().
			Where("CustomerID", localcache.OpEQ, 1).
			OrderBy("Total", localcache.Desc).
			Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(customerOrders) != 2 {
			t.Fatalf("expected 2 orders for customer 1, got %d", len(customerOrders))
		}
		if customerOrders[0].Total < customerOrders[1].Total {
			t.Fatal("expected descending sort by Total")
		}
	})

	t.Run("Time range query via fmt.Sprintf fallback", func(t *testing.T) {
		sinceTime := baseTime.Add(-24 * time.Hour)
		recentOrders, err := oc.Query().
			Where("CreatedAt", localcache.OpGTE, sinceTime).
			Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(recentOrders) != 6 {
			t.Fatalf("expected 6 orders in last 24h (includes 24h boundary), got %d", len(recentOrders))
		}
	})

	t.Run("Sort by time descending (newest first)", func(t *testing.T) {
		sinceTime := baseTime.Add(-24 * time.Hour)
		recentOrdersSorted, err := oc.Query().
			Where("CreatedAt", localcache.OpGTE, sinceTime).
			OrderBy("CreatedAt", localcache.Desc).
			Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(recentOrdersSorted) != 6 {
			t.Fatal("expected 5 recent orders sorted")
		}
		if recentOrdersSorted[0].ID != 2 {
			t.Fatalf("expected newest order ID=2, got ID=%d", recentOrdersSorted[0].ID)
		}
	})

	t.Run("High-value orders (no index on Total)", func(t *testing.T) {
		bigOrders, err := oc.Query().Where("Total", localcache.OpGTE, 1000.0).Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(bigOrders) != 3 {
			t.Fatalf("expected 3 high-value orders, got %d", len(bigOrders))
		}
	})

	t.Run("Cross-cache enrichment", func(t *testing.T) {
		cc := localcache.New[int, Customer](localcache.WithKey(customerKey))
		if err := cc.Load(ctx, testCustomers); err != nil {
			t.Fatalf("Load customers failed: %v", err)
		}

		type EnrichedOrder struct {
			Order
			CustomerName string
		}

		custOrders, _ := oc.Query().Where("CustomerID", localcache.OpEQ, 1).Execute(ctx)
		enriched := make([]EnrichedOrder, len(custOrders))
		for i, o := range custOrders {
			cust, _ := cc.Get(o.CustomerID)
			enriched[i] = EnrichedOrder{Order: o, CustomerName: cust.Name}
		}
		if enriched[0].CustomerName != "Alice Smith" {
			t.Fatalf("expected CustomerName=Alice Smith, got %s", enriched[0].CustomerName)
		}
	})

	t.Run("Empty cache and non-existent key edges", func(t *testing.T) {
		empty := localcache.New[int, Order](localcache.WithKey(orderKey))
		r, err := empty.Query().Where("Status", localcache.OpEQ, "pending").Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(r) != 0 {
			t.Fatal("expected 0 results from empty cache")
		}

		_, ok := empty.Get(999)
		if ok {
			t.Fatal("expected Get(999)=false")
		}
	})
}

// --- Test 4: DataSource Fallback Chain ---
// Exercises: ConnectorFunc, DataSource primary→fallback, all-sources-fail,
// Degraded() signal.

func TestScenario_DataSourceFallback(t *testing.T) {
	ctx := context.Background()
	var fallbackCalled atomic.Bool

	mysql := localcache.NewConnectorFunc("mysql", func(ctx context.Context, table string, dest any) error {
		return fmt.Errorf("connection refused")
	})

	redis := localcache.NewConnectorFunc("redis", func(ctx context.Context, table string, dest any) error {
		fallbackCalled.Store(true)
		*(dest.(*[]Product)) = []Product{
			{ID: 1, Name: "Stale Product", Category: "Fallback", Price: 10, Stock: 1},
		}
		return nil
	})

	t.Run("Primary fails, fallback succeeds", func(t *testing.T) {
		fallbackCalled.Store(false)
		ds := localcache.NewDataSource(mysql, redis)

		var products []Product
		err := ds.ScanAll(ctx, "products", &products)
		if err != nil {
			t.Fatalf("ScanAll should succeed via fallback: %v", err)
		}
		if !fallbackCalled.Load() {
			t.Fatal("expected fallback to be called")
		}
		if len(products) != 1 || products[0].Name != "Stale Product" {
			t.Fatalf("expected fallback data, got %+v", products)
		}
		if ds.Degraded() {
			t.Fatal("expected Degraded()=false after successful fallback")
		}
	})

	t.Run("All sources fail", func(t *testing.T) {
		allFail := localcache.NewDataSource(
			localcache.NewConnectorFunc("a", func(ctx context.Context, table string, dest any) error {
				return fmt.Errorf("source a down")
			}),
			localcache.NewConnectorFunc("b", func(ctx context.Context, table string, dest any) error {
				return fmt.Errorf("source b down")
			}),
		)
		err := allFail.ScanAll(ctx, "test", &[]string{})
		if err == nil {
			t.Fatal("expected error when all sources fail")
		}
		if !allFail.Degraded() {
			t.Fatal("expected Degraded()=true after all sources fail")
		}
	})

	t.Run("Repeated degradation resets on success", func(t *testing.T) {
		ds := localcache.NewDataSource(mysql)
		_ = ds.ScanAll(ctx, "test", &[]string{})
		if !ds.Degraded() {
			t.Fatal("expected Degraded()=true after primary fail with no fallback")
		}
	})
}

// --- Test 5: View Coordinated Refresh ---
// Exercises: NewTable, View.Add, View.Refresh, View.TryRefresh, QueryView,
// VersionStamp, RequireVersion, StalenessError, all-or-nothing semantics,
// non-existent table QueryView.

func TestScenario_ViewConsistency(t *testing.T) {
	ctx := context.Background()

	products := localcache.NewTable[int, Product]("products",
		localcache.WithKey(productKey),
		localcache.WithIndex[int, Product, string]("Category", func(p Product) string { return p.Category }),
	)

	customers := localcache.NewTable[int, Customer]("customers",
		localcache.WithKey(customerKey),
		localcache.WithIndex[int, Customer, string]("Tier", func(c Customer) string { return c.Tier }),
	)

	db := localcache.NewConnectorFunc("testdb", func(ctx context.Context, query string, dest any) error {
		switch query {
		case "SELECT * FROM products":
			*(dest.(*[]Product)) = testProducts
		case "SELECT * FROM customers":
			*(dest.(*[]Customer)) = testCustomers
		}
		return nil
	})

	view := localcache.NewView("dashboard", 5*time.Minute)
	view.Add(products, "SELECT * FROM products")
	view.Add(customers, "SELECT * FROM customers")
	view.SetDataSource(localcache.NewDataSource(db))

	t.Run("First refresh increments version and sets epoch", func(t *testing.T) {
		if err := view.Refresh(ctx); err != nil {
			t.Fatalf("first Refresh failed: %v", err)
		}
		if view.Version() != 1 {
			t.Fatalf("expected version=1, got %d", view.Version())
		}
		if view.Epoch().IsZero() {
			t.Fatal("expected non-zero epoch after refresh")
		}
	})

	t.Run("QueryView returns version-stamped results", func(t *testing.T) {
		vq := localcache.QueryView[int, Product](view, "products")
		if vq == nil {
			t.Fatal("expected non-nil ViewQuery for products")
		}

		results, stamp, err := vq.Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 6 {
			t.Fatalf("expected 6 products (all), got %d", len(results))
		}
		if stamp.ViewVersion != 1 {
			t.Fatalf("expected stamp version=1, got %d", stamp.ViewVersion)
		}
		if stamp.Epoch.IsZero() {
			t.Fatal("expected non-zero stamp epoch")
		}

		filtered, err := products.Query().Where("Category", localcache.OpEQ, "Electronics").Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(filtered) != 3 {
			t.Fatalf("expected 3 Electronics via table Query, got %d", len(filtered))
		}
	})

	t.Run("Multiple QueryViews share the same version", func(t *testing.T) {
		vq2 := localcache.QueryView[int, Customer](view, "customers")
		if vq2 == nil {
			t.Fatal("expected non-nil ViewQuery for customers")
		}
		_, custStamp, err := vq2.Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if custStamp.ViewVersion != 1 {
			t.Fatalf("expected stamp version=1, got %d", custStamp.ViewVersion)
		}
	})

	t.Run("Second refresh bumps version", func(t *testing.T) {
		if err := view.Refresh(ctx); err != nil {
			t.Fatalf("second Refresh failed: %v", err)
		}
		if view.Version() != 2 {
			t.Fatalf("expected version=2, got %d", view.Version())
		}
	})

	t.Run("RequireVersion satisfied", func(t *testing.T) {
		vq := localcache.QueryView[int, Product](view, "products").RequireVersion(2)
		_, stamp, err := vq.Execute(ctx)
		if err != nil {
			t.Fatalf("RequireVersion(2) should pass: %v", err)
		}
		if stamp.ViewVersion != 2 {
			t.Fatalf("expected stamp version=2, got %d", stamp.ViewVersion)
		}
	})

	t.Run("RequireVersion not satisfied returns StalenessError", func(t *testing.T) {
		vq := localcache.QueryView[int, Product](view, "products").RequireVersion(99)
		_, stamp, err := vq.Execute(ctx)
		if err == nil {
			t.Fatal("expected StalenessError for RequireVersion(99)")
		}
		if !localcache.IsStalenessError(err) {
			t.Fatalf("expected IsStalenessError, got %T: %v", err, err)
		}
		if stamp.ViewVersion != 2 {
			t.Fatalf("expected stamp version=2, got %d", stamp.ViewVersion)
		}
	})

	t.Run("TryRefresh is no-op when interval not elapsed", func(t *testing.T) {
		if err := view.TryRefresh(ctx); err != nil {
			t.Fatalf("TryRefresh should not error: %v", err)
		}
		if view.Version() != 2 {
			t.Fatal("expected version=2 after no-op TryRefresh")
		}
	})

	t.Run("All-or-nothing: failing table does not bump version", func(t *testing.T) {
		failingTbl := localcache.NewTable[int, Product]("failing",
			localcache.WithKey(productKey),
		)
		view.Add(failingTbl, "SELECT * FROM nonexistent")

		restrictedDB := localcache.NewConnectorFunc("restricted", func(ctx context.Context, query string, dest any) error {
			if query == "SELECT * FROM nonexistent" {
				return fmt.Errorf("table not found: nonexistent")
			}
			switch query {
			case "SELECT * FROM products":
				*(dest.(*[]Product)) = testProducts
			case "SELECT * FROM customers":
				*(dest.(*[]Customer)) = testCustomers
			}
			return nil
		})
		view.SetDataSource(localcache.NewDataSource(restrictedDB))

		err := view.Refresh(ctx)
		if err == nil {
			t.Fatal("expected error when one table fails to load")
		}
		if view.Version() != 2 {
			t.Fatalf("expected version=2 (unchanged), got %d", view.Version())
		}
	})

	t.Run("QueryView on non-existent table returns nil", func(t *testing.T) {
		badVQ := localcache.QueryView[int, Product](view, "ghost")
		if badVQ != nil {
			t.Fatal("expected nil for non-existent table")
		}
	})
}

// --- Test 6: Concurrent Workload ---
// Exercises: concurrent Load/Query/Get/Snapshot under mixed read-write load,
// verifies no data races and cache remains functional after stress.

func TestScenario_ConcurrentWorkload(t *testing.T) {
	ctx := context.Background()

	c := localcache.New[int, Product](
		localcache.WithKey(productKey),
		localcache.WithIndex[int, Product, string]("Category", func(p Product) string { return p.Category }),
		localcache.WithIndex[int, Product, float64]("Price", func(p Product) float64 { return p.Price }),
	)

	_ = c.Load(ctx, testProducts)

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, _ = c.Query().
					Where("Category", localcache.OpIn, []string{"Electronics", "Furniture"}).
					Where("Price", localcache.OpGTE, 50.0).
					OrderBy("Price", localcache.Asc).
					Limit(5).
					Execute(ctx)
				_, _ = c.Get(1)
			}
		}()
	}

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			items := make([]Product, len(testProducts))
			copy(items, testProducts)
			for i := range items {
				items[i].Stock = i + n*100
			}
			for j := 0; j < 10; j++ {
				_ = c.Load(ctx, items)
			}
		}(i)
	}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = c.Snapshot()
			}
		}()
	}

	wg.Wait()

	if c.Len() != len(testProducts) {
		t.Fatalf("expected Len=%d after concurrent workload, got %d", len(testProducts), c.Len())
	}
	p, ok := c.Get(1)
	if !ok || p.Name != "MacBook Pro" {
		t.Fatal("cache data corrupted after concurrent workload")
	}
}
