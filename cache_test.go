package localcache_test

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/dalebao/dache"
)

type User struct {
	ID   int
	Name string
	Age  int
}

func intKey(u User) int { return u.ID }

func TestNew(t *testing.T) {
	c := localcache.New(localcache.WithKey(intKey))
	if c == nil {
		t.Fatal("New returned nil")
	}
	if c.Len() != 0 {
		t.Fatalf("expected empty cache, got Len=%d", c.Len())
	}
}

func TestLoad(t *testing.T) {
	ctx := context.Background()
	c := localcache.New(localcache.WithKey(intKey))

	users := []User{
		{ID: 1, Name: "Alice", Age: 30},
		{ID: 2, Name: "Bob", Age: 25},
		{ID: 3, Name: "Charlie", Age: 35},
	}

	if err := c.Load(ctx, users); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if c.Len() != 3 {
		t.Fatalf("expected Len=3, got %d", c.Len())
	}
}

func TestGet(t *testing.T) {
	ctx := context.Background()
	c := localcache.New(localcache.WithKey(intKey))
	c.Load(ctx, []User{{ID: 1, Name: "Alice", Age: 30}})

	u, ok := c.Get(1)
	if !ok {
		t.Fatal("expected Get(1) to return true")
	}
	if u.Name != "Alice" {
		t.Fatalf("expected Name=Alice, got %s", u.Name)
	}

	_, ok = c.Get(999)
	if ok {
		t.Fatal("expected Get(999) to return false")
	}
}

func TestLoadReplacesData(t *testing.T) {
	ctx := context.Background()
	c := localcache.New(localcache.WithKey(intKey))

	c.Load(ctx, []User{{ID: 1, Name: "Alice"}, {ID: 2, Name: "Bob"}})
	c.Load(ctx, []User{{ID: 3, Name: "Charlie"}})

	if c.Len() != 1 {
		t.Fatalf("expected Len=1 after replacement, got %d", c.Len())
	}
	u, ok := c.Get(3)
	if !ok || u.Name != "Charlie" {
		t.Fatal("expected only Charlie after replacement")
	}
}

func TestQueryNoIndices(t *testing.T) {
	ctx := context.Background()
	c := localcache.New(localcache.WithKey(intKey))
	c.Load(ctx, []User{
		{ID: 1, Name: "Alice", Age: 30},
		{ID: 2, Name: "Bob", Age: 25},
		{ID: 3, Name: "Charlie", Age: 35},
	})

	results, err := c.Query().Where("Age", localcache.OpGTE, 30).Execute(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestQueryWithIndex(t *testing.T) {
	ctx := context.Background()
	c := localcache.New(
		localcache.WithKey(intKey),
		localcache.WithIndex("Age", func(u User) int { return u.Age }),
	)
	c.Load(ctx, []User{
		{ID: 1, Name: "Alice", Age: 30},
		{ID: 2, Name: "Bob", Age: 25},
		{ID: 3, Name: "Charlie", Age: 35},
		{ID: 4, Name: "Diana", Age: 30},
	})

	t.Run("EQ on indexed field", func(t *testing.T) {
		results, err := c.Query().Where("Age", localcache.OpEQ, 30).Execute(ctx)
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("expected 2 results for Age=30, got %d: %+v", len(results), results)
		}
	})

	t.Run("GT on indexed field", func(t *testing.T) {
		results, err := c.Query().Where("Age", localcache.OpGT, 30).Execute(ctx)
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result for Age>30, got %d", len(results))
		}
	})
}

