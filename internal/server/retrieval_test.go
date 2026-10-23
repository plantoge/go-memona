package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"time"

	"memona/internal/memory"

	_ "modernc.org/sqlite"
)

func retrievalIDs(notes []memory.Note) []string {
	ids := make([]string, len(notes))
	for i, n := range notes {
		ids[i] = n.ID
	}
	return ids
}

func TestRankedNotesRelevance(t *testing.T) {
	notes := []memory.Note{
		{ID: "source", Source: "alur pasien", UpdatedAt: "2026-10-22"},
		{ID: "content", Content: "ALUR pasien", UpdatedAt: "2026-10-21"},
		{ID: "title", Title: "Alur Pasien", UpdatedAt: "2020-01-01"},
		{ID: "unrelated", Title: "Inventaris farmasi"},
		{ID: "archived", Title: "Alur Pasien", Archived: true},
	}
	got, err := rankedNotes(context.Background(), notes, "alur pasien")
	if err != nil || !reflect.DeepEqual(retrievalIDs(got), []string{"title", "content", "source"}) {
		t.Fatalf("ranking Bahasa Indonesia: %v, %v", retrievalIDs(got), err)
	}
	// Repeating keywords does not overpower a title match.
	notes[1].Content = strings.Repeat("alur pasien ", 1000)
	got, err = rankedNotes(context.Background(), notes, "alur alur pasien")
	if err != nil || got[0].ID != "title" {
		t.Fatalf("repetisi memengaruhi title boost: %v, %v", retrievalIDs(got), err)
	}
}

func TestRankedNotesContextPriority(t *testing.T) {
	notes := []memory.Note{
		{ID: "handoff", Title: "Memona", Kind: "handoff", UpdatedAt: "2026-10-22"},
		{ID: "knowledge", Title: "Memona", Kind: "knowledge", UpdatedAt: "2020-01-01"},
		{ID: "summary", Title: "Memona", Kind: "knowledge", IsProjectSummary: true, UpdatedAt: "2019-01-01"},
	}
	for _, query := range []string{"", " \n\t", "MEMONA"} {
		got, err := rankedNotes(context.Background(), notes, query)
		if err != nil || !reflect.DeepEqual(retrievalIDs(got), []string{"summary", "knowledge", "handoff"}) {
			t.Fatalf("ringkasan/knowledge harus mengalahkan recency handoff, query %q: %v, %v", query, retrievalIDs(got), err)
		}
	}
	// An unrelated overview must not displace a relevant handoff.
	notes[2].Title = "Inventaris"
	got, err := rankedNotes(context.Background(), notes, "memona")
	if err != nil || !reflect.DeepEqual(retrievalIDs(got), []string{"knowledge", "handoff"}) {
		t.Fatalf("ringkasan tak relevan: %v, %v", retrievalIDs(got), err)
	}
}

func TestRankedNotesUnicodeAndCodeTerms(t *testing.T) {
	for _, tc := range []struct {
		name, title, query string
		match              bool
	}{
		{"Indonesia", "DefinisiLaporan: alur rekam medis", "DEFINISILAPORAN", true},
		{"identifier", "get_context memakai expected_revision", "GET_CONTEXT", true},
		{"bukan substring identifier", "expected_revision", "revision", false},
		{"path", "internal/server/retrieval.go", "retrieval.go", true},
		{"hyphen", "E-Klaim BPJS", "e-klaim", true},
		{"SQL bukan operator", "Alur pasien", "pasien OR zzunrelated", true},
		{"Unicode", "Kafé résumé 日本語", "KAFÉ", true},
		{"CJK", "日本語", "日本語", true},
		{"tanda baca", "Alur pasien", "***", false},
		{"emoji tanpa token", "🩺", "🩺", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := rankedNotes(context.Background(), []memory.Note{{ID: "n", Title: tc.title}}, tc.query)
			if err != nil || (len(got) == 1) != tc.match {
				t.Fatalf("hasil = %v, %v; match = %v", got, err, tc.match)
			}
		})
	}
}

