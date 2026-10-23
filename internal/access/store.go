package access

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

var ErrDenied = errors.New("access denied")
var ErrInvalid = errors.New("invalid key settings")
var slug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

type Key struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Prefix      string   `json:"prefix"`
	Permissions []string `json:"permissions"`
	Projects    []string `json:"projects"`
	CreatedAt   string   `json:"created_at"`
	ExpiresAt   string   `json:"expires_at"`
	Revoked     bool     `json:"revoked"`
}
type KeyInput struct {
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
	Projects    []string `json:"projects"`
	ExpiresAt   string   `json:"expires_at"`
}
type Event struct {
	Time   string `json:"time"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Target string `json:"target"`
}
type Store struct{ db *sql.DB }

func Random() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}
func owned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}
func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func Open(dir, password string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 || !owned(info) {
		return nil, errors.New("data directory must be private (0700), not a symlink; repair permissions explicitly before startup")
	}
	path := filepath.Join(dir, "memona.db")
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || !owned(info)) {
		return nil, errors.New("database must be a private regular file (0600)")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	f.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	ok := false
	defer func() {
		if !ok {
			db.Close()
		}
	}()
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
 CREATE TABLE IF NOT EXISTS admin (id INTEGER PRIMARY KEY CHECK(id=1),salt TEXT NOT NULL,hash TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS api_keys (id TEXT PRIMARY KEY,name TEXT NOT NULL,prefix TEXT NOT NULL,hash TEXT UNIQUE NOT NULL,permissions TEXT NOT NULL,projects TEXT NOT NULL,created_at TEXT NOT NULL,expires_at TEXT NOT NULL,revoked INTEGER NOT NULL DEFAULT 0);
 CREATE TABLE IF NOT EXISTS sessions (hash TEXT PRIMARY KEY,csrf TEXT NOT NULL,expires INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS activity (id INTEGER PRIMARY KEY AUTOINCREMENT,time TEXT NOT NULL,actor TEXT NOT NULL,action TEXT NOT NULL,target TEXT NOT NULL);`)
	if err != nil {
		return nil, err
	}
	if err := s.initProjects(); err != nil {
		return nil, err
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM admin").Scan(&count); err != nil {
		return nil, err
	}
	if count == 0 {
		if len(password) < 16 || len(password) > 1024 {
			return nil, errors.New("first startup requires MEMONA_ADMIN_PASSWORD (16–1024 characters)")
		}
		salt, e := Random()
		if e != nil {
			return nil, e
		}
		hash, e := passwordHash(password, salt)
		if e != nil {
			return nil, e
		}
		if _, err = db.Exec("INSERT INTO admin(id,salt,hash) VALUES(1,?,?)", salt, hash); err != nil {
			return nil, err
		}
	}
	ok = true
	return s, nil
}
func passwordHash(password, salt string) (string, error) {
	b, e := pbkdf2.Key(sha256.New, password, []byte(salt), 600000, 32)
	return hex.EncodeToString(b), e
}
func (s *Store) Close() error { return s.db.Close() }

// A transaction may already have been rolled back by database/sql when its
// context expires. Keep the request failure rather than that secondary error.
func requestContextError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return err
}

