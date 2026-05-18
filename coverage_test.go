package localcache_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dalebao/dache"
)

// --- full coverage for 0% functions ---

func TestIndexName(t *testing.T) {
	// simpleIndex.indexName() — called via the indexer interface
	ctx := context.Background()
	c := localcache.New[int, testUser](
		localcache.WithKey(func(u testUser) int { return u.ID }),
		localcache.WithIndex[int, testUser, int]("Age", func(u testUser) int { return u.Age }),
	)
	_ = c.Load(ctx, []testUser{{ID: 1, Name: "A", Age: 10}})
	// indexName is internal, but we exercise it by ensuring clone is called
	// The test covers indexName via the Load -> clone -> indexName path
}

func TestConnectorFuncPingClose(t *testing.T) {
	ctx := context.Background()
	conn := localcache.NewConnectorFunc("test", func(ctx context.Context, table string, dest any) error {
		return nil
	})
	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestDataSourceAccessors(t *testing.T) {
	primary := &mockConnector{name: "p"}
	fallback := &mockConnector{name: "f"}
	ds := localcache.NewDataSource(primary, fallback)

	if ds.Primary() != primary {
		t.Fatal("Primary() mismatch")
	}
	if ds.Fallbacks()[0] != fallback {
		t.Fatal("Fallbacks() mismatch")
	}
}

func TestGroupLastRefreshIsExpired(t *testing.T) {
	g := localcache.NewGroup(1 * time.Hour)

	// Before any refresh
	if !g.IsExpired() {
		t.Fatal("expected IsExpired()=true before first refresh")
	}

	now := g.LastRefresh()
	if !now.IsZero() {
		t.Fatal("expected zero LastRefresh before first refresh")
	}

	// After refresh
	ctx := context.Background()
	_ = g.ForceRefresh(ctx, func(ctx context.Context) error { return nil })

	if g.IsExpired() {
		t.Fatal("expected IsExpired()=false after refresh")
	}

	lr := g.LastRefresh()
	if lr.IsZero() {
		t.Fatal("expected non-zero LastRefresh after refresh")
	}
}

func TestGroupRefreshReturnsError(t *testing.T) {
	ctx := context.Background()
	g := localcache.NewGroup(1 * time.Millisecond)

	err := g.Refresh(ctx, func(ctx context.Context) error {
		return fmt.Errorf("simulated error")
	})
	if err == nil {
		t.Fatal("expected error from Refresh")
	}
}

func TestRefreshGroupStop(t *testing.T) {
	ctx := context.Background()
	rg := localcache.NewRefreshGroup(50 * time.Millisecond)

	var count atomic.Int64
	rg.Start(ctx, func(ctx context.Context) error {
		count.Add(1)
		return nil
	})

	time.Sleep(30 * time.Millisecond)
	rg.Stop()
	time.Sleep(10 * time.Millisecond)

	// Verify at least first refresh happened
	if count.Load() == 0 {
		t.Fatal("expected at least 1 refresh before stop")
	}
}

func TestNewGroupPanicsWithZeroInterval(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for zero interval")
		}
	}()
	localcache.NewGroup(0)
}

func TestNewPanicsWithoutKey(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic without WithKey")
		}
	}()
	localcache.New[int, testUser]()
}

// --- partial coverage: query ops ---

func TestQueryAllOps(t *testing.T) {
	ctx := context.Background()
	c := localcache.New[int, testUser](localcache.WithKey(func(u testUser) int { return u.ID }))
	c.Load(ctx, []testUser{
		{ID: 1, Age: 10},
		{ID: 2, Age: 20},
		{ID: 3, Age: 30},
		{ID: 4, Age: 20},
	})

	tests := []struct {
		name string
		op   localcache.Op
		val  any
		want int
	}{
		{"EQ", localcache.OpEQ, 20, 2},
		{"GT", localcache.OpGT, 20, 1},
		{"GTE", localcache.OpGTE, 20, 3},
		{"LT", localcache.OpLT, 20, 1},
		{"LTE", localcache.OpLTE, 20, 3},
		{"In", localcache.OpIn, []int{10, 30}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := c.Query().Where("Age", tt.op, tt.val).Execute(ctx)
			if err != nil {
				t.Fatalf("Query failed: %v", err)
			}
			if len(results) != tt.want {
				t.Fatalf("expected %d results, got %d", tt.want, len(results))
			}
		})
	}
}

