package server

import (
	"bytes"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"testing"

	"memona/internal/webui"
)

func TestDashboardBuiltAssets(t *testing.T) {
	f := setup(t)
	index, err := fs.ReadFile(webui.Files, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	files := []string{"/", "/index.html"}
	assetCount := 0
	err = fs.WalkDir(webui.Files, "assets", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			files = append(files, "/"+name)
			assetCount++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if assetCount == 0 || !bytes.Contains(index, []byte("/assets/")) {
		t.Fatal("dashboard must reference built assets")
	}
	const csp = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"
	for _, urlPath := range files {
		t.Run(urlPath, func(t *testing.T) {
			name := strings.TrimPrefix(urlPath, "/")
			if name == "" {
				name = "index.html"
			}
			want, err := fs.ReadFile(webui.Files, name)
			if err != nil {
				t.Fatal(err)
			}
			get := request(f.app, http.MethodGet, urlPath, "", nil, "")
			if get.Code != http.StatusOK || !bytes.Equal(get.Body.Bytes(), want) {
				t.Fatalf("GET %s: status %d or content mismatch", urlPath, get.Code)
			}
			mediaType, _, err := mime.ParseMediaType(get.Header().Get("Content-Type"))
			if err != nil {
				t.Fatal(err)
			}
			switch path.Ext(name) {
			case ".html":
				if mediaType != "text/html" {
					t.Fatalf("HTML MIME: %s", mediaType)
				}
			case ".js":
				if mediaType != "text/javascript" && mediaType != "application/javascript" {
					t.Fatalf("JavaScript MIME: %s", mediaType)
				}
			case ".css":
				if mediaType != "text/css" {
					t.Fatalf("CSS MIME: %s", mediaType)
				}
			}
			if get.Header().Get("Content-Security-Policy") != csp || get.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("static assets must retain security headers")
			}
			head := request(f.app, http.MethodHead, urlPath, "", nil, "")
			if head.Code != http.StatusOK || head.Body.Len() != 0 {
				t.Fatalf("HEAD %s: status %d, body length %d", urlPath, head.Code, head.Body.Len())
			}
			for _, header := range []string{"Content-Type", "Content-Length", "Content-Security-Policy", "X-Content-Type-Options"} {
				if head.Header().Get(header) != get.Header().Get(header) {
					t.Errorf("GET/HEAD %s mismatch", header)
				}
			}
			if head.Header().Get("Content-Length") != strconv.Itoa(len(want)) {
				t.Fatal("HEAD must describe the complete asset")
			}
		})
	}
}

func TestDashboardRejectsPrivateAndMissingPaths(t *testing.T) {
	f := setup(t)
	for _, urlPath := range []string{
		"/.env", "/config.json", "/data/notes/private.md", "/internal/webui/embed.go",
		"/dist/index.html", "/app.js", "/style.css", "/notes", "/assets",
		"/assets/", "/assets/missing.js", "/assets/config.json", "/index.html/extra",
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(method+urlPath, func(t *testing.T) {
				w := request(f.app, method, urlPath, "", nil, "")
				if w.Code != http.StatusNotFound {
					t.Fatalf("status %d, want 404", w.Code)
				}
				if method == http.MethodHead && w.Body.Len() != 0 {
					t.Fatal("HEAD 404 must not contain a body")
				}
			})
		}
	}
	for _, name := range []string{"app.js", "style.css", "embed.go", "dist/index.html"} {
		if _, err := fs.Stat(webui.Files, name); err == nil {
			t.Errorf("embedded filesystem exposes non-build file %s", name)
		}
	}
	w := request(f.app, http.MethodPost, "/", "", nil, "")
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET, HEAD" {
		t.Fatal("dashboard must reject mutation methods")
	}
}

func TestDashboardDoesNotFallbackForAPI(t *testing.T) {
	f := setup(t)
	w := request(f.app, http.MethodGet, "/api/not-a-route", "", nil, "")
	if w.Code != http.StatusUnauthorized || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatal("API authentication boundary changed")
	}
	cookie, _ := login(t, f)
	for _, urlPath := range []string{"/api/not-a-route", "/api/assets/missing.js", "/api/index.html"} {
		w := request(f.app, http.MethodGet, urlPath, "", cookie, "")
		if w.Code != http.StatusNotFound || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("%s: expected API 404, got %d %s", urlPath, w.Code, w.Header().Get("Content-Type"))
		}
	}
}
