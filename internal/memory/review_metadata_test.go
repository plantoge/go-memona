package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func reviewTime(value string) *string { return &value }
func reviewFlag(value bool) *bool     { return &value }

func reviewInput() Input {
	in := fixtureInput()
	in.Status, in.Source = "verified", "Synthetic test evidence: local fixture"
	in.VerifiedAt, in.NeedsReview = reviewTime("2026-01-02T03:04:05.123456789Z"), reviewFlag(true)
	return in
}

func TestReviewMetadataLegacyCanonicalAndInputPresence(t *testing.T) {
	legacy := []byte("---\n" + `{"id":"0123456789abcdef0123456789abcdef","project":"global","kind":"fact","title":"Legacy","content":"Body","status":"verified","source":"Test evidence","author":"tester","revision":1,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","archived":false}` + "\n---\n\nBody\n")
	s, root := openTestStore(t)
	id := "0123456789abcdef0123456789abcdef"
	dir := filepath.Join(root, "notes", id)
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "000001.md")
	if err := os.WriteFile(path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	n, err := s.Get(id)
	if err != nil || n.VerifiedAt != "" || n.NeedsReview || !bytes.Equal(encode(n), legacy) {
		t.Fatal("legacy canonical bytes changed", err)
	}
	for _, suffix := range []string{`,"verified_at":""`, `,"verified_at":null`, `,"needs_review":false`, `,"needs_review":null`} {
		bad := bytes.Replace(legacy, []byte(`"archived":false}`), []byte(`"archived":false`+suffix+`}`), 1)
		if err := os.WriteFile(path, bad, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Get(id); !errors.Is(err, errCorrupt) {
			t.Fatalf("noncanonical %s accepted: %v", suffix, err)
		}
	}
	for _, tc := range []struct {
		data string
		at   *string
		flag *bool
	}{
		{`{}`, nil, nil}, {`{"verified_at":null,"needs_review":null}`, nil, nil},
		{`{"verified_at":"","needs_review":false}`, reviewTime(""), reviewFlag(false)},
		{`{"verified_at":"2026-01-02T03:04:05Z","needs_review":true}`, reviewTime("2026-01-02T03:04:05Z"), reviewFlag(true)},
	} {
		var in Input
		if err := json.Unmarshal([]byte(tc.data), &in); err != nil || !reflect.DeepEqual(in.VerifiedAt, tc.at) || !reflect.DeepEqual(in.NeedsReview, tc.flag) {
			t.Fatalf("presence lost: %s %+v %v", tc.data, in, err)
		}
		data, err := json.Marshal(in)
		if err != nil || bytes.Contains(data, []byte("verified_at")) != (tc.at != nil) || bytes.Contains(data, []byte("needs_review")) != (tc.flag != nil) {
			t.Fatalf("marshal presence lost: %s %v", data, err)
		}
	}
}

func TestReviewMetadataValidation(t *testing.T) {
	for _, kind := range []string{"fact", "decision", "knowledge", "handoff"} {
		for _, at := range []*string{nil, reviewTime(""), reviewTime("2026-01-02T03:04:05Z"), reviewTime("2026-01-02T03:04:05.123456789Z"), reviewTime("9999-12-31T23:59:59Z")} {
			in := reviewInput()
			in.Kind, in.VerifiedAt = kind, at
			if kind == "knowledge" {
				in.IsProjectSummary = reviewFlag(true)
			}
			if err := validate(in, "tester", false); err != nil {
				t.Fatalf("valid %s timestamp %v rejected: %v", kind, at, err)
			}
		}
	}
	for _, value := range []string{" ", "2026-01-02", "2026-01-02T03:04:05+00:00", "2026-01-02T04:04:05+01:00", "2026-01-02T03:04:05.100Z", "2026-01-02T03:04:05.000Z", "2026-01-02T03:04:05,1Z", "2026-01-02T03:04:05.1234567891Z", "2026-01-02T03:04:60Z", "2026-01-02T03:04:05Z\n", "\xff"} {
		in := reviewInput()
		in.VerifiedAt = reviewTime(value)
		if err := validate(in, "tester", false); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid timestamp %q accepted: %v", value, err)
		}
	}
	for _, status := range []string{"unverified", "draft"} {
		in := reviewInput()
		in.Status = status
		if err := validate(in, "tester", false); !errors.Is(err, ErrInvalid) {
			t.Fatalf("timestamp allowed with %s: %v", status, err)
		}
		in.VerifiedAt = reviewTime("")
		if err := validate(in, "tester", false); err != nil {
			t.Fatal("clear rejected", err)
		}
	}
	in := reviewInput()
	in.Source = " "
	if err := validate(in, "tester", false); !errors.Is(err, ErrInvalid) {
		t.Fatal("timestamp accepted without evidence", err)
	}
	for _, at := range []*string{nil, reviewTime(""), reviewInput().VerifiedAt} {
		for _, flag := range []*bool{nil, reviewFlag(false), reviewFlag(true)} {
			in := skillInput()
			in.VerifiedAt, in.NeedsReview = at, flag
			err := validate(in, "tester", false)
			if at == nil && flag == nil {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrInvalid) {
				t.Fatal("skill accepted explicit metadata, including clear", err)
			}
		}
	}
	for _, n := range []Note{{Kind: "skill", VerifiedAt: *reviewInput().VerifiedAt}, {Kind: "skill", NeedsReview: true}} {
		if err := validateReviewMetadata(noteInput(n)); !errors.Is(err, ErrInvalid) {
			t.Fatal("noteInput concealed invalid skill metadata", err)
		}
	}
}

