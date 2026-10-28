package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"memona/internal/access"
	"memona/internal/memory"
)

type fixture struct {
	app   *App
	auth  *access.Store
	notes *memory.Store
}

func setup(t *testing.T) *fixture {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "private")
	notes, e := memory.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	auth, e := access.Open(dir, "test-admin-password-12345")
	if e != nil {
		notes.Close()
		t.Fatal(e)
	}
	app, e := New(Config{PublicURL: "http://localhost", Listen: "127.0.0.1:8765"}, auth, notes)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { auth.Close(); notes.Close() })
	return &fixture{app, auth, notes}
}
func request(app http.Handler, method, path, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	if csrf != "" {
		r.Header.Set("X-CSRF-Token", csrf)
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	return w
}
func login(t *testing.T, f *fixture) (*http.Cookie, string) {
	t.Helper()
	w := request(f.app, "POST", "/api/login", `{"password":"test-admin-password-12345"}`, nil, "")
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		CSRF string `json:"csrf"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	cookies := w.Result().Cookies()
	if len(cookies) == 0 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe cookie")
	}
	return cookies[0], out.CSRF
}
func TestDashboardAuthKeysAndCSRF(t *testing.T) {
	f := setup(t)
	if w := request(f.app, "GET", "/api/keys", "", nil, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	c, csrf := login(t, f)
	body := `{"name":"laptop","permissions":["read","write"],"projects":["simrs"]}`
	if w := request(f.app, "POST", "/api/keys", body, c, ""); w.Code != 403 {
		t.Fatal("CSRF not enforced", w.Code)
	}
	w := request(f.app, "POST", "/api/keys", body, c, csrf)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		Key   access.Key `json:"key"`
		Token string     `json:"token"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &created); e != nil {
		t.Fatal(e)
	}
	w = request(f.app, "GET", "/api/keys", "", c, "")
	if w.Code != 200 || strings.Contains(w.Body.String(), created.Token) {
		t.Fatal("secret leaked in key list")
	}
	w = request(f.app, "POST", "/api/keys/"+created.Key.ID+"/revoke", `{}`, c, csrf)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if _, e := f.auth.Authenticate(created.Token); e == nil {
		t.Fatal("revocation failed")
	}
	w = request(f.app, "POST", "/api/logout", `{}`, c, csrf)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = request(f.app, "GET", "/api/session", "", c, ""); w.Code != 401 {
		t.Fatal("logout failed")
	}
}
func TestHTTPBoundaryAndMarkdownSafety(t *testing.T) {
	f := setup(t)
	c, _ := login(t, f)
	r := httptest.NewRequest("GET", "http://attacker.example/api/notes", nil)
	r.AddCookie(c)
	w := httptest.NewRecorder()
	f.app.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("Host accepted")
	}
	r = httptest.NewRequest("POST", "http://localhost/api/login", strings.NewReader(`{}`))
	r.Header.Set("Origin", "https://attacker.example")
	w = httptest.NewRecorder()
	f.app.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("Origin accepted")
	}
	n, e := f.notes.Create(memory.Input{Project: "simrs", Kind: "fact", Title: "XSS test", Content: "<script>alert(1)</script>\n\n[x](javascript:alert(1))", Status: "unverified"}, "test")
	if e != nil {
		t.Fatal(e)
	}
	w = request(f.app, "GET", "/api/notes/"+n.ID, "", c, "")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var out struct {
		HTML string `json:"html"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if strings.Contains(out.HTML, "<script>") || strings.Contains(out.HTML, `href="javascript:`) {
		t.Fatal("unsafe rendered markdown", out.HTML)
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatal("missing CSP")
	}
	if w = request(f.app, "GET", "/.env", "", nil, ""); w.Code != 404 {
		t.Fatal("unexpected static file route")
	}
}

