package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"memona/internal/access"
	"memona/internal/memory"
)

func operationArgs() map[string]any {
	return map[string]any{"project": "demo", "kind": "fact", "title": "Idempotent fixture", "content": "Synthetic operation", "status": "unverified", "source": "", "operation_id": "retry-synthetic-001"}
}

func operationNote(t *testing.T, session *mcp.ClientSession, tool string, args map[string]any) memory.Note {
	t.Helper()
	r := call(t, session, tool, args)
	if r.IsError {
		t.Fatal(resultText(t, r))
	}
	var n memory.Note
	if err := json.Unmarshal([]byte(resultText(t, r)), &n); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(resultText(t, r), "_operation") {
		t.Fatal("internal operation receipt exposed")
	}
	return n
}

func operationServer(t *testing.T, f *fixture) *httptest.Server {
	t.Helper()
	ts := httptest.NewUnstartedServer(nil)
	app, err := New(Config{Listen: "127.0.0.1:8765", PublicURL: "http://" + ts.Listener.Addr().String()}, f.auth, f.notes)
	if err != nil {
		t.Fatal(err)
	}
	ts.Config.Handler = app
	ts.Start()
	t.Cleanup(ts.Close)
	return ts
}

func operationKey(t *testing.T, f *fixture, permissions, projects []string) (access.Key, string) {
	t.Helper()
	k, token, err := f.auth.CreateKey(access.KeyInput{Name: "operation-fixture", Permissions: permissions, Projects: projects})
	if err != nil {
		t.Fatal(err)
	}
	return k, token
}

func TestMCPOperationRetryConflictsAndNamespaces(t *testing.T) {
	f := setup(t)
	ts := operationServer(t, f)
	_, token := operationKey(t, f, []string{"read", "write", "archive"}, []string{"demo", "other"})
	session := connect(t, ts.URL, token)
	args := operationArgs()
	first := operationNote(t, session, "save_memory", args)
	if n := operationNote(t, session, "save_memory", args); !reflect.DeepEqual(n, first) {
		t.Fatal("save retry did not replay original result")
	}
	args["related_to"] = []string{}
	if r := call(t, session, "save_memory", args); !r.IsError {
		t.Fatal("MCP omitted/clear metadata shared payload fingerprint")
	}
	delete(args, "related_to")
	args["content"] = "changed payload"
	r := call(t, session, "save_memory", args)
	if !r.IsError || !strings.Contains(resultText(t, r), "operation_id already used") {
		t.Fatal("different payload was not rejected")
	}
	args = operationArgs()
	args["project"] = "other"
	if r := call(t, session, "save_memory", args); !r.IsError || strings.Contains(resultText(t, r), first.ID) {
		t.Fatal("cross-project replay accepted/leaked")
	}
	args = operationArgs()
	handoff := operationNote(t, session, "save_handoff", args)
	if handoff.ID == first.ID || handoff.Kind != "handoff" {
		t.Fatal("tools shared namespace")
	}
	if n := operationNote(t, session, "save_handoff", args); !reflect.DeepEqual(n, handoff) {
		t.Fatal("handoff retry duplicated")
	}
	_, token2 := operationKey(t, f, []string{"read", "write"}, []string{"demo"})
	other := operationNote(t, connect(t, ts.URL, token2), "save_memory", args)
	if other.ID == first.ID {
		t.Fatal("keys shared operation namespace")
	}
	args["id"], args["expected_revision"], args["content"] = first.ID, 1, "new revision"
	second := operationNote(t, session, "revise_memory", args)
	archiveArgs := map[string]any{"id": first.ID, "expected_revision": 2, "operation_id": "retry-synthetic-001"}
	archived := call(t, session, "archive_memory", archiveArgs)
	if archived.IsError {
		t.Fatal(resultText(t, archived))
	}
	// Replay succeeds even though expected_revision is now stale and note archived.
	if n := operationNote(t, session, "revise_memory", args); !reflect.DeepEqual(n, second) {
		t.Fatal("revise retry returned current state instead of original revision")
	}
	if n := operationNote(t, session, "save_memory", operationArgs()); !reflect.DeepEqual(n, first) {
		t.Fatal("create retry returned current archived revision")
	}
	if _, err := f.notes.Restore(first.ID, 3, "admin"); err != nil {
		t.Fatal(err)
	}
	if r := call(t, session, "archive_memory", archiveArgs); r.IsError || resultText(t, r) != resultText(t, archived) {
		t.Fatal("archive retry re-archived restored note")
	}
	latest, err := f.notes.Get(first.ID)
	if err != nil || latest.Revision != 4 || latest.Archived {
		t.Fatal("replay altered latest state", err)
	}
	args["expected_revision"] = 2
	if r := call(t, session, "revise_memory", args); !r.IsError {
		t.Fatal("changed expected_revision did not conflict")
	}
	args["id"], args["expected_revision"] = other.ID, 1
	if r := call(t, session, "revise_memory", args); !r.IsError || strings.Contains(resultText(t, r), second.Content) {
		t.Fatal("changed target did not conflict or leaked previous result")
	}
	for _, invalid := range []string{"short", "../unsafe-operation", strings.Repeat("x", 129)} {
		args = operationArgs()
		args["operation_id"] = invalid
		if r := call(t, session, "save_memory", args); !r.IsError {
			t.Fatal("invalid operation ID accepted")
		}
	}
}

func TestMCPOperationLegacyAndEmptyIDRemainNonIdempotent(t *testing.T) {
	f := setup(t)
	ts := operationServer(t, f)
	_, token := operationKey(t, f, []string{"write"}, []string{"demo"})
	session := connect(t, ts.URL, token)
	args := operationArgs()
	delete(args, "operation_id")
	first := operationNote(t, session, "save_memory", args)
	second := operationNote(t, session, "save_memory", args)
	if first.ID == second.ID {
		t.Fatal("legacy create deduplicated without operation ID")
	}
	args["operation_id"] = ""
	third := operationNote(t, session, "save_memory", args)
	if third.ID == second.ID || third.ID == first.ID {
		t.Fatal("empty operation ID unexpectedly enabled idempotency")
	}
}

