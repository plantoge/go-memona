package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"memona/internal/access"
	"memona/internal/memory"
)

// These sessions are synthetic: no OIDC request, Google credential, or live data is used.
type workspaceTestUser struct {
	cookie *http.Cookie
	csrf   string
}

func workspaceLogin(t *testing.T, f *fixture, sub, email string) workspaceTestUser {
	t.Helper()
	_, token, csrf, err := f.auth.UserLoginContext(context.Background(), sub, email, sub)
	if err != nil {
		t.Fatal(err)
	}
	return workspaceTestUser{&http.Cookie{Name: "memona_user_session", Value: token}, csrf}
}

func workspaceRequest(t *testing.T, f *fixture, u workspaceTestUser, method, path, body string, status int) *httptest.ResponseRecorder {
	t.Helper()
	w := request(f.app, method, path, body, u.cookie, u.csrf)
	if w.Code != status {
		t.Fatalf("%s %s: got %d, want %d: %s", method, path, w.Code, status, w.Body.String())
	}
	return w
}

func workspaceJSON[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func workspaceNote(t *testing.T, f *fixture, u workspaceTestUser, id, title string) memory.Note {
	t.Helper()
	body, err := json.Marshal(map[string]string{"workspace_id": id, "kind": "fact", "title": title, "content": "Integration test content", "status": "unverified", "source": "synthetic integration test"})
	if err != nil {
		t.Fatal(err)
	}
	return workspaceJSON[memory.Note](t, workspaceRequest(t, f, u, "POST", "/api/user/notes", string(body), 201))
}

func workspaceKey(t *testing.T, f *fixture, u workspaceTestUser, id string) (access.Key, string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"workspace_id": id, "name": "integration key", "permissions": []string{"read", "write"}})
	if err != nil {
		t.Fatal(err)
	}
	out := workspaceJSON[struct {
		Key   access.Key `json:"key"`
		Token string     `json:"token"`
	}](t, workspaceRequest(t, f, u, "POST", "/api/user/keys", string(body), 201))
	if out.Key.ID == "" || out.Token == "" {
		t.Fatal("key creation returned no ID or token")
	}
	return out.Key, out.Token
}

