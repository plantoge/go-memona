package server

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"memona/internal/memory"
)

func TestUserGraphWorkspaceIsolation(t *testing.T) {
	f := setup(t)
	alice := workspaceLogin(t, f, "graph-alice", "graph-alice@example.test")
	bob := workspaceLogin(t, f, "graph-bob", "graph-bob@example.test")
	aliceWS := workspaceJSON[[]struct {
		ID      string `json:"id"`
		Project string `json:"project"`
	}](t, workspaceRequest(t, f, alice, "GET", "/api/user/workspaces", "", http.StatusOK))[0]
	bobWS := workspaceJSON[[]struct {
		ID      string `json:"id"`
		Project string `json:"project"`
	}](t, workspaceRequest(t, f, bob, "GET", "/api/user/workspaces", "", http.StatusOK))[0]
	first := workspaceNote(t, f, alice, aliceWS.ID, "First")
	second := workspaceNote(t, f, alice, aliceWS.ID, "Second")
	foreign := workspaceNote(t, f, bob, bobWS.ID, "Foreign")
	archived := workspaceNote(t, f, alice, aliceWS.ID, "Archived")
	if _, err := f.notes.ReviseContext(context.Background(), second.ID, second.Revision, memory.Input{
		Project: aliceWS.Project, Kind: second.Kind, Title: second.Title, Content: second.Content,
		Status: second.Status, Source: second.Source, RelatedTo: []string{first.ID},
	}, "synthetic"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.notes.ArchiveContext(context.Background(), archived.ID, archived.Revision, "synthetic"); err != nil {
		t.Fatal(err)
	}
	path := "/api/user/graph?workspace_id=" + aliceWS.ID
	if w := request(f.app, "GET", path, "", nil, ""); w.Code != 401 {
		t.Fatalf("anonymous graph: %d", w.Code)
	}
	admin, _ := login(t, f)
	if w := request(f.app, "GET", path, "", admin, ""); w.Code != 401 {
		t.Fatalf("admin cookie on user graph: %d", w.Code)
	}
	if w := workspaceRequest(t, f, bob, "GET", path, "", 403); strings.Contains(w.Body.String(), first.Title) {
		t.Fatal("foreign workspace contents exposed")
	}
	out := workspaceJSON[graphResult](t, workspaceRequest(t, f, alice, "GET", path, "", 200))
	if len(out.Nodes) != 2 || len(out.Edges) != 1 || out.Truncated || out.Edges[0].From != second.ID || out.Edges[0].To != first.ID {
		t.Fatalf("unexpected scoped graph: %+v", out)
	}
	for _, n := range out.Nodes {
		if n.Project != aliceWS.Project || n.ID == foreign.ID || n.ID == archived.ID {
			t.Fatalf("foreign or archived node leaked: %+v", n)
		}
	}
	limited := workspaceJSON[graphResult](t, workspaceRequest(t, f, alice, "GET", path+"&limit=1", "", 200))
	if len(limited.Nodes) != 1 || !limited.Truncated {
		t.Fatal("limit not enforced")
	}
	for _, query := range []string{"project=" + bobWS.Project, "archived=true", "limit=0", "limit=501", "limit=x", "q=" + strings.Repeat("x", 2001)} {
		workspaceRequest(t, f, alice, "GET", path+"&"+query, "", 400)
	}
}
