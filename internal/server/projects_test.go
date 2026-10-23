package server

import (
	"encoding/json"
	"testing"

	"memona/internal/access"
)

func TestProjectRegistryParentRoutingAndGuards(t *testing.T) {
	f := setup(t)
	cookie, csrf := login(t, f)
	body := `{"slug":"demo","name":"Demo","description":"Synthetic","aliases":["worktree-a"],"archived":false,"expected_revision":0}`
	if w := request(f.app, "GET", "/api/project-registry", "", nil, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request(f.app, "POST", "/api/project-registry", body, cookie, ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := request(f.app, "POST", "/api/project-registry", body, cookie, csrf); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request(f.app, "POST", "/api/project-registry", body, cookie, csrf); w.Code != 409 {
		t.Fatal(w.Code)
	}
	w := request(f.app, "GET", "/api/project-registry", "", cookie, "")
	var projects []access.Project
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &projects) != nil || len(projects) != 1 || projects[0].Slug != "demo" || projects[0].Revision != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request(f.app, "POST", "/api/project-registry", `{"slug":"demo","unknown":true}`, cookie, csrf); w.Code != 400 {
		t.Fatal(w.Code)
	}
}
