package access

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestWorkspaceCapacityLimits(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "private"), "long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	u, _, _, err := s.UserLoginContext(ctx, "limit-sub", "limit@example.test", "Limit")
	if err != nil {
		t.Fatal(err)
	}
	var team Workspace
	for n := 0; n < maxTeamsPerOwner; n++ {
		team, err = s.CreateTeam(ctx, u.ID, fmt.Sprintf("Team %d", n))
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateTeam(ctx, u.ID, "Over limit"); !errors.Is(err, ErrWorkspaceLimit) {
		t.Fatalf("team limit: %v", err)
	}
	for n := 0; n < maxPendingInvitesPerTeam; n++ {
		if _, err = s.InviteUser(ctx, u.ID, team.ID, fmt.Sprintf("invite%d@example.test", n)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.InviteUser(ctx, u.ID, team.ID, "over@example.test"); !errors.Is(err, ErrWorkspaceLimit) {
		t.Fatalf("invitation limit: %v", err)
	}
	for n := 0; n < maxKeysPerUser; n++ {
		if _, _, err = s.CreateUserKey(ctx, u.ID, team.ID, KeyInput{Name: fmt.Sprintf("Key %d", n), Permissions: []string{"read"}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := s.CreateUserKey(ctx, u.ID, team.ID, KeyInput{Name: "Over limit", Permissions: []string{"read"}}); !errors.Is(err, ErrWorkspaceLimit) {
		t.Fatalf("key limit: %v", err)
	}
}

func TestWorkspaceIsolationAndMembershipRevocation(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "private"), "long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Keep test independent of startup wiring while it is added in parallel.
	if err = s.initUsers(); err != nil {
		t.Fatal(err)
	}
	if err = s.InitWorkspaces(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	alice, _, _, err := s.UserLoginContext(ctx, "subject-a", "alice@example.test", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	bob, _, _, err := s.UserLoginContext(ctx, "subject-b", "bob@example.test", "Bob")
	if err != nil {
		t.Fatal(err)
	}
	personal, err := s.EnsurePersonalWorkspace(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	same, err := s.EnsurePersonalWorkspace(ctx, alice.ID)
	if err != nil || same.ID != personal.ID {
		t.Fatalf("personal not unique: %v %v", same, err)
	}
	if _, err = s.UserWorkspace(ctx, bob.ID, personal.ID); !errors.Is(err, ErrDenied) {
		t.Fatalf("personal leak: %v", err)
	}
	team, err := s.CreateTeam(ctx, alice.ID, "A Team")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.CreateUserKey(ctx, bob.ID, team.ID, KeyInput{Name: "forged", Permissions: []string{"read"}}); !errors.Is(err, ErrDenied) {
		t.Fatalf("nonmember key: %v", err)
	}
	if _, _, err = s.CreateUserKey(ctx, alice.ID, team.ID, KeyInput{Name: "forged", Permissions: []string{"read"}, Projects: []string{"*"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("forged scope: %v", err)
	}
	invite, err := s.InviteUser(ctx, alice.ID, team.ID, " BOB@EXAMPLE.TEST ")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AcceptInvitation(ctx, bob.ID, invite.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.AcceptInvitation(ctx, bob.ID, invite.ID); !errors.Is(err, ErrDenied) {
		t.Fatalf("invite reused: %v", err)
	}
	k, token, err := s.CreateUserKey(ctx, bob.ID, team.ID, KeyInput{Name: "member", Permissions: []string{"read", "write"}})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := s.AuthenticateWorkspaceKeyContext(ctx, token)
	if err != nil || !loaded.Allows("read", team.Project) || loaded.Allows("read", personal.Project) {
		t.Fatalf("scope: %+v %v", loaded, err)
	}
	if _, err = s.db.ExecContext(ctx, "DELETE FROM workspace_members WHERE workspace_id=? AND user_id=?", team.ID, bob.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateWorkspaceKeyContext(ctx, token); !errors.Is(err, ErrDenied) {
		t.Fatalf("removed member key accepted: %v", err)
	}
	if _, err = s.UserWorkspace(ctx, bob.ID, team.ID); !errors.Is(err, ErrDenied) {
		t.Fatalf("removed member workspace accepted: %v", err)
	}
	keys, err := s.UserKeys(ctx, bob.ID)
	if err != nil || len(keys) != 0 {
		t.Fatalf("removed member key listing: %+v %v", keys, err)
	}
	if err = s.RevokeUserKey(ctx, bob.ID, k.ID); !errors.Is(err, ErrDenied) {
		t.Fatalf("removed member revoke allowed: %v", err)
	}
}

func TestLegacyWildcardWorkspaceBoundary(t *testing.T) {
	k := Key{Permissions: []string{"read"}, Projects: []string{"*"}}
	if k.Allows("read", "ws-0123456789abcdef0123456789abcdef") {
		t.Fatal("legacy wildcard reads reserved workspace")
	}
	if !k.Allows("read", "ordinary") {
		t.Fatal("legacy wildcard must retain non-workspace access")
	}
}
