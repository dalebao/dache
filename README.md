# dache — Typed, Indexed In-Memory Cache for Go

**dache** is a generic, thread-safe in-memory cache library with ORM-style query capabilities, secondary indexes, unified data source abstraction with fallback chains, and a versioned View layer for cross-table temporal consistency.

## Features

| Layer | Type | Description |
|-------|------|-------------|
| **Cache** | `Cache[K, V]` | Generic typed map with RWMutex, atomic bulk replacement (`Load`), point lookup (`Get`), consistent snapshot |
| **Index** | `WithIndex` | Secondary index on any field; index type is type-safe via Go generics |
| **Query** | `Query[K, V]` | Clone-on-write builder: `Where`, `OrderBy`, `Limit`, `Offset`; operators: `EQ`, `GT`, `GTE`, `LT`, `LTE`, `In` |
| **Planner** | automatic | Selects the most selective index among multiple `OpEQ` filters to minimize scan |
| **Refresh** | `Group` / `RefreshGroup` | Time-interval gated refresh; optional background goroutine loop with `Start`/`Stop` |
| **DataSource** | `DataSource` | Primary + fallback connector chain; auto-degradation detection |
| **Connector** | interface | Unified `ScanAll`/`Ping`/`Close`; implementations for SQL, Redis, Memcached; adapter via `ConnectorFunc` |
| **View** | `View` | Multi-table coordinated refresh with all-or-nothing semantics, atomic version counter, and epoch timestamp |
| **Staleness** | `RequireVersion` | `ViewQuery` with minimum version guard; returns `StalenessError` when data is too stale |

## Quick Start

```go
import "github.com/dalebao/dache"

type Product struct {
    ID       int
    Name     string
    Category string
    Price    float64
}

cache := localcache.New(
    localcache.WithKey(func(p Product) int { return p.ID }),
    localcache.WithIndex("Category", func(p Product) string { return p.Category }),
)

cache.Load(ctx, []Product{...})

// Query builder
results, _ := cache.Query().
    Where("Category", localcache.OpEQ, "Electronics").
    Where("Price", localcache.OpGTE, 100.0).
    OrderBy("Price", localcache.Asc).
    Limit(10).
    Execute(ctx)
```

## View — Coordinated Multi-Table Consistency

```go
view := localcache.NewView("dashboard", 5*time.Minute)
view.Add(products, "SELECT * FROM products")
view.Add(customers, "SELECT * FROM customers")
view.SetDataSource(localcache.NewDataSource(mysqlConn))

view.Refresh(ctx) // atomic: all-or-nothing, version++

results, stamp, _ := localcache.QueryView[int, Product](view, "products").
    RequireVersion(1).
    Where("Category", localcache.OpEQ, "Electronics").
    Execute(ctx)
// stamp.ViewVersion, stamp.Epoch
```

## DataSource with Fallback

```go
ds := localcache.NewDataSource(primaryMySQL, fallbackRedis)
ds.ScanAll(ctx, "products", &products)
if ds.Degraded() { /* primary is down */ }
```

## Connector Implementations

| Package | Source |
|---------|--------|
| `conn/sql` | `database/sql` (MySQL, PostgreSQL, SQLite, etc.) |
| `conn/redis` | `go-redis/v9` |
| `conn/memcache` | `gomemcache` |

## Build & Test

```bash
go build ./...
go test -race -count=1 ./...
go test -coverprofile=coverage.out ./...
```

## Design Principles

- **Generics-first API**: `Cache[K, V]`, `Table[K, V]`, `ViewQuery[K, V]` provide compile-time type safety
- **Non-generic interface bridge**: `TableAdder` interface enables heterogeneous table storage in `View`
- **Clone-on-write queries**: Every `Where`/`OrderBy`/`Limit`/`Offset` call returns a new `Query`, enabling safe reuse
- **Atomic replacement**: `Cache.Load` builds new maps and index maps under the write lock, then atomically swaps pointers — zero-downtime refresh
- **Index-aware query planner**: Among multiple `OpEQ` filters, selects the index with fewest candidates for minimal scan

## Scenario Tests

The file `scenario_test.go` provides an e-commerce order management scenario demonstrating all major features:

- **Product Inventory**: multi-index cache, index planner optimization, multi-field WHERE/ORDER BY, OpIn
- **Customer Segments**: competing index selection, multi-field sort string ordering, empty result edges
- **Order Dashboard**: time.Time range queries via fallback comparison, cross-cache enrichment, status/customer indexing
- **DataSource Fallback**: primary failure → fallback serves stale data, all-sources-fail error, Degraded() signal
- **View Consistency**: version stamping, RequireVersion, StalenessError, all-or-nothing semantics, TryRefresh interval gating
- **Concurrent Workload**: 10 concurrent readers + 3 writers + 5 snapshot readers under race detector
