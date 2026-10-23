package access

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// These are feasibility tests, not a recovery implementation. In particular,
// an fd URI must not be treated as proof that SQLite pins sidecar paths.
func recoveryPathFixture(t *testing.T) string {
	t.Helper()
	parent := os.Getenv("GOTMPDIR")
	if parent == "" {
		parent = t.TempDir()
	}
	dir, err := os.MkdirTemp(parent, "admin-path-probe-")
	if err != nil {
		t.Fatal("create synthetic path probe")
	}
	return dir // Retained when the project GOTMPDIR is configured.
}

func TestRecoverySQLiteFDURICanonicalizesToPath(t *testing.T) {
	root := recoveryPathFixture(t)
	dir := filepath.Join(root, "data ?#% Unicode-\u00e9")
	seed := filepath.Join(root, "seed")
	s, err := Open(seed, "synthetic-path-probe-password")
	if err != nil {
		t.Fatal("bootstrap synthetic database")
	}
	if err := s.Close(); err != nil {
		t.Fatal("close synthetic database")
	}
	// Bootstrap uses a plain DSN, so seed at a plain path before testing URI
	// semantics. This test does not change the existing startup path handling.
	if err := os.Rename(seed, dir); err != nil {
		t.Fatal("prepare synthetic URI path")
	}
	fd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal("open synthetic directory descriptor")
	}
	defer unix.Close(fd)
	u := url.URL{Scheme: "file", Path: fmt.Sprintf("/proc/self/fd/%d/memona.db", fd)}
	u.RawQuery = url.Values{"mode": {"rw"}}.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		t.Fatal("construct synthetic fd connection")
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	var sequence int
	var name, filename string
	if err := db.QueryRow("PRAGMA database_list").Scan(&sequence, &name, &filename); err != nil {
		t.Fatal("read synthetic SQLite filename")
	}
	if name != "main" || sequence != 0 || strings.Contains(filename, "/proc/self/fd/") {
		t.Fatal("driver path semantics changed; review recovery pinning assumptions")
	}
	if filename != filepath.Join(dir, "memona.db") {
		t.Fatal("unexpected SQLite filename; review recovery pinning assumptions")
	}
	// The descriptor still pins the directory after rename, but SQLite's stored
	// filename does not track it. This invalidates the proposed fd-URI shortcut.
	moved := filepath.Join(root, "moved")
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal("rename synthetic directory")
	}
	var st unix.Stat_t
	if err := unix.Fstatat(fd, "memona.db", &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		t.Fatal("descriptor did not retain synthetic database")
	}
	if _, err := os.Lstat(filename); !os.IsNotExist(err) {
		t.Fatal("SQLite filename unexpectedly tracks descriptor rename")
	}
	if err := db.Close(); err != nil {
		t.Fatal("close synthetic fd connection")
	}
}

func TestRecoverySQLiteRWURIDoesNotCreateMissingDatabase(t *testing.T) {
	root := recoveryPathFixture(t)
	path := filepath.Join(root, "absent ?#%.db")
	u := url.URL{Scheme: "file", Path: path}
	u.RawQuery = url.Values{"mode": {"rw"}}.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		t.Fatal("construct synthetic existing-only connection")
	}
	defer db.Close()
	if err := db.Ping(); err == nil {
		t.Fatal("existing-only URI accepted missing database")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("existing-only URI created a file")
	}
}
