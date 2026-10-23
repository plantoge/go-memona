package server

import (
	"context"
	"fmt"
	"memona/internal/memory"
	"strings"
	"testing"
)

func BenchmarkRetrievalSnapshot(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprintf("notes%d", count), func(b *testing.B) {
			notes := make([]memory.Note, count)
			for i := range notes {
				notes[i] = memory.Note{ID: fmt.Sprintf("%032x", i), Project: "demo", Title: fmt.Sprintf("Alur pasien %d", i), Content: strings.Repeat("loket pemeriksaan farmasi kasir ", 32), Status: "unverified", Kind: "knowledge", Revision: 1}
			}
			b.Run("rank", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					out, err := rankedNotes(context.Background(), notes, "alur pasien")
					if err != nil || len(out) != count {
						b.Fatal(err, len(out))
					}
				}
			})
			b.Run("duplicate", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					out, err := duplicateCandidates(context.Background(), notes, "demo", "Alur pasien", notes[0].Content)
					if err != nil || len(out) != 10 {
						b.Fatal(err, len(out))
					}
				}
			})
		})
	}
}