func (s *Store) Login(password string) (string, string, error) {
	return s.LoginContext(context.Background(), password)
}
func (s *Store) LoginContext(ctx context.Context, password string) (string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	if len(password) > 1024 {
		return "", "", ErrDenied
	}
	var salt, want string
	if err := s.db.QueryRowContext(ctx, "SELECT salt,hash FROM admin WHERE id=1").Scan(&salt, &want); err != nil {
		return "", "", err
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	// PBKDF2 is not preemptible; check cancellation on both sides of the work.
	got, err := passwordHash(password, salt)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", "", ctxErr
	}
	if err != nil {
		return "", "", err
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		return "", "", ErrDenied
	}
	token, err := Random()
	if err != nil {
		return "", "", err
	}
	csrf, err := Random()
	if err != nil {
		return "", "", err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO sessions(hash,csrf,expires) VALUES(?,?,?)", digest(token), csrf, time.Now().Add(8*time.Hour).Unix())
	return token, csrf, err
}
func (s *Store) Session(token string) (string, error) {
	return s.SessionContext(context.Background(), token)
}
func (s *Store) SessionContext(ctx context.Context, token string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(token) != 64 {
		return "", ErrDenied
	}
	var csrf string
	var expires int64
	if err := s.db.QueryRowContext(ctx, "SELECT csrf,expires FROM sessions WHERE hash=?", digest(token)).Scan(&csrf, &expires); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", err
		}
		return "", ErrDenied
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if time.Now().Unix() >= expires {
		return "", ErrDenied
	}
	return csrf, nil
}
func (s *Store) Logout(token string) error {
	return s.LogoutContext(context.Background(), token)
}
func (s *Store) LogoutContext(ctx context.Context, token string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "UPDATE sessions SET expires=0 WHERE hash=?", digest(token))
	return err
}
func (s *Store) CreateKey(in KeyInput) (Key, string, error) {
	return s.CreateKeyContext(context.Background(), in)
}
func (s *Store) CreateKeyContext(ctx context.Context, in KeyInput) (Key, string, error) {
	if err := ctx.Err(); err != nil {
		return Key{}, "", err
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 || len(in.Permissions) == 0 || len(in.Permissions) > 3 || len(in.Projects) == 0 || len(in.Projects) > 100 {
		return Key{}, "", ErrInvalid
	}
	for _, p := range in.Permissions {
		if p != "read" && p != "write" && p != "archive" {
			return Key{}, "", ErrInvalid
		}
	}
	for _, p := range in.Projects {
		if p != "*" && !slug.MatchString(p) {
			return Key{}, "", ErrInvalid
		}
	}
	if in.ExpiresAt != "" {
		t, e := time.Parse(time.RFC3339, in.ExpiresAt)
		if e != nil || !t.After(time.Now()) {
			return Key{}, "", ErrInvalid
		}
		in.ExpiresAt = t.UTC().Format(time.RFC3339)
	}
	id, e := Random()
	if e != nil {
		return Key{}, "", e
	}
	secret, e := Random()
	if e != nil {
		return Key{}, "", e
	}
	token := "mm_" + secret
	k := Key{ID: id[:24], Name: in.Name, Prefix: token[:11], Permissions: in.Permissions, Projects: in.Projects, CreatedAt: time.Now().UTC().Format(time.RFC3339), ExpiresAt: in.ExpiresAt}
	permissions, _ := json.Marshal(k.Permissions)
	projects, _ := json.Marshal(k.Projects)
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return Key{}, "", requestContextError(ctx, e)
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(ctx, "INSERT INTO api_keys(id,name,prefix,hash,permissions,projects,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?)", k.ID, k.Name, k.Prefix, digest(token), string(permissions), string(projects), k.CreatedAt, k.ExpiresAt)
	if e != nil {
		return Key{}, "", requestContextError(ctx, e)
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO activity(time,actor,action,target) VALUES(?,?,?,?)", k.CreatedAt, "admin", "key.create", k.ID)
	if e != nil {
		return Key{}, "", requestContextError(ctx, e)
	}
	if e = ctx.Err(); e != nil {
		return Key{}, "", e
	}
	// A commit error can have an uncertain outcome; do not retry automatically.
	if e = tx.Commit(); e != nil {
		return Key{}, "", requestContextError(ctx, e)
	}
	return k, token, nil
}
func scanKey(row interface{ Scan(...any) error }) (Key, error) {
	var k Key
	var perms, projects string
	err := row.Scan(&k.ID, &k.Name, &k.Prefix, &perms, &projects, &k.CreatedAt, &k.ExpiresAt, &k.Revoked)
	if err != nil {
		return k, err
	}
	if err = json.Unmarshal([]byte(perms), &k.Permissions); err != nil {
		return k, err
	}
	err = json.Unmarshal([]byte(projects), &k.Projects)
	return k, err
}

const keyColumns = "id,name,prefix,permissions,projects,created_at,expires_at,revoked"

func (s *Store) Authenticate(token string) (Key, error) {
	return s.AuthenticateContext(context.Background(), token)
}
func (s *Store) AuthenticateContext(ctx context.Context, token string) (Key, error) {
	if err := ctx.Err(); err != nil {
		return Key{}, err
	}
	if len(token) != 67 || !strings.HasPrefix(token, "mm_") {
		return Key{}, ErrDenied
	}
	k, err := scanKey(s.db.QueryRowContext(ctx, "SELECT "+keyColumns+" FROM api_keys WHERE hash=?", digest(token)))
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Key{}, ctxErr
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return Key{}, err
	}
	if err != nil || k.Revoked {
		return Key{}, ErrDenied
	}
	if k.ExpiresAt != "" {
		t, e := time.Parse(time.RFC3339, k.ExpiresAt)
		if e != nil || !t.After(time.Now()) {
			return Key{}, ErrDenied
		}
	}
	return k, nil
}
func (k Key) Allows(permission, project string) bool {
	permitted := false
	for _, p := range k.Permissions {
		if p == permission {
			permitted = true
		}
	}
	if !permitted {
		return false
	}
	for _, p := range k.Projects {
		if p == "*" || p == project {
			return true
		}
	}
	return false
}
func (s *Store) Keys() ([]Key, error) {
	return s.KeysContext(context.Background())
}
func (s *Store) KeysContext(ctx context.Context) ([]Key, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, e := s.db.QueryContext(ctx, "SELECT "+keyColumns+" FROM api_keys ORDER BY created_at DESC,id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Key{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		k, e := scanKey(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, k)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, rows.Err()
}
func (s *Store) Revoke(id string) error {
	return s.RevokeContext(context.Background(), id)
}
func (s *Store) RevokeContext(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return requestContextError(ctx, e)
	}
	defer tx.Rollback()
	r, e := tx.ExecContext(ctx, "UPDATE api_keys SET revoked=1 WHERE id=?", id)
	if e != nil {
		return requestContextError(ctx, e)
	}
	n, e := r.RowsAffected()
	if e != nil {
		return requestContextError(ctx, e)
	}
	if n == 0 {
		return ErrInvalid
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO activity(time,actor,action,target) VALUES(?,?,?,?)", time.Now().UTC().Format(time.RFC3339), "admin", "key.revoke", id)
	if e != nil {
		return requestContextError(ctx, e)
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	// A commit error can have an uncertain outcome; do not retry automatically.
	if e = tx.Commit(); e != nil {
		return requestContextError(ctx, e)
	}
	return nil
}
func (s *Store) Audit(actor, action, target string) error {
	return s.AuditContext(context.Background(), actor, action, target)
}
func (s *Store) AuditContext(ctx context.Context, actor, action, target string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, e := s.db.ExecContext(ctx, "INSERT INTO activity(time,actor,action,target) VALUES(?,?,?,?)", time.Now().UTC().Format(time.RFC3339), actor, action, target)
	return e
}
func (s *Store) Activity() ([]Event, error) {
	return s.ActivityContext(context.Background())
}
func (s *Store) ActivityContext(ctx context.Context) ([]Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, e := s.db.QueryContext(ctx, "SELECT time,actor,action,target FROM activity ORDER BY id DESC LIMIT 200")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var v Event
		if e := rows.Scan(&v.Time, &v.Actor, &v.Action, &v.Target); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, rows.Err()
}
func (k Key) Actor() string { return fmt.Sprintf("key:%s (%s)", k.ID, k.Name) }
