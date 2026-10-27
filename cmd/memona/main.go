package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"memona/internal/access"
	"memona/internal/memory"
	"memona/internal/server"
)

func main() {
	if err := run(); err != nil {
		if errors.Is(err, errInspectionFindings) {
			os.Exit(2)
		}
		slog.Error("memona stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) >= 2 && os.Args[1] == "inspect" {
		return runInspect(os.Args[2:])
	}
	if len(os.Args) < 2 || os.Args[1] != "serve" {
		return errors.New("usage: memona serve -config config.json | memona inspect -data-dir existing-directory")
	}
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	configFile := flags.String("config", "config.json", "JSON config path")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	f, err := os.Open(*configFile)
	if err != nil {
		return err
	}
	defer f.Close()
	cfg := server.Config{Listen: "127.0.0.1:8765", DataDir: "./data", PublicURL: "http://127.0.0.1:8765"}
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if err = d.Decode(&cfg); err != nil {
		return err
	}
	if err = checkGoogleConfigFile(*configFile, f, cfg); err != nil {
		return err
	}
	if err = server.ValidateConfig(cfg); err != nil {
		return err
	}
	if cfg.DataDir == "" {
		return errors.New("data_dir cannot be empty")
	}
	cfg.DataDir, err = filepath.Abs(cfg.DataDir)
	if err != nil {
		return err
	}
	// The store owns the lifetime writer lock before SQLite is opened.
	notes, err := memory.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer notes.Close()
	auth, err := access.Open(cfg.DataDir, os.Getenv("MEMONA_ADMIN_PASSWORD"))
	if err != nil {
		return err
	}
	defer auth.Close()
	os.Unsetenv("MEMONA_ADMIN_PASSWORD")
	app, err := server.New(cfg, auth, notes)
	if err != nil {
		return err
	}
	srv := &http.Server{Addr: cfg.Listen, Handler: app, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- srv.Serve(listener) }()
	fmt.Printf("Memona listening on %s (public origin: %s)\n", cfg.Listen, cfg.PublicURL)
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	}
}

// A config with an OAuth client secret must be a private file owned by the
// running user. Compare the opened descriptor with the path to reject symlink
// indirection or a path replaced while the configuration was being read.
func checkGoogleConfigFile(path string, f *os.File, cfg server.Config) error {
	if cfg.GoogleClientID == "" && cfg.GoogleClientSecret == "" {
		return nil
	}
	opened, err := f.Stat()
	if err != nil {
		return err
	}
	entry, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := opened.Sys().(*syscall.Stat_t)
	if !opened.Mode().IsRegular() || !entry.Mode().IsRegular() || !os.SameFile(opened, entry) || opened.Mode().Perm()&0077 != 0 || !ok || stat.Uid != uint32(os.Geteuid()) {
		return errors.New("config with Google credentials must be an owned regular file with permission 0600; do not use a symlink")
	}
	return nil
}
