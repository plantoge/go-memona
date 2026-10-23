// Package webui contains the self-contained administration dashboard.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed dist
var embedded embed.FS

// Files holds only the built dashboard assets, rooted at the web root.
// The legacy source files alongside embed.go are not exposed.
var Files fs.FS = func() fs.FS {
	files, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic(err)
	}
	return files
}()
