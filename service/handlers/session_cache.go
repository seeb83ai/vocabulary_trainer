package handlers

import (
	"sync"
	"time"
)

// invalidationCache is a small short-TTL cache of per-user sessions-invalidated-at
// cutoffs so session validation does not issue a DB lookup on every request.
// Entries are dropped explicitly on password change and otherwise expire after
// the TTL, bounding how long a stale cutoff can be served.
type invalidationCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[int64]invalidationEntry
}

type invalidationEntry struct {
	cut     time.Time
	expires time.Time
}

func newInvalidationCache(ttl time.Duration) *invalidationCache {
	return &invalidationCache{ttl: ttl, entries: make(map[int64]invalidationEntry)}
}

// lookup returns the cutoff for userID, calling load on a cache miss or expiry
// and caching the result for ttl. The DB lock is not held while load runs; a
// concurrent miss may load twice, which is harmless.
func (c *invalidationCache) lookup(userID int64, now time.Time, load func() (time.Time, error)) (time.Time, error) {
	c.mu.Lock()
	if e, ok := c.entries[userID]; ok && now.Before(e.expires) {
		c.mu.Unlock()
		return e.cut, nil
	}
	c.mu.Unlock()

	cut, err := load()
	if err != nil {
		return time.Time{}, err
	}
	c.mu.Lock()
	c.entries[userID] = invalidationEntry{cut: cut, expires: now.Add(c.ttl)}
	c.mu.Unlock()
	return cut, nil
}

// invalidate drops the cached entry for userID (called on password change).
func (c *invalidationCache) invalidate(userID int64) {
	c.mu.Lock()
	delete(c.entries, userID)
	c.mu.Unlock()
}