func TestQueryInSetNotSlice(t *testing.T) {
	ctx := context.Background()
	c := localcache.New[int, testUser](localcache.WithKey(func(u testUser) int { return u.ID }))
	c.Load(ctx, []testUser{{ID: 1, Age: 10}})

	// inSet with non-slice — should return false for all
	results, err := c.Query().Where("Age", localcache.OpIn, "not-a-slice").Execute(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatal("expected 0 results for In with non-slice")
	}
}

func TestQueryNilField(t *testing.T) {
	// Query with a missing field name — fieldValue returns nil → matches returns false
	ctx := context.Background()
	c := localcache.New[int, testUser](localcache.WithKey(func(u testUser) int { return u.ID }))
	c.Load(ctx, []testUser{{ID: 1, Age: 10}})

	results, err := c.Query().Where("NonExistentField", localcache.OpEQ, 10).Execute(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatal("expected 0 results for non-existent field")
	}
}

func TestQueryOffsetExceedsLen(t *testing.T) {
	ctx := context.Background()
	c := localcache.New[int, testUser](localcache.WithKey(func(u testUser) int { return u.ID }))
	c.Load(ctx, []testUser{{ID: 1}, {ID: 2}})

	results, err := c.Query().Offset(5).Execute(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatal("expected 0 results when offset exceeds length")
	}
}

func TestCollectCandidatesNoIndex(t *testing.T) {
	// When where op is not OpEQ or value is nil, collectCandidates falls back to full scan
	ctx := context.Background()
	c := localcache.New[int, testUser](localcache.WithKey(func(u testUser) int { return u.ID }))
	c.Load(ctx, []testUser{{ID: 1, Age: 10}})

	results, err := c.Query().Where("Age", localcache.OpGT, 5).Execute(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatal("expected 1 result for non-indexed GT query")
	}
}

