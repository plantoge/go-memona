package memory

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestMutationContextCancellationBeforeCommit(t *testing.T) {
	s, _ := openTestStore(t)
	n := mustCreate(t, s, fixtureInput())
	op := Operation{Principal: "0123456789abcdef01234567", Tool: "save_memory", ID: "cancel-operation"}
	methods := map[string]func(context.Context) (Note, error){
		"create": func(ctx context.Context) (Note, error) { return s.CreateContext(ctx, fixtureInput(), "tester") },
		"revise": func(ctx context.Context) (Note, error) {
			return s.ReviseContext(ctx, n.ID, n.Revision, fixtureInput(), "tester")
		},
		"archive": func(ctx context.Context) (Note, error) { return s.ArchiveContext(ctx, n.ID, n.Revision, "tester") },
		"restore": func(ctx context.Context) (Note, error) { return s.RestoreContext(ctx, n.ID, n.Revision, "tester") },
		"publish": func(ctx context.Context) (Note, error) { return s.PublishContext(ctx, n.ID, n.Revision, "tester") },
		"execute": func(ctx context.Context) (Note, error) { return s.ExecuteContext(ctx, op, fixtureInput(), "tester") },
	}
	for name, mutate := range methods {
		t.Run(name, func(t *testing.T) {
			s.mu.Lock()
			defer s.mu.Unlock()
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() { _, err := mutate(ctx); done <- err }()
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("canceled mutation waited for mutex")
			}
		})
	}
	got, err := s.Get(n.ID)
	if err != nil || !reflect.DeepEqual(got, n) {
		t.Fatal("cancellation changed note", err)
	}
	names, err := directoryNames(s.stagingFD)
	if err != nil || len(names) != 0 {
		t.Fatal("cancellation staged files", err)
	}
	first, err := s.ExecuteContext(t.Context(), op, fixtureInput(), "tester")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.ExecuteContext(t.Context(), op, fixtureInput(), "tester")
	if err != nil || !reflect.DeepEqual(first, replay) {
		t.Fatal("retry not idempotent", err)
	}
}

func TestRebuildOperationsBudgetPublishesOnlyCompleteIndex(t *testing.T) {
	s, _ := openTestStore(t)
	op := Operation{Principal: "0123456789abcdef01234567", Tool: "save_memory", ID: "rebuild-operation"}
	n, err := s.Execute(op, fixtureInput(), "tester")
	if err != nil {
		t.Fatal(err)
	}
	s.operationsDirty = true
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := s.rebuildOperationsBudget(&readBudget{ctx: ctx}); !errors.Is(err, context.Canceled) || !s.operationsDirty {
		t.Fatal(err)
	}
	if err := s.rebuildOperationsBudget(&readBudget{ctx: t.Context(), entries: ReadMaxEntries}); !errors.Is(err, ErrReadLimit) || !s.operationsDirty {
		t.Fatal(err)
	}
	got, err := s.ExecuteContext(t.Context(), op, fixtureInput(), "tester")
	if err != nil || got.ID != n.ID || got.Revision != n.Revision || s.operationsDirty {
		t.Fatal("recovery replay failed", err)
	}
}
