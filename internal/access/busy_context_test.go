package access

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// A separate SQLite connection holds a real writer lock, not a database/sql
// pool slot. The driver may only observe cancellation after busy_timeout.
func TestAuditContextSQLiteWriterBusy(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	s, err := Open(dir, "synthetic-test-password")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	blocker, err := sql.Open("sqlite", filepath.Join(dir, "memona.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	conn, err := blocker.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err = conn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	started := time.Now()
	err = s.AuditContext(ctx, "synthetic", "busy-test", "redacted")
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("write unexpectedly passed writer lock")
	}
	if ctx.Err() == nil {
		t.Fatalf("operation failed before test deadline: %v", err)
	}
	t.Logf("real SQLite writer lock: elapsed=%s error=%v", elapsed, err)
	if elapsed > 7*time.Second {
		t.Fatalf("busy wait exceeded configured 5s timeout plus scheduling allowance: %s", elapsed)
	}
	if _, err = conn.ExecContext(context.Background(), "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	events, err := s.ActivityContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Action == "busy-test" {
			t.Fatal("canceled busy audit persisted")
		}
	}
	if err = s.AuditContext(context.Background(), "synthetic", "after-busy", "redacted"); err != nil {
		t.Fatal("connection unusable after busy cancellation", err)
	}
}
