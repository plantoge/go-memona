package access

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestKeysAndSessions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	s, e := Open(dir, "a-test-password-at-least-16")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if _, _, e = s.Login("wrong"); !errors.Is(e, ErrDenied) {
		t.Fatal("bad password accepted")
	}
	session, csrf, e := s.Login("a-test-password-at-least-16")
	if e != nil || csrf == "" {
		t.Fatal(e)
	}
	if got, e := s.Session(session); e != nil || got != csrf {
		t.Fatal(e)
	}
	if e = s.Logout(session); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Session(session); e == nil {
		t.Fatal("logged out session accepted")
	}
	k, token, e := s.CreateKey(KeyInput{Name: "opencode", Permissions: []string{"read", "write"}, Projects: []string{"simrs"}})
	if e != nil {
		t.Fatal(e)
	}
	loaded, e := s.Authenticate(token)
	if e != nil || loaded.ID != k.ID {
		t.Fatal(e)
	}
	if !loaded.Allows("read", "simrs") || loaded.Allows("read", "other") || loaded.Allows("archive", "simrs") {
		t.Fatal("scope enforcement failed")
	}
	var hash string
	if e = s.db.QueryRow("SELECT hash FROM api_keys WHERE id=?", k.ID).Scan(&hash); e != nil {
		t.Fatal(e)
	}
	if hash == token || strings.Contains(hash, token) {
		t.Fatal("plaintext key persisted")
	}
	if e = s.Revoke(k.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Authenticate(token); e == nil {
		t.Fatal("revoked token accepted")
	}
	keys, e := s.Keys()
	if e != nil || len(keys) != 1 || !keys[0].Revoked {
		t.Fatal(e)
	}
	if _, _, e = s.CreateKey(KeyInput{Name: "x", Permissions: []string{"delete"}, Projects: []string{"*"}}); e == nil {
		t.Fatal("unsupported permission accepted")
	}
	if _, _, e = s.CreateKey(KeyInput{Name: "x", Permissions: []string{"read"}, Projects: []string{"../escape"}}); e == nil {
		t.Fatal("bad scope accepted")
	}
	if _, _, e = s.CreateKey(KeyInput{Name: "x", Permissions: []string{"read"}, Projects: []string{"ws-legacy"}}); !errors.Is(e, ErrInvalid) {
		t.Fatal("admin key accepted reserved workspace scope")
	}
	if _, _, e = s.CreateKey(KeyInput{Name: "x", Permissions: []string{"read"}, Projects: []string{"*"}, ExpiresAt: time.Now().Add(-time.Hour).Format(time.RFC3339)}); e == nil {
		t.Fatal("past expiry accepted")
	}
}
func TestAdminBootstrapNotOverwritten(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	s, e := Open(dir, "first-password-for-testing")
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(dir, "different-password-for-testing")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if _, _, e = s.Login("different-password-for-testing"); e == nil {
		t.Fatal("existing password overwritten")
	}
	if _, _, e = s.Login("first-password-for-testing"); e != nil {
		t.Fatal(e)
	}
}
func TestBootstrapRequired(t *testing.T) {
	s, e := Open(t.TempDir(), "")
	if e == nil {
		s.Close()
		t.Fatal("bootstrap allowed without password")
	}
}
func TestBootstrapPasswordLength(t *testing.T) {
	for _, tc := range []struct {
		name, password string
		wantValid      bool
	}{
		{"nine bytes rejected", "123456789", false},
		{"ten bytes accepted", "1234567890", true},
		{"max bytes accepted", strings.Repeat("x", 1024), true},
		{"over max rejected", strings.Repeat("x", 1025), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Open(filepath.Join(t.TempDir(), "private"), tc.password)
			if !tc.wantValid {
				if err == nil {
					s.Close()
					t.Fatal("invalid bootstrap password accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if _, _, err := s.Login(tc.password); err != nil {
				t.Fatalf("valid bootstrap password cannot log in: %v", err)
			}
		})
	}
}
func TestRejectDBSymlink(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if e := os.Mkdir(dir, 0700); e != nil {
		t.Fatal(e)
	}
	target := filepath.Join(t.TempDir(), "secret")
	if e := os.WriteFile(target, []byte("private"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(target, filepath.Join(dir, "memona.db")); e != nil {
		t.Fatal(e)
	}
	if s, e := Open(dir, "test-password-123456"); e == nil {
		s.Close()
		t.Fatal("database symlink followed")
	}
}