func TestRetrievalValidation(t *testing.T) {
	for _, query := range []string{strings.Repeat("x", 2001), strings.Repeat("é", 1001), "\xff", "pasien\x00", "pasien\x1b"} {
		if got, err := rankedNotes(context.Background(), nil, query); !errors.Is(err, memory.ErrInvalid) || got != nil {
			t.Fatalf("query invalid diterima: %q => %v, %v", query[:min(len(query), 20)], got, err)
		}
	}
	for _, query := range []string{strings.Repeat("x", 2000), strings.Repeat("é", 1000), "alur\npasien"} {
		if _, err := rankedNotes(context.Background(), nil, query); err != nil {
			t.Fatalf("query valid ditolak: %v", err)
		}
	}
	for _, tc := range []struct{ project, title, content string }{
		{"", "judul", ""}, {"../private", "judul", ""}, {"Global", "judul", ""},
		{"-project", "judul", ""}, {strings.Repeat("a", 65), "judul", ""},
		{"p", "", "konten"}, {"p", " \t", "konten"}, {"p", strings.Repeat("t", 201), ""},
		{"p", strings.Repeat("é", 101), ""}, {"p", "judul\n", ""}, {"p", "\xff", ""},
		{"p", "judul", strings.Repeat("c", 65537)}, {"p", "judul", strings.Repeat("é", 32769)},
		{"p", "judul", "\xff"}, {"p", "judul", "konten\x00"}, {"p", "judul", "konten\x1b"},
	} {
		if got, err := duplicateCandidates(context.Background(), nil, tc.project, tc.title, tc.content); !errors.Is(err, memory.ErrInvalid) || got != nil {
			t.Fatalf("input invalid diterima (project=%q, title bytes=%d, content bytes=%d): %v, %v", tc.project, len(tc.title), len(tc.content), got, err)
		}
	}
	for _, tc := range []struct{ project, title, content string }{
		{strings.Repeat("a", 64), strings.Repeat("t", 200), strings.Repeat("c", 65536)},
		{"p-1", strings.Repeat("é", 100), strings.Repeat("é", 32768)},
		{"global", "judul", ""}, {"p", "judul", "baris\nbaru\r\n\ttab"},
	} {
		if _, err := duplicateCandidates(context.Background(), nil, tc.project, tc.title, tc.content); err != nil {
			t.Fatalf("input valid ditolak: %v", err)
		}
	}
}

func TestDuplicateCandidatesLexicalSimilarity(t *testing.T) {
	notes := []memory.Note{
		{ID: "exact", Project: "simrs", Title: "Alur pasien BPJS", Content: "Loket menuju pemeriksaan farmasi kasir", Revision: 3},
		{ID: "near", Project: "simrs", Title: "Alur pasien BPJS rawat", Content: "Loket menuju pemeriksaan farmasi kasir pulang", Revision: 2},
		{ID: "copy", Project: "simrs", Title: "Prosedur pelayanan", Content: "Loket menuju pemeriksaan farmasi kasir", Revision: 1},
		{ID: "unrelated", Project: "simrs", Title: "Inventaris server", Content: "Backup jaringan hardware"},
	}
	got, err := duplicateCandidates(context.Background(), notes, "simrs", "ALUR PASIEN BPJS", "loket menuju pemeriksaan farmasi kasir")
	if err != nil || len(got) != 3 || got[0].ID != "exact" || got[0].Score != 1 || got[0].Revision != 3 {
		t.Fatalf("kandidat duplikat Indonesia: %+v, %v", got, err)
	}
	for _, candidate := range got {
		if candidate.Score < .6 || candidate.Score > 1 || !strings.Contains(candidate.Reason, "tinjauan manual") {
			t.Fatalf("metadata advisory tidak valid: %+v", candidate)
		}
	}
	if got[1].ID != "copy" || got[1].Score != .9 || !strings.Contains(got[1].Reason, "Konten sama") {
		t.Fatalf("konten disalin/judul diubah tidak terdeteksi: %+v", got)
	}
	// Empty content must not count as a full content match, or suggest all notes.
	got, err = duplicateCandidates(context.Background(), []memory.Note{
		{ID: "same", Project: "p", Title: "get_context expected_revision"},
		{ID: "different", Project: "p", Title: "bukan duplikat"},
	}, "p", "GET_CONTEXT expected_revision", "")
	if err != nil || len(got) != 1 || got[0].ID != "same" || got[0].Score != .7 {
		t.Fatalf("konten kosong/code terms: %+v, %v", got, err)
	}
}

func TestDuplicateCandidatesUnicode(t *testing.T) {
	got, err := duplicateCandidates(context.Background(), []memory.Note{
		{ID: "unicode", Project: "p", Title: "Kafé résumé 日本語", Content: "Rekam medis 🩺 résumé"},
	}, "p", "KAFÉ RÉSUMÉ 日本語", "rekam\nmedis 🩺 RÉSUMÉ")
	if err != nil || len(got) != 1 || got[0].Score != 1 {
		t.Fatalf("Unicode/case/multiline: %+v, %v", got, err)
	}
	got, err = duplicateCandidates(context.Background(), []memory.Note{{ID: "punct", Project: "p", Title: "***", Content: "🩺"}}, "p", "***", "🩺")
	if err != nil || len(got) != 0 {
		t.Fatalf("tanpa token bukan bukti duplikat: %+v, %v", got, err)
	}
}