type bearerTransport struct {
	token string
	next  http.RoundTripper
}

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.next.RoundTrip(r)
}
func connect(t *testing.T, url, token string) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "memona-test-client", Version: "1"}, nil)
	session, e := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: url + "/mcp", HTTPClient: &http.Client{Transport: bearerTransport{token, http.DefaultTransport}}}, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
func call(t *testing.T, s *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, e := s.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func resultText(t *testing.T, r *mcp.CallToolResult) string {
	t.Helper()
	out := ""
	for _, c := range r.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			out += text.Text
		}
	}
	return out
}
func TestMCPRoundTripAndScopes(t *testing.T) {
	f := setup(t)
	ts := httptest.NewUnstartedServer(nil)
	cfg := Config{Listen: "127.0.0.1:8765", PublicURL: "http://" + ts.Listener.Addr().String()}
	app, e := New(cfg, f.auth, f.notes)
	if e != nil {
		t.Fatal(e)
	}
	ts.Config.Handler = app
	ts.Start()
	defer ts.Close()
	_, token, e := f.auth.CreateKey(access.KeyInput{Name: "opencode-test", Permissions: []string{"read", "write", "archive"}, Projects: []string{"simrs", "global"}})
	if e != nil {
		t.Fatal(e)
	}
	session := connect(t, ts.URL, token)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tools, e := session.ListTools(ctx, nil)
	if e != nil || len(tools.Tools) != 12 {
		t.Fatalf("tools %v %v", tools, e)
	}
	guide := call(t, session, "get_memory_guide", map[string]any{})
	if guide.IsError || !strings.Contains(resultText(t, guide), "Jangan gunakan status active") {
		t.Fatal("missing status guidance")
	}
	resources, e := session.ListResources(ctx, nil)
	if e != nil || len(resources.Resources) != 1 || resources.Resources[0].URI != "memona://guide" {
		t.Fatalf("guide resource discovery failed: %v", e)
	}
	resource, e := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "memona://guide"})
	if e != nil || len(resource.Contents) != 1 || !strings.Contains(resource.Contents[0].Text, "expected_revision") {
		t.Fatalf("guide resource read failed: %v", e)
	}
	invalid := call(t, session, "save_memory", map[string]any{"project": "simrs", "kind": "fact", "title": "Invalid status fixture", "content": "test", "status": "active", "source": "test"})
	if !invalid.IsError || !strings.Contains(resultText(t, invalid), "reserved for admin-published skills") {
		t.Fatal("status error not actionable", resultText(t, invalid))
	}
	in := map[string]any{"project": "simrs", "kind": "decision", "title": "Cross harness fixture", "content": "Use shared Markdown", "status": "verified", "source": "test fixture decision"}
	r := call(t, session, "save_memory", in)
	if r.IsError {
		t.Fatal(resultText(t, r))
	}
	var n memory.Note
	if e = json.Unmarshal([]byte(resultText(t, r)), &n); e != nil {
		t.Fatal(e)
	}
	if n.ID == "" {
		t.Fatal("missing note ID")
	}
	_, secondToken, e := f.auth.CreateKey(access.KeyInput{Name: "hermes-test", Permissions: []string{"read", "write"}, Projects: []string{"simrs"}})
	if e != nil {
		t.Fatal(e)
	}
	second := connect(t, ts.URL, secondToken)
	r = call(t, second, "read_memory", map[string]any{"id": n.ID})
	if r.IsError || !strings.Contains(resultText(t, r), "Use shared Markdown") {
		t.Fatal(resultText(t, r))
	}
	r = call(t, second, "save_handoff", map[string]any{"project": "simrs", "kind": "fact", "title": "Handoff", "content": "Verified fixture; next task pending.", "status": "unverified", "source": "test"})
	if r.IsError {
		t.Fatal(resultText(t, r))
	}
	r = call(t, session, "get_context", map[string]any{"project": "simrs"})
	if r.IsError || !strings.Contains(resultText(t, r), "Handoff") {
		t.Fatal(resultText(t, r))
	}
	// Other project cannot be read or found, even with a known ID.
	restrictedKey, restrictedToken, e := f.auth.CreateKey(access.KeyInput{Name: "restricted", Permissions: []string{"read"}, Projects: []string{"other"}})
	if e != nil {
		t.Fatal(e)
	}
	restricted := connect(t, ts.URL, restrictedToken)
	r = call(t, restricted, "read_memory", map[string]any{"id": n.ID})
	if !r.IsError {
		t.Fatal("cross-project read allowed")
	}
	r = call(t, restricted, "search_memory", map[string]any{})
	if r.IsError || strings.Contains(resultText(t, r), n.ID) {
		t.Fatal("search scope leak")
	}
	r = call(t, restricted, "save_memory", in)
	if !r.IsError {
		t.Fatal("read-only key wrote")
	}
	r = call(t, second, "archive_memory", map[string]any{"id": n.ID, "expected_revision": n.Revision})
	if !r.IsError {
		t.Fatal("archive permission bypass")
	}
	// Revisions enforce optimistic concurrency.
	revision := map[string]any{"id": n.ID, "expected_revision": n.Revision, "project": "simrs", "kind": "decision", "title": n.Title, "content": "Updated content", "status": "verified", "source": "test evidence"}
	r = call(t, session, "revise_memory", revision)
	if r.IsError {
		t.Fatal(resultText(t, r))
	}
	r = call(t, session, "revise_memory", revision)
	if !r.IsError {
		t.Fatal("stale revision accepted")
	}
	if e = f.auth.Revoke(restrictedKey.ID); e != nil {
		t.Fatal(e)
	}
	req, _ := http.NewRequest("POST", ts.URL+"/mcp", bytes.NewBufferString(`{}`))
	req.Header.Set("Authorization", "Bearer "+restrictedToken)
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatal("revoked key accepted", resp.StatusCode)
	}
	c, csrf := login(t, f)
	r = call(t, session, "save_memory", map[string]any{"project": "simrs", "kind": "skill", "title": "Test procedure", "content": "# Review\nRead and verify.", "status": "draft", "source": "test"})
	if r.IsError {
		t.Fatal(resultText(t, r))
	}
	var skill memory.Note
	json.Unmarshal([]byte(resultText(t, r)), &skill)
	r = call(t, session, "get_skill", map[string]any{"id": skill.ID})
	if !r.IsError {
		t.Fatal("draft skill exposed as active")
	}
	body, _ := json.Marshal(map[string]int{"expected_revision": skill.Revision})
	w := request(f.app, "POST", "/api/notes/"+skill.ID+"/publish", string(body), c, csrf)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	r = call(t, session, "get_skill", map[string]any{"id": skill.ID})
	if r.IsError {
		t.Fatal(resultText(t, r))
	}
	_, archiveToken, e := f.auth.CreateKey(access.KeyInput{Name: "archive-only", Permissions: []string{"archive"}, Projects: []string{"simrs"}})
	if e != nil {
		t.Fatal(e)
	}
	archiveOnly := connect(t, ts.URL, archiveToken)
	r = call(t, archiveOnly, "read_memory", map[string]any{"id": skill.ID})
	if !r.IsError {
		t.Fatal("archive key read content")
	}
	r = call(t, archiveOnly, "archive_memory", map[string]any{"id": skill.ID, "expected_revision": 2})
	if r.IsError {
		t.Fatal(resultText(t, r))
	}
	receipt := resultText(t, r)
	if strings.Contains(receipt, "content") || strings.Contains(receipt, "source") || strings.Contains(receipt, "title") {
		t.Fatal("archive leaked content", receipt)
	}
	w = request(f.app, "POST", "/api/notes/"+skill.ID+"/restore", `{"expected_revision":3}`, c, csrf)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	r = call(t, session, "get_skill", map[string]any{"id": skill.ID})
	if r.IsError {
		t.Fatal("restored skill unavailable", resultText(t, r))
	}
}
