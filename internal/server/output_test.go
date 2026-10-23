package server

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJSONResponseBoundBeforeStatus(t *testing.T) {
	data, err := encodeBoundedJSON(map[string]string{"text": "<script>\n\"\\"})
	if err != nil || !json.Valid(data) {
		t.Fatal(err)
	}
	huge := strings.Repeat("x", MaxJSONResponseBytes)
	if out, err := encodeBoundedJSON(huge); !errors.Is(err, errResponseLimit) || out != nil {
		t.Fatal("limit bypass", err)
	}
	w := httptest.NewRecorder()
	respond(w, 200, huge)
	if w.Code != 503 || !json.Valid(w.Body.Bytes()) || w.Body.Len() > 1024 {
		t.Fatal("partial output", w.Code)
	}
	if strings.Contains(w.Body.String(), strings.Repeat("x", 100)) {
		t.Fatal("oversize data leaked")
	}
	var buffer cappedJSONBuffer
	if n, err := buffer.Write([]byte(strings.Repeat("x", MaxJSONResponseBytes))); err != nil || n != MaxJSONResponseBytes {
		t.Fatal(err)
	}
	if n, err := buffer.Write([]byte("x")); n != 0 || !errors.Is(err, errResponseLimit) {
		t.Fatal("exact limit failed", err)
	}
}