func TestDuplicateCandidatesScopeAndJSON(t *testing.T) {
	got, err := duplicateCandidates(context.Background(), []memory.Note{
		{ID: "allowed", Project: "p", Title: "Judul", Content: "Konten", Revision: 7, Source: "source not returned", Author: "author not returned"},
		{ID: "private", Project: "other", Title: "Judul", Content: "Konten"},
		{ID: "global", Project: "global", Title: "Judul", Content: "Konten"},
		{ID: "archived", Project: "p", Title: "Judul", Content: "Konten", Archived: true},
	}, "p", "Judul", "Konten")
	if err != nil || len(got) != 1 || got[0].ID != "allowed" {
		t.Fatalf("scope/arsip bocor: %+v, %v", got, err)
	}
	encoded, err := json.Marshal(got[0])
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 6 {
		t.Fatalf("hanya enam field metadata diperbolehkan: %s", encoded)
	}
	for _, name := range []string{"id", "title", "project", "revision", "score", "reason"} {
		if _, ok := fields[name]; !ok {
			t.Fatalf("field JSON %q hilang: %s", name, encoded)
		}
	}
	// The helpers cannot discover notes outside the authorized snapshot.
	if got, err := duplicateCandidates(context.Background(), nil, "p", "Judul", "Konten"); err != nil || len(got) != 0 {
		t.Fatalf("snapshot kosong: %+v, %v", got, err)
	}
}

func TestRetrievalDeterminismBoundAndPurity(t *testing.T) {
	notes := make([]memory.Note, 40)
	for i := range notes {
		notes[i] = memory.Note{ID: fmt.Sprintf("%02d", i), Project: "p", Title: "Alur pasien", Content: "Konten sama", Revision: 1, UpdatedAt: "2026-10-22", RelatedTo: []string{"unchanged"}}
	}
	before, _ := json.Marshal(notes)
	wantRanked, err := rankedNotes(context.Background(), notes, "pasien")
	if err != nil {
		t.Fatal(err)
	}
	wantDuplicates, err := duplicateCandidates(context.Background(), notes, "p", "Alur pasien", "Konten sama")
	if err != nil || len(wantDuplicates) != 10 || wantDuplicates[0].ID != "00" || wantDuplicates[9].ID != "09" {
		t.Fatalf("batas sepuluh/top ID: %+v, %v", wantDuplicates, err)
	}
	after, _ := json.Marshal(notes)
	if string(before) != string(after) {
		t.Fatal("helper memutasi snapshot masukan")
	}
	rng := rand.New(rand.NewSource(42))
	for range 50 {
		shuffled := append([]memory.Note(nil), notes...)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		ranked, err := rankedNotes(context.Background(), shuffled, "pasien")
		if err != nil || !reflect.DeepEqual(wantRanked, ranked) {
			t.Fatalf("ranking nondeterministik: %v", err)
		}
		duplicates, err := duplicateCandidates(context.Background(), shuffled, "p", "Alur pasien", "Konten sama")
		if err != nil || !reflect.DeepEqual(wantDuplicates, duplicates) {
			t.Fatalf("kandidat nondeterministik: %+v, %v", duplicates, err)
		}
	}
	// Result slice reordering/replacement must not alias the caller's slice.
	wantRanked[0] = memory.Note{}
	if notes[0].ID != "00" {
		t.Fatal("slice keluaran mengalias snapshot")
	}
}

func TestDuplicateCandidatesRepeatedSnapshotRows(t *testing.T) {
	notes := []memory.Note{
		{ID: "same", Project: "p", Title: "Alur pasien", Content: "Konten", Revision: 1},
		{ID: "same", Project: "p", Title: "Alur pasien", Content: "Konten", Revision: 2},
		{ID: "other", Project: "p", Title: "Alur pasien", Content: "Konten", Revision: 1},
	}
	for range 2 {
		got, err := duplicateCandidates(context.Background(), notes, "p", "Alur pasien", "Konten")
		if err != nil || len(got) != 2 || got[1].ID != "same" || got[1].Revision != 2 {
			t.Fatalf("ID berulang harus satu kandidat deterministik: %+v, %v", got, err)
		}
		notes[0], notes[1] = notes[1], notes[0]
	}
}