func TestReviewMetadataCreateReviseAndIsolation(t *testing.T) {
	for _, contextual := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "context"}[contextual], func(t *testing.T) {
			s, root := openTestStore(t)
			create := func(in Input) (Note, error) {
				if contextual {
					return s.CreateContext(t.Context(), in, "tester")
				}
				return s.Create(in, "tester")
			}
			revise := func(n Note, in Input) Note {
				t.Helper()
				var got Note
				var err error
				if contextual {
					got, err = s.ReviseContext(t.Context(), n.ID, n.Revision, in, "tester")
				} else {
					got, err = s.Revise(n.ID, n.Revision, in, "tester")
				}
				if err != nil {
					t.Fatal(err)
				}
				return got
			}
			for _, in := range []Input{fixtureInput(), func() Input {
				in := fixtureInput()
				in.VerifiedAt, in.NeedsReview = reviewTime(""), reviewFlag(false)
				return in
			}()} {
				n, err := create(in)
				if err != nil || n.VerifiedAt != "" || n.NeedsReview || bytes.Contains(encode(n), []byte("verified_at")) || bytes.Contains(encode(n), []byte("needs_review")) {
					t.Fatal("absent/clear create added metadata", err)
				}
			}
			in := reviewInput()
			wantAt := *in.VerifiedAt
			n, err := create(in)
			if err != nil || n.VerifiedAt != wantAt || !n.NeedsReview {
				t.Fatal("create lost metadata", err)
			}
			*in.VerifiedAt, *in.NeedsReview = "mutated", false
			mapped := noteInput(n)
			*mapped.VerifiedAt, *mapped.NeedsReview = "mapping mutation", false
			if n.VerifiedAt != wantAt || !n.NeedsReview {
				t.Fatal("input/mapping aliases note")
			}
			in.VerifiedAt, in.NeedsReview = nil, nil
			in.Title, in.Content, in.Source = "Edited title", "Edited body", "Changed synthetic evidence"
			n = revise(n, in)
			if n.VerifiedAt != wantAt || !n.NeedsReview {
				t.Fatal("omission did not preserve")
			}
			in.VerifiedAt, in.NeedsReview = reviewTime(""), reviewFlag(false)
			n = revise(n, in)
			if n.VerifiedAt != "" || n.NeedsReview {
				t.Fatal("clear failed")
			}
			in.VerifiedAt, in.NeedsReview = nil, nil
			n = revise(n, in)
			if n.VerifiedAt != "" || n.NeedsReview {
				t.Fatal("omission resurrected metadata")
			}
			in = reviewInput()
			n = revise(n, in)
			*in.VerifiedAt, *in.NeedsReview = "mutated after revise", false
			if n.VerifiedAt != wantAt || !n.NeedsReview {
				t.Fatal("revision aliases input")
			}
			in.VerifiedAt, in.NeedsReview, in.Status, in.Source = nil, nil, "unverified", ""
			n = revise(n, in)
			if n.VerifiedAt != "" || !n.NeedsReview {
				t.Fatal("status transition must clear inherited time only")
			}
			in.Status, in.Source = "verified", "New synthetic evidence"
			n = revise(n, in)
			if n.VerifiedAt != "" || !n.NeedsReview {
				t.Fatal("status transition resurrected time")
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(root)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			h, err := reopened.History(n.ID)
			if err != nil || len(h) != 7 || h[0].VerifiedAt != wantAt || !h[0].NeedsReview || h[2].VerifiedAt != "" || h[2].NeedsReview || h[5].VerifiedAt != "" {
				t.Fatal("history/restart failed", err)
			}
		})
	}
}

