package main

import (
	"fmt"
	"os"
	"path/filepath"

	"memona/internal/memory"
)

func importDemo500() {
	if len(os.Args) != 3 {
		panic("usage: go run ./scripts <demo-directory> <active-data-directory>")
	}
	sourcePath, err := filepath.Abs(os.Args[1])
	if err != nil {
		panic(err)
	}
	targetPath, err := filepath.Abs(os.Args[2])
	if err != nil || sourcePath == targetPath {
		panic("invalid source or destination")
	}
	for _, path := range []string{sourcePath, targetPath} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() {
			panic("source and destination must be existing directories")
		}
	}
	source, err := memory.Open(sourcePath)
	if err != nil {
		panic(err)
	}
	defer source.Close()
	original, err := source.List()
	if err != nil {
		panic(err)
	}
	byID := make(map[string]memory.Note, len(original))
	root := ""
	for _, note := range original {
		if note.Project != "demo-500" || !note.IsTesting || note.Archived || note.Kind != "knowledge" || note.Status != "unverified" || note.Revision != 1 || len(note.Supersedes) != 0 || len(note.RelatedTo) > 1 {
			panic("demo store contains unexpected notes")
		}
		byID[note.ID] = note
		if len(note.RelatedTo) == 0 {
			if root != "" {
				panic("demo store has multiple roots")
			}
			root = note.ID
		}
	}
	if len(original) != 500 || root == "" {
		panic("demo store must contain exactly 500 connected notes")
	}
	children := make(map[string]string, 499)
	for _, note := range original {
		if len(note.RelatedTo) == 1 {
			parent := note.RelatedTo[0]
			if _, ok := byID[parent]; !ok || children[parent] != "" {
				panic("invalid demo chain")
			}
			children[parent] = note.ID
		}
	}
	order := make([]memory.Note, 0, 500)
	for id := root; id != ""; id = children[id] {
		if len(order) == 500 {
			panic("cycle in demo chain")
		}
		order = append(order, byID[id])
	}
	if len(order) != 500 {
		panic("demo chain is disconnected")
	}
	target, err := memory.Open(targetPath)
	if err != nil {
		panic(err)
	}
	defer target.Close()
	before, err := target.List()
	if err != nil {
		panic(err)
	}
	for _, note := range before {
		if note.Project == "demo-500" {
			panic("destination already has demo-500 notes; refusing duplicate import")
		}
	}
	mapped := make(map[string]string, 500)
	for i, note := range order {
		testing := true
		input := memory.Input{Project: note.Project, Kind: note.Kind, Title: note.Title, Content: note.Content, Status: note.Status, Source: note.Source, IsTesting: &testing}
		if len(note.RelatedTo) == 1 {
			input.RelatedTo = []string{mapped[note.RelatedTo[0]]}
		}
		created, err := target.Create(input, "demo-import")
		if err != nil {
			panic(fmt.Errorf("import stopped after %d notes: %w", i, err))
		}
		mapped[note.ID] = created.ID
	}
	after, err := target.List()
	if err != nil {
		panic(err)
	}
	count, edges := 0, 0
	for _, note := range after {
		if note.Project == "demo-500" && note.IsTesting {
			count++
			edges += len(note.RelatedTo)
		}
	}
	if len(after) != len(before)+500 || count != 500 || edges != 499 {
		panic(fmt.Sprintf("verification failed: total=%d demo=%d links=%d", len(after), count, edges))
	}
	fmt.Printf("Imported %d synthetic notes and %d links into active data\n", count, edges)
}
