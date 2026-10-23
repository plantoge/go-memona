package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"memona/internal/memory"
)

// MaxJSONResponseBytes limits buffered API JSON before any status is committed.
// This is not a storage quota; large histories require pagination in a later UI.
const MaxJSONResponseBytes = 8 << 20

var errResponseLimit = errors.New("response resource limit exceeded")

type cappedJSONBuffer struct{ bytes.Buffer }

func (b *cappedJSONBuffer) Write(p []byte) (int, error) {
	if len(p) > MaxJSONResponseBytes-b.Len() {
		return 0, errResponseLimit
	}
	return b.Buffer.Write(p)
}
func encodeBoundedJSON(v any) ([]byte, error) {
	var b cappedJSONBuffer
	if err := json.NewEncoder(&b).Encode(v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Safe public errors contain neither filesystem paths nor data volume counts.
func publicReadError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	case errors.Is(err, memory.ErrReadLimit):
		return memory.ErrReadLimit
	default:
		return errors.New("memory unavailable")
	}
}

func readFailure(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, context.Canceled):
		fail(w, http.StatusRequestTimeout, "Permintaan dibatalkan.")
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, memory.ErrReadLimit):
		w.Header().Set("Retry-After", "5")
		fail(w, http.StatusServiceUnavailable, "Pembacaan mencapai batas resource atau waktu; periksa kapasitas penyimpanan.")
	default:
		return false
	}
	return true
}
