package server

import (
	"encoding/json"
	"memona/internal/access"
	"testing"
)

func TestKeyPageAdminValidationAndUsage(t *testing.T) {
	f := setup(t)
	if w := request(f.app, "GET", "/api/keys-page", "", nil, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	cookie, _ := login(t, f)
	for i := 0; i < 2; i++ {
		if _, _, err := f.auth.CreateKey(access.KeyInput{Name: "synthetic", Permissions: []string{"read"}, Projects: []string{"demo"}}); err != nil {
			t.Fatal(err)
		}
	}
	w := request(f.app, "GET", "/api/keys-page?limit=1", "", cookie, "")
	var out struct {
		Items []access.Key `json:"items"`
		Next  string       `json:"next_cursor"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out.Items) != 1 || out.Next == "" {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, path := range []string{"/api/keys-page?limit=101", "/api/keys-page?limit=no", "/api/keys-page?cursor=bad"} {
		if w := request(f.app, "GET", path, "", cookie, ""); w.Code != 400 {
			t.Fatal(path, w.Code)
		}
	}
}
