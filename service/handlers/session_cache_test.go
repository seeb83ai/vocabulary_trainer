package handlers

import (
	"testing"
	"time"
)

func TestInvalidationCache_Hit(t *testing.T) {
	c := newInvalidationCache(time.Minute)
	calls := 0
	load := func() (time.Time, error) { calls++; return time.Unix(100, 0), nil }
	now := time.Now()
	if _, err := c.lookup(7, now, load); err != nil {
		t.Fatal(err)
	}
	if _, err := c.lookup(7, now, load); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected a single load on cache hit, got %d", calls)
	}
}

func TestInvalidationCache_InvalidateReloads(t *testing.T) {
	c := newInvalidationCache(time.Minute)
	calls := 0
	load := func() (time.Time, error) { calls++; return time.Unix(int64(calls), 0), nil }
	now := time.Now()
	c.lookup(7, now, load)
	c.invalidate(7)
	if _, err := c.lookup(7, now, load); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected a reload after invalidate, got %d loads", calls)
	}
}

func TestInvalidationCache_Expiry(t *testing.T) {
	c := newInvalidationCache(time.Second)
	calls := 0
	load := func() (time.Time, error) { calls++; return time.Time{}, nil }
	base := time.Now()
	c.lookup(7, base, load)
	c.lookup(7, base.Add(2*time.Second), load) // past TTL
	if calls != 2 {
		t.Fatalf("expected a reload after TTL expiry, got %d loads", calls)
	}
}