func TestReviewMetadataNoImplicitReviewAndWrongKindClear(t *testing.T) {
	s, _ := openTestStore(t)
	in := reviewInput()
	in.NeedsReview = reviewFlag(false)
	n := mustCreate(t, s, in)
	in.VerifiedAt, in.NeedsReview = nil, nil
	in.Title, in.Content, in.Source = "Changed", "Changed body", "Changed evidence"
	got, err := s.Revise(n.ID, n.Revision, in, "tester")
	if err != nil || got.VerifiedAt != n.VerifiedAt || got.NeedsReview {
		t.Fatal("editing inferred a recheck or review flag", err)
	}
	for _, kind := range []string{"skill", "decision"} {
		bad := in
		bad.Kind, bad.VerifiedAt, bad.NeedsReview = kind, reviewTime(""), reviewFlag(false)
		if _, err := s.Revise(got.ID, got.Revision, bad, "tester"); !errors.Is(err, ErrInvalid) {
			t.Fatal("clear allowed kind change", err)
		}
	}
	if fresh, err := s.Get(got.ID); err != nil || !reflect.DeepEqual(fresh, got) {
		t.Fatal("invalid kind clear changed stored metadata", err)
	}
	in.Status, in.Source = "draft", ""
	got, err = s.Revise(got.ID, got.Revision, in, "tester")
	if err != nil || got.VerifiedAt != "" || got.NeedsReview {
		t.Fatal("draft transition did not clear only time", err)
	}
}

