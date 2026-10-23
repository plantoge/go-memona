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

func testingFlag(value bool) *bool { return &value }

func TestTestingMetadataCanonicalCompatibility(t *testing.T) {
	// Literal pre-is_testing bytes, independent of the current Note definition.
	legacy := []byte("---\n" + `{"id":"0123456789abcdef0123456789abcdef","project":"global","kind":"fact","title":"Legacy","content":"Body","status":"unverified","source":"","author":"tester","revision":1,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","archived":false}` + "\n---\n\nBody\n")
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
	if err != nil || n.IsTesting || !bytes.Equal(encode(n), legacy) {
		t.Fatal("legacy canonical revision changed", err)
	}
	public, err := json.Marshal(n)
	if err != nil || bytes.Contains(public, []byte("is_testing")) {
		t.Fatal("false must be omitted from public JSON", err)
	}
	// A redundant false is not canonical, just like other omitempty flags.
	noncanonical := bytes.Replace(legacy, []byte(`"archived":false}`), []byte(`"archived":false,"is_testing":false}`), 1)
	if err := os.WriteFile(path, noncanonical, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(id); !errors.Is(err, errCorrupt) {
		t.Fatal("noncanonical explicit false accepted", err)
	}
}

func TestTestingInputJSONPresence(t *testing.T) {
	for _, tc := range []struct {
		json string
		want *bool
	}{{`{}`, nil}, {`{"is_testing":true}`, testingFlag(true)}, {`{"is_testing":false}`, testingFlag(false)}} {
		var in Input
		if err := json.Unmarshal([]byte(tc.json), &in); err != nil || !reflect.DeepEqual(in.IsTesting, tc.want) {
			t.Fatalf("decode %s: %+v %v", tc.json, in.IsTesting, err)
		}
		data, err := json.Marshal(in)
		if err != nil || bytes.Contains(data, []byte("is_testing")) != (tc.want != nil) {
			t.Fatalf("presence lost: %s %v", data, err)
		}
	}
}

func TestTestingMetadataCreateReviseHistory(t *testing.T) {
	for _, contextual := range []bool{false, true} {
		name := map[bool]string{false: "direct", true: "context"}[contextual]
		t.Run(name, func(t *testing.T) {
			s, root := openTestStore(t)
			in := fixtureInput()
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
			for _, flag := range []*bool{nil, testingFlag(false)} {
				in.IsTesting = flag
				n, err := create(in)
				if err != nil || n.IsTesting || bytes.Contains(encode(n), []byte("is_testing")) {
					t.Fatal("absent/false create must retain legacy encoding", err)
				}
			}
			in.IsTesting = testingFlag(true)
			n, err := create(in)
			if err != nil || !n.IsTesting || !bytes.Contains(encode(n), []byte(`"is_testing":true`)) {
				t.Fatal("create lost testing flag", err)
			}
			original := n
			in.IsTesting = nil
			in.Content = "Revised, flag omitted"
			n, err = revise(n, in)
			if err != nil || !n.IsTesting {
				t.Fatal("omitted revise must preserve", err)
			}
			mapped := noteInput(n)
			if mapped.IsTesting == nil || !*mapped.IsTesting {
				t.Fatal("noteInput lost true")
			}
			in.IsTesting = testingFlag(false)
			n, err = revise(n, in)
			if err != nil || n.IsTesting || bytes.Contains(encode(n), []byte("is_testing")) {
				t.Fatal("explicit false must clear", err)
			}
			mapped = noteInput(n)
			if mapped.IsTesting == nil || *mapped.IsTesting {
				t.Fatal("noteInput lost explicit false")
			}
			in.IsTesting = nil
			n, err = revise(n, in)
			if err != nil || n.IsTesting {
				t.Fatal("omission resurrected cleared metadata", err)
			}
			in.IsTesting = testingFlag(true)
			n, err = revise(n, in)
			if err != nil || !n.IsTesting {
				t.Fatal("explicit true revise must set metadata", err)
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
			if err != nil || len(history) != 5 || !reflect.DeepEqual(history[0], original) {
				t.Fatal("history did not retain immutable original", err)
			}
			for i, want := range []bool{true, true, false, false, true} {
				if history[i].IsTesting != want {
					t.Fatalf("revision %d: flag = %v, want %v", i+1, history[i].IsTesting, want)
				}
			}
		})
	}
}

func TestTestingMetadataLifecycleAndTransitions(t *testing.T) {
	for _, contextual := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "context"}[contextual], func(t *testing.T) {
			s, _ := openTestStore(t)
			in := fixtureInput()
			in.Kind, in.Status, in.IsTesting = "skill", "draft", testingFlag(true)
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
				if err != nil || !n.IsTesting {
					t.Fatalf("step %d lost testing flag: %v", i, err)
				}
				if !validHistoryTransition([]Note{previous}, n) {
					t.Fatalf("step %d valid transition rejected", i)
				}
				bad := n
				bad.IsTesting = false
				if validHistoryTransition([]Note{previous}, bad) {
					t.Fatalf("step %d allowed metadata change during publish/archive/restore", i)
				}
			}
			// Revising active skills still returns to draft and preserves the flag.
			in.Status, in.IsTesting = "active", nil
			var err error
			n, err = s.Revise(n.ID, n.Revision, in, "tester")
			if err != nil || !n.IsTesting || n.Status != "draft" {
				t.Fatal("skill edit changed metadata/status contract", err)
			}
			if h, err := s.History(n.ID); err != nil || len(h) != 5 {
				t.Fatal("lifecycle history invalid", err)
			}
		})
	}
}

