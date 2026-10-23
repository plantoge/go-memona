package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"memona/internal/memory"
)

const resourceTestWait = 5 * time.Second

func resourceAwait[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(resourceTestWait):
		t.Fatal("timed out waiting for resource fixture")
		var zero T
		return zero
	}
}

func resourceHTTPStatus(t *testing.T, client *http.Client, url string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), resourceTestWait)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode
}

// This is the real admission boundary, with synthetic work in place of auth and
// disk I/O. All requests use the configured host, not a host/origin bypass.
func resourceBoundaryServer(t *testing.T, next http.Handler) (*App, *httptest.Server, <-chan string) {
	t.Helper()
	ts := httptest.NewUnstartedServer(nil)
	a, err := New(Config{Listen: "127.0.0.1:8765", PublicURL: "http://" + ts.Listener.Addr().String()}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	returned := make(chan string, 64)
	boundary := a.boundary(next)
	ts.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		boundary.ServeHTTP(w, r)
		returned <- r.URL.Path
	})
	ts.Start()
	t.Cleanup(ts.Close)
	return a, ts, returned
}

func TestResourceAdmissionGlobalEightSlotsAndRelease(t *testing.T) {
	started := make(chan string, 8)
	a, ts, returned := resourceBoundaryServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/block/"):
			started <- r.URL.Path
			<-r.Context().Done()
		case r.URL.Path == "/error":
			http.Error(w, "synthetic work error", http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	client := ts.Client()
	finished := make(chan error, 8)
	cancels := make([]context.CancelFunc, 0, 8)
	// Cleanup must disconnect the blocking clients before httptest closes.
	t.Cleanup(func() {
		for _, cancel := range cancels {
			cancel()
		}
	})
	for i := 0; i < 8; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancels = append(cancels, cancel)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/block/%d", ts.URL, i), nil)
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			resp, err := client.Do(req)
			if resp != nil {
				resp.Body.Close()
			}
			finished <- err
		}()
		resourceAwait(t, started)
	}
	if len(a.workSlots) != 8 || cap(a.workSlots) != 8 {
		t.Fatalf("admission slots = %d/%d, want 8/8", len(a.workSlots), cap(a.workSlots))
	}
	// Different endpoints still share the same global pool.
	for _, path := range []string{"/mcp", "/api/notes", "/healthz"} {
		if got := resourceHTTPStatus(t, client, ts.URL+path); got != http.StatusServiceUnavailable {
			t.Fatalf("ninth request to %s = %d, want 503", path, got)
		}
		resourceAwait(t, returned)
	}
	cancels[0]()
	if err := resourceAwait(t, finished); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled HTTP request = %v", err)
	}
	if path := resourceAwait(t, returned); path != "/block/0" {
		t.Fatalf("returned request = %s, want /block/0", path)
	}
	if got := resourceHTTPStatus(t, client, ts.URL+"/error"); got != http.StatusInternalServerError {
		t.Fatalf("request after cancellation = %d, want synthetic 500", got)
	}
	resourceAwait(t, returned)
	if len(a.workSlots) != 7 {
		t.Fatalf("slots after error = %d, want 7", len(a.workSlots))
	}
	if got := resourceHTTPStatus(t, client, ts.URL+"/healthz"); got != http.StatusNoContent {
		t.Fatalf("request after error = %d, want 204", got)
	}
	resourceAwait(t, returned)
	for _, cancel := range cancels[1:] {
		cancel()
	}
	for i := 1; i < 8; i++ {
		resourceAwait(t, finished)
		resourceAwait(t, returned)
	}
	if len(a.workSlots) != 0 {
		t.Fatalf("slots after all disconnects = %d, want 0", len(a.workSlots))
	}
}

