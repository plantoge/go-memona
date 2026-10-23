package server

import (
	"context"
	"memona/internal/memory"
	"strings"
)

// Selection explanations expose only field names already authorized by caller,
// never additional text or a semantic confidence claim.
type selectionReason struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

func retrievalReason(ctx context.Context, n memory.Note, query string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if n.IsProjectSummary {
		return "Explicit project summary; prioritized as overview even when task text does not match.", nil
	}
	if strings.TrimSpace(query) == "" {
		if n.Kind == "handoff" {
			return "Recent handoff within the category quota; no task query supplied.", nil
		}
		return "Durable note before handoffs, then recency; no task query supplied.", nil
	}
	q, err := lexicalize(ctx, query)
	if err != nil {
		return "", err
	}
	fields := []string{}
	for i, text := range []string{n.Title, n.Content, n.Source} {
		field, err := lexicalize(ctx, text)
		if err != nil {
			return "", err
		}
		match := strings.Contains(strings.ToLower(text), strings.ToLower(query))
		for term := range q.terms {
			if _, ok := field.terms[term]; ok {
				match = true
				break
			}
		}
		if match {
			fields = append(fields, []string{"title", "content", "source"}[i])
		}
	}
	if len(fields) == 0 {
		return "Selected by retrieval policy; inspect source and current state.", nil
	}
	return "Lexical/substring match in " + strings.Join(fields, ", ") + "; ranking weights title 6, content 2, source 1 (not semantic confidence).", nil
}
