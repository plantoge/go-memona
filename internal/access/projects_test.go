package access

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func projectTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "private"), "project-tests-password-12345")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	// Keep these tests independent of the parent's Open integration change.
	if err := s.initProjects(); err != nil {
		t.Fatal(err)
	}
	if err := s.initProjects(); err != nil {
		t.Fatalf("idempotent init: %v", err)
	}
	return s
}

func TestProjectRegistryLifecycleAndScopes(t *testing.T) {
	s := projectTestStore(t)
	ctx := context.Background()
	list, err := s.ProjectListContext(ctx)
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("empty registry: %#v %v", list, err)
	}
	_, token, err := s.CreateKey(KeyInput{Name: "existing scope", Permissions: []string{"read", "write"}, Projects: []string{"legacy"}})
	if err != nil {
		t.Fatal(err)
	}
	in := ProjectInput{Slug: "zeta", Name: " Zeta display ", Description: " Display metadata ", Aliases: []string{"legacy", "https://example.invalid/reference", "/private/not-scanned"}}
	created, err := s.SaveProjectContext(ctx, in)
	if err != nil || created.Revision != 1 || created.Name != "Zeta display" || created.Description != "Display metadata" || !reflect.DeepEqual(created.Aliases, in.Aliases) {
		t.Fatalf("create: %#v %v", created, err)
	}
	if _, err := s.SaveProjectContext(ctx, in); !errors.Is(err, ErrProjectConflict) {
		t.Fatalf("duplicate create: %v", err)
	}
	in.ExpectedRevision = 1
	in.Archived = true
	archived, err := s.SaveProjectContext(ctx, in)
	if err != nil || !archived.Archived || archived.Revision != 2 {
		t.Fatalf("archive: %#v %v", archived, err)
	}
	if _, err := s.SaveProjectContext(ctx, in); !errors.Is(err, ErrProjectConflict) {
		t.Fatalf("stale update: %v", err)
	}
	in.ExpectedRevision = 2
	in.Archived = false
	restored, err := s.SaveProjectContext(ctx, in)
	if err != nil || restored.Archived || restored.Revision != 3 {
		t.Fatalf("restore: %#v %v", restored, err)
	}
	// A different slug at an existing revision cannot rename the identity.
	in.Slug = "renamed"
	in.ExpectedRevision = 3
	if _, err := s.SaveProjectContext(ctx, in); !errors.Is(err, ErrProjectConflict) {
		t.Fatalf("rename accepted: %v", err)
	}
	if _, err := s.SaveProjectContext(ctx, ProjectInput{Slug: "alpha", Name: "Alpha"}); err != nil {
		t.Fatal(err)
	}
	list, err = s.ProjectListContext(ctx)
	if err != nil || len(list) != 2 || list[0].Slug != "alpha" || list[1].Slug != "zeta" || !reflect.DeepEqual(list[1].Aliases, created.Aliases) || list[0].Aliases == nil {
		t.Fatalf("ordered round trip: %#v %v", list, err)
	}
	key, err := s.Authenticate(token)
	if err != nil || !key.Allows("read", "legacy") || key.Allows("read", "zeta") || key.Allows("read", "/private/not-scanned") || key.Allows("read", "https://example.invalid/reference") {
		t.Fatalf("registry broadened key scope: %#v %v", key, err)
	}
	other := Key{Permissions: []string{"read"}, Projects: []string{"zeta"}}
	if other.Allows("read", "legacy") {
		t.Fatal("alias treated as authorization identity")
	}
	events, err := s.Activity()
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, event := range events {
		if event.Target == "zeta" {
			if event.Actor != "admin" {
				t.Fatalf("audit actor: %#v", event)
			}
			actions = append(actions, event.Action)
		}
	}
	if !reflect.DeepEqual(actions, []string{"project.update", "project.update", "project.create"}) {
		t.Fatalf("unexpected audit actions: %v", actions)
	}
}