func TestResourceToolContextLegacyHTTPDisconnect(t *testing.T) {
	started := make(chan time.Time, 1)
	stopped := make(chan error, 1)
	mh := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		s := mcp.NewServer(&mcp.Implementation{Name: "resource-fixture", Version: "1"}, &mcp.ServerOptions{SupportedProtocolVersions: []string{"2025-03-26"}})
		toolContext(r.Context(), s, "wait_for_cancel", "Synthetic blocking read", true, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, struct{}, error) {
			deadline, _ := ctx.Deadline()
			started <- deadline
			<-ctx.Done()
			stopped <- ctx.Err()
			return nil, struct{}{}, ctx.Err()
		})
		return s
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	a, ts, returned := resourceBoundaryServer(t, mh)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// A plain HTTP client sends no notifications/cancelled. Canceling it closes
	// the actual request connection, isolating the HTTP-context bridge.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"wait_for_cancel","arguments":{}}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-03-26")
	finished := make(chan error, 1)
	go func() {
		resp, err := ts.Client().Do(req)
		if resp != nil {
			resp.Body.Close()
		}
		finished <- err
	}()
	deadline := resourceAwait(t, started)
	if remaining := time.Until(deadline); deadline.IsZero() || remaining <= 0 || remaining > memory.ReadTimeout {
		t.Fatalf("read tool deadline = %v, want bounded by %v", deadline, memory.ReadTimeout)
	}
	cancel()
	if err := resourceAwait(t, stopped); !errors.Is(err, context.Canceled) {
		t.Fatalf("tool after HTTP disconnect = %v, want context.Canceled", err)
	}
	if err := resourceAwait(t, finished); !errors.Is(err, context.Canceled) {
		t.Fatalf("HTTP client cancellation = %v", err)
	}
	resourceAwait(t, returned)
	if len(a.workSlots) != 0 {
		t.Fatalf("canceled MCP request leaked %d admission slots", len(a.workSlots))
	}
}

func TestResourceToolContextPreservesSDKCancellation(t *testing.T) {
	started := make(chan struct{}, 1)
	stopped := make(chan error, 1)
	// Deliberately keep requestCtx alive: only SDK cancellation can stop this
	// tool. Replacing the SDK context with requestCtx would leak the handler.
	requestCtx, release := context.WithCancel(context.Background())
	defer release()
	s := mcp.NewServer(&mcp.Implementation{Name: "resource-sdk-fixture", Version: "1"}, &mcp.ServerOptions{SupportedProtocolVersions: []string{"2025-03-26"}})
	toolContext(requestCtx, s, "wait_for_cancel", "Synthetic blocking read", true, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, struct{}, error) {
		started <- struct{}{}
		<-ctx.Done()
		stopped <- ctx.Err()
		return nil, struct{}{}, ctx.Err()
	})
	ts := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, &mcp.StreamableHTTPOptions{JSONResponse: true}))
	defer ts.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "resource-test-client", Version: "1"}, nil)
	connectCtx, connectCancel := context.WithTimeout(context.Background(), resourceTestWait)
	defer connectCancel()
	session, err := client.Connect(connectCtx, &mcp.StreamableClientTransport{Endpoint: ts.URL, HTTPClient: ts.Client()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if got := session.InitializeResult().ProtocolVersion; got != "2025-03-26" {
		t.Fatalf("negotiated protocol = %s, want legacy 2025-03-26", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "wait_for_cancel", Arguments: map[string]any{}})
		finished <- err
	}()
	resourceAwait(t, started)
	cancel()
	if err := resourceAwait(t, stopped); !errors.Is(err, context.Canceled) {
		t.Fatalf("SDK tool cancellation = %v", err)
	}
	if err := resourceAwait(t, finished); !errors.Is(err, context.Canceled) {
		t.Fatalf("SDK client cancellation = %v", err)
	}
	if requestCtx.Err() != nil {
		t.Fatal("fixture HTTP context was canceled; SDK-only assertion is invalid")
	}
}

