package main

import (
	"fmt"
	"os"
	"path/filepath"

	"memona/internal/memory"
)

func main() {
	if len(os.Args) == 3 {
		importDemo500()
		return
	}
	if len(os.Args) != 2 {
		panic("usage: go run ./scripts/demo500.go <new-directory>")
	}
	root, err := filepath.Abs(os.Args[1])
	if err != nil {
		panic(err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		panic("destination must not exist")
	}
	store, err := memory.Open(root)
	if err != nil {
		panic(err)
	}
	defer store.Close()

	testing := true
	previous := ""
	for i := 1; i <= 500; i++ {
		input := memory.Input{
			Project:   "demo-500",
			Kind:      "knowledge",
			Title:     fmt.Sprintf("Contoh alur %03d", i),
			Content:   fmt.Sprintf("Data sintetis untuk menguji jaringan 500 memori. Simpul %03d berada dalam rangkaian proses demo-500.", i),
			Status:    "unverified",
			Source:    "contoh sintetis",
			IsTesting: &testing,
		}
		if previous != "" {
			input.RelatedTo = []string{previous}
		}
		note, err := store.Create(input, "demo-generator")
		if err != nil {
			panic(fmt.Errorf("catatan %d: %w", i, err))
		}
		previous = note.ID
	}
	items, err := store.List()
	if err != nil {
		panic(err)
	}
	edges := 0
	for _, item := range items {
		if item.Project != "demo-500" || !item.IsTesting || item.Status != "unverified" {
			panic("unexpected note in demo store")
		}
		edges += len(item.RelatedTo)
	}
	if len(items) != 500 || edges != 499 {
		panic(fmt.Sprintf("verification failed: %d notes, %d links", len(items), edges))
	}
	fmt.Printf("%s: %d synthetic notes, %d related_to links\n", root, len(items), edges)
}