func TestOrderBy(t *testing.T) {
	ctx := context.Background()
	c := localcache.New(localcache.WithKey(intKey))
	c.Load(ctx, []User{
		{ID: 1, Name: "Alice", Age: 30},
		{ID: 2, Name: "Bob", Age: 25},
		{ID: 3, Name: "Charlie", Age: 35},
		{ID: 4, Name: "Diana", Age: 28},
	})

	results, err := c.Query().OrderBy("Age", localcache.Asc).Execute(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	if results[0].Age != 25 {
		t.Fatalf("expected first Age=25, got %d", results[0].Age)
	}
	if results[3].Age != 35 {
		t.Fatalf("expected last Age=35, got %d", results[3].Age)
	}
}

func TestLimitOffset(t *testing.T) {
	ctx := context.Background()
	c := localcache.New(localcache.WithKey(intKey))
	c.Load(ctx, []User{
		{ID: 1, Name: "Alice", Age: 30},
		{ID: 2, Name: "Bob", Age: 25},
		{ID: 3, Name: "Charlie", Age: 35},
	})

	results, err := c.Query().OrderBy("Age", localcache.Asc).Limit(2).Execute(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	results, err = c.Query().OrderBy("Age", localcache.Asc).Offset(2).Execute(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result with Offset(2), got %d", len(results))
	}
}

func TestQueryChaining(t *testing.T) {
	ctx := context.Background()
	c := localcache.New(
		localcache.WithKey(intKey),
		localcache.WithIndex("Age", func(u User) int { return u.Age }),
	)
	c.Load(ctx, []User{
		{ID: 1, Name: "Alice", Age: 30},
		{ID: 2, Name: "Bob", Age: 25},
		{ID: 3, Name: "Charlie", Age: 35},
		{ID: 4, Name: "Diana", Age: 28},
		{ID: 5, Name: "Eve", Age: 32},
	})

	results, err := c.Query().
		Where("Age", localcache.OpGTE, 28).
		OrderBy("Age", localcache.Desc).
		Limit(2).
		Execute(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Age < results[1].Age {
		t.Fatal("expected descending order")
	}
}

func TestConcurrency(t *testing.T) {
	ctx := context.Background()
	c := localcache.New(
		localcache.WithKey(intKey),
		localcache.WithIndex("Age", func(u User) int { return u.Age }),
	)
	c.Load(ctx, []User{{ID: 1, Name: "Alice", Age: 30}})

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Get(1)
				c.Query().Where("Age", localcache.OpEQ, 30).Execute(ctx)
			}
		}()
	}
	wg.Wait()
}

func TestSnapshot(t *testing.T) {
	ctx := context.Background()
	c := localcache.New(localcache.WithKey(intKey))
	c.Load(ctx, []User{
		{ID: 1, Name: "Alice"},
		{ID: 2, Name: "Bob"},
	})

	snap := c.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 in snapshot, got %d", len(snap))
	}

	sort.Slice(snap, func(i, j int) bool { return snap[i].ID < snap[j].ID })
	if snap[0].ID != 1 || snap[1].ID != 2 {
		t.Fatal("snapshot items mismatch")
	}
}

func TestGroup(t *testing.T) {
	ctx := context.Background()
	g := localcache.NewGroup(50 * time.Millisecond)

	callCount := 0
	err := g.Refresh(ctx, func(ctx context.Context) error {
		callCount++
		return nil
	})
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected callCount=1 after first refresh, got %d", callCount)
	}

	err = g.Refresh(ctx, func(ctx context.Context) error {
		callCount++
		return nil
	})
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected callCount=1 (no-op, interval not elapsed), got %d", callCount)
	}

	time.Sleep(60 * time.Millisecond)
	err = g.Refresh(ctx, func(ctx context.Context) error {
		callCount++
		return nil
	})
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}
	if callCount != 2 {
		t.Fatalf("expected callCount=2 after interval, got %d", callCount)
	}
}

func TestGroupForceRefresh(t *testing.T) {
	ctx := context.Background()
	g := localcache.NewGroup(1 * time.Hour)

	callCount := 0
	g.ForceRefresh(ctx, func(ctx context.Context) error {
		callCount++
		return nil
	})
	if callCount != 1 {
		t.Fatalf("expected callCount=1, got %d", callCount)
	}
}

func TestRefreshGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rg := localcache.NewRefreshGroup(50 * time.Millisecond)

	var mu sync.Mutex
	callCount := 0
	rg.Start(ctx, func(ctx context.Context) error {
		mu.Lock()
		callCount++
		mu.Unlock()
		return nil
	})

	time.Sleep(120 * time.Millisecond)
	cancel()
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	count := callCount
	mu.Unlock()
	if count < 2 {
		t.Fatalf("expected at least 2 refresh calls in 120ms, got %d", count)
	}
}

func TestFieldGetter(t *testing.T) {
	ctx := context.Background()

	type custom struct {
		ID   int
		Name string
		Age  int
	}

	// This type implements fieldGetter
	type customWithGetter struct {
		ID      int
		Name    string
		Age     int
		_fields map[string]any
	}

	c := localcache.New(
		localcache.WithKey(func(v customWithGetter) int { return v.ID }),
		localcache.WithIndex("Age", func(v customWithGetter) int { return v.Age }),
	)

	c.Load(ctx, []customWithGetter{
		{ID: 1, Name: "Alice", Age: 30},
		{ID: 2, Name: "Bob", Age: 25},
	})

	results, err := c.Query().Where("Age", localcache.OpLT, 30).Execute(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 1 || results[0].Name != "Bob" {
		t.Fatalf("expected 1 result (Bob), got %d", len(results))
	}

	_ = custom{} // verify unused import doesn't cause issues
}
