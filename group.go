package localcache

import (
	"context"
	"sync"
	"time"
)

// Group manages a shared refresh cycle for multiple caches.
// It coordinates refresh timing so that all associated caches
// are reloaded together, ensuring cross-cache temporal consistency.
type Group struct {
	interval    time.Duration
	lastRefresh time.Time
	mu          sync.Mutex
	once        sync.Once
}

// NewGroup creates a new Group with the given refresh interval.
func NewGroup(interval time.Duration) *Group {
	if interval <= 0 {
		panic("localcache: Group interval must be positive")
	}
	return &Group{interval: interval}
}

// Refresh calls fn if the refresh interval has elapsed since the last refresh.
// elapsed is the duration since the last refresh (useful for incremental loads).
// Returns nil if the interval hasn't elapsed yet (no-op).
func (g *Group) Refresh(ctx context.Context, fn func(context.Context) error) error {
	g.mu.Lock()
	elapsed := time.Since(g.lastRefresh)
	if elapsed < g.interval && !g.lastRefresh.IsZero() {
		g.mu.Unlock()
		return nil
	}
	g.mu.Unlock()

	if err := fn(ctx); err != nil {
		return err
	}

	g.mu.Lock()
	g.lastRefresh = time.Now()
	g.mu.Unlock()
	return nil
}

// ForceRefresh calls fn immediately regardless of the interval.
func (g *Group) ForceRefresh(ctx context.Context, fn func(context.Context) error) error {
	if err := fn(ctx); err != nil {
		return err
	}
	g.mu.Lock()
	g.lastRefresh = time.Now()
	g.mu.Unlock()
	return nil
}

// LastRefresh returns the last refresh time.
func (g *Group) LastRefresh() time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.lastRefresh
}

// IsExpired reports whether the refresh interval has elapsed.
func (g *Group) IsExpired() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.lastRefresh.IsZero() {
		return true
	}
	return time.Since(g.lastRefresh) >= g.interval
}

// RefreshGroup extends Group with automatic background refresh.
// It ticks at the given interval and calls the loader function.
type RefreshGroup struct {
	*Group
	stopCh chan struct{}
}

// NewRefreshGroup creates a new RefreshGroup.
func NewRefreshGroup(interval time.Duration) *RefreshGroup {
	return &RefreshGroup{
		Group:  NewGroup(interval),
		stopCh: make(chan struct{}),
	}
}

// Start begins a background refresh loop.
// fn is called each tick to reload data into the associated caches.
// The loop stops when ctx is cancelled.
func (rg *RefreshGroup) Start(ctx context.Context, fn func(context.Context) error) {
	rg.once.Do(func() {
		go rg.loop(ctx, fn)
	})
}

func (rg *RefreshGroup) loop(ctx context.Context, fn func(context.Context) error) {
	ticker := time.NewTicker(rg.interval)
	defer ticker.Stop()

	// Do an immediate first refresh
	_ = rg.ForceRefresh(ctx, fn)

	for {
		select {
		case <-ticker.C:
			_ = rg.Refresh(ctx, fn)
		case <-ctx.Done():
			return
		case <-rg.stopCh:
			return
		}
	}
}

// Stop stops the background refresh loop.
func (rg *RefreshGroup) Stop() {
	close(rg.stopCh)
}
