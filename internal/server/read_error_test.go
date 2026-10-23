package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"memona/internal/access"
	"memona/internal/memory"
)

func TestReadFailureSafeStatusAndJSON(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
	}{{context.Canceled, 408}, {context.DeadlineExceeded, 503}, {memory.ErrReadLimit, 503}} {
		w := httptest.NewRecorder()
		if !readFailure(w, tc.err) || w.Code != tc.code || !json.Valid(w.Body.Bytes()) {
			t.Fatal("invalid read failure response", w.Code)
		}
		if strings.Contains(w.Body.String(), "/home/") {
			t.Fatal("path leaked")
		}
		if tc.code == 503 && w.Header().Get("Retry-After") != "5" {
			t.Fatal("missing retry hint")
		}
		if !errors.Is(publicReadError(tc.err), tc.err) {
			t.Fatal("lost public error identity")
		}
	}
	w := httptest.NewRecorder()
	if readFailure(w, nil) || w.Body.Len() != 0 {
		t.Fatal("nil handled as failure")
	}
	private := errors.New("synthetic /private/data/path")
	if strings.Contains(publicReadError(private).Error(), "/private/") {
		t.Fatal("raw error leaked")
	}
}

func TestReadDeniedBeforeSnapshot(t *testing.T) {
	// A nil store is intentional: denied keys must never start any disk scan.
	a := &App{}
	for _, permission := range []string{"write", "archive"} {
		k := access.Key{Permissions: []string{permission}, Projects: []string{"*"}}
		if _, err := a.readableSnapshotContext(t.Context(), k); !errors.Is(err, access.ErrDenied) {
			t.Fatal("read denied key reached snapshot", err)
		}
		if _, err := a.readViewContext(t.Context(), k, "synthetic"); !errors.Is(err, access.ErrDenied) {
			t.Fatal("read denied key reached read view", err)
		}
	}
}

func TestAdminReadAPIsUseCanceledContext(t *testing.T) {
	f := setup(t)
	for _, path := range []string{"/api/notes", "/api/projects", "/api/keys", "/api/activity"} {
		t.Run(path, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			r := httptest.NewRequest("GET", "http://localhost"+path, nil).WithContext(ctx)
			w := httptest.NewRecorder()
			// Exercise the endpoint after auth, independently of unchanged
			// session SQL. The normal authenticated routes have regression tests.
			f.app.api(w, r)
			if w.Code != 408 || !json.Valid(w.Body.Bytes()) {
				t.Fatal("API ignored canceled read context", w.Code)
			}
		})
	}
}
