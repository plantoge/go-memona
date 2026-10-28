package server

import (
	"bytes"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"memona/internal/webui"
)

// dashboardAsset serves the user home, explicit admin entry point, and bundled assets.
// It deliberately has no SPA fallback or access to the on-disk filesystem.
func dashboardAsset(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	} else if name == "admin" || name == "admin/" {
		name = "admin.html"
	}
	if !fs.ValidPath(name) || (name != "index.html" && name != "admin.html" && !strings.HasPrefix(name, "assets/")) {
		assetNotFound(w, r)
		return
	}
	content, err := fs.ReadFile(webui.Files, name)
	if err != nil {
		assetNotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = http.DetectContentType(content)
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(content))
}

func assetNotFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	if r.Method != http.MethodHead {
		w.Write([]byte("404 page not found\n"))
	}
}