func TestProjectRegistryValidation(t *testing.T) {
	s := projectTestStore(t)
	cases := []ProjectInput{
		{Slug: "../escape", Name: "Name"}, {Slug: "Upper", Name: "Name"},
		{Slug: "*", Name: "Name"}, {Slug: strings.Repeat("a", 65), Name: "Name"},
		{Slug: "valid", Name: " "}, {Slug: "valid", Name: strings.Repeat("n", 201)},
		{Slug: "valid", Name: "Name", Description: strings.Repeat("d", 2001)},
		{Slug: "valid", Name: "Name", Aliases: make([]string, 21)},
		{Slug: "valid", Name: "Name", Aliases: []string{" "}},
		{Slug: "valid", Name: "Name", Aliases: []string{strings.Repeat("a", 201)}},
		{Slug: "valid", Name: "Name", Aliases: []string{"same", " same "}},
		{Slug: "valid", Name: "Name", ExpectedRevision: -1},
		{Slug: "valid", Name: "Name", ExpectedRevision: int(^uint(0) >> 1)},
	}
	for i, in := range cases {
		if _, err := s.SaveProjectContext(context.Background(), in); !errors.Is(err, ErrProjectInvalid) {
			t.Errorf("case %d: %v", i, err)
		}
	}
	aliases := []string{" Reference "}
	valid := ProjectInput{Slug: strings.Repeat("a", 64), Name: strings.Repeat("n", 200), Description: strings.Repeat("d", 2000), Aliases: aliases}
	if _, err := s.SaveProjectContext(context.Background(), valid); err != nil {
		t.Fatal(err)
	}
	if aliases[0] != " Reference " {
		t.Fatal("caller aliases mutated")
	}
}

func TestProjectRegistryAuditFailureRollsBack(t *testing.T) {
	s := projectTestStore(t)
	ctx := context.Background()
	if _, err := s.SaveProjectContext(ctx, ProjectInput{Slug: "existing", Name: "Original"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER fail_project_audit BEFORE INSERT ON activity WHEN NEW.action LIKE 'project.%' BEGIN SELECT RAISE(ABORT, 'synthetic audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	for _, in := range []ProjectInput{{Slug: "new", Name: "New"}, {Slug: "existing", Name: "Changed", Archived: true, ExpectedRevision: 1}} {
		if _, err := s.SaveProjectContext(ctx, in); err == nil {
			t.Fatal("audit failure accepted")
		}
	}
	list, err := s.ProjectListContext(ctx)
	if err != nil || len(list) != 1 || list[0].Name != "Original" || list[0].Revision != 1 || list[0].Archived {
		t.Fatalf("save not rolled back: %#v %v", list, err)
	}
}

func TestProjectRegistryLimit(t *testing.T) {
	s := projectTestStore(t)
	ctx := context.Background()
	// Synthetic rows avoid 1000 audit writes while exercising the exact cap.
	if _, err := s.db.Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<1000)
 INSERT INTO project_registry(slug,name,description,aliases,archived,revision) SELECT printf('p-%04d',x),'Name','','[]',0,1 FROM n`); err != nil {
		t.Fatal(err)
	}
	list, err := s.ProjectListContext(ctx)
	if err != nil || len(list) != 1000 {
		t.Fatalf("at cap: %d %v", len(list), err)
	}
	if _, err := s.SaveProjectContext(ctx, ProjectInput{Slug: "overflow", Name: "Overflow"}); !errors.Is(err, ErrProjectLimit) {
		t.Fatalf("cap not enforced: %v", err)
	}
	if _, err := s.SaveProjectContext(ctx, ProjectInput{Slug: "p-0001", Name: "Duplicate"}); !errors.Is(err, ErrProjectConflict) {
		t.Fatalf("existing slug at cap: %v", err)
	}
	if _, err := s.SaveProjectContext(ctx, ProjectInput{Slug: "p-0001", Name: "Updated at cap", ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO project_registry VALUES('extra','Extra','','[]',0,1)`); err != nil {
		t.Fatal(err)
	}
	if list, err := s.ProjectListContext(ctx); !errors.Is(err, ErrProjectLimit) || list != nil {
		t.Fatalf("oversized registry silently truncated: %d %v", len(list), err)
	}
}

func TestProjectRegistryConcurrentRevision(t *testing.T) {
	s := projectTestStore(t)
	ctx := context.Background()
	if _, err := s.SaveProjectContext(ctx, ProjectInput{Slug: "shared", Name: "Original"}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := range 8 {
		wg.Go(func() {
			_, err := s.SaveProjectContext(ctx, ProjectInput{Slug: "shared", Name: fmt.Sprintf("Writer %d", i), ExpectedRevision: 1})
			results <- err
		})
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrProjectConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 7 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
}

func TestProjectRegistryCanceledContext(t *testing.T) {
	s := projectTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.ProjectListContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := s.SaveProjectContext(ctx, ProjectInput{Slug: "canceled", Name: "Canceled"}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	list, err := s.ProjectListContext(context.Background())
	if err != nil || len(list) != 0 {
		t.Fatalf("canceled request wrote metadata: %#v %v", list, err)
	}
}