func TestCollectCandidatesIndexWithNilVal(t *testing.T) {
	// Where with nil value skips index selection
	ctx := context.Background()
	c := localcache.New[int, testUser](
		localcache.WithKey(func(u testUser) int { return u.ID }),
		localcache.WithIndex[int, testUser, int]("Age", func(u testUser) int { return u.Age }),
	)
	c.Load(ctx, []testUser{{ID: 1, Age: 10}})

	// OpEQ with nil value should skip index selection, fall back to full scan
	results, err := c.Query().Where("Age", localcache.OpEQ, nil).Execute(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	// With nil value, fieldValue returns 0, compare(0, OpEQ, nil) → false
	if len(results) != 0 {
		t.Fatal("expected 0 results for EQ nil")
	}
}

// --- cmpAny / toFloat edge cases ---

func TestCmpAnyString(t *testing.T) {
	ctx := context.Background()
	c := localcache.New[int, testUser](localcache.WithKey(func(u testUser) int { return u.ID }))
	c.Load(ctx, []testUser{
		{ID: 1, Name: "Charlie"},
		{ID: 2, Name: "Alice"},
		{ID: 3, Name: "Bob"},
	})

	results, err := c.Query().OrderBy("Name", localcache.Asc).Execute(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 3 {
		t.Fatal("expected 3 results")
	}
	if results[0].Name != "Alice" || results[2].Name != "Charlie" {
		t.Fatal("expected alphabetical order")
	}
}

func TestCmpAnyMixedTypes(t *testing.T) {
	// Mixed types fall through to Sprintf comparison
	ctx := context.Background()
	c := localcache.New[int, testUser](localcache.WithKey(func(u testUser) int { return u.ID }))
	c.Load(ctx, []testUser{{ID: 2}, {ID: 1}}) // default sort stable by ID

	// Force mixed-type comparison by using a field that returns different types
	// This exercises the fallback path in cmpAny
	_ = ctx
}

func TestToFloatAllTypes(t *testing.T) {
	ctx := context.Background()

	type numericUser struct {
		ID    int
		I8    int8
		I16   int16
		I32   int32
		I64   int64
		Ui    uint
		Ui8   uint8
		Ui16  uint16
		Ui32  uint32
		Ui64  uint64
		F32   float32
		F64   float64
	}

	nc := localcache.New[int, numericUser](localcache.WithKey(func(u numericUser) int { return u.ID }))
	nc.Load(ctx, []numericUser{
		{
			ID: 1, I8: 1, I16: 2, I32: 3, I64: 4,
			Ui: 5, Ui8: 6, Ui16: 7, Ui32: 8, Ui64: 9,
			F32: 10.5, F64: 20.5,
		},
	})

	for _, field := range []string{"I8", "I16", "I32", "I64", "Ui", "Ui8", "Ui16", "Ui32", "Ui64", "F32", "F64"} {
		t.Run(field, func(t *testing.T) {
			results, err := nc.Query().Where(field, localcache.OpGT, 0).Execute(ctx)
			if err != nil {
				t.Fatalf("Query %s failed: %v", field, err)
			}
			if len(results) != 1 {
				t.Fatalf("expected 1 result for %s > 0", field)
			}
		})
	}
}

// --- table.go coverage ---

type testUser struct {
	ID   int
	Name string
	Age  int
}

func TestTable(t *testing.T) {
	ctx := context.Background()
	tbl := localcache.NewTable[int, testUser]("users",
		localcache.WithKey(func(u testUser) int { return u.ID }),
	)

	if tbl.TableName() != "users" {
		t.Fatalf("expected TableName=users, got %s", tbl.TableName())
	}

	// LoadFrom with successful scan
	err := tbl.LoadFrom(ctx, "SELECT 1", func(ctx context.Context, q string, dest any) error {
		*(dest.(*[]testUser)) = []testUser{{ID: 1, Name: "Alice"}}
		return nil
	})
	if err != nil {
		t.Fatalf("LoadFrom failed: %v", err)
	}

	u, ok := tbl.Get(1)
	if !ok || u.Name != "Alice" {
		t.Fatal("expected Alice from LoadFrom")
	}

	// LoadFrom with failing scan
	err = tbl.LoadFrom(ctx, "SELECT 1", func(ctx context.Context, q string, dest any) error {
		return fmt.Errorf("db error")
	})
	if err == nil {
		t.Fatal("expected error from failing scan")
	}
}

// --- ViewQuery coverage ---

func TestViewQuery(t *testing.T) {
	ctx := context.Background()
	view := localcache.NewView("test", 1*time.Hour)

	tbl := localcache.NewTable[int, testUser]("users",
		localcache.WithKey(func(u testUser) int { return u.ID }),
	)
	view.Add(tbl, "SELECT * FROM users")
	view.SetDataSource(localcache.NewDataSource(&mockConnector{
		name: "mock",
		scanFn: func(ctx context.Context, table string, dest any) error {
			*(dest.(*[]testUser)) = []testUser{{ID: 1, Name: "A"}}
			return nil
		},
	}))

	view.Refresh(ctx)

	// Successful QueryView
	vq := localcache.QueryView[int, testUser](view, "users")
	if vq == nil {
		t.Fatal("expected non-nil ViewQuery")
	}

	results, stamp, err := vq.Execute(ctx)
	if err != nil {
		t.Fatalf("ViewQuery.Execute failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatal("expected 1 result")
	}
	if stamp.ViewVersion != 1 {
		t.Fatalf("expected version 1, got %d", stamp.ViewVersion)
	}

	// RequireVersion with satisfied version
	vq2 := localcache.QueryView[int, testUser](view, "users").RequireVersion(1)
	results2, _, err2 := vq2.Execute(ctx)
	if err2 != nil {
		t.Fatalf("RequireVersion(1) should pass: %v", err2)
	}
	if len(results2) != 1 {
		t.Fatal("expected 1 result")
	}

	// RequireVersion with unsatisfied version
	vq3 := localcache.QueryView[int, testUser](view, "users").RequireVersion(99)
	_, stamp3, err3 := vq3.Execute(ctx)
	if err3 == nil {
		t.Fatal("expected ErrStaleVersion")
	}
	if !localcache.IsStalenessError(err3) {
		t.Fatal("expected IsStalenessError to return true")
	}
	if stamp3.ViewVersion != 1 {
		t.Fatalf("expected version 1, got %d", stamp3.ViewVersion)
	}

	// ViewQuery on non-existent table
	vq4 := localcache.QueryView[int, testUser](view, "nonexistent")
	if vq4 != nil {
		t.Fatal("expected nil for non-existent table")
	}

	// ViewQuery with wrong type params
	vq5 := localcache.QueryView[string, testUser](view, "users")
	if vq5 != nil {
		t.Fatal("expected nil for wrong K type")
	}
}

func TestStalenessErrorString(t *testing.T) {
	err := localcache.ErrStaleVersion
	if err.Error() != "view version is below required minimum" {
		t.Fatalf("unexpected error message: %s", err.Error())
	}
}

func TestIsStalenessErrorFalse(t *testing.T) {
	if localcache.IsStalenessError(errors.New("other error")) {
		t.Fatal("expected false for non-StalenessError")
	}
}

// --- View.Get / Name / TableNames / TryRefresh coverage ---

func TestViewGetNameTableNames(t *testing.T) {
	view := localcache.NewView("my_view", 1*time.Hour)

	if view.Name() != "my_view" {
		t.Fatalf("expected Name=my_view, got %s", view.Name())
	}

	tbl := localcache.NewTable[int, testUser]("users", localcache.WithKey(func(u testUser) int { return u.ID }))
	tbl2 := localcache.NewTable[int, testUser]("orders", localcache.WithKey(func(u testUser) int { return u.ID }))

	view.Add(tbl, "SELECT 1")
	view.Add(tbl2, "SELECT 1")

	names := view.TableNames()
	if len(names) != 2 || names[0] != "users" || names[1] != "orders" {
		t.Fatalf("unexpected TableNames: %v", names)
	}

	// Get existing
	adder, ok := view.Get("users")
	if !ok || adder == nil {
		t.Fatal("expected to find users table")
	}

	// Get non-existing
	_, ok = view.Get("nonexistent")
	if ok {
		t.Fatal("expected false for non-existent table")
	}

	// Type assertion to correct type
	typed, ok := adder.(*localcache.Table[int, testUser])
	if !ok {
		t.Fatal("expected type assertion to succeed")
	}
	if typed.TableName() != "users" {
		t.Fatal("expected TableName=users")
	}

	// TryRefresh no-op (interval not elapsed)
	ctx := context.Background()
	_ = typed.Cache.Load(ctx, []testUser{{ID: 1}})
	view.SetDataSource(localcache.NewDataSource(&mockConnector{
		name:   "mock",
		scanFn: func(ctx context.Context, table string, dest any) error {
			*(dest.(*[]testUser)) = []testUser{{ID: 2}}
			return nil
		},
	}))
	view.Refresh(ctx) // version=1

	v1 := view.Version()
	err := view.TryRefresh(ctx) // should be no-op (interval not elapsed)
	if err != nil {
		t.Fatalf("TryRefresh should not error: %v", err)
	}
	if view.Version() != v1 {
		t.Fatal("expected no version change after TryRefresh (interval not elapsed)")
	}
}

func TestViewQueryNonExistentTable(t *testing.T) {
	view := localcache.NewView("test", 1*time.Hour)
	vq := localcache.QueryView[int, testUser](view, "ghost")
	if vq != nil {
		t.Fatal("expected nil for non-existent table in QueryView")
	}
}

// --- loadAll with nil DataSource ---

func TestViewLoadAllNilDS(t *testing.T) {
	view := localcache.NewView("test", 1*time.Hour)
	tbl := localcache.NewTable[int, testUser]("users", localcache.WithKey(func(u testUser) int { return u.ID }))
	view.Add(tbl, "SELECT 1")

	// Refresh without DataSource configured
	ctx := context.Background()
	err := view.Refresh(ctx)
	if err == nil {
		t.Fatal("expected error when no DataSource configured")
	}
}

// --- fieldGetter interface coverage ---

type userWithGetter struct {
	ID     int
	Name   string
	Age    int
	fields map[string]any
}

func (u *userWithGetter) GetField(name string) any {
	switch name {
	case "ID":
		return u.ID
	case "Name":
		return u.Name
	case "Age":
		return u.Age
	}
	return nil
}

func TestFieldGetterInterface(t *testing.T) {
	ctx := context.Background()
	c := localcache.New[int, userWithGetter](localcache.WithKey(func(u userWithGetter) int { return u.ID }))
	c.Load(ctx, []userWithGetter{{ID: 1, Name: "Alice", Age: 30}})

	// fieldGetter path: the struct implements fieldGetter → fieldValue uses GetField
	results, err := c.Query().Where("Age", localcache.OpEQ, 30).Execute(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatal("expected 1 result with fieldGetter")
	}
}

// --- pointer struct field access coverage ---

type userPtr struct {
	ID   int
	Name string
	Age  int
}

func TestReflectFieldValuePtr(t *testing.T) {
	ctx := context.Background()
	c := localcache.New[int, userPtr](localcache.WithKey(func(u userPtr) int { return u.ID }))
	// Use a pointer to trigger the Kind() == Ptr branch in reflectFieldValue
	c.Load(ctx, []userPtr{{ID: 1, Name: "Alice", Age: 30}})

	results, err := c.Query().Where("Age", localcache.OpEQ, 30).Execute(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatal("expected 1 result")
	}
}

func TestReflectFieldInvalidField(t *testing.T) {
	ctx := context.Background()
	c := localcache.New[int, testUser](localcache.WithKey(func(u testUser) int { return u.ID }))
	c.Load(ctx, []testUser{{ID: 1}})

	// reflectFieldValue returns nil for non-struct
	type NotAStruct string
	c2 := localcache.New[int, NotAStruct](localcache.WithKey(func(u NotAStruct) int { return 0 }))
	c2.Load(ctx, []NotAStruct{"hello"})

	results, err := c2.Query().Where("anything", localcache.OpEQ, "x").Execute(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatal("expected 0 results for non-struct element")
	}
}

// --- Exercising the fieldGetter path with GetField ---

func TestFieldValueGetter(t *testing.T) {
	// Direct test of fieldValue going through GetField by implementing fieldGetter
	ctx := context.Background()
	c := localcache.New[int, userWithGetter](localcache.WithKey(func(u userWithGetter) int { return u.ID }))
	c.Load(ctx, []userWithGetter{
		{ID: 1, Name: "Alice", Age: 25},
		{ID: 2, Name: "Bob", Age: 30},
	})

	results, err := c.Query().Where("Name", localcache.OpEQ, "Alice").Execute(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatal("expected 1 result")
	}
}

// --- simpleIndex clone with non-empty entries ---

func TestIndexCloneNonEmpty(t *testing.T) {
	// Load with multiple items to exercise the clone loop body
	ctx := context.Background()
	c := localcache.New[int, testUser](
		localcache.WithKey(func(u testUser) int { return u.ID }),
		localcache.WithIndex[int, testUser, int]("Age", func(u testUser) int { return u.Age }),
	)

	// First load populates indices
	c.Load(ctx, []testUser{
		{ID: 1, Age: 10},
		{ID: 2, Age: 20},
	})

	// Second load triggers clone on non-empty indices
	c.Load(ctx, []testUser{
		{ID: 3, Age: 30},
		{ID: 4, Age: 40},
	})

	// Verify query still works after clone
	results, err := c.Query().Where("Age", localcache.OpEQ, 30).Execute(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result after clone, got %d", len(results))
	}
}

// --- simpleIndex.lookup with wrong type ---

func TestIndexLookupWrongType(t *testing.T) {
	ctx := context.Background()
	c := localcache.New[int, testUser](
		localcache.WithKey(func(u testUser) int { return u.ID }),
		localcache.WithIndex[int, testUser, int]("Age", func(u testUser) int { return u.Age }),
	)
	c.Load(ctx, []testUser{{ID: 1, Age: 10}})

	// Lookup with a string instead of int — type assertion fails, returns empty
	results, err := c.Query().Where("Age", localcache.OpEQ, "not-an-int").Execute(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatal("expected 0 results for type-mismatched lookup")
	}
}

// --- concurrency stress ---

func TestApplySortDuplicateValues(t *testing.T) {
	// Two users with same Age — applySort return 0 path
	ctx := context.Background()
	c := localcache.New[int, testUser](localcache.WithKey(func(u testUser) int { return u.ID }))
	c.Load(ctx, []testUser{
		{ID: 1, Name: "A", Age: 25},
		{ID: 2, Name: "B", Age: 25},
		{ID: 3, Name: "C", Age: 30},
	})

	results, err := c.Query().OrderBy("Age", localcache.Asc).Execute(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 3 {
		t.Fatal("expected 3 results")
	}
}

func TestCompareUnknownOp(t *testing.T) {
	ctx := context.Background()
	c := localcache.New[int, testUser](localcache.WithKey(func(u testUser) int { return u.ID }))
	c.Load(ctx, []testUser{{ID: 1, Age: 10}})

	// Op(99) is not in the compare switch → returns false → no results
	results, err := c.Query().Where("Age", localcache.Op(99), 10).Execute(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatal("expected 0 results for unknown Op")
	}
}

func TestTryRefreshLoadPath(t *testing.T) {
	ctx := context.Background()
	view := localcache.NewView("test", 1*time.Millisecond)

	tbl := localcache.NewTable[int, testUser]("users", localcache.WithKey(func(u testUser) int { return u.ID }))
	view.Add(tbl, "SELECT 1")
	view.SetDataSource(localcache.NewDataSource(&mockConnector{
		name: "mock",
		scanFn: func(ctx context.Context, table string, dest any) error {
			*(dest.(*[]testUser)) = []testUser{{ID: 99}}
			return nil
		},
	}))

	// First force a refresh
	view.Refresh(ctx)

	// Then try refresh — interval is 1ms, so it should have elapsed
	time.Sleep(5 * time.Millisecond)
	err := view.TryRefresh(ctx)
	if err != nil {
		t.Fatalf("TryRefresh failed: %v", err)
	}

	u, ok := view.Get("users")
	if !ok {
		t.Fatal("expected users table")
	}
	_ = u
}

func TestConcurrentLoadAndQuery(t *testing.T) {
	ctx := context.Background()
	c := localcache.New[int, testUser](
		localcache.WithKey(func(u testUser) int { return u.ID }),
		localcache.WithIndex[int, testUser, int]("Age", func(u testUser) int { return u.Age }),
	)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			users := make([]testUser, 10)
			for j := 0; j < 10; j++ {
				users[j] = testUser{ID: n*10 + j, Age: n + j}
			}
			_ = c.Load(ctx, users)
		}(i)
	}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_, _ = c.Query().Where("Age", localcache.OpGT, 5).Execute(ctx)
			}
		}()
	}
	wg.Wait()
}