func TestResourceAppendReferenceBoundedDeterministic(t *testing.T) {
	for _, descending := range []bool{false, true} {
		t.Run(fmt.Sprintf("descending=%v", descending), func(t *testing.T) {
			var group []noteReference
			truncated := false
			for i := 0; i < 4096; i++ {
				id := i
				if descending {
					id = 4095 - i
				}
				appendReference(&group, noteReference{ID: fmt.Sprintf("note-%04d", id)}, &truncated)
				if len(group) > 20 || cap(group) > 40 {
					t.Fatalf("unbounded intermediate references: len=%d cap=%d", len(group), cap(group))
				}
				if truncated != (i >= 20) {
					t.Fatalf("truncated after %d entries = %v", i+1, truncated)
				}
			}
			if len(group) != 20 {
				t.Fatalf("retained references = %d, want 20", len(group))
			}
			for i, ref := range group {
				if want := fmt.Sprintf("note-%04d", i); ref.ID != want {
					t.Fatalf("reference[%d] = %s, want %s", i, ref.ID, want)
				}
			}
		})
	}
}

func resourceConnectionFixture(count int) (memory.Note, []memory.Note) {
	n := memory.Note{ID: "target", Project: "synthetic", Kind: "fact"}
	all := make([]memory.Note, 0, count)
	for i := count - 1; i >= 0; i-- {
		id := fmt.Sprintf("note-%04d", i)
		n.RelatedTo = append(n.RelatedTo, id)
		n.Supersedes = append(n.Supersedes, id)
		all = append(all, memory.Note{ID: id, Project: n.Project, Kind: "fact", Title: "Synthetic reference " + id, RelatedTo: []string{n.ID}, Supersedes: []string{n.ID}})
	}
	return n, all
}

func TestResourceConnectionsHighFanInBounded(t *testing.T) {
	for _, count := range []int{20, 1024} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			n, all := resourceConnectionFixture(count)
			// Lower IDs must not let another project, self, or archived backlinks
			// displace legitimate references in the bounded result.
			all = append(all,
				memory.Note{ID: "000-other-project", Project: "private", RelatedTo: []string{n.ID}, Supersedes: []string{n.ID}},
				memory.Note{ID: n.ID, Project: n.Project, RelatedTo: []string{n.ID}, Supersedes: []string{n.ID}},
				memory.Note{ID: "000-archived", Project: n.Project, Archived: true, RelatedTo: []string{n.ID}, Supersedes: []string{n.ID}},
			)
			got, err := connectionsForContext(context.Background(), n, all)
			if err != nil {
				t.Fatal(err)
			}
			if got.Truncated != (count > 20) {
				t.Fatalf("truncated = %v for %d references", got.Truncated, count)
			}
			for name, group := range map[string][]noteReference{"related_to": got.RelatedTo, "supersedes": got.Supersedes, "referenced_by": got.ReferencedBy, "superseded_by": got.SupersededBy} {
				if len(group) != 20 {
					t.Fatalf("%s count = %d, want 20", name, len(group))
				}
				for i, ref := range group {
					if want := fmt.Sprintf("note-%04d", i); ref.ID != want || ref.Project != n.Project {
						t.Fatalf("%s[%d] = %+v, want %s in %s", name, i, ref, want, n.Project)
					}
				}
			}
			for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
				all[i], all[j] = all[j], all[i]
			}
			reversed, err := connectionsForContext(context.Background(), n, all)
			if err != nil || !reflect.DeepEqual(got, reversed) {
				t.Fatalf("snapshot order changed references: err=%v", err)
			}
		})
	}
}

// Cancel on a chosen cooperative check, so both mid-scan and final-check
// cancellation are deterministic rather than scheduler-dependent.
type resourceCancelOnCheck struct {
	context.Context
	cancel context.CancelFunc
	after  int
}

func (c *resourceCancelOnCheck) Err() error {
	c.after--
	if c.after <= 0 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestResourceConnectionsCancellationNoPartial(t *testing.T) {
	n, all := resourceConnectionFixture(32)
	for _, check := range []int{1, 3, len(all) + 1} {
		t.Run(fmt.Sprintf("check=%d", check), func(t *testing.T) {
			base, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := &resourceCancelOnCheck{Context: base, cancel: cancel, after: check}
			got, err := connectionsForContext(ctx, n, all)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
			if !reflect.DeepEqual(got, noteConnections{}) {
				t.Fatalf("canceled scan returned partial connections: %+v", got)
			}
		})
	}
}
