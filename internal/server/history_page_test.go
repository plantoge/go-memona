package server

import (
	"encoding/json"
	"memona/internal/memory"
	"testing"
)

func TestHistoryPageAdminAndLegacyCompatibility(t *testing.T) {
	f := setup(t)
	n := makeNote(t, f, "demo", "knowledge", "synthetic history", nil)
	for i := 0; i < 21; i++ {
		var err error
		n, err = f.notes.Revise(n.ID, n.Revision, memory.Input{Project: n.Project, Kind: n.Kind, Title: n.Title, Content: n.Content, Status: n.Status}, "tester")
		if err != nil {
			t.Fatal(err)
		}
	}
	path := "/api/notes/" + n.ID
	if w := request(f.app, "GET", path+"/history-page", "", nil, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	cookie, _ := login(t, f)
	var out struct {
		History []memory.Note `json:"history"`
		Next    int           `json:"history_next_before"`
	}
	w := request(f.app, "GET", path+"?history=paged", "", cookie, "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out.History) != 20 || out.Next != 3 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(f.app, "GET", path, "", cookie, "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out.History) != 22 {
		t.Fatal(w.Code)
	}
	w = request(f.app, "GET", path+"/history-page?before=3&limit=20", "", cookie, "")
	var page memory.HistoryPage
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Items) != 2 || page.Items[0].Revision != 2 || page.Before != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, q := range []string{"limit=51", "before=-1", "limit=bad"} {
		if w := request(f.app, "GET", path+"/history-page?"+q, "", cookie, ""); w.Code != 400 {
			t.Fatal(q, w.Code)
		}
	}
}
