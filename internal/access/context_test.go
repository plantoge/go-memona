package access

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func contextTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "private"), "synthetic-test-password")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func seedContextLists(t *testing.T, s *Store) ([]Key, []Event) {
	t.Helper()
	keys := []Key{
		{ID: "new-a", Name: "synthetic A", Prefix: "test-a", Permissions: []string{"read", "write"}, Projects: []string{"demo"}, CreatedAt: "2026-02-01T00:00:00Z", ExpiresAt: "2027-02-01T00:00:00Z", Revoked: true},
		{ID: "new-b", Name: "synthetic B", Prefix: "test-b", Permissions: []string{"read"}, Projects: []string{"*"}, CreatedAt: "2026-02-01T00:00:00Z"},
		{ID: "old", Name: "synthetic old", Prefix: "test-old", Permissions: []string{"archive"}, Projects: []string{"demo"}, CreatedAt: "2025-01-01T00:00:00Z"},
	}
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	// Insert out of listing order, including a revoked key and a timestamp tie.
	for _, i := range []int{2, 1, 0} {
		k := keys[i]
		perms := []string{`["read","write"]`, `["read"]`, `["archive"]`}[i]
		projects := []string{`["demo"]`, `["*"]`, `["demo"]`}[i]
		_, err := tx.Exec("INSERT INTO api_keys(id,name,prefix,hash,permissions,projects,created_at,expires_at,revoked) VALUES(?,?,?,?,?,?,?,?,?)", k.ID, k.Name, k.Prefix, "synthetic-hash-"+k.ID, perms, projects, k.CreatedAt, k.ExpiresAt, k.Revoked)
		if err != nil {
			t.Fatal(err)
		}
	}
	var events []Event
	for i := 0; i < 202; i++ {
		event := Event{Time: "2026-02-01T00:00:00Z", Actor: "synthetic", Action: "test.list", Target: fmt.Sprintf("target-%03d", i)}
		if _, err := tx.Exec("INSERT INTO activity(time,actor,action,target) VALUES(?,?,?,?)", event.Time, event.Actor, event.Action, event.Target); err != nil {
			t.Fatal(err)
		}
		if i >= 2 {
			events = append([]Event{event}, events...)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return keys, events
}

// Include all persisted columns, even those intentionally absent from listings.
func contextListRows(t *testing.T, s *Store) map[string][][]any {
	t.Helper()
	out := make(map[string][][]any)
	for _, table := range []string{"api_keys", "activity"} {
		rows, err := s.db.Query("SELECT * FROM " + table + " ORDER BY id")
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			t.Fatal(err)
		}
		for rows.Next() {
			values := make([]any, len(columns))
			dest := make([]any, len(columns))
			for i := range values {
				dest[i] = &values[i]
			}
			if err := rows.Scan(dest...); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			out[table] = append(out[table], values)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestReadListsContextNormal(t *testing.T) {
	s := contextTestStore(t)
	keys, err := s.KeysContext(context.Background())
	if err != nil || keys == nil || len(keys) != 0 {
		t.Fatalf("empty keys = %v, %v", keys, err)
	}
	events, err := s.ActivityContext(context.Background())
	if err != nil || events == nil || len(events) != 0 {
		t.Fatalf("empty activity = %v, %v", events, err)
	}
	wantKeys, wantEvents := seedContextLists(t, s)
	before := contextListRows(t, s)
	keys, err = s.KeysContext(context.Background())
	if err != nil || !reflect.DeepEqual(keys, wantKeys) {
		t.Fatalf("keys = %v, %v; want %v", keys, err, wantKeys)
	}
	events, err = s.ActivityContext(context.Background())
	if err != nil || !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("activity length = %d, error = %v; want newest 200 events", len(events), err)
	}
	legacyKeys, err := s.Keys()
	if err != nil || !reflect.DeepEqual(legacyKeys, wantKeys) {
		t.Fatalf("legacy keys = %v, %v", legacyKeys, err)
	}
	legacyEvents, err := s.Activity()
	if err != nil || !reflect.DeepEqual(legacyEvents, wantEvents) {
		t.Fatalf("legacy activity length = %d, error = %v", len(legacyEvents), err)
	}
	if !reflect.DeepEqual(contextListRows(t, s), before) {
		t.Fatal("listing changed persisted rows")
	}
}

func TestReadListsContextCancellation(t *testing.T) {
	for _, name := range []string{"keys", "activity"} {
		for _, waiting := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/waiting=%t", name, waiting), func(t *testing.T) {
				s := contextTestStore(t)
				seedContextLists(t, s)
				before := contextListRows(t, s)
				list := func(ctx context.Context) error {
					if name == "keys" {
						rows, err := s.KeysContext(ctx)
						if rows != nil {
							return errors.New("canceled key listing returned rows")
						}
						return err
					}
					rows, err := s.ActivityContext(ctx)
					if rows != nil {
						return errors.New("canceled activity listing returned rows")
					}
					return err
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if !waiting {
					cancel()
					if err := list(ctx); !errors.Is(err, context.Canceled) {
						t.Fatalf("pre-canceled listing error = %v", err)
					}
				} else {
					conn, err := s.db.Conn(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					defer conn.Close()
					waitCount := s.db.Stats().WaitCount
					result := make(chan error, 1)
					go func() { result <- list(ctx) }()
					timer := time.NewTimer(5 * time.Second)
					defer timer.Stop()
					for s.db.Stats().WaitCount == waitCount {
						select {
						case err := <-result:
							t.Fatalf("listing returned before waiting for connection: %v", err)
						case <-timer.C:
							t.Fatal("listing did not wait for the held connection")
						default:
							runtime.Gosched()
						}
					}
					cancel()
					select {
					case err := <-result:
						if !errors.Is(err, context.Canceled) {
							t.Fatalf("waiting listing error = %v", err)
						}
					case <-timer.C:
						t.Fatal("listing did not release its wait after cancellation")
					}
					if stats := s.db.Stats(); stats.InUse != 1 || stats.WaitCount != waitCount+1 {
						t.Fatalf("unexpected pool stats while connection held: %+v", stats)
					}
					if err := conn.Close(); err != nil {
						t.Fatal(err)
					}
				}
				checkCtx, checkCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer checkCancel()
				if _, err := s.KeysContext(checkCtx); err != nil {
					t.Fatalf("keys after cancellation: %v", err)
				}
				if _, err := s.ActivityContext(checkCtx); err != nil {
					t.Fatalf("activity after cancellation: %v", err)
				}
				if stats := s.db.Stats(); stats.InUse != 0 {
					t.Fatalf("listing leaked a connection: %+v", stats)
				}
				if !reflect.DeepEqual(contextListRows(t, s), before) {
					t.Fatal("canceled listing changed persisted rows")
				}
			})
		}
	}
}
