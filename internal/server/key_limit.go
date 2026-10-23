package server

import (
	"container/list"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// keyLimiter holds process-local buckets indexed by stable key.ID or a
// canonical direct peer (separate tables). It never holds bearer tokens and
// never evicts active buckets to admit new identities: that would reset quotas.
// Cleanup is lazy, under the same lock as admission; no goroutine is needed.
type keyLimiter struct {
	mu         sync.Mutex
	maxEntries int
	perSec     rate.Limit
	burst      int
	idle       time.Duration
	entries    map[string]*list.Element
	lru        list.List // Oldest lastSeen first; ties follow access order.
	lastNow    time.Time
}

type keyLimitEntry struct {
	id       string
	bucket   *rate.Limiter
	lastSeen time.Time
}

// newKeyLimiter fails closed for nonpositive settings. Callers must supply the
// stable authenticated key ID or canonical direct peer, never a bearer token
// or a forwarded IP address. Peer and key budgets must use separate instances.
func newKeyLimiter(maxEntries int, perSec rate.Limit, burst int, idle time.Duration) *keyLimiter {
	return &keyLimiter{
		maxEntries: maxEntries,
		perSec:     perSec,
		burst:      burst,
		idle:       idle,
		entries:    make(map[string]*list.Element),
	}
}

// size returns only a bounded aggregate; bucket identities never leave the table.
func (l *keyLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

// Allow consumes one token, if available. Empty IDs and admission to a full map
// of active keys are denied. Even denied requests refresh an existing key's idle
// time, so sustained traffic cannot reset its bucket by letting it expire.
func (l *keyLimiter) Allow(id string, now time.Time) bool {
	if id == "" {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.maxEntries <= 0 || !(l.perSec > 0) || l.burst <= 0 || l.idle <= 0 {
		return false
	}

	// Serialize time as well as access. A backward wall clock (or out-of-order
	// caller timestamps) must not mint tokens or move the TTL/LRU clock back.
	if now.Before(l.lastNow) {
		now = l.lastNow
	}
	l.lastNow = now
	for oldest := l.lru.Front(); oldest != nil; oldest = l.lru.Front() {
		entry := oldest.Value.(*keyLimitEntry)
		if now.Sub(entry.lastSeen) < l.idle {
			break
		}
		delete(l.entries, entry.id)
		l.lru.Remove(oldest)
	}

	if element, ok := l.entries[id]; ok {
		entry := element.Value.(*keyLimitEntry)
		entry.lastSeen = now
		l.lru.MoveToBack(element)
		return entry.bucket.AllowN(now, 1)
	}
	if len(l.entries) >= l.maxEntries {
		return false
	}
	entry := &keyLimitEntry{id: id, bucket: rate.NewLimiter(l.perSec, l.burst), lastSeen: now}
	l.entries[id] = l.lru.PushBack(entry)
	return entry.bucket.AllowN(now, 1)
}
