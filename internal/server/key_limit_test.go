package server

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestKeyLimiterPerKeyIsolation(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter := newKeyLimiter(4, 10, 2, time.Minute)
	for _, id := range []string{"key-a", "key-b"} {
		for i := 0; i < 2; i++ {
			if !limiter.Allow(id, now) {
				t.Fatalf("%s denied within its own burst", id)
			}
		}
		if limiter.Allow(id, now) {
			t.Fatalf("%s exceeded its burst", id)
		}
	}
	if !limiter.Allow("key-a", now.Add(100*time.Millisecond)) {
		t.Fatal("key-a did not refill at the configured rate")
	}
	if limiter.Allow("key-a", now.Add(100*time.Millisecond)) {
		t.Fatal("key-a refilled more than one token")
	}
	if !limiter.Allow("key-b", now.Add(100*time.Millisecond)) {
		t.Fatal("key-a consumed key-b's refill")
	}
	if limiter.Allow("", now.Add(time.Hour)) {
		t.Fatal("empty ID allowed")
	}
	if len(limiter.entries) != 2 {
		t.Fatal("empty ID changed the map")
	}
}

func TestKeyLimiterCapacityDoesNotResetActiveBuckets(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter := newKeyLimiter(2, 1, 1, time.Minute)
	if !limiter.Allow("a", now) || !limiter.Allow("b", now) {
		t.Fatal("initial keys denied")
	}
	for i := 0; i < 1000; i++ {
		if limiter.Allow(fmt.Sprintf("new-%d", i), now) {
			t.Fatal("new key admitted at active capacity")
		}
		if len(limiter.entries) > 2 || limiter.lru.Len() > 2 {
			t.Fatal("map or LRU exceeded capacity")
		}
	}
	if limiter.Allow("a", now) || limiter.Allow("b", now) {
		t.Fatal("capacity pressure reset an active bucket")
	}
}

func TestKeyLimiterIdleEvictionAndLRU(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter := newKeyLimiter(2, 0.01, 1, 10*time.Second)
	limiter.Allow("a", now)
	limiter.Allow("b", now)
	if limiter.Allow("a", now.Add(5*time.Second)) {
		t.Fatal("unexpected refill")
	}
	// A denied access still refreshes a and moves it behind b in the LRU.
	if got := limiter.lru.Front().Value.(*keyLimitEntry).id; got != "b" {
		t.Fatalf("oldest key = %q, want b", got)
	}
	if limiter.Allow("c", now.Add(10*time.Second-time.Nanosecond)) {
		t.Fatal("evicted a key before the idle boundary")
	}
	if !limiter.Allow("c", now.Add(10*time.Second)) {
		t.Fatal("idle b did not free capacity at the exact boundary")
	}
	if _, ok := limiter.entries["b"]; ok {
		t.Fatal("idle b retained")
	}
	if limiter.Allow("a", now.Add(10*time.Second)) {
		t.Fatal("active a's exhausted bucket was reset")
	}
	if !limiter.Allow("d", now.Add(20*time.Second)) {
		t.Fatal("expired entries did not free capacity")
	}
	if len(limiter.entries) != 1 || limiter.lru.Len() != 1 {
		t.Fatal("lazy cleanup did not remove all idle entries")
	}
	if !limiter.Allow("a", now.Add(20*time.Second)) {
		t.Fatal("evicted key did not receive a new bucket")
	}
}

func TestKeyLimiterBackwardTime(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter := newKeyLimiter(2, 1, 1, 10*time.Second)
	if !limiter.Allow("a", now) {
		t.Fatal("initial request denied")
	}
	for i := 0; i < 5; i++ {
		if limiter.Allow("a", now.Add(-time.Hour)) || limiter.Allow("a", now) {
			t.Fatal("backward timestamps minted tokens")
		}
	}
	if !limiter.Allow("b", now.Add(-time.Hour)) {
		t.Fatal("new key unexpectedly denied")
	}
	if limiter.entries["b"].Value.(*keyLimitEntry).lastSeen != now {
		t.Fatal("new key used a backward timestamp")
	}
	if !limiter.Allow("a", now.Add(time.Second)) {
		t.Fatal("forward time did not refill")
	}
	if limiter.Allow("c", now.Add(10*time.Second-time.Nanosecond)) {
		t.Fatal("backward access caused early idle eviction")
	}
	if !limiter.Allow("c", now.Add(10*time.Second)) {
		t.Fatal("oldest idle entry was not evicted")
	}
	if _, ok := limiter.entries["a"]; !ok {
		t.Fatal("newer active entry evicted")
	}
}

func TestKeyLimiterInvalidSettingsFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		max   int
		limit rate.Limit
		burst int
		idle  time.Duration
	}{
		{"zero capacity", 0, 1, 1, time.Minute},
		{"negative capacity", -1, 1, 1, time.Minute},
		{"zero rate", 1, 0, 1, time.Minute},
		{"negative rate", 1, -1, 1, time.Minute},
		{"NaN rate", 1, rate.Limit(math.NaN()), 1, time.Minute},
		{"zero burst", 1, 1, 0, time.Minute},
		{"negative burst", 1, 1, -1, time.Minute},
		{"zero idle", 1, 1, 1, 0},
		{"negative idle", 1, 1, 1, -time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limiter := newKeyLimiter(tc.max, tc.limit, tc.burst, tc.idle)
			if limiter.Allow("a", time.Now()) || len(limiter.entries) != 0 {
				t.Fatal("invalid configuration admitted or retained a key")
			}
		})
	}
}

func TestKeyLimiterConcurrentBurst(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter := newKeyLimiter(4, 10, 20, time.Minute)
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if limiter.Allow("shared", now) {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := allowed.Load(); got != 20 {
		t.Fatalf("concurrent admissions = %d, want 20", got)
	}
}

func TestKeyLimiterConcurrentCapacityAndExpiry(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter := newKeyLimiter(8, 1, 1, time.Minute)
	for round := 0; round < 3; round++ {
		var allowed atomic.Int64
		var wg sync.WaitGroup
		for i := 0; i < 64; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				if limiter.Allow(fmt.Sprintf("round-%d-key-%d", round, i), now.Add(time.Duration(round)*time.Minute)) {
					allowed.Add(1)
				}
			}(i)
		}
		wg.Wait()
		if allowed.Load() != 8 || len(limiter.entries) != 8 || limiter.lru.Len() != 8 {
			t.Fatalf("round %d: admitted %d, map %d, LRU %d; want 8 each", round, allowed.Load(), len(limiter.entries), limiter.lru.Len())
		}
	}
}
