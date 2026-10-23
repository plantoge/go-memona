package server

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"memona/internal/memory"
)

func TestGraphExplicitBoundedAndFiltered(t *testing.T) {
	all := []memory.Note{
		{ID: "b", Project: "p", Kind: "fact", Title: "linked", RelatedTo: []string{"a"}, Supersedes: []string{"a"}},
		{ID: "a", Project: "p", Kind: "fact", Title: "alone target"},
		{ID: "c", Project: "p", Kind: "fact", Title: "isolated"},
		{ID: "d", Project: "other", RelatedTo: []string{"a"}},
		{ID: "e", Project: "p", Archived: true},
	}
	out, err := buildGraph(t.Context(), all, "p", "", false, 200)
	if err != nil || len(out.Nodes) != 3 || len(out.Edges) != 2 || out.Truncated {
		t.Fatal(out, err)
	}
	if out.Edges[0].From != "b" || out.Edges[0].To != "a" {
		t.Fatal("wrong direction")
	}
	all[0], all[1] = all[1], all[0]
	again, err := buildGraph(t.Context(), all, "p", "", false, 200)
	if err != nil || !reflect.DeepEqual(out, again) {
		t.Fatal("order not deterministic", err)
	}
	limited, err := buildGraph(t.Context(), all, "p", "", false, 1)
	if err != nil || len(limited.Nodes) != 1 || !limited.Truncated || len(limited.Edges) != 0 {
		t.Fatal(limited, err)
	}
	filtered, err := buildGraph(t.Context(), all, "p", "linked", false, 200)
	if err != nil || len(filtered.Nodes) != 1 || !filtered.Truncated || len(filtered.Edges) != 0 {
		t.Fatal(filtered, err)
	}
	archived, err := buildGraph(t.Context(), all, "p", "", true, 200)
	if err != nil || len(archived.Nodes) != 4 {
		t.Fatal(archived, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got, err := buildGraph(ctx, all, "", "", false, 200); !errors.Is(err, context.Canceled) || len(got.Nodes) != 0 {
		t.Fatal(err)
	}
}
func TestGraphAPIAdminOnlyAndValidation(t *testing.T) {
	f := setup(t)
	if w := request(f.app, "GET", "/api/graph", "", nil, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	c, _ := login(t, f)
	n, err := f.notes.Create(memory.Input{Project: "p", Kind: "fact", Title: "standalone", Status: "unverified"}, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	w := request(f.app, "GET", "/api/graph?project=p", "", c, "")
	var out graphResult
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out.Nodes) != 1 || out.Nodes[0].ID != n.ID || len(out.Edges) != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, q := range []string{"limit=0", "limit=501", "limit=bad", "archived=yes"} {
		if w := request(f.app, "GET", "/api/graph?"+q, "", c, ""); w.Code != 400 {
			t.Fatal(q, w.Code)
		}
	}
}
