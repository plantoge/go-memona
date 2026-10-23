package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func skillInput() Input {
	in := fixtureInput()
	in.Kind, in.Status = "skill", "draft"
	in.SkillMetadata = &SkillMetadata{Trigger: "Use for local release checks", Version: "1.0", Compatibility: []string{"Go 1.26", "Linux"}, TestEvidence: "Local unit tests; source: release-check_test.go"}
	return in
}

func TestSkillMetadataLegacyCanonicalAndEmptyNormalization(t *testing.T) {
	// Includes the recently added is_testing flag, with no skill metadata.
	legacy := []byte("---\n" + `{"id":"0123456789abcdef0123456789abcdef","project":"global","kind":"skill","title":"Legacy","content":"Body","status":"draft","source":"","author":"tester","revision":1,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","archived":false,"is_testing":true}` + "\n---\n\nBody\n")
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
	if err != nil || n.SkillMetadata != nil || !bytes.Equal(encode(n), legacy) {
		t.Fatal("legacy canonical bytes changed", err)
	}
	n.SkillMetadata = &SkillMetadata{}
	if !bytes.Equal(encode(n), legacy) {
		t.Fatal("empty note metadata must normalize away")
	}
	for _, suffix := range []string{`,"skill_metadata":{}`, `,"skill_metadata":null`, `,"skill_metadata":{"trigger":""}`, `,"skill_metadata":{"compatibility":[]}`} {
		noncanonical := bytes.Replace(legacy, []byte(`"is_testing":true}`), []byte(`"is_testing":true`+suffix+`}`), 1)
		if err := os.WriteFile(path, noncanonical, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Get(id); !errors.Is(err, errCorrupt) {
			t.Fatalf("noncanonical metadata %s accepted: %v", suffix, err)
		}
	}
}

func TestSkillMetadataInputJSONPresence(t *testing.T) {
	for _, tc := range []struct {
		data string
		want *SkillMetadata
	}{
		{`{}`, nil}, {`{"skill_metadata":null}`, nil}, {`{"skill_metadata":{}}`, &SkillMetadata{}},
		{`{"skill_metadata":{"trigger":"Use here","version":"v1","compatibility":["Linux"],"test_evidence":"Test suite"}}`, &SkillMetadata{Trigger: "Use here", Version: "v1", Compatibility: []string{"Linux"}, TestEvidence: "Test suite"}},
	} {
		var in Input
		if err := json.Unmarshal([]byte(tc.data), &in); err != nil || !reflect.DeepEqual(in.SkillMetadata, tc.want) {
			t.Fatalf("decode %s: %+v %v", tc.data, in.SkillMetadata, err)
		}
		encoded, err := json.Marshal(in)
		if err != nil || bytes.Contains(encoded, []byte("skill_metadata")) != (tc.want != nil) {
			t.Fatalf("input intent lost: %s %v", encoded, err)
		}
	}
}

func TestSkillMetadataValidation(t *testing.T) {
	base := skillInput()
	boundary := &SkillMetadata{Trigger: strings.Repeat("é", 500), Version: strings.Repeat("v", 100), TestEvidence: strings.Repeat("t", 2000), Compatibility: []string{strings.Repeat("c", 900)}}
	for _, metadata := range []*SkillMetadata{nil, {}, base.SkillMetadata, boundary, {Compatibility: make([]string, 8)}} {
		in := base
		in.SkillMetadata = metadata
		if err := validate(in, "tester", false); err != nil {
			t.Fatalf("valid metadata rejected: %+v %v", metadata, err)
		}
	}
	cases := map[string]*SkillMetadata{
		"trigger bytes": {Trigger: strings.Repeat("é", 501)},
		"version":       {Version: strings.Repeat("v", 101)},
		"evidence":      {TestEvidence: strings.Repeat("t", 2001)},
		"aggregate":     {Trigger: strings.Repeat("a", 1000), TestEvidence: strings.Repeat("b", 2000), Compatibility: []string{strings.Repeat("c", 1001)}},
		"compat count":  {Compatibility: make([]string, 9)},
		"compat bytes":  {Compatibility: []string{strings.Repeat("x", 4001)}},
	}
	for _, field := range []string{"trigger", "version", "test_evidence", "compatibility"} {
		for label, value := range map[string]string{"utf8": "\xff", "nul": "\x00", "newline": "a\nb", "control": "\x7f"} {
			m := &SkillMetadata{}
			switch field {
			case "trigger":
				m.Trigger = value
			case "version":
				m.Version = value
			case "test_evidence":
				m.TestEvidence = value
			case "compatibility":
				m.Compatibility = []string{value}
			}
			cases[field+" "+label] = m
		}
	}
	for name, metadata := range cases {
		t.Run(name, func(t *testing.T) {
			in := base
			in.SkillMetadata = metadata
			if err := validate(in, "tester", false); !errors.Is(err, ErrInvalid) {
				t.Fatal("invalid metadata accepted", err)
			}
		})
	}
	for _, kind := range []string{"fact", "decision", "knowledge", "handoff"} {
		for _, metadata := range []*SkillMetadata{{}, base.SkillMetadata} {
			in := fixtureInput()
			in.Kind, in.SkillMetadata = kind, metadata
			if err := validate(in, "tester", false); !errors.Is(err, ErrInvalid) {
				t.Fatalf("metadata accepted for %s: %v", kind, err)
			}
		}
	}
}

func TestSkillMetadataCreateReviseIsolation(t *testing.T) {
	for _, contextual := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "context"}[contextual], func(t *testing.T) {
			s, root := openTestStore(t)
			create := func(in Input) (Note, error) {
				if contextual {
					return s.CreateContext(t.Context(), in, "tester")
				}
				return s.Create(in, "tester")
			}
			revise := func(n Note, in Input) (Note, error) {
				if contextual {
					return s.ReviseContext(t.Context(), n.ID, n.Revision, in, "tester")
				}
				return s.Revise(n.ID, n.Revision, in, "tester")
			}
			for _, metadata := range []*SkillMetadata{nil, {}, {Compatibility: []string{}}} {
				in := skillInput()
				in.SkillMetadata = metadata
				n, err := create(in)
				if err != nil || n.SkillMetadata != nil || bytes.Contains(encode(n), []byte("skill_metadata")) {
					t.Fatal("absent/empty create must omit metadata", err)
				}
			}
			in := skillInput()
			want := cloneSkillMetadata(in.SkillMetadata)
			n, err := create(in)
			if err != nil || !reflect.DeepEqual(n.SkillMetadata, want) {
				t.Fatal("create lost metadata", err)
			}
			in.SkillMetadata.Trigger = "input mutation"
			in.SkillMetadata.Compatibility[0] = "input mutation"
			if !reflect.DeepEqual(n.SkillMetadata, want) {
				t.Fatal("create aliases input")
			}
			mapped := noteInput(n)
			mapped.SkillMetadata.Version = "mapping mutation"
			mapped.SkillMetadata.Compatibility[0] = "mapping mutation"
			if !reflect.DeepEqual(n.SkillMetadata, want) {
				t.Fatal("noteInput aliases note")
			}
			n.SkillMetadata.Trigger = "returned mutation"
			n.SkillMetadata.Compatibility[0] = "returned mutation"
			fresh, err := s.Get(n.ID)
			if err != nil || !reflect.DeepEqual(fresh.SkillMetadata, want) {
				t.Fatal("returned metadata changed stored revision", err)
			}
			in.SkillMetadata = nil
			n, err = revise(fresh, in)
			if err != nil || !reflect.DeepEqual(n.SkillMetadata, want) {
				t.Fatal("omission must preserve metadata", err)
			}
			in.SkillMetadata = &SkillMetadata{}
			n, err = revise(n, in)
			if err != nil || n.SkillMetadata != nil || bytes.Contains(encode(n), []byte("skill_metadata")) {
				t.Fatal("empty object must clear", err)
			}
			in.SkillMetadata = nil
			n, err = revise(n, in)
			if err != nil || n.SkillMetadata != nil {
				t.Fatal("omission resurrected metadata", err)
			}
			in.SkillMetadata = &SkillMetadata{Version: "v2", Compatibility: []string{"new"}}
			n, err = revise(n, in)
			if err != nil || !reflect.DeepEqual(n.SkillMetadata, in.SkillMetadata) {
				t.Fatal("replacement failed", err)
			}
			in.SkillMetadata.Compatibility[0] = "mutation"
			if n.SkillMetadata.Compatibility[0] != "new" {
				t.Fatal("revise aliases input")
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(root)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			history, err := reopened.History(n.ID)
			if err != nil || len(history) != 5 || !reflect.DeepEqual(history[0].SkillMetadata, want) || history[2].SkillMetadata != nil || !reflect.DeepEqual(history[4].SkillMetadata, n.SkillMetadata) {
				t.Fatal("metadata history/restart invalid", err)
			}
		})
	}
}

