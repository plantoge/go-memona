package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type readBenchmarkProfile struct {
	notes, revisions, contentBytes int
}

func (p readBenchmarkProfile) name() string {
	return fmt.Sprintf("notes%d_revisions%d_content%dKiB", p.notes, p.revisions, p.contentBytes/1024)
}

func largerReadBenchmarkProfiles() []readBenchmarkProfile {
	return []readBenchmarkProfile{
		{1000, 1, 1024},
		{1000, 8, 1024},
		{128, 4, 64 * 1024},
	}
}

// Build retained, synthetic canonical histories directly, outside measurement.
// This measures scan cost, not mutation/fsync throughput or maximum capacity.
// Only benchmarks call this helper; ordinary go test creates no large fixtures.
func benchmarkReadStore(b *testing.B, p readBenchmarkProfile) *Store {
	b.Helper()
	// Bound each fixture well below the request guardrails, including metadata.
	// Repeated explicit benchmark invocations still accumulate retained fixtures.
	const maxEntries = 10000
	const maxBytes = 96 << 20
	if p.notes <= 0 || p.revisions <= 0 || p.revisions >= maxEntries || p.contentBytes <= 0 || p.contentBytes > 64*1024 || p.notes > maxEntries/(p.revisions+1) {
		b.Fatal("benchmark profile exceeds fixture entry/content bounds")
	}
	stamp := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	n := Note{ID: fmt.Sprintf("%032x", 1), Project: "benchmark", Kind: "fact", Title: "Synthetic scan", Content: strings.Repeat("x", p.contentBytes), Status: "unverified", Author: "benchmark", CreatedAt: stamp, UpdatedAt: stamp, Revision: p.revisions}
	// The largest revision number gives an upper bound for encoded file size.
	if int64(p.notes)*int64(p.revisions)*int64(len(encode(n))) > maxBytes || maxEntries >= ReadMaxEntries || maxBytes >= ReadMaxBytes {
		b.Fatal("benchmark profile exceeds fixture byte bounds or request guardrails")
	}
	parent := os.Getenv("GOTMPDIR")
	if parent == "" {
		b.Fatal("set GOTMPDIR to the project cache: benchmark fixtures are retained")
	}
	root, err := os.MkdirTemp(parent, "read-bench-")
	if err != nil {
		b.Fatal(err)
	}
	s, err := Open(filepath.Join(root, "data"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := s.Close(); err != nil {
			b.Error(err)
		}
	})
	var fixtureBytes int64
	for i := 0; i < p.notes; i++ {
		n.ID = fmt.Sprintf("%032x", i+1)
		dir := filepath.Join(root, "data", "notes", n.ID)
		if err := os.Mkdir(dir, 0700); err != nil {
			b.Fatal(err)
		}
		for j := 1; j <= p.revisions; j++ {
			n.Revision = j
			data := encode(n)
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%06d.md", j)), data, 0600); err != nil {
				b.Fatal(err)
			}
			fixtureBytes += int64(len(data))
		}
	}
	b.Logf("retained fixture=%s entries=%d revision-files=%d canonical-bytes=%d", root, p.notes*(p.revisions+1), p.notes*p.revisions, fixtureBytes)
	return s
}

func BenchmarkListContext(b *testing.B) {
	profiles := append([]readBenchmarkProfile{{32, 1, 1024}, {64, 8, 1024}}, largerReadBenchmarkProfiles()...)
	for _, p := range profiles {
		b.Run(p.name(), func(b *testing.B) {
			s := benchmarkReadStore(b, p)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				out, err := s.ListContext(context.Background())
				if err != nil || len(out) != p.notes {
					b.Fatalf("scan: notes=%d want=%d err=%v", len(out), p.notes, err)
				}
			}
		})
	}
}

// One operation is one batch of exactly eight ListContext calls on ONE store.
// A barrier releases all readers together; the store mutex serializes scans.
// ns/op and allocations/op include goroutine/channel overhead for the batch,
// not per-reader latency. Every reader must return a complete snapshot.
func BenchmarkListContextConcurrent8(b *testing.B) {
	const readers = 8
	for _, p := range largerReadBenchmarkProfiles() {
		b.Run(p.name(), func(b *testing.B) {
			s := benchmarkReadStore(b, p)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				start := make(chan struct{})
				results := make(chan error, readers)
				for range readers {
					go func() {
						<-start
						out, err := s.ListContext(context.Background())
						if err == nil && len(out) != p.notes {
							err = fmt.Errorf("scan: notes=%d want=%d", len(out), p.notes)
						}
						results <- err
					}()
				}
				close(start)
				var firstErr error
				for range readers {
					if err := <-results; err != nil && firstErr == nil {
						firstErr = err
					}
				}
				// Drain all readers before failing or closing the shared store.
				if firstErr != nil {
					b.Fatal(firstErr)
				}
			}
		})
	}
}
