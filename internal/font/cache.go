package font

import (
	"sync"
	"sync/atomic"
)

// cache is a concurrent map for read-mostly caches shared by goroutines.
// Lookups do not lock; it is emptied when it reaches max entries, so
// untrusted input cannot grow it without bound.
type cache[K comparable, V any] struct {
	m   sync.Map
	n   atomic.Int64
	max int64 // 0 means maxCachedGlyphs
}

func (c *cache[K, V]) get(k K) (V, bool) {
	v, ok := c.m.Load(k)
	if !ok {
		var zero V
		return zero, false
	}
	return v.(V), true
}

func (c *cache[K, V]) put(k K, v V) {
	limit := c.max
	if limit == 0 {
		limit = maxCachedGlyphs
	}
	if c.n.Add(1) > limit {
		c.m.Clear()
		c.n.Store(1)
	}
	c.m.Store(k, v)
}