func TestTestingLegacyOperationFingerprintAndReplay(t *testing.T) {
	s, root := openTestStore(t)
	op, in := testOperation("save_memory"), fixtureInput()
	// Pre-is_testing payload, with its exact original field names/order/nulls.
	legacyPayload := json.RawMessage(`{"NoteID":"","Project":"global","Kind":"fact","Title":"A durable note","Content":"Body\nwith **Markdown**.\n---\n","Status":"unverified","Source":"","ExpectedRevision":0,"IsProjectSummary":null,"RelatedTo":null,"Supersedes":null}`)
	record := &operationRecord{Version: 1, Scope: hashJSON([]string{op.Principal, op.Tool, op.ID}), Payload: hashJSON(legacyPayload)}
	current, err := op.record(in)
	if err != nil || *current != *record {
		t.Fatal("omitted testing flag changed legacy receipt fingerprint", err)
	}
	// Seed an old-style receipt independently, then exercise startup reconstruction.
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
	got, err := reopened.Execute(op, in, "renamed-tester")
	if err != nil || !reflect.DeepEqual(got, n) {
		t.Fatal("legacy receipt no longer replays original payload", err)
	}
	for _, flag := range []bool{false, true} {
		in.IsTesting = testingFlag(flag)
		if _, err := reopened.Execute(op, in, "tester"); !errors.Is(err, ErrOperationConflict) {
			t.Fatal("legacy receipt accepted changed metadata presence", err)
		}
	}
}

func TestTestingOperationReplayAndConflict(t *testing.T) {
	for _, tool := range []string{"save_memory", "save_handoff", "revise_memory", "archive_memory"} {
		t.Run(tool, func(t *testing.T) {
			s, root := openTestStore(t)
			in, op := fixtureInput(), testOperation(tool)
			in.IsTesting = testingFlag(true)
			if tool == "revise_memory" || tool == "archive_memory" {
				n := mustCreate(t, s, in)
				op.NoteID, op.ExpectedRevision = n.ID, n.Revision
				// Revision omission preserves; archive has no metadata arguments.
				in.IsTesting = nil
			}
			if tool == "archive_memory" {
				in = Input{}
			}
			first, err := s.ExecuteContext(t.Context(), op, in, "tester")
			if err != nil || !first.IsTesting {
				t.Fatal("execute lost testing flag", err)
			}
			if first.Archived {
				if _, err := s.Restore(first.ID, first.Revision, "admin"); err != nil {
					t.Fatal(err)
				}
			} else {
				clear := noteInput(first)
				clear.IsTesting = testingFlag(false)
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
				t.Fatal("replay returned latest instead of original", err)
			}
			for _, flag := range []*bool{nil, testingFlag(false), testingFlag(true)} {
				changed := in
				changed.IsTesting = flag
				if reflect.DeepEqual(changed, in) {
					continue
				}
				if _, err := reopened.Execute(op, changed, "tester"); !errors.Is(err, ErrOperationConflict) {
					t.Fatal("changed testing metadata did not conflict", err)
				}
			}
			h, err := reopened.History(first.ID)
			if err != nil || len(h) != first.Revision+1 {
				t.Fatal("replay wrote extra revision", err)
			}
		})
	}
}

func TestTestingMetadataCancellationDoesNotMutateOrReserve(t *testing.T) {
	s, _ := openTestStore(t)
	in := fixtureInput()
	in.Kind, in.Status, in.IsTesting = "skill", "draft", testingFlag(true)
	n := mustCreate(t, s, in)
	archived := mustCreate(t, s, in)
	var err error
	archived, err = s.Archive(archived.ID, archived.Revision, "tester")
	if err != nil {
		t.Fatal(err)
	}
	clear := in
	clear.IsTesting = testingFlag(false)
	op := testOperation("revise_memory")
	op.NoteID, op.ExpectedRevision = n.ID, n.Revision
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for name, mutate := range map[string]func() (Note, error){
		"create":  func() (Note, error) { return s.CreateContext(ctx, in, "tester") },
		"revise":  func() (Note, error) { return s.ReviseContext(ctx, n.ID, n.Revision, clear, "tester") },
		"archive": func() (Note, error) { return s.ArchiveContext(ctx, n.ID, n.Revision, "tester") },
		"publish": func() (Note, error) { return s.PublishContext(ctx, n.ID, n.Revision, "admin") },
		"restore": func() (Note, error) { return s.RestoreContext(ctx, archived.ID, archived.Revision, "admin") },
		"execute": func() (Note, error) { return s.ExecuteContext(ctx, op, clear, "tester") },
	} {
		if _, err := mutate(); !errors.Is(err, context.Canceled) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for _, want := range []Note{n, archived} {
		got, err := s.Get(want.ID)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatal("cancellation mutated metadata/history", err)
		}
	}
	if names, err := directoryNames(s.stagingFD); err != nil || len(names) != 0 {
		t.Fatal("cancellation staged data", err)
	}
	got, err := s.ExecuteContext(t.Context(), op, clear, "tester")
	if err != nil || got.IsTesting || got.Revision != 2 {
		t.Fatal("cancellation reserved operation or clear failed", err)
	}
	replay, err := s.ExecuteContext(t.Context(), op, clear, "tester")
	if err != nil || !reflect.DeepEqual(got, replay) {
		t.Fatal("clear receipt did not replay", err)
	}
	clear.IsTesting = nil
	if _, err := s.Execute(op, clear, "tester"); !errors.Is(err, ErrOperationConflict) {
		t.Fatal("clear and omit have same receipt", err)
	}
}
