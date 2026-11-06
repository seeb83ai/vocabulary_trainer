package handlers

import (
	"sync"
	"time"
)

// sessionCache is a small short-TTL cache of validated session tokens so
// session validation does not issue a DB lookup on every request. Entries are
// dropped explicitly on logout and password change (the server is a single
// process, so this takes effect at once) and otherwise expire after the TTL.
type sessionCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]sessionEntry
}

type sessionEntry struct {
	userID      int64
	expiresAt   time.Time // when the session itself expires
	cachedUntil time.Time
}

func newSessionCache(ttl time.Duration) *sessionCache {
	return &sessionCache{ttl: ttl, entries: make(map[string]sessionEntry)}
}

// lookup returns the user ID and session expiry for token, calling load on a
// cache miss or expiry. A missing session (user ID 0) is not cached, so a new
// login is seen at once. The lock is not held while load runs; a concurrent
// miss may load twice, which is harmless.
func (c *sessionCache) lookup(token string, now time.Time, load func() (int64, time.Time, error)) (int64, time.Time, error) {
	c.mu.Lock()
	if e, ok := c.entries[token]; ok && now.Before(e.cachedUntil) {
		c.mu.Unlock()
		return e.userID, e.expiresAt, nil
	}
	c.mu.Unlock()

	userID, expiresAt, err := load()
	if err != nil || userID == 0 {
		return userID, expiresAt, err
	}
	c.mu.Lock()
	c.entries[token] = sessionEntry{userID: userID, expiresAt: expiresAt, cachedUntil: now.Add(c.ttl)}
	c.mu.Unlock()
	return userID, expiresAt, nil
}

// invalidate drops the cached entry for token (called on logout).
func (c *sessionCache) invalidate(token string) {
	c.mu.Lock()
	delete(c.entries, token)
	c.mu.Unlock()
}

// invalidateUser drops all cached sessions of userID (called on password change).
func (c *sessionCache) invalidateUser(userID int64) {
	c.mu.Lock()
	for token, e := range c.entries {
		if e.userID == userID {
			delete(c.entries, token)
		}
	}
	c.mu.Unlock()
}
