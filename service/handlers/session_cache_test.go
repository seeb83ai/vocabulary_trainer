package handlers

import (
	"testing"
	"time"
)

func countingLoad(calls *int, userID int64) func() (int64, time.Time, error) {
	return func() (int64, time.Time, error) { *calls++; return userID, time.Unix(100, 0), nil }
}

func TestSessionCache_Hit(t *testing.T) {
	c := newSessionCache(time.Minute)
	calls := 0
	now := time.Now()
	c.lookup("tok", now, countingLoad(&calls, 7))
	uid, _, err := c.lookup("tok", now, countingLoad(&calls, 7))
	if err != nil || uid != 7 {
		t.Fatalf("want user 7, got %d (%v)", uid, err)
	}
	if calls != 1 {
		t.Fatalf("expected a single load on cache hit, got %d", calls)
	}
}

func TestSessionCache_InvalidateReloads(t *testing.T) {
	c := newSessionCache(time.Minute)
	calls := 0
	now := time.Now()
	c.lookup("tok", now, countingLoad(&calls, 7))
	c.invalidate("tok")
	c.lookup("tok", now, countingLoad(&calls, 7))
	if calls != 2 {
		t.Fatalf("expected a reload after invalidate, got %d loads", calls)
	}
}

func TestSessionCache_InvalidateUserDropsAllSessionsOfUser(t *testing.T) {
	c := newSessionCache(time.Minute)
	calls := 0
	now := time.Now()
	c.lookup("a", now, countingLoad(&calls, 7))
	c.lookup("b", now, countingLoad(&calls, 7))
	c.lookup("c", now, countingLoad(&calls, 8))
	c.invalidateUser(7)
	calls = 0
	c.lookup("a", now, countingLoad(&calls, 7))
	c.lookup("b", now, countingLoad(&calls, 7))
	c.lookup("c", now, countingLoad(&calls, 8))
	if calls != 2 {
		t.Fatalf("want reloads for user 7's two sessions only, got %d", calls)
	}
}

func TestSessionCache_MissingSessionNotCached(t *testing.T) {
	c := newSessionCache(time.Minute)
	calls := 0
	now := time.Now()
	c.lookup("tok", now, countingLoad(&calls, 0))
	c.lookup("tok", now, countingLoad(&calls, 0))
	if calls != 2 {
		t.Fatalf("a missing session must not be cached, got %d loads", calls)
	}
}

func TestSessionCache_Expiry(t *testing.T) {
	c := newSessionCache(time.Second)
	calls := 0
	base := time.Now()
	c.lookup("tok", base, countingLoad(&calls, 7))
	c.lookup("tok", base.Add(2*time.Second), countingLoad(&calls, 7)) // past TTL
	if calls != 2 {
		t.Fatalf("expected a reload after TTL expiry, got %d loads", calls)
	}
}
