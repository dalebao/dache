package localcache

import (
	"context"
	"time"
)

// TableAdder is the interface View uses to coordinate refresh.
// Implemented by Table[K, V].
type TableAdder interface {
	TableName() string
	LoadFrom(ctx context.Context, query string, scan func(ctx context.Context, q string, dest any) error) error
}

// Table is a named Cache bound to a query, ready to be added to a View.
type Table[K comparable, V any] struct {
	*Cache[K, V]
	name string
}

// NewTable creates a named Table.
//
//	users := localcache.NewTable("users",
//	    localcache.WithKey(func(u User) int { return u.ID }),
//	    localcache.WithIndex("age", func(u User) int { return u.Age }),
//	)
func NewTable[K comparable, V any](name string, opts ...Option[K, V]) *Table[K, V] {
	return &Table[K, V]{
		Cache: New(opts...),
		name:  name,
	}
}

func (t *Table[K, V]) TableName() string { return t.name }

func (t *Table[K, V]) LoadFrom(ctx context.Context, query string, scan func(ctx context.Context, q string, dest any) error) error {
	var items []V
	if err := scan(ctx, query, &items); err != nil {
		return err
	}
	return t.Cache.Load(ctx, items)
}

// ViewQuery extends Query with version/epoch metadata for consistency checks.
type ViewQuery[K comparable, V any] struct {
	*Query[K, V]
	stampFunc func() VersionStamp
	minVer    int64
}

// RequireVersion rejects the query if the View's version is below the threshold.
func (vq *ViewQuery[K, V]) RequireVersion(min int64) *ViewQuery[K, V] {
	vq.minVer = min
	return vq
}

// Execute runs the query and attaches a VersionStamp.
func (vq *ViewQuery[K, V]) Execute(ctx context.Context) ([]V, VersionStamp, error) {
	results, err := vq.Query.Execute(ctx)
	if err != nil {
		return nil, VersionStamp{}, err
	}
	stamp := vq.stampFunc()
	if vq.minVer > 0 && stamp.ViewVersion < vq.minVer {
		return nil, stamp, ErrStaleVersion
	}
	return results, stamp, nil
}

// VersionStamp captures the state of a View at query time.
type VersionStamp struct {
	ViewVersion int64
	Epoch       time.Time
}

// ErrStaleVersion is returned when a View's version is below the required minimum.
var ErrStaleVersion = &StalenessError{Message: "view version is below required minimum"}

// StalenessError indicates data did not meet freshness requirements.
type StalenessError struct {
	Message string
}

func (e *StalenessError) Error() string { return e.Message }

// IsStalenessError reports whether err is a StalenessError.
func IsStalenessError(err error) bool {
	_, ok := err.(*StalenessError)
	return ok
}
