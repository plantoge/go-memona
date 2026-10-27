package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
	"memona/internal/access"
)

// These are process-local resource bounds, not durable quotas. In particular,
// pre-auth admission cannot know whether a credential will prove valid.
type requestSecurity struct {
	preAuth      *rate.Limiter
	invalidMCP   *rate.Limiter
	mcpPeers     *keyLimiter
	loginPeers   *keyLimiter
	authSlots    chan struct{}
	auditLimit   *rate.Limiter
	auditSlots   chan struct{}
	failures     atomic.Uint64
	limited      atomic.Uint64
	authBusy     atomic.Uint64
	auditSampled atomic.Uint64
	auditDropped atomic.Uint64
	auditErrors  atomic.Uint64
}

func (a *App) requestSecurityAPI(w http.ResponseWriter) {
	s := a.security
	respond(w, http.StatusOK, map[string]any{
		"scope": "process-local; resets on restart; aggregate only",
		"counters": map[string]uint64{
			"auth_failures": s.failures.Load(), "rate_limited": s.limited.Load(),
			"auth_busy": s.authBusy.Load(), "audit_sampled": s.auditSampled.Load(),
			"audit_dropped": s.auditDropped.Load(), "audit_errors": s.auditErrors.Load(),
		},
		"limits": map[string]any{
			"boundary_in_flight": 8, "authentication_in_flight": 2,
			"mcp_pre_auth":               map[string]any{"per_second": 120, "burst": 240},
			"mcp_direct_peer":            map[string]any{"per_second": 60, "burst": 120},
			"mcp_invalid":                map[string]any{"per_second": 10, "burst": 20},
			"mcp_authenticated":          map[string]any{"per_second": 30, "burst": 60},
			"mcp_key":                    map[string]any{"per_second": 10, "burst": 20},
			"login_aggregate_and_peer":   map[string]any{"refill_seconds": 12, "burst": 5},
			"bucket_table_capacity_each": 4096, "bucket_idle_seconds": 900,
			"failure_audit": map[string]any{"interval_seconds": 60, "burst": 1, "in_flight": 1, "timeout_milliseconds": 250},
		},
		"current": map[string]int{
			"boundary_in_flight": len(a.workSlots), "authentication_in_flight": len(s.authSlots),
			"audit_in_flight": len(s.auditSlots), "key_buckets": a.keyLimits.size(),
			"mcp_peer_buckets": s.mcpPeers.size(), "login_peer_buckets": s.loginPeers.size(),
		},
		"audit_retention": "append-only; no automatic purge; operational retention/growth policy outstanding",
	})
}

func newRequestSecurity() *requestSecurity {
	return &requestSecurity{
		preAuth: rate.NewLimiter(120, 240), invalidMCP: rate.NewLimiter(10, 20),
		mcpPeers:   newKeyLimiter(4096, 60, 120, 15*time.Minute),
		loginPeers: newKeyLimiter(4096, rate.Every(12*time.Second), 5, 15*time.Minute),
		authSlots:  make(chan struct{}, 2), auditLimit: rate.NewLimiter(rate.Every(time.Minute), 1),
		auditSlots: make(chan struct{}, 1),
	}
}

// directPeer deliberately ignores Forwarded/X-Forwarded-For/X-Real-IP. Reject
// malformed input into one shared bucket, not attacker-controlled table keys.
func directPeer(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "unknown"
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return "unknown"
	}
	return ip.WithZone("").Unmap().String()
}

func securityLimited(w http.ResponseWriter, retry string) {
	w.Header().Set("Retry-After", retry)
	fail(w, http.StatusTooManyRequests, "request limit exceeded")
}

func (a *App) reserveAuthentication(w http.ResponseWriter) bool {
	select {
	case a.security.authSlots <- struct{}{}:
		return true
	default:
		a.security.authBusy.Add(1)
		w.Header().Set("Retry-After", "1")
		fail(w, http.StatusServiceUnavailable, "authentication busy; retry later")
		return false
	}
}

// Fixed identity/action/target only; never pass request data to the audit API.
// Synchronous, bounded best effort: no unbounded background audit goroutines.
func (a *App) authenticationFailed(ctx context.Context) {
	s := a.security
	s.failures.Add(1)
	if !s.auditLimit.Allow() {
		s.auditDropped.Add(1)
		return
	}
	select {
	case s.auditSlots <- struct{}{}:
		defer func() { <-s.auditSlots }()
	default:
		s.auditDropped.Add(1)
		return
	}
	s.auditSampled.Add(1)
	ctx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	if err := a.auth.AuditContext(ctx, "anonymous", "auth.failure", ""); err != nil {
		s.auditErrors.Add(1)
	}
}

func (a *App) secureMCP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := a.security
		// Per-peer first: one rejected peer cannot spend the shared pre-auth
		// bucket. This budget is independent of authenticated MCP admission.
		if !s.mcpPeers.Allow(directPeer(r), time.Now()) || !s.preAuth.Allow() {
			s.limited.Add(1)
			securityLimited(w, "1")
			return
		}
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			a.authenticationFailed(r.Context())
			if !s.invalidMCP.Allow() {
				s.limited.Add(1)
				securityLimited(w, "1")
			} else {
				fail(w, 401, "API key required")
			}
			return
		}
		if !a.reserveAuthentication(w) {
			return
		}
		// Release the expensive-auth reservation before authenticated work;
		// the enclosing boundary's eight-slot cap remains held throughout.
		k, err := func() (access.Key, error) {
			defer func() { <-s.authSlots }()
			return a.auth.AuthenticateWorkspaceKeyContext(r.Context(), strings.TrimPrefix(h, "Bearer "))
		}()
		if err != nil {
			if errors.Is(err, access.ErrDenied) {
				a.authenticationFailed(r.Context())
				if !s.invalidMCP.Allow() {
					s.limited.Add(1)
					securityLimited(w, "1")
					return
				}
			}
			if !readFailure(w, err) {
				fail(w, 401, "invalid or expired API key")
			}
			return
		}
		if !a.keyLimits.Allow(k.ID, time.Now()) {
			s.limited.Add(1)
			w.Header().Set("Retry-After", "1")
			fail(w, 429, "API key request limit exceeded")
			return
		}
		// Rejected per-key calls never drain another key's shared budget.
		if !a.mcpLimit.Allow() {
			s.limited.Add(1)
			securityLimited(w, "1")
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), authenticatedKeyContext{}, k))
		next.ServeHTTP(w, r)
	})
}