func TestSkillMetadataLifecycleAndTransitions(t *testing.T) {
	for _, contextual := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "context"}[contextual], func(t *testing.T) {
			s, _ := openTestStore(t)
			in := skillInput()
			n := mustCreate(t, s, in)
			steps := []func(Note) (Note, error){
				func(n Note) (Note, error) {
					if contextual {
						return s.PublishContext(t.Context(), n.ID, n.Revision, "admin")
					}
					return s.Publish(n.ID, n.Revision, "admin")
				},
				func(n Note) (Note, error) {
					if contextual {
						return s.ArchiveContext(t.Context(), n.ID, n.Revision, "admin")
					}
					return s.Archive(n.ID, n.Revision, "admin")
				},
				func(n Note) (Note, error) {
					if contextual {
						return s.RestoreContext(t.Context(), n.ID, n.Revision, "admin")
					}
					return s.Restore(n.ID, n.Revision, "admin")
				},
			}
			for i, step := range steps {
				previous := n
				var err error
				n, err = step(n)
				if err != nil || !reflect.DeepEqual(n.SkillMetadata, in.SkillMetadata) || !validHistoryTransition([]Note{previous}, n) {
					t.Fatalf("lifecycle step %d: %v", i, err)
				}
				bad := n
				bad.SkillMetadata = cloneSkillMetadata(n.SkillMetadata)
				bad.SkillMetadata.Version = "unauthorized change"
				if validHistoryTransition([]Note{previous}, bad) {
					t.Fatalf("step %d allows metadata changes", i)
				}
				if n.Archived {
					if _, err := s.Revise(n.ID, n.Revision, in, "tester"); !errors.Is(err, ErrConflict) {
						t.Fatal("archived edit accepted", err)
					}
				}
			}
			in.Status, in.SkillMetadata = "active", nil
			var err error
			n, err = s.Revise(n.ID, n.Revision, in, "tester")
			if err != nil || n.Status != "draft" || n.SkillMetadata == nil {
				t.Fatal("active edit must remain draft and preserve metadata", err)
			}
			if h, err := s.History(n.ID); err != nil || len(h) != 5 {
				t.Fatal("lifecycle history invalid", err)
			}
		})
	}
}