func TestUserTeamWorkspaceIsolationAndMCP(t *testing.T) {
	f := setup(t) // Each test owns a temporary notes directory and SQLite database.
	alice := workspaceLogin(t, f, "alice-sub", "alice@example.test")
	bob := workspaceLogin(t, f, "bob-sub", "bob@example.test")
	charlie := workspaceLogin(t, f, "charlie-sub", "charlie@example.test")
	adminCookie, _ := login(t, f)
	if w := request(f.app, "GET", "/api/user/session", "", adminCookie, ""); w.Code != 401 {
		t.Fatalf("admin session accessed user API: %d", w.Code)
	}
	if w := request(f.app, "GET", "/api/session", "", alice.cookie, ""); w.Code != 401 {
		t.Fatalf("user session accessed admin API: %d", w.Code)
	}
	if w := request(f.app, "GET", "/api/user/workspaces", "", nil, ""); w.Code != 401 {
		t.Fatalf("anonymous workspace list: %d", w.Code)
	}
	aliceWorkspaces := workspaceJSON[[]access.Workspace](t, workspaceRequest(t, f, alice, "GET", "/api/user/workspaces", "", 200))
	bobWorkspaces := workspaceJSON[[]access.Workspace](t, workspaceRequest(t, f, bob, "GET", "/api/user/workspaces", "", 200))
	if len(aliceWorkspaces) != 1 || len(bobWorkspaces) != 1 || aliceWorkspaces[0].Kind != "personal" || bobWorkspaces[0].Kind != "personal" || aliceWorkspaces[0].Project == bobWorkspaces[0].Project {
		t.Fatalf("personal workspaces not isolated: %#v %#v", aliceWorkspaces, bobWorkspaces)
	}
	ap, bp := aliceWorkspaces[0], bobWorkspaces[0]
	aliceNote := workspaceNote(t, f, alice, ap.ID, "Alice private")
	bobNote := workspaceNote(t, f, bob, bp.ID, "Bob private")
	if aliceNote.Project != ap.Project || bobNote.Project != bp.Project {
		t.Fatal("user note was not assigned to its workspace project")
	}
	for _, tc := range []struct {
		user workspaceTestUser
		id   string
	}{{bob, ap.ID}, {alice, bp.ID}} {
		workspaceRequest(t, f, tc.user, "GET", "/api/user/notes?workspace_id="+tc.id, "", 403)
		workspaceRequest(t, f, tc.user, "POST", "/api/user/notes", `{"workspace_id":"`+tc.id+`","kind":"fact","title":"Intrusion","content":"No","status":"unverified","source":"test"}`, 403)
	}
	for _, tc := range []struct {
		user    workspaceTestUser
		foreign memory.Note
		ownID   string
	}{{bob, aliceNote, bp.ID}, {alice, bobNote, ap.ID}} {
		// A known foreign note ID must not become readable or mutable by supplying an owned workspace ID.
		path := "/api/user/notes/" + tc.foreign.ID + "?workspace_id=" + tc.ownID
		workspaceRequest(t, f, tc.user, "GET", path, "", 404)
		workspaceRequest(t, f, tc.user, "POST", path, `{"expected_revision":1,"kind":"fact","title":"changed","content":"changed","status":"unverified","source":"test"}`, 404)
	}
	if notes := workspaceJSON[[]memory.Note](t, workspaceRequest(t, f, bob, "GET", "/api/user/notes?workspace_id="+bp.ID, "", 200)); len(notes) != 1 || notes[0].ID != bobNote.ID {
		t.Fatalf("personal note listing leaked: %#v", notes)
	}

	if w := request(f.app, "POST", "/api/user/teams", `{"name":"Integration team"}`, alice.cookie, ""); w.Code != 403 {
		t.Fatalf("team creation without CSRF: %d", w.Code)
	}
	team := workspaceJSON[access.Workspace](t, workspaceRequest(t, f, alice, "POST", "/api/user/teams", `{"name":"Integration team"}`, 201))
	if team.Kind != "team" || team.Role != "owner" || team.Project == ap.Project {
		t.Fatalf("invalid team: %#v", team)
	}
	teamNote := workspaceNote(t, f, alice, team.ID, "Team shared")
	workspaceRequest(t, f, bob, "GET", "/api/user/notes?workspace_id="+team.ID, "", 403)
	workspaceRequest(t, f, bob, "GET", "/api/user/graph?workspace_id="+team.ID, "", 403)
	workspaceRequest(t, f, bob, "POST", "/api/user/teams/"+team.ID+"/invite", `{"email":"charlie@example.test"}`, 403)
	invitePath := "/api/user/teams/" + team.ID + "/invite"
	if w := request(f.app, "POST", invitePath, `{"email":"bob@example.test"}`, alice.cookie, ""); w.Code != 403 {
		t.Fatalf("invitation without CSRF: %d", w.Code)
	}
	invitation := workspaceJSON[access.Invitation](t, workspaceRequest(t, f, alice, "POST", invitePath, `{"email":"bob@example.test"}`, 201))
	if invitation.ID == "" || invitation.WorkspaceID != team.ID {
		t.Fatalf("invalid invitation: %#v", invitation)
	}
	if pending := workspaceJSON[[]access.Invitation](t, workspaceRequest(t, f, bob, "GET", "/api/user/invitations", "", 200)); len(pending) != 1 || pending[0].ID != invitation.ID {
		t.Fatalf("invitation not visible to invitee: %#v", pending)
	}
	acceptPath := "/api/user/invitations/" + invitation.ID + "/accept"
	workspaceRequest(t, f, charlie, "POST", acceptPath, `{}`, 403)
	if w := request(f.app, "POST", acceptPath, `{}`, bob.cookie, ""); w.Code != 403 {
		t.Fatalf("acceptance without CSRF: %d", w.Code)
	}
	workspaceRequest(t, f, bob, "POST", acceptPath, `{}`, 200)
	workspaceRequest(t, f, bob, "POST", acceptPath, `{}`, 403)
	bobAfter := workspaceJSON[[]access.Workspace](t, workspaceRequest(t, f, bob, "GET", "/api/user/workspaces", "", 200))
	if len(bobAfter) != 2 || bobAfter[1].ID != team.ID || bobAfter[1].Role != "member" {
		t.Fatalf("accepted membership missing: %#v", bobAfter)
	}
	if notes := workspaceJSON[[]memory.Note](t, workspaceRequest(t, f, bob, "GET", "/api/user/notes?workspace_id="+team.ID, "", 200)); len(notes) != 1 || notes[0].ID != teamNote.ID {
		t.Fatalf("shared team note missing: %#v", notes)
	}
	if graph := workspaceJSON[graphResult](t, workspaceRequest(t, f, bob, "GET", "/api/user/graph?workspace_id="+team.ID, "", 200)); len(graph.Nodes) != 1 || graph.Nodes[0].ID != teamNote.ID {
		t.Fatalf("shared team graph missing: %#v", graph)
	}
	memberNote := workspaceNote(t, f, bob, team.ID, "Member contribution")
	if n := workspaceJSON[memory.Note](t, workspaceRequest(t, f, alice, "GET", "/api/user/notes/"+memberNote.ID+"?workspace_id="+team.ID, "", 200)); n.ID != memberNote.ID || n.Project != team.Project {
		t.Fatalf("team owner cannot read member contribution: %#v", n)
	}
	workspaceRequest(t, f, charlie, "GET", "/api/user/notes?workspace_id="+team.ID, "", 403)
	workspaceRequest(t, f, charlie, "GET", "/api/user/graph?workspace_id="+team.ID, "", 403)
	workspaceRequest(t, f, charlie, "GET", "/api/user/notes/"+teamNote.ID+"?workspace_id="+team.ID, "", 404)

	keyBody := `{"workspace_id":"` + team.ID + `","name":"no csrf","permissions":["read","write"]}`
	if w := request(f.app, "POST", "/api/user/keys", keyBody, bob.cookie, ""); w.Code != 403 {
		t.Fatalf("key generation without CSRF: %d", w.Code)
	}
	workspaceRequest(t, f, charlie, "POST", "/api/user/keys", keyBody, 403)
	personalKey, personalToken := workspaceKey(t, f, alice, ap.ID)
	teamKey, teamToken := workspaceKey(t, f, bob, team.ID)
	if len(personalKey.Projects) != 1 || personalKey.Projects[0] != ap.Project || len(teamKey.Projects) != 1 || teamKey.Projects[0] != team.Project {
		t.Fatal("generated key scope not bound to its workspace")
	}
	if keys := workspaceJSON[[]access.Key](t, workspaceRequest(t, f, alice, "GET", "/api/user/keys", "", 200)); len(keys) != 1 || keys[0].ID != personalKey.ID {
		t.Fatalf("other user's keys visible: %#v", keys)
	}
	if w := workspaceRequest(t, f, bob, "GET", "/api/user/keys", "", 200); strings.Contains(w.Body.String(), teamToken) {
		t.Fatal("key list exposed token")
	}

	ts := httptest.NewUnstartedServer(nil)
	app, err := New(Config{Listen: "127.0.0.1:8765", PublicURL: "http://" + ts.Listener.Addr().String()}, f.auth, f.notes)
	if err != nil {
		t.Fatal(err)
	}
	ts.Config.Handler = app
	ts.Start()
	defer ts.Close()
	personal := connect(t, ts.URL, personalToken)
	shared := connect(t, ts.URL, teamToken)
	_, wildcardToken, err := f.auth.CreateKey(access.KeyInput{Name: "legacy wildcard", Permissions: []string{"read", "write"}, Projects: []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}
	wildcard := connect(t, ts.URL, wildcardToken)
	// Authorized read/write and explicit denials are checked at the actual MCP tool boundary.
	for _, tc := range []struct {
		name         string
		sessionToken string
		project      string
		allowed      bool
	}{
		{"personal key personal", "personal", ap.Project, true},
		{"personal key team", "personal", team.Project, false},
		{"team key team", "team", team.Project, true},
		{"team key personal", "team", ap.Project, false},
		{"team key Bob personal", "team", bp.Project, false},
		{"legacy wildcard personal", "wildcard", ap.Project, false},
		{"legacy wildcard team", "wildcard", team.Project, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := personal
			switch tc.sessionToken {
			case "team":
				s = shared
			case "wildcard":
				s = wildcard
			}
			r := call(t, s, "save_memory", map[string]any{"project": tc.project, "kind": "fact", "title": "MCP scope probe", "content": "Synthetic fixture", "status": "unverified", "source": "integration test"})
			if r.IsError == tc.allowed {
				t.Fatalf("write authorization mismatch (allowed=%v): %s", tc.allowed, resultText(t, r))
			}
		})
	}
	for _, tc := range []struct {
		name    string
		token   string
		id      string
		allowed bool
	}{
		{"personal reads personal", "personal", aliceNote.ID, true},
		{"personal denies team", "personal", teamNote.ID, false},
		{"team reads team", "team", teamNote.ID, true},
		{"team denies personal", "team", bobNote.ID, false},
		{"wildcard denies personal", "wildcard", aliceNote.ID, false},
		{"wildcard denies team", "wildcard", teamNote.ID, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := personal
			switch tc.token {
			case "team":
				s = shared
			case "wildcard":
				s = wildcard
			}
			r := call(t, s, "read_memory", map[string]any{"id": tc.id})
			if r.IsError == tc.allowed {
				t.Fatalf("read authorization mismatch (allowed=%v): %s", tc.allowed, resultText(t, r))
			}
		})
	}
	if r := call(t, shared, "get_context", map[string]any{}); r.IsError || !strings.Contains(resultText(t, r), teamNote.ID) || strings.Contains(resultText(t, r), aliceNote.ID) {
		t.Fatal("default team context must use the key's workspace, not legacy global")
	}
	if r := call(t, wildcard, "search_memory", map[string]any{}); r.IsError || strings.Contains(resultText(t, r), teamNote.ID) || strings.Contains(resultText(t, r), aliceNote.ID) {
		t.Fatal("legacy wildcard search leaked workspace notes")
	}
	if w := request(f.app, "POST", "/api/user/keys/"+teamKey.ID+"/revoke", `{}`, alice.cookie, alice.csrf); w.Code != 403 {
		t.Fatalf("foreign key revocation: %d", w.Code)
	}
	if w := request(f.app, "POST", "/api/user/keys/"+teamKey.ID+"/revoke", `{}`, bob.cookie, ""); w.Code != 403 {
		t.Fatalf("key revocation without CSRF: %d", w.Code)
	}
	workspaceRequest(t, f, bob, "POST", "/api/user/keys/"+teamKey.ID+"/revoke", `{}`, 200)
	req, err := http.NewRequest("POST", ts.URL+"/mcp", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+teamToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("revoked MCP key accepted: %d", resp.StatusCode)
	}
	// Revoking the team key does not revoke a separate personal workspace key.
	if r := call(t, personal, "read_memory", map[string]any{"id": aliceNote.ID}); r.IsError {
		t.Fatal("unrelated personal key affected by revocation", resultText(t, r))
	}
}
