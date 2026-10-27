package main

import (
	"os"
	"path/filepath"
	"testing"

	"memona/internal/server"
)

func TestGoogleConfigRequiresPrivateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"google_client_id":"example","google_client_secret":"synthetic"}`), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg := server.Config{GoogleClientID: "example", GoogleClientSecret: "synthetic"}
	if err := checkGoogleConfigFile(path, f, cfg); err != nil {
		t.Fatalf("private config refused: %v", err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := checkGoogleConfigFile(path, f, cfg); err == nil {
		t.Fatal("world-readable config accepted")
	}
	if err := checkGoogleConfigFile(path, f, server.Config{}); err != nil {
		t.Fatalf("legacy config needs no new permission: %v", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "link.json")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if err := checkGoogleConfigFile(alias, f, cfg); err == nil {
		t.Fatal("symlinked config accepted")
	}
}