func TestMCPOperationAuthorizationAndRedaction(t *testing.T) {
	f := setup(t)
	ts := operationServer(t, f)
	k, _ := operationKey(t, f, []string{"read", "write", "archive"}, []string{"demo"})
	args := operationArgs()
	op := memory.Operation{Principal: k.ID, Tool: "save_memory", ID: args["operation_id"].(string)}
	input := memory.Input{Project: "demo", Kind: "fact", Title: args["title"].(string), Content: args["content"].(string), Status: "unverified"}
	n, err := f.notes.Execute(op, input, k.Actor())
	if err != nil {
		t.Fatal(err)
	}
	// Same stable principal, but simulate permissions/scope narrowing. A cached
	// receipt must not bypass the current handler checks (no key-edit API added).
	for _, current := range []access.Key{
		{ID: k.ID, Name: k.Name, Permissions: []string{"read"}, Projects: []string{"demo"}},
		{ID: k.ID, Name: k.Name, Permissions: []string{"write"}, Projects: []string{"other"}},
	} {
		client := mcp.NewClient(&mcp.Implementation{Name: "permission-test", Version: "1"}, nil)
		serverTransport, clientTransport := mcp.NewInMemoryTransports()
		serverSession, err := f.app.mcpServer(current).Connect(t.Context(), serverTransport, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { serverSession.Close() })
		session, err := client.Connect(t.Context(), clientTransport, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { session.Close() })
		r := call(t, session, "save_memory", args)
		if !r.IsError || strings.Contains(resultText(t, r), n.ID) || strings.Contains(resultText(t, r), n.Content) {
			t.Fatal("replay bypassed current authorization")
		}
	}
	_, writeToken := operationKey(t, f, []string{"write"}, []string{"demo"})
	writer := connect(t, ts.URL, writeToken)
	target := makeNote(t, f, "demo", "fact", "Private link target", nil)
	linked := makeNote(t, f, "demo", "fact", "Linked note", func(in *memory.Input) { in.RelatedTo = []string{target.ID} })
	args = operationArgs()
	args["id"], args["expected_revision"] = linked.ID, 1
	for i := 0; i < 2; i++ {
		r := call(t, writer, "revise_memory", args)
		if r.IsError || strings.Contains(resultText(t, r), target.ID) || strings.Contains(resultText(t, r), "_operation") {
			t.Fatal("write-only replay disclosed preserved metadata")
		}
	}
	_, archiveToken := operationKey(t, f, []string{"archive"}, []string{"demo"})
	archiver := connect(t, ts.URL, archiveToken)
	archiveArgs := map[string]any{"id": linked.ID, "expected_revision": 2, "operation_id": "archive-only-001"}
	for i := 0; i < 2; i++ {
		r := call(t, archiver, "archive_memory", archiveArgs)
		if r.IsError || strings.Contains(resultText(t, r), "content") || strings.Contains(resultText(t, r), target.ID) {
			t.Fatal("archive-only replay disclosed content")
		}
	}
}

type dropCommittedResponse struct {
	base http.RoundTripper
}

func (d dropCommittedResponse) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := d.base.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	// Let the server finish/commit, but deliver none of its bytes to the caller.
	_, err = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if err != nil {
		return nil, err
	}
	return nil, context.DeadlineExceeded
}

func TestMCPOperationTimeoutAfterCommitAndConcurrentRetry(t *testing.T) {
	f := setup(t)
	ts := operationServer(t, f)
	key, token := operationKey(t, f, []string{"read", "write"}, []string{"demo"})
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "save_memory", "arguments": operationArgs()}})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(t.Context(), "POST", ts.URL+"/mcp", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Transport: dropCommittedResponse{http.DefaultTransport}}
	if _, err := client.Do(req); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("expected simulated response loss", err)
	}
	before, err := f.notes.List()
	if err != nil || len(before) != 1 {
		t.Fatal("request did not commit before response loss", err)
	}
	session := connect(t, ts.URL, token)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Go(func() {
			// Admission can reject simultaneous authentication before a tool
			// executes. Retry the SAME operation/payload through the real HTTP
			// client on 503, retaining four concurrent callers and production caps.
			var r *mcp.CallToolResult
			var err error
			for attempt := 0; attempt < 4; attempt++ {
				r, err = session.CallTool(t.Context(), &mcp.CallToolParams{Name: "save_memory", Arguments: operationArgs()})
				if err == nil || !strings.Contains(err.Error(), "Service Unavailable") {
					break
				}
				timer := time.NewTimer(time.Second)
				select {
				case <-timer.C:
				case <-t.Context().Done():
					timer.Stop()
					return
				}
			}
			if err != nil || r == nil || r.IsError {
				t.Errorf("concurrent retry failed: %v", err)
				return
			}
			var n memory.Note
			if json.Unmarshal([]byte(resultText(t, r)), &n) != nil || n.ID != before[0].ID {
				t.Error("concurrent retry returned different note")
			}
		})
	}
	wg.Wait()
	after, err := f.notes.List()
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("retry duplicated mutation", err)
	}
	// Revoke must still reject replays at the HTTP boundary, not serve cache.
	if err := f.auth.Revoke(key.ID); err != nil {
		t.Fatal(err)
	}
	req, _ = http.NewRequestWithContext(t.Context(), "POST", ts.URL+"/mcp", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("revoked key replay accepted", response.StatusCode)
	}
}