func TestSkillMetadataLegacyFingerprintAndReplay(t *testing.T) {
	for _, testing := range []*bool{nil, testingFlag(false), testingFlag(true)} {
		s, root := openTestStore(t)
		op, in := testOperation("save_memory"), fixtureInput()
		in.IsTesting = testing
		payload := `{"NoteID":"","Project":"global","Kind":"fact","Title":"A durable note","Content":"Body\nwith **Markdown**.\n---\n","Status":"unverified","Source":"","ExpectedRevision":0,"IsProjectSummary":null,"RelatedTo":null,"Supersedes":null`
		if testing != nil {
			if *testing {
				payload += `,"IsTesting":true`
			} else {
				payload += `,"IsTesting":false`
			}
		}
		payload += `}`
		record := &operationRecord{Version: 1, Scope: hashJSON([]string{op.Principal, op.Tool, op.ID}), Payload: hashJSON(json.RawMessage(payload))}
		gotRecord, err := op.record(in)
		if err != nil || *gotRecord != *record {
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
		got, err := reopened.Execute(op, in, "tester")
		if err != nil || !reflect.DeepEqual(got, n) {
			t.Fatal("legacy receipt failed", err)
		}
		in.SkillMetadata = &SkillMetadata{}
		if _, err := reopened.Execute(op, in, "tester"); !errors.Is(err, ErrOperationConflict) {
			t.Fatal("explicit empty object must differ from absence", err)
		}
	}
}

func TestSkillMetadataCancellationDoesNotMutateOrReserve(t *testing.T) {
	s, _ := openTestStore(t)
	in := skillInput()
	n := mustCreate(t, s, in)
	clear := in
	clear.SkillMetadata = &SkillMetadata{}
	op := testOperation("revise_memory")
	op.NoteID, op.ExpectedRevision = n.ID, n.Revision
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for name, mutate := range map[string]func() (Note, error){
		"create":  func() (Note, error) { return s.CreateContext(ctx, in, "tester") },
		"revise":  func() (Note, error) { return s.ReviseContext(ctx, n.ID, n.Revision, clear, "tester") },
		"execute": func() (Note, error) { return s.ExecuteContext(ctx, op, clear, "tester") },
	} {
		if _, err := mutate(); !errors.Is(err, context.Canceled) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if got, err := s.Get(n.ID); err != nil || !reflect.DeepEqual(got, n) {
		t.Fatal("cancellation changed metadata", err)
	}
	if names, err := directoryNames(s.stagingFD); err != nil || len(names) != 0 {
		t.Fatal("cancellation staged data", err)
	}
	invalid := clear
	invalid.SkillMetadata = &SkillMetadata{Trigger: "bad\ntext"}
	if _, err := s.Execute(op, invalid, "tester"); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid metadata accepted", err)
	}
	got, err := s.ExecuteContext(t.Context(), op, clear, "tester")
	if err != nil || got.Revision != 2 || got.SkillMetadata != nil {
		t.Fatal("cancel/validation reserved receipt or clear failed", err)
	}
}

func TestSkillMetadataReceiptReplayAndClearIntent(t *testing.T) {
	for _, tool := range []string{"save_memory", "revise_memory", "archive_memory"} {
		t.Run(tool, func(t *testing.T) {
			s, root := openTestStore(t)
			in, op := skillInput(), testOperation(tool)
			if tool != "save_memory" {
				n := mustCreate(t, s, in)
				op.NoteID, op.ExpectedRevision = n.ID, n.Revision
				in.SkillMetadata = &SkillMetadata{}
			}
			if tool == "archive_memory" {
				in = Input{}
			}
			first, err := s.ExecuteContext(t.Context(), op, in, "tester")
			if err != nil {
				t.Fatal(err)
			}
			if tool == "revise_memory" && first.SkillMetadata != nil {
				t.Fatal("clear did not normalize")
			}
			if tool != "revise_memory" && first.SkillMetadata == nil {
				t.Fatal("metadata lost")
			}
			if first.Archived {
				if _, err := s.Restore(first.ID, first.Revision, "admin"); err != nil {
					t.Fatal(err)
				}
			} else {
				clear := noteInput(first)
				clear.SkillMetadata = &SkillMetadata{}
				if _, err := s.Revise(first.ID, first.Revision, clear, "tester"); err != nil {
					t.Fatal(err)
				}
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
				t.Fatal("receipt did not replay original", err)
			}
			for _, metadata := range []*SkillMetadata{nil, {}, {Version: "changed"}} {
				changed := in
				changed.SkillMetadata = metadata
				if reflect.DeepEqual(changed, in) {
					continue
				}
				if _, err := reopened.Execute(op, changed, "tester"); !errors.Is(err, ErrOperationConflict) {
					t.Fatal("changed metadata intent did not conflict", err)
				}
			}
			if h, err := reopened.History(first.ID); err != nil || len(h) != first.Revision+1 {
				t.Fatal("replay appended history", err)
			}
		})
	}
}
