package server

import (
	"encoding/json"
	"testing"

	"memona/internal/access"
	"memona/internal/memory"
)

func TestUserCodeReferencesScopedAndPreserved(t *testing.T) {
	f := setup(t)
	alice := workspaceLogin(t, f, "citation-alice", "citation-alice@example.test")
	bob := workspaceLogin(t, f, "citation-bob", "citation-bob@example.test")
	aliceWS := workspaceJSON[[]access.Workspace](t, workspaceRequest(t, f, alice, "GET", "/api/user/workspaces", "", 200))[0]
	bobWS := workspaceJSON[[]access.Workspace](t, workspaceRequest(t, f, bob, "GET", "/api/user/workspaces", "", 200))[0]
	ref := memory.CodeReference{Repository: "synthetic", Path: "main.go", Symbol: "Handler", Line: 1}
	body, err := json.Marshal(map[string]any{"workspace_id": aliceWS.ID, "kind": "decision", "title": "Citation", "content": "Synthetic", "status": "unverified", "source": "test", "code_references": []memory.CodeReference{ref}})
	if err != nil {
		t.Fatal(err)
	}
	n := workspaceJSON[memory.Note](t, workspaceRequest(t, f, alice, "POST", "/api/user/notes", string(body), 201))
	if len(n.CodeReferences) != 1 || n.CodeReferences[0] != ref {
		t.Fatal("user citation missing")
	}
	workspaceRequest(t, f, bob, "GET", "/api/user/notes/"+n.ID+"?workspace_id="+bobWS.ID, "", 404)
	path := "/api/user/notes/" + n.ID + "?workspace_id=" + aliceWS.ID
	workspaceRequest(t, f, bob, "POST", path, `{"expected_revision":1,"kind":"decision","title":"Changed","content":"No","status":"unverified","source":"test"}`, 404)
	if got := workspaceJSON[memory.Note](t, workspaceRequest(t, f, alice, "GET", path, "", 200)); len(got.CodeReferences) != 1 {
		t.Fatal("own citation unreadable")
	}
	revision := `{"expected_revision":1,"kind":"decision","title":"Citation","content":"Updated","status":"unverified","source":"test"}`
	n = workspaceJSON[memory.Note](t, workspaceRequest(t, f, alice, "POST", path, revision, 200))
	if len(n.CodeReferences) != 1 || n.CodeReferences[0] != ref {
		t.Fatal("citation not preserved")
	}
}