func TestReviewMetadataCanonicalInvalidStoredValuesRejected(t *testing.T) {
	for _, mutate := range []func(*Note){
		func(n *Note) { n.Kind, n.Status = "skill", "draft" },
		func(n *Note) { n.VerifiedAt = ""; n.Kind, n.Status = "skill", "draft" },
		func(n *Note) { n.Status = "unverified" },
		func(n *Note) { n.Source = "" },
		func(n *Note) { n.VerifiedAt = "2026-01-02T03:04:05+00:00" },
	} {
		s, root := openTestStore(t)
		n := mustCreate(t, s, reviewInput())
		mutate(&n)
		if err := os.WriteFile(filepath.Join(root, "notes", n.ID, "000001.md"), encode(n), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Get(n.ID); !errors.Is(err, errCorrupt) {
			t.Fatal("canonical invalid stored review metadata accepted", err)
		}
	}
}

func TestReviewMetadataLifecycleAndTransitions(t *testing.T) {
	for _, contextual := range []bool{false, true} {
		s, _ := openTestStore(t)
		n := mustCreate(t, s, reviewInput())
		for _, archive := range []bool{true, false} {
			previous := n
			var err error
			if archive && contextual {
				n, err = s.ArchiveContext(t.Context(), n.ID, n.Revision, "admin")
			} else if archive {
				n, err = s.Archive(n.ID, n.Revision, "admin")
			} else if contextual {
				n, err = s.RestoreContext(t.Context(), n.ID, n.Revision, "admin")
			} else {
				n, err = s.Restore(n.ID, n.Revision, "admin")
			}
			if err != nil || n.VerifiedAt != previous.VerifiedAt || n.NeedsReview != previous.NeedsReview || !validHistoryTransition([]Note{previous}, n) {
				t.Fatal("lifecycle lost metadata", err)
			}
			for _, timeChange := range []bool{false, true} {
				bad := n
				if timeChange {
					bad.VerifiedAt = "2026-02-01T00:00:00Z"
				} else {
					bad.NeedsReview = false
				}
				if validHistoryTransition([]Note{previous}, bad) {
					t.Fatal("lifecycle permits simultaneous metadata change")
				}
			}
		}
		if _, err := s.Publish(n.ID, n.Revision, "admin"); !errors.Is(err, ErrInvalid) {
			t.Fatal("non-skill published", err)
		}
		if h, err := s.History(n.ID); err != nil || len(h) != 3 {
			t.Fatal("lifecycle history invalid", err)
		}
		op := testOperation("archive_memory")
		op.NoteID, op.ExpectedRevision = n.ID, n.Revision
		for _, in := range []Input{{VerifiedAt: reviewTime("")}, {NeedsReview: reviewFlag(false)}} {
			if _, err := s.Execute(op, in, "tester"); !errors.Is(err, ErrInvalid) {
				t.Fatal("archive accepted metadata mutation input", err)
			}
		}
	}
}

func TestReviewMetadataLegacyFingerprintAndReceiptReplay(t *testing.T) {
	s, root := openTestStore(t)
	op, in := testOperation("save_memory"), skillInput()
	// This is the exact pre-review-metadata payload, including skill metadata.
	payload := `{"NoteID":"","Project":"global","Kind":"skill","Title":"A durable note","Content":"Body\nwith **Markdown**.\n---\n","Status":"draft","Source":"","ExpectedRevision":0,"IsProjectSummary":null,"RelatedTo":null,"Supersedes":null,"SkillMetadata":{"trigger":"Use for local release checks","version":"1.0","compatibility":["Go 1.26","Linux"],"test_evidence":"Local unit tests; source: release-check_test.go"}}`
	record := &operationRecord{Version: 1, Scope: hashJSON([]string{op.Principal, op.Tool, op.ID}), Payload: hashJSON(json.RawMessage(payload))}
	got, err := op.record(in)
	if err != nil || *got != *record {
		t.Fatal("legacy fingerprint changed", err)
	}
	n, err := s.create(in, "tester", record)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	replayed, err := reopened.Execute(op, in, "tester")
	if err != nil || !reflect.DeepEqual(n, replayed) {
		t.Fatal("legacy receipt failed", err)
	}
	for _, change := range []Input{{VerifiedAt: reviewTime("")}, {NeedsReview: reviewFlag(false)}} {
		changed := in
		changed.VerifiedAt, changed.NeedsReview = change.VerifiedAt, change.NeedsReview
		if _, err := reopened.Execute(op, changed, "tester"); !errors.Is(err, ErrOperationConflict) {
			t.Fatal("clear must differ from absent legacy input", err)
		}
	}
}

func TestReviewMetadataReceiptReplayAndClearIntent(t *testing.T) {
	for _, tool := range []string{"save_memory", "save_handoff", "revise_memory", "archive_memory"} {
		t.Run(tool, func(t *testing.T) {
			s, root := openTestStore(t)
			in, op := reviewInput(), testOperation(tool)
			if tool == "save_handoff" {
				in.Kind = "handoff"
			}
			if tool == "revise_memory" || tool == "archive_memory" {
				n := mustCreate(t, s, in)
				op.NoteID, op.ExpectedRevision = n.ID, n.Revision
				in.VerifiedAt, in.NeedsReview = reviewTime(""), reviewFlag(false)
			}
			if tool == "archive_memory" {
				in = Input{}
			}
			first, err := s.ExecuteContext(t.Context(), op, in, "tester")
			if err != nil {
				t.Fatal(err)
			}
			if tool == "revise_memory" && (first.VerifiedAt != "" || first.NeedsReview) {
				t.Fatal("clear lost")
			}
			if tool != "revise_memory" && (first.VerifiedAt == "" || !first.NeedsReview) {
				t.Fatal("metadata lost")
			}
			if first.Archived {
				_, err = s.Restore(first.ID, first.Revision, "admin")
			} else {
				clear := noteInput(first)
				clear.VerifiedAt, clear.NeedsReview = reviewTime(""), reviewFlag(false)
				_, err = s.Revise(first.ID, first.Revision, clear, "tester")
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(root)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			got, err := reopened.ExecuteContext(t.Context(), op, in, "tester")
			if err != nil || !reflect.DeepEqual(got, first) {
				t.Fatal("replay lost original result", err)
			}
			if tool != "archive_memory" {
				for _, changeTime := range []bool{false, true} {
					changed := in
					if changeTime {
						changed.VerifiedAt = nil
					} else {
						changed.NeedsReview = nil
					}
					if _, err := reopened.Execute(op, changed, "tester"); !errors.Is(err, ErrOperationConflict) {
						t.Fatal("changed intent not distinguished", err)
					}
				}
			}
			if h, err := reopened.History(first.ID); err != nil || len(h) != first.Revision+1 {
				t.Fatal("replay appended revision", err)
			}
		})
	}
}

func TestReviewMetadataCancellationAndValidationDoNotReserve(t *testing.T) {
	s, _ := openTestStore(t)
	in := reviewInput()
	n := mustCreate(t, s, in)
	clear := in
	clear.VerifiedAt, clear.NeedsReview = reviewTime(""), reviewFlag(false)
	op := testOperation("revise_memory")
	op.NoteID, op.ExpectedRevision = n.ID, n.Revision
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for name, mutate := range map[string]func() (Note, error){
		"create":  func() (Note, error) { return s.CreateContext(ctx, in, "tester") },
		"revise":  func() (Note, error) { return s.ReviseContext(ctx, n.ID, n.Revision, clear, "tester") },
		"execute": func() (Note, error) { return s.ExecuteContext(ctx, op, clear, "tester") },
		"archive": func() (Note, error) { return s.ArchiveContext(ctx, n.ID, n.Revision, "tester") },
	} {
		if _, err := mutate(); !errors.Is(err, context.Canceled) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if got, err := s.Get(n.ID); err != nil || !reflect.DeepEqual(got, n) {
		t.Fatal("cancellation changed note", err)
	}
	if names, err := directoryNames(s.stagingFD); err != nil || len(names) != 0 {
		t.Fatal("cancellation staged data", err)
	}
	invalid := clear
	invalid.Status, invalid.VerifiedAt = "unverified", in.VerifiedAt
	if _, err := s.Execute(op, invalid, "tester"); !errors.Is(err, ErrInvalid) {
		t.Fatal("explicit nonempty timestamp silently cleared", err)
	}
	if got, err := s.Get(n.ID); err != nil || !reflect.DeepEqual(got, n) {
		t.Fatal("validation changed note", err)
	}
	got, err := s.ExecuteContext(t.Context(), op, clear, "tester")
	if err != nil || got.Revision != 2 || got.VerifiedAt != "" || got.NeedsReview {
		t.Fatal("cancel/validation reserved operation or clear failed", err)
	}
}
