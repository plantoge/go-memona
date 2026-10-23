package server

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"memona/internal/access"
	"memona/internal/memory"
)

// TestSyntheticSemanticEvaluation records lexical behavior, not semantic quality.
// Relevance labels are hand-specified for this tiny invented corpus, independently
// of the expected implementation results. Known misses remain explicit assertions.
func TestSyntheticSemanticEvaluation(t *testing.T) {
	ctx := context.Background()
	key := access.Key{Permissions: []string{"read"}, Projects: []string{"demo"}}
	notes := []memory.Note{
		{ID: "queue", Project: "demo", Title: "Antrian permintaan", Content: "Permintaan diproses berurutan."},
		{ID: "store", Project: "demo", Title: "Penyimpanan catatan", Content: "Simpan memori secara atomik."},
		{ID: "identifier", Project: "demo", Title: "Kontrak expected_revision", Content: "Revisi wajib cocok."},
		{ID: "safe", Project: "demo", Title: "Revisi aman", Content: "Perubahan diperiksa sebelum disimpan."},
		{ID: "alternate", Project: "demo", Title: "Aman dalam revisi", Content: "Validasi perubahan sebelum penulisan."},
		{ID: "conflict", Project: "demo", Title: "Revisi paralel", Content: "Konflik ditolak."},
		{ID: "outside-scope", Project: "other", Title: "Deployment rahasia"},
		{ID: "archived", Project: "demo", Title: "Deployment rahasia", Archived: true},
	}
	// rankedNotes requires an already authorized snapshot; querySnapshot must
	// independently filter the complete supplied snapshot by the same read key.
	accessible := []memory.Note{}
	for _, n := range notes {
		if key.Allows("read", n.Project) {
			accessible = append(accessible, n)
		}
	}
	cases := []struct {
		name         string
		query        string
		relevant     []string
		wantRanked   []string
		wantSnapshot []string
	}{
		{name: "Indonesian spelling synonym miss", query: "antrean", relevant: []string{"queue"}},
		{name: "code paraphrase miss", query: "persist note", relevant: []string{"store"}},
		{name: "English Indonesian paraphrase miss", query: "atomic write", relevant: []string{"store"}},
		{name: "exact identifier", query: "expected_revision", relevant: []string{"identifier"}, wantRanked: []string{"identifier"}, wantSnapshot: []string{"identifier"}},
		{name: "partial identifier compatibility", query: "expected_rev", relevant: []string{"identifier"}, wantSnapshot: []string{"identifier"}},
		{name: "multiword exact set suppresses alternative", query: "revisi aman", relevant: []string{"safe", "alternate"}, wantRanked: []string{"safe", "alternate", "conflict", "identifier"}, wantSnapshot: []string{"safe"}},
		{name: "multiword any term fallback", query: "konflik revisi", relevant: []string{"conflict"}, wantRanked: []string{"conflict", "alternate", "safe", "identifier"}, wantSnapshot: []string{"conflict", "alternate", "safe", "identifier"}},
		{name: "literal Indonesian control", query: "antrian", relevant: []string{"queue"}, wantRanked: []string{"queue"}, wantSnapshot: []string{"queue"}},
		{name: "no match", query: "galaksi kuasar"},
		{name: "wrong scope and archive excluded", query: "deployment rahasia"},
	}
	ids := func(notes []memory.Note) []string {
		var out []string
		for _, n := range notes {
			out = append(out, n.ID)
		}
		return out
	}
	hits := func(got []memory.Note, relevant []string) int {
		count := 0
		for _, id := range relevant {
			for _, n := range got {
				if n.ID == id {
					count++
					break
				}
			}
		}
		return count
	}
	var totalRelevant, rankedHits, snapshotHits, rankedReturned, snapshotReturned int
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, id := range tc.relevant {
				found := false
				for _, n := range accessible {
					if n.ID == id && !n.Archived {
						found = true
					}
				}
				if !found {
					t.Fatalf("invalid relevance label %q: not an accessible current note", id)
				}
			}
			ranked, err := rankedNotes(ctx, accessible, tc.query)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := querySnapshot(ctx, key, queryInput{Project: "demo", Query: tc.query}, notes)
			if err != nil {
				t.Fatal(err)
			}
			if got := ids(ranked); !reflect.DeepEqual(got, tc.wantRanked) {
				t.Errorf("rankedNotes IDs = %v, want %v", got, tc.wantRanked)
			}
			if got := ids(snapshot); !reflect.DeepEqual(got, tc.wantSnapshot) {
				t.Errorf("querySnapshot IDs = %v, want %v", got, tc.wantSnapshot)
			}
			rh, sh := hits(ranked, tc.relevant), hits(snapshot, tc.relevant)
			totalRelevant += len(tc.relevant)
			rankedHits += rh
			snapshotHits += sh
			rankedReturned += len(ranked)
			snapshotReturned += len(snapshot)
			if len(tc.relevant) == 0 {
				t.Logf("synthetic negative query=%q ranked=%v snapshot=%v; recall undefined (no relevant IDs)", tc.query, ids(ranked), ids(snapshot))
			} else {
				t.Logf("synthetic query=%q relevant=%v ranked=%v hits=%d/%d snapshot=%v hits=%d/%d", tc.query, tc.relevant, ids(ranked), rh, len(tc.relevant), ids(snapshot), sh, len(tc.relevant))
			}
		})
	}
	// Full returned-set micro recall, not recall@k, a benchmark, or a user-corpus estimate.
	if totalRelevant != 9 || rankedHits != 5 || snapshotHits != 5 || rankedReturned != 10 || snapshotReturned != 8 {
		t.Fatalf("synthetic totals changed: relevant=%d ranked=%d/%d snapshot=%d/%d", totalRelevant, rankedHits, rankedReturned, snapshotHits, snapshotReturned)
	}
	t.Logf("TINY SYNTHETIC ONLY: %d queries; %d relevance pairs; ranked recall=%d/%d (%.1f%%), returned=%d; querySnapshot recall=%d/%d (%.1f%%), returned=%d; no claim about actual user corpus", len(cases), totalRelevant, rankedHits, totalRelevant, 100*float64(rankedHits)/float64(totalRelevant), rankedReturned, snapshotHits, totalRelevant, 100*float64(snapshotHits)/float64(totalRelevant), snapshotReturned)

	t.Run("explicit wrong project denied", func(t *testing.T) {
		got, err := querySnapshot(ctx, key, queryInput{Project: "other", Query: "deployment rahasia"}, notes)
		if !errors.Is(err, access.ErrDenied) || len(got) != 0 {
			t.Fatalf("wrong-scope query returned %v, err=%v; want access denied and no notes", ids(got), err)
		}
	})
}
