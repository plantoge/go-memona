package access

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestUserIdentityAndSeparateSession(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "private"), "strong-admin-password")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	u, token, csrf, err := s.UserLoginContext(ctx, "google-sub-1", "first@example.org", "First")
	if err != nil {
		t.Fatal(err)
	}
	if u.GoogleSub != "google-sub-1" || csrf == "" || token == "" {
		t.Fatalf("invalid user login: %+v", u)
	}
	if _, err = s.SessionContext(ctx, token); !errors.Is(err, ErrDenied) {
		t.Fatalf("user token accepted as admin: %v", err)
	}
	same, token2, _, err := s.UserLoginContext(ctx, "google-sub-1", "changed@example.org", "Changed")
	if err != nil || same.ID != u.ID {
		t.Fatalf("subject identity changed: %+v %v", same, err)
	}
	other, _, _, err := s.UserLoginContext(ctx, "google-sub-2", "changed@example.org", "Other")
	if err != nil || other.ID == u.ID {
		t.Fatalf("email improperly linked: %+v %v", other, err)
	}
	current, _, err := s.UserSessionContext(ctx, token)
	if err != nil || current.Email != "changed@example.org" {
		t.Fatalf("session not current: %+v %v", current, err)
	}
	if err = s.UserLogoutContext(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.UserSessionContext(ctx, token); !errors.Is(err, ErrDenied) {
		t.Fatalf("logout not applied: %v", err)
	}
	if _, _, err = s.UserSessionContext(ctx, token2); err != nil {
		t.Fatal(err)
	}
}
