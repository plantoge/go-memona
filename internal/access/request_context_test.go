package access

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"modernc.org/sqlite"
)

func requestKeyInput() KeyInput {
	return KeyInput{Name: " synthetic request ", Permissions: []string{"read", "write"}, Projects: []string{"demo"}}
}

func requestRows(t *testing.T, s *Store) map[string][][]any {
	t.Helper()
	out := contextListRows(t, s)
	for _, table := range []string{"admin", "sessions"} {
		rows, err := s.db.Query("SELECT * FROM " + table + " ORDER BY 1")
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

func TestRequestContextCancellation(t *testing.T) {
	s := contextTestStore(t)
	key, token, err := s.CreateKey(requestKeyInput())
	if err != nil {
		t.Fatal(err)
	}
	session := strings.Repeat("a", 64)
	if _, err := s.db.Exec("INSERT INTO sessions(hash,csrf,expires) VALUES(?,?,?)", digest(session), "synthetic-csrf", time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	operations := []struct {
		name string
		call func(context.Context) error
	}{
		{"session", func(ctx context.Context) error { _, err := s.SessionContext(ctx, session); return err }},
		{"authenticate", func(ctx context.Context) error { _, err := s.AuthenticateContext(ctx, token); return err }},
		{"login", func(ctx context.Context) error { _, _, err := s.LoginContext(ctx, "synthetic-test-password"); return err }},
		{"logout", func(ctx context.Context) error { return s.LogoutContext(ctx, session) }},
		{"create", func(ctx context.Context) error { _, _, err := s.CreateKeyContext(ctx, requestKeyInput()); return err }},
		{"revoke", func(ctx context.Context) error { return s.RevokeContext(ctx, key.ID) }},
		{"audit", func(ctx context.Context) error { return s.AuditContext(ctx, "synthetic", "test.cancel", "demo") }},
	}
	for _, op := range operations {
		for _, mode := range []string{"pre-canceled", "pre-deadline", "pool-cancel", "pool-deadline"} {
			t.Run(op.name+"/"+mode, func(t *testing.T) {
				before := requestRows(t, s)
				var ctx context.Context
				var cancel context.CancelFunc
				want := error(context.Canceled)
				if strings.Contains(mode, "deadline") {
					want = context.DeadlineExceeded
					delay := 100 * time.Millisecond
					if mode == "pre-deadline" {
						delay = -time.Second
					}
					ctx, cancel = context.WithTimeout(context.Background(), delay)
				} else {
					ctx, cancel = context.WithCancel(context.Background())
				}
				defer cancel()
				if strings.HasPrefix(mode, "pre-") {
					if mode == "pre-canceled" {
						cancel()
					}
					if err := op.call(ctx); !errors.Is(err, want) || errors.Is(err, ErrDenied) {
						t.Fatalf("request error = %v; want %v, not denial", err, want)
					}
				} else {
					conn, err := s.db.Conn(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					defer conn.Close()
					waitCount := s.db.Stats().WaitCount
					result := make(chan error, 1)
					go func() { result <- op.call(ctx) }()
					timer := time.NewTimer(5 * time.Second)
					defer timer.Stop()
					for s.db.Stats().WaitCount == waitCount {
						select {
						case err := <-result:
							t.Fatalf("returned before pool wait: %v", err)
						case <-timer.C:
							t.Fatal("request did not enter pool wait")
						default:
							runtime.Gosched()
						}
					}
					if mode == "pool-cancel" {
						cancel()
					}
					select {
					case err := <-result:
						if !errors.Is(err, want) || errors.Is(err, ErrDenied) {
							t.Fatalf("pool error = %v; want %v, not denial", err, want)
						}
					case <-timer.C:
						t.Fatal("request did not stop waiting after cancellation")
					}
					if err := conn.Close(); err != nil {
						t.Fatal(err)
					}
				}
				if after := requestRows(t, s); !reflect.DeepEqual(after, before) {
					t.Fatal("canceled request changed persisted rows")
				}
				if stats := s.db.Stats(); stats.InUse != 0 {
					t.Fatalf("request leaked connection: %+v", stats)
				}
			})
		}
	}
}

// Wrap the real SQLite driver only in tests. Cancel immediately after a chosen
// successful statement, so the transaction has genuine uncommitted mutations.
// No production hooks or dependency changes are needed.
type requestConnector struct {
	path  string
	after func(string)
}

func (c requestConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := (&sqlite.Driver{}).Open(c.path)
	if err != nil {
		return nil, err
	}
	return &requestConn{Conn: conn, after: c.after}, nil
}
func (c requestConnector) Driver() driver.Driver { return &sqlite.Driver{} }

type requestConn struct {
	driver.Conn
	after func(string)
}

func (c *requestConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
}
func (c *requestConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	result, err := c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
	if err == nil {
		c.after(query)
	}
	return result, err
}

func TestRequestContextTransactionRollback(t *testing.T) {
	for _, operation := range []string{"create", "revoke"} {
		for _, stage := range []string{"mutation", "audit"} {
			t.Run(operation+"/"+stage, func(t *testing.T) {
				s := contextTestStore(t)
				key, token, err := s.CreateKey(requestKeyInput())
				if err != nil {
					t.Fatal(err)
				}
				before := requestRows(t, s)
				var path string
				var seq int
				var name string
				if err := s.db.QueryRow("PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				statements := 0
				wrapped := sql.OpenDB(requestConnector{path: path, after: func(query string) {
					statements++
					if (stage == "mutation" && !strings.Contains(query, "INTO activity")) || (stage == "audit" && strings.Contains(query, "INTO activity")) {
						cancel()
					}
				}})
				defer wrapped.Close()
				wrapped.SetMaxOpenConns(1)
				requestStore := &Store{db: wrapped}
				if operation == "create" {
					got, secret, err := requestStore.CreateKeyContext(ctx, requestKeyInput())
					if !errors.Is(err, context.Canceled) || got.ID != "" || secret != "" {
						t.Fatalf("canceled create returned id=%q, secret-present=%t, error=%v", got.ID, secret != "", err)
					}
				} else if err := requestStore.RevokeContext(ctx, key.ID); !errors.Is(err, context.Canceled) {
					t.Fatalf("canceled revoke error = %v", err)
				}
				wantStatements := 1
				if stage == "audit" {
					wantStatements = 2
				}
				if statements != wantStatements {
					t.Fatalf("executed %d statements; want %d successful writes before cancellation", statements, wantStatements)
				}
				// Closing waits for any database/sql cancellation rollback before
				// inspecting the independent original connection.
				if err := wrapped.Close(); err != nil {
					t.Fatal(err)
				}
				if after := requestRows(t, s); !reflect.DeepEqual(after, before) {
					t.Fatal("canceled transaction persisted mutation or audit")
				}
				if _, err := s.Authenticate(token); err != nil {
					t.Fatalf("canceled revoke invalidated original key: %v", err)
				}
			})
		}
	}
}

func TestRequestContextNormalAndDenials(t *testing.T) {
	s := contextTestStore(t)
	ctx := context.Background()
	beforeLogin := time.Now().Add(8 * time.Hour).Unix()
	session, csrf, err := s.LoginContext(ctx, "synthetic-test-password")
	if err != nil || len(session) != 64 || len(csrf) != 64 {
		t.Fatalf("login shape or error: %v", err)
	}
	var persistedHash, persistedCSRF string
	var expiry int64
	if err := s.db.QueryRow("SELECT hash,csrf,expires FROM sessions").Scan(&persistedHash, &persistedCSRF, &expiry); err != nil {
		t.Fatal(err)
	}
	if persistedHash != digest(session) || persistedCSRF != csrf || expiry < beforeLogin || expiry > time.Now().Add(8*time.Hour).Unix() {
		t.Fatal("session hashing, CSRF or eight-hour lifespan changed")
	}
	for _, check := range []func(string) (string, error){s.Session, func(token string) (string, error) { return s.SessionContext(ctx, token) }} {
		if got, err := check(session); err != nil || got != csrf {
			t.Fatalf("session lookup = %q, %v", got, err)
		}
		for _, invalid := range []string{"bad", strings.Repeat("b", 64)} {
			if _, err := check(invalid); !errors.Is(err, ErrDenied) {
				t.Fatalf("invalid session error = %v", err)
			}
		}
	}
	for _, password := range []string{"wrong", strings.Repeat("x", 1025)} {
		if _, _, err := s.LoginContext(ctx, password); !errors.Is(err, ErrDenied) {
			t.Fatalf("invalid password error = %v", err)
		}
	}
	if err := s.LogoutContext(ctx, session); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionContext(ctx, session); !errors.Is(err, ErrDenied) {
		t.Fatalf("logged out session error = %v", err)
	}
	if err := s.LogoutContext(ctx, "missing"); err != nil {
		t.Fatalf("logout of unknown token changed semantics: %v", err)
	}
	if _, err := s.db.Exec("UPDATE sessions SET expires=? WHERE hash=?", time.Now().Unix(), digest(session)); err != nil {
		t.Fatal(err)
	}
	beforeExpired := requestRows(t, s)
	if _, err := s.SessionContext(ctx, session); !errors.Is(err, ErrDenied) {
		t.Fatalf("expired session error = %v", err)
	}
	if !reflect.DeepEqual(beforeExpired, requestRows(t, s)) {
		t.Fatal("expired session lookup mutated rows")
	}

	key, token, err := s.CreateKeyContext(ctx, requestKeyInput())
	if err != nil || key.Name != "synthetic request" || len(token) != 67 || !strings.HasPrefix(token, "mm_") {
		t.Fatalf("create shape or error: %v", err)
	}
	var persistedToken string
	if err := s.db.QueryRow("SELECT hash FROM api_keys WHERE id=?", key.ID).Scan(&persistedToken); err != nil || persistedToken != digest(token) {
		t.Fatalf("key hashing changed: %v", err)
	}
	for _, check := range []func(string) (Key, error){s.Authenticate, func(token string) (Key, error) { return s.AuthenticateContext(ctx, token) }} {
		got, err := check(token)
		if err != nil || !reflect.DeepEqual(got, key) || !got.Allows("read", "demo") || got.Allows("archive", "demo") || got.Allows("read", "other") {
			t.Fatalf("key/scope changed: %v", err)
		}
		for _, invalid := range []string{"bad", "xx_" + strings.Repeat("a", 64), "mm_" + strings.Repeat("b", 64)} {
			if _, err := check(invalid); !errors.Is(err, ErrDenied) {
				t.Fatalf("invalid token error = %v", err)
			}
		}
	}
	for _, expiry := range []string{time.Now().Add(-time.Hour).Format(time.RFC3339), "malformed"} {
		if _, err := s.db.Exec("UPDATE api_keys SET expires_at=? WHERE id=?", expiry, key.ID); err != nil {
			t.Fatal(err)
		}
		before := requestRows(t, s)
		if _, err := s.AuthenticateContext(ctx, token); !errors.Is(err, ErrDenied) {
			t.Fatalf("expired/malformed key error = %v", err)
		}
		if !reflect.DeepEqual(before, requestRows(t, s)) {
			t.Fatal("invalid key lookup mutated rows")
		}
	}
	if _, err := s.db.Exec("UPDATE api_keys SET expires_at='' WHERE id=?", key.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeContext(ctx, key.ID); err != nil {
		t.Fatal(err)
	}
	beforeRevoked := requestRows(t, s)
	if _, err := s.AuthenticateContext(ctx, token); !errors.Is(err, ErrDenied) {
		t.Fatalf("revoked key error = %v", err)
	}
	if !reflect.DeepEqual(beforeRevoked, requestRows(t, s)) {
		t.Fatal("revoked lookup mutated rows")
	}
	if err := s.RevokeContext(ctx, "missing"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown revoke error = %v", err)
	}
	if err := s.AuditContext(ctx, "synthetic", "test.context", key.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Audit("synthetic", "test.wrapper", key.ID); err != nil {
		t.Fatal(err)
	}
	events, err := s.Activity()
	if err != nil || len(events) != 4 {
		t.Fatalf("activity count = %d, error = %v", len(events), err)
	}
	for i, action := range []string{"test.wrapper", "test.context", "key.revoke", "key.create"} {
		if events[i].Action != action || events[i].Target != key.ID {
			t.Fatalf("event %d mismatch: %+v", i, events[i])
		}
	}
}

func TestRequestContextValidationUnchanged(t *testing.T) {
	s := contextTestStore(t)
	cases := []KeyInput{
		{},
		{Name: strings.Repeat("x", 101), Permissions: []string{"read"}, Projects: []string{"demo"}},
		{Name: "test", Permissions: []string{"delete"}, Projects: []string{"demo"}},
		{Name: "test", Permissions: []string{"read", "write", "archive", "read"}, Projects: []string{"demo"}},
		{Name: "test", Permissions: []string{"read"}, Projects: []string{"../escape"}},
		{Name: "test", Permissions: []string{"read"}, Projects: []string{"demo"}, ExpiresAt: "invalid"},
		{Name: "test", Permissions: []string{"read"}, Projects: []string{"demo"}, ExpiresAt: time.Now().Add(-time.Hour).Format(time.RFC3339)},
	}
	before := requestRows(t, s)
	for i, in := range cases {
		if _, _, err := s.CreateKeyContext(context.Background(), in); !errors.Is(err, ErrInvalid) {
			t.Fatalf("case %d error = %v", i, err)
		}
	}
	if !reflect.DeepEqual(before, requestRows(t, s)) {
		t.Fatal("invalid input changed persisted rows")
	}
	// A caller may use a separate bounded context after its request is canceled.
	request, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.AuditContext(request, "synthetic", "test.request", "demo"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	bounded, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := s.AuditContext(bounded, "synthetic", "test.committed", "demo"); err != nil {
		t.Fatal(err)
	}
	events, err := s.Activity()
	if err != nil || len(events) != 1 || events[0].Action != "test.committed" {
		t.Fatalf("bounded audit = %v, %v", events, err)
	}
}

func TestRequestContextTransactionAuditFailure(t *testing.T) {
	for _, operation := range []string{"create", "revoke"} {
		t.Run(operation, func(t *testing.T) {
			s := contextTestStore(t)
			key, _, err := s.CreateKey(requestKeyInput())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.db.Exec(`CREATE TRIGGER synthetic_audit_failure BEFORE INSERT ON activity BEGIN SELECT RAISE(ABORT, 'synthetic audit failure'); END`); err != nil {
				t.Fatal(err)
			}
			before := requestRows(t, s)
			if operation == "create" {
				_, _, err = s.CreateKeyContext(context.Background(), requestKeyInput())
			} else {
				err = s.RevokeContext(context.Background(), key.ID)
			}
			if err == nil || !strings.Contains(fmt.Sprint(err), "synthetic audit failure") {
				t.Fatalf("audit failure error = %v", err)
			}
			if !reflect.DeepEqual(before, requestRows(t, s)) {
				t.Fatal("failed audit did not roll back key mutation")
			}
		})
	}
}