func TestRankedNotesRelevanceBeforeSummaryAndNoInferredSupersession(t *testing.T) {
	notes := []memory.Note{
		{ID: "summary", Project: "p", Kind: "knowledge", Title: "Ringkasan", Content: "alur pasien", IsProjectSummary: true},
		{ID: "handoff", Project: "p", Kind: "handoff", Title: "Alur pasien", UpdatedAt: "2026-10-22"},
		{ID: "older", Project: "p", Kind: "knowledge", Title: "Alur pasien", UpdatedAt: "2020-01-01"},
	}
	got, err := rankedNotes(context.Background(), notes, "alur pasien")
	if err != nil || !reflect.DeepEqual(retrievalIDs(got), []string{"older", "handoff", "summary"}) {
		t.Fatalf("relevansi sebelum ringkasan; catatan lama tidak otomatis terganti: %v, %v", retrievalIDs(got), err)
	}
}

func TestRetrievalRecencyAndProjectTies(t *testing.T) {
	notes := []memory.Note{
		{ID: "a", Project: "z", UpdatedAt: "2026-01-01"},
		{ID: "z", Project: "a", UpdatedAt: "2026-01-01"},
		{ID: "recent", Project: "z", UpdatedAt: "2026-02-01"},
		{ID: "a", Project: "a", UpdatedAt: "2026-01-01"},
	}
	got, err := rankedNotes(context.Background(), notes, "")
	if err != nil || !reflect.DeepEqual(retrievalIDs(got), []string{"recent", "a", "z", "a"}) || got[1].Project != "a" {
		t.Fatalf("tie-break recency/project/ID: %+v, %v", got, err)
	}
}

// cancelAfterChecks makes mid-work cancellation deterministic, without goroutine
// scheduling races or wall-clock sleeps. Err has the same monotonic contract as
// an ordinary canceled context.
type retrievalCancelContext struct {
	context.Context
	checks int
	left   int
}

func (c *retrievalCancelContext) Err() error {
	c.checks++
	if c.checks >= c.left {
		return context.Canceled
	}
	return nil
}

func TestRetrievalCancellation(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	for _, ctx := range []context.Context{canceled, expired} {
		if got, err := rankedNotes(ctx, nil, ""); !errors.Is(err, ctx.Err()) || got != nil {
			t.Fatalf("rank cancellation: %v, %v", got, err)
		}
		if got, err := duplicateCandidates(ctx, nil, "p", "judul", ""); !errors.Is(err, ctx.Err()) || got != nil {
			t.Fatalf("duplicate cancellation: %v, %v", got, err)
		}
	}
	if _, err := rankedNotes(nil, nil, ""); !errors.Is(err, memory.ErrInvalid) {
		t.Fatalf("nil context: %v", err)
	}
	if _, err := duplicateCandidates(nil, nil, "p", "judul", ""); !errors.Is(err, memory.ErrInvalid) {
		t.Fatalf("nil context: %v", err)
	}
	notes := []memory.Note{{ID: "n", Project: "p", Title: "judul", Content: strings.Repeat("konten ", 9000)}}
	ctx := &retrievalCancelContext{Context: context.Background(), left: 12}
	if got, err := rankedNotes(ctx, notes, "konten"); !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("pembatalan saat tokenisasi ranking: %v, %v (checks=%d)", got, err, ctx.checks)
	}
	ctx = &retrievalCancelContext{Context: context.Background(), left: 12}
	if got, err := duplicateCandidates(ctx, notes, "p", "judul", "konten"); !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("pembatalan saat tokenisasi duplikat: %v, %v (checks=%d)", got, err, ctx.checks)
	}
}

func TestFTS5Availability(t *testing.T) {
	// Synthetic, in-memory capability probe only: no application data/index and
	// no dependency change. Lexical helpers above do not depend on FTS5.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("CREATE VIRTUAL TABLE synthetic_notes USING fts5(title, content)"); err != nil {
		t.Fatalf("pinned modernc.org/sqlite tidak menyediakan FTS5: %v", err)
	}
	if _, err := db.Exec("INSERT INTO synthetic_notes(title, content) VALUES (?, ?)", "Alur pasien", "get_context rekam medis"); err != nil {
		t.Fatal(err)
	}
	var title string
	if err := db.QueryRow("SELECT title FROM synthetic_notes WHERE synthetic_notes MATCH ?", "pasien").Scan(&title); err != nil || title != "Alur pasien" {
		t.Fatalf("FTS5 query sintetis: title=%q, err=%v", title, err)
	}
	t.Log("FTS5 tersedia pada driver SQLite terpin; CREATE/INSERT/MATCH in-memory berhasil, tanpa indeks persisten")
}
