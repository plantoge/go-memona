package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
	"memona/internal/access"
)

func securityKey(t *testing.T, f *fixture, expires string) (access.Key, string) {
	t.Helper()
	k, token, err := f.auth.CreateKey(access.KeyInput{Name: "synthetic", Permissions: []string{"read"}, Projects: []string{"synthetic"}, ExpiresAt: expires})
	if err != nil {
		t.Fatal(err)
	}
	return k, token
}

func securityRequest(h http.Handler, path, token, peer string, headers map[string]string) *httptest.ResponseRecorder {
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"security-test","version":"1"}}}`
	if path == "/api/login" {
		body = `{"password":"synthetic-wrong-password"}`
	}
	r := httptest.NewRequest("POST", "http://localhost"+path, strings.NewReader(body))
	r.RemoteAddr = peer
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func securityNoWork(a *App) http.Handler {
	return a.boundary(a.secureMCP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Context().Value(authenticatedKeyContext{}).(access.Key); !ok {
			panic("missing authenticated key")
		}
		w.WriteHeader(http.StatusNoContent)
	})))
}

func TestSecurityInvalidExhaustionDoesNotSpendAuthenticatedBudget(t *testing.T) {
	f := setup(t)
	_, token := securityKey(t, f, "")
	f.app.mcpLimit = rate.NewLimiter(1e-9, 1)
	for i := 0; i < 40; i++ {
		// Include syntactically plausible unknown credentials, not just missing headers.
		invalid := "mm_" + strings.Repeat("0", 64)
		if i%2 == 0 {
			invalid = ""
		}
		w := securityRequest(f.app, "/mcp", invalid, "192.0.2.1:1234", nil)
		want := 401
		if i >= 20 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("invalid request %d = %d, want %d", i, w.Code, want)
		}
	}
	// Real SDK initialization through the production route, using the same peer.
	if w := securityRequest(f.app, "/mcp", token, "192.0.2.1:1234", nil); w.Code != 200 {
		t.Fatalf("valid after invalid exhaustion = %d %s", w.Code, w.Body.String())
	}
	if f.app.security.failures.Load() != 40 || f.app.security.limited.Load() != 20 {
		t.Fatal("invalid traffic counters do not reflect failures/limits")
	}
}

func TestSecurityPerKeyRejectionsDoNotSpendSharedBudget(t *testing.T) {
	f := setup(t)
	_, noisy := securityKey(t, f, "")
	_, other := securityKey(t, f, "")
	f.app.keyLimits = newKeyLimiter(4096, 1e-9, 1, 15*time.Minute)
	f.app.mcpLimit = rate.NewLimiter(1e-9, 2)
	h := securityNoWork(f.app)
	if w := securityRequest(h, "/mcp", noisy, "192.0.2.1:1", nil); w.Code != 204 {
		t.Fatal(w.Code)
	}
	for i := 0; i < 80; i++ {
		w := securityRequest(h, "/mcp", noisy, "192.0.2.1:1", nil)
		if w.Code != 429 || !strings.Contains(w.Body.String(), "API key request limit") || w.Header().Get("Retry-After") == "" {
			t.Fatalf("noisy request %d = %d %s", i, w.Code, w.Body.String())
		}
	}
	if w := securityRequest(h, "/mcp", other, "192.0.2.1:1", nil); w.Code != 204 {
		t.Fatalf("other key starved = %d %s", w.Code, w.Body.String())
	}
}

func TestSecurityPeerSpoofingAndCanonicalization(t *testing.T) {
	for _, tc := range []struct{ remote, want string }{
		{"192.0.2.1:12", "192.0.2.1"}, {"192.0.2.1:99", "192.0.2.1"},
		{"[::ffff:192.0.2.1]:12", "192.0.2.1"}, {"[2001:0db8::1]:12", "2001:db8::1"},
		{"[fe80::1%eth0]:12", "fe80::1"}, {"not-an-ip:12", "unknown"}, {"broken", "unknown"},
	} {
		r := httptest.NewRequest("GET", "http://localhost", nil)
		r.RemoteAddr = tc.remote
		if got := directPeer(r); got != tc.want {
			t.Fatalf("peer(%q) = %q, want %q", tc.remote, got, tc.want)
		}
	}
	f := setup(t)
	_, token := securityKey(t, f, "")
	f.app.security.mcpPeers = newKeyLimiter(2, 1e-9, 1, 15*time.Minute)
	h := securityNoWork(f.app)
	if w := securityRequest(h, "/mcp", token, "192.0.2.1:12", nil); w.Code != 204 {
		t.Fatal(w.Code)
	}
	for i := 0; i < 20; i++ {
		headers := map[string]string{"Forwarded": fmt.Sprintf("for=198.51.100.%d", i), "X-Forwarded-For": fmt.Sprintf("198.51.100.%d", i), "X-Real-IP": fmt.Sprintf("198.51.100.%d", i)}
		if w := securityRequest(h, "/mcp", token, "192.0.2.1:99", headers); w.Code != 429 {
			t.Fatalf("spoof bypass = %d", w.Code)
		}
	}
	if w := securityRequest(h, "/mcp", token, "192.0.2.2:12", nil); w.Code != 204 {
		t.Fatalf("separate peer denied = %d", w.Code)
	}
	if len(f.app.security.mcpPeers.entries) != 2 {
		t.Fatal("forwarded headers added peer entries")
	}
}

func TestSecurityPreAuthBoundAndAuthSlotRelease(t *testing.T) {
	f := setup(t)
	_, token := securityKey(t, f, "")
	s := f.app.security
	h := securityNoWork(f.app)
	// Two expensive authentications can be outstanding, but cannot occupy all
	// eight boundary slots. The reservation is released on every auth outcome.
	for i := 0; i < cap(s.authSlots); i++ {
		s.authSlots <- struct{}{}
	}
	if w := securityRequest(h, "/mcp", token, "192.0.2.1:1", nil); w.Code != 503 {
		t.Fatal("auth concurrency cap not enforced", w.Code)
	}
	if w := request(f.app, "GET", "/healthz", "", nil, ""); w.Code != 200 {
		t.Fatal("auth reservation blocked health", w.Code)
	}
	for i := 0; i < cap(s.authSlots); i++ {
		<-s.authSlots
	}
	for _, tok := range []string{"mm_" + strings.Repeat("0", 64), token} {
		securityRequest(h, "/mcp", tok, "192.0.2.1:1", nil)
		if len(s.authSlots) != 0 || len(f.app.workSlots) != 0 {
			t.Fatal("request leaked reservations")
		}
	}
	s.preAuth = rate.NewLimiter(1e-9, 1)
	securityRequest(h, "/mcp", "", "192.0.2.1:1", nil)
	if w := securityRequest(h, "/mcp", token, "192.0.2.2:1", nil); w.Code != 429 {
		t.Fatal("shared pre-auth protection absent", w.Code)
	}
	// Pre-auth exhaustion does not alter the authenticated bucket itself.
	if !f.app.mcpLimit.Allow() {
		t.Fatal("pre-auth protection drained authenticated budget")
	}
}

func TestSecurityRevocationAndExpiryNotCached(t *testing.T) {
	f := setup(t)
	k, token := securityKey(t, f, "")
	h := securityNoWork(f.app)
	if w := securityRequest(h, "/mcp", token, "192.0.2.1:1", nil); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if err := f.auth.Revoke(k.ID); err != nil {
		t.Fatal(err)
	}
	if w := securityRequest(h, "/mcp", token, "192.0.2.1:1", nil); w.Code != 401 {
		t.Fatal("revoked key accepted", w.Code)
	}
	expires := time.Now().Add(2 * time.Second).Truncate(time.Second)
	_, expiring := securityKey(t, f, expires.Format(time.RFC3339))
	if w := securityRequest(h, "/mcp", expiring, "192.0.2.1:1", nil); w.Code != 204 {
		t.Fatal(w.Code)
	}
	time.Sleep(time.Until(expires) + time.Millisecond)
	if w := securityRequest(h, "/mcp", expiring, "192.0.2.1:1", nil); w.Code != 401 {
		t.Fatal("expired key accepted", w.Code)
	}
}

func TestSecurityLoginPeerAndAggregateCaps(t *testing.T) {
	f := setup(t)
	f.app.security.loginPeers = newKeyLimiter(4096, 1e-9, 1, 15*time.Minute)
	f.app.loginLimit = rate.NewLimiter(1e-9, 2)
	if w := securityRequest(f.app, "/api/login", "", "192.0.2.1:1", nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
	for i := 0; i < 10; i++ {
		if w := securityRequest(f.app, "/api/login", "", "192.0.2.1:2", map[string]string{"X-Forwarded-For": fmt.Sprintf("198.51.100.%d", i)}); w.Code != 429 {
			t.Fatal("peer login cap bypassed", w.Code)
		}
	}
	if w := securityRequest(f.app, "/api/login", "", "192.0.2.2:1", nil); w.Code != 401 {
		t.Fatal("peer rejections drained aggregate", w.Code)
	}
	if w := securityRequest(f.app, "/api/login", "", "192.0.2.3:1", nil); w.Code != 429 || w.Header().Get("Retry-After") != "12" {
		t.Fatal("aggregate login cap bypassed", w.Code)
	}
	if len(f.app.security.authSlots) != 0 {
		t.Fatal("login leaked auth slot")
	}
}

func TestSecurityFailureAuditSamplingRedactionAndBounds(t *testing.T) {
	f := setup(t)
	for i := 0; i < 50; i++ {
		securityRequest(f.app, "/mcp", "mm_"+strings.Repeat("a", 64), "192.0.2.123:4567", map[string]string{"X-Forwarded-For": "private-client-address"})
	}
	securityRequest(f.app, "/api/login", "", "192.0.2.123:4567", nil)
	events, err := f.auth.Activity()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Actor != "anonymous" || events[0].Action != "auth.failure" || events[0].Target != "" {
		t.Fatalf("audit contains request-specific data or unbounded events: %+v", events)
	}
	encoded, _ := json.Marshal(events)
	for _, secret := range []string{"mm_", "192.0.2.123", "private-client-address", "synthetic-wrong-password"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("audit leaked request data")
		}
	}
	s := f.app.security
	if s.failures.Load() != 51 || s.auditSampled.Load() != 1 || s.auditDropped.Load() != 50 || s.auditErrors.Load() != 0 {
		t.Fatal("failure sampling counters incorrect")
	}
	// Enforce the independent single audit-in-flight bound even if sampling
	// would otherwise allow another event. Canceled writes are counted too.
	s.auditLimit = rate.NewLimiter(rate.Inf, 1)
	s.auditSlots <- struct{}{}
	f.app.authenticationFailed(context.Background())
	<-s.auditSlots
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.app.authenticationFailed(ctx)
	if s.auditDropped.Load() != 51 || s.auditErrors.Load() != 1 || len(s.auditSlots) != 0 {
		t.Fatal("audit reservation or error counter incorrect")
	}
}

func TestSecurityAdminAggregateEndpoint(t *testing.T) {
	f := setup(t)
	if w := request(f.app, "GET", "/api/request-security", "", nil, ""); w.Code != 401 {
		t.Fatal("unauthorized security snapshot", w.Code)
	}
	_, token := securityKey(t, f, "")
	securityRequest(f.app, "/mcp", "mm_"+strings.Repeat("a", 64), "192.0.2.123:4567", nil)
	securityRequest(securityNoWork(f.app), "/mcp", token, "192.0.2.123:4567", nil)
	cookie, _ := login(t, f)
	w := request(f.app, "GET", "/api/request-security", "", cookie, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out struct {
		Counters  map[string]uint64          `json:"counters"`
		Limits    map[string]json.RawMessage `json:"limits"`
		Current   map[string]int             `json:"current"`
		Retention string                     `json:"audit_retention"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Counters["auth_failures"] != 1 || out.Counters["audit_sampled"] != 1 || out.Counters["audit_dropped"] != 0 || out.Counters["audit_errors"] != 0 {
		t.Fatal("wrong aggregate counters", out.Counters)
	}
	if out.Current["key_buckets"] != 1 || out.Current["mcp_peer_buckets"] != 1 || out.Current["login_peer_buckets"] != 1 || out.Current["boundary_in_flight"] != 1 {
		t.Fatal("wrong aggregate table/slot sizes", out.Current)
	}
	if string(out.Limits["boundary_in_flight"]) != "8" || string(out.Limits["authentication_in_flight"]) != "2" || string(out.Limits["bucket_table_capacity_each"]) != "4096" {
		t.Fatal("missing resource bounds", out.Limits)
	}
	if !strings.Contains(out.Retention, "append-only") || !strings.Contains(out.Retention, "no automatic purge") {
		t.Fatal("retention blocker missing")
	}
	for _, secret := range []string{token, "192.0.2.123", "mm_"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("snapshot leaked identity")
		}
	}
}

func TestSecurityPeerTablesBoundedWithoutActiveEviction(t *testing.T) {
	s := newRequestSecurity()
	now := time.Now()
	for _, table := range []*keyLimiter{s.mcpPeers, s.loginPeers} {
		for i := 0; i < 5000; i++ {
			allowed := table.Allow(fmt.Sprintf("192.0.%d.%d", i/256, i%256), now)
			if allowed != (i < 4096) {
				t.Fatalf("capacity admission %d = %v", i, allowed)
			}
		}
		if len(table.entries) != 4096 || table.lru.Len() != 4096 {
			t.Fatal("peer table exceeded capacity")
		}
		if !table.Allow("new-peer", now.Add(15*time.Minute)) || len(table.entries) != 1 {
			t.Fatal("peer table failed idle eviction")
		}
	}
}
