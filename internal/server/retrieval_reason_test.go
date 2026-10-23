package server

import (
	"context"
	"memona/internal/memory"
	"strings"
	"testing"
)

func TestRetrievalReasonAuthorizedFieldExplanation(t *testing.T) {
	n := memory.Note{Title: "expected_revision", Content: "private synthetic body", Source: "synthetic evidence"}
	reason, err := retrievalReason(t.Context(), n, "expected_rev")
	if err != nil || !strings.Contains(reason, "title") || strings.Contains(reason, n.Content) || strings.Contains(reason, n.Source) {
		t.Fatal(reason, err)
	}
	n.IsProjectSummary = true
	reason, err = retrievalReason(t.Context(), n, "absent")
	if err != nil || !strings.Contains(reason, "summary") {
		t.Fatal(reason, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := retrievalReason(ctx, n, ""); err == nil {
		t.Fatal("cancellation ignored")
	}
}
