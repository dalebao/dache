// Package localcache provides a typed, indexed in-memory cache
// with ORM-style query capabilities and unified refresh groups.
package localcache

import (
	"context"
	"fmt"
	"sync"
)

// Cache is a typed in-memory cache with index support.
// K is the key type (comparable), V is the value type.
type Cache[K comparable, V any] struct {
	mu      sync.RWMutex
	items   map[K]*V
	indices map[string]indexer[V]
	keyFn   func(V) K
}

// Option configures a Cache at creation time.
type Option[K comparable, V any] interface {
	apply(*Cache[K, V])
}

type optionFunc[K comparable, V any] func(*Cache[K, V])

func (f optionFunc[K, V]) apply(c *Cache[K, V]) { f(c) }

// WithKey sets the key extraction function.
// Required for Get and internal deduplication.
func WithKey[K comparable, V any](fn func(V) K) Option[K, V] {
	return optionFunc[K, V](func(c *Cache[K, V]) {
		c.keyFn = fn
	})
}

// WithIndex creates an index on a field.
// The fn extracts the indexed value from a record.
func WithIndex[K comparable, V any, I comparable](name string, fn func(V) I) Option[K, V] {
	return optionFunc[K, V](func(c *Cache[K, V]) {
		if c.indices == nil {
			c.indices = make(map[string]indexer[V])
		}
		c.indices[name] = &simpleIndex[V, I]{
			name:    name,
			extract: func(v *V) I { return fn(*v) },
			entries: make(map[I][]*V),
		}
	})
}

// New creates a new Cache.
// At minimum, WithKey must be provided.
func New[K comparable, V any](opts ...Option[K, V]) *Cache[K, V] {
	c := &Cache[K, V]{
		items:   make(map[K]*V),
		indices: make(map[string]indexer[V]),
	}
	for _, opt := range opts {
		opt.apply(c)
	}
	if c.keyFn == nil {
		panic("localcache: WithKey option is required")
	}
	return c
}

// Load replaces all data in the cache atomically.
// Existing data is discarded and all indices are rebuilt.
func (c *Cache[K, V]) Load(ctx context.Context, items []V) error {
	newItems := make(map[K]*V, len(items))
	for i := range items {
		v := &items[i]
		newItems[c.keyFn(*v)] = v
	}

	newIndices := make(map[string]indexer[V])
	for name, idx := range c.indices {
		newIndices[name] = idx.clone()
	}

	for _, v := range newItems {
		for _, idx := range newIndices {
			idx.add(v)
		}
	}

	c.mu.Lock()
	c.items = newItems
	c.indices = newIndices
	c.mu.Unlock()

	return nil
}

// Get retrieves a value by key.
// Returns the zero value and false if not found.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	v, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	return *v, true
}

// Query returns a new Query builder for this cache.
func (c *Cache[K, V]) Query() *Query[K, V] {
	return &Query[K, V]{cache: c}
}

// Len returns the number of items in the cache.
func (c *Cache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Snapshot returns a consistent read snapshot of all items.
func (c *Cache[K, V]) Snapshot() []V {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]V, 0, len(c.items))
	for _, v := range c.items {
		result = append(result, *v)
	}
	return result
}

// indexer is the internal interface for all index types.
type indexer[V any] interface {
	name() string
	add(*V)
	clone() indexer[V]
	lookup(any) []*V
}

type simpleIndex[V any, I comparable] struct {
	name    string
	extract func(*V) I
	entries map[I][]*V
}

func (idx *simpleIndex[V, I]) name() string { return idx.name }

func (idx *simpleIndex[V, I]) add(v *V) {
	key := idx.extract(v)
	idx.entries[key] = append(idx.entries[key], v)
}

func (idx *simpleIndex[V, I]) clone() indexer[V] {
	entries := make(map[I][]*V, len(idx.entries))
	for k, v := range idx.entries {
		slice := make([]*V, len(v))
		copy(slice, v)
		entries[k] = slice
	}
	return &simpleIndex[V, I]{
		name:    idx.name,
		extract: idx.extract,
		entries: entries,
	}
}

func (idx *simpleIndex[V, I]) lookup(val any) []*V {
	key, ok := val.(I)
	if !ok {
		return nil
	}
	return idx.entries[key]
}

// ErrIndexNotFound is returned when querying a nonexistent index.
var ErrIndexNotFound = fmt.Errorf("localcache: index not found")
