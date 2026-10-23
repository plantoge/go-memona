package server

import (
	"encoding/json"
	"memona/internal/access"
	"strings"
	"testing"
)

func TestRecordedKeyUsageExactIdentityAndNewestEvent(t *testing.T) {
	key := access.Key{ID: "synthetic-id", Name: "test"}
	events := []access.Event{
		{Actor: key.Actor() + "extra", Action: "memory.read", Time: "wrong"},
		{Actor: key.Actor(), Action: "key.revoke", Time: "wrong"},
		{Actor: key.Actor(), Action: "memory.read", Time: "newest"},
		{Actor: key.Actor(), Action: "memory.save", Time: "older"},
	}
	got := recordedKeyUsage([]access.Key{key}, events)
	if len(got) != 1 || got[key.ID].LastRecordedAt != "newest" {
		t.Fatal(got)
	}
}
func TestKeyUsageEndpointAdminOnlyAndRedacted(t *testing.T) {
	f := setup(t)
	if w := request(f.app, "GET", "/api/key-usage", "", nil, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	cookie, _ := login(t, f)
	key, token, err := f.auth.CreateKey(access.KeyInput{Name: "synthetic", Permissions: []string{"read"}, Projects: []string{"demo"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.auth.Audit(key.Actor(), "memory.read", "private-note-id"); err != nil {
		t.Fatal(err)
	}
	w := request(f.app, "GET", "/api/key-usage", "", cookie, "")
	var out struct {
		Usage  map[string]keyUsage `json:"usage"`
		Window int                 `json:"audit_window"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Window != 200 || out.Usage[key.ID].Action != "memory.read" {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), token) || strings.Contains(w.Body.String(), "private-note-id") {
		t.Fatal("sensitive output")
	}
}
