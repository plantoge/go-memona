package server

import (
	"context"
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"memona/internal/memory"
)

const duplicateCandidateLimit = 10

// duplicateCandidate is advisory metadata only, not permission to save or merge.
// Score is lexical similarity in [0,1], not a probability of semantic identity.
type duplicateCandidate struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Project  string  `json:"project"`
	Revision int     `json:"revision"`
	Score    float64 `json:"score"`
	Reason   string  `json:"reason"`
}

type lexicalText struct {
	terms  map[string]struct{}
	phrase string
}

// lexicalize deliberately has no language-specific stemming, stop words, query
// operators, or embeddings. Unicode letters/marks/digits and underscores retain
// Indonesian text and identifiers such as get_context and expected_revision.
// Other punctuation separates words; repeated tokens cannot inflate a score.
func lexicalize(ctx context.Context, text string) (lexicalText, error) {
	if err := ctx.Err(); err != nil {
		return lexicalText{}, err
	}
	words := []string{}
	start := -1
	checkpoint := 0
	for i, r := range text {
		if i >= checkpoint {
			if err := ctx.Err(); err != nil {
				return lexicalText{}, err
			}
			checkpoint = i + 1024
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '_' {
			if start < 0 {
				start = i
			}
		} else if start >= 0 {
			words = append(words, strings.ToLower(text[start:i]))
			start = -1
		}
	}
	if start >= 0 {
		words = append(words, strings.ToLower(text[start:]))
	}
	out := lexicalText{terms: make(map[string]struct{}, len(words)), phrase: strings.Join(words, " ")}
	for i, word := range words {
		if i%256 == 0 {
			if err := ctx.Err(); err != nil {
				return lexicalText{}, err
			}
		}
		out.terms[word] = struct{}{}
	}
	return out, ctx.Err()
}

func validRetrievalText(text string, max int, multiline, required bool) bool {
	if len(text) > max || !utf8.ValidString(text) || (required && strings.TrimSpace(text) == "") {
		return false
	}
	return !strings.ContainsFunc(text, func(r rune) bool {
		return unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\r' || r == '\t'))
	})
}

func validRetrievalProject(project string) bool {
	if len(project) == 0 || len(project) > 64 {
		return false
	}
	for i, r := range project {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || i > 0 && r == '-') {
			return false
		}
	}
	return true
}

// rankedNotes uses ONLY the supplied authorized, current-revision snapshot.
// The caller must gate access and exclude explicitly superseded notes when
// building context; this function never infers supersession from recency.
// Nonempty queries retain notes matching any lexical query term. Text relevance
// comes first (title > content > source), then project overview, durable knowledge
// before handoffs, recency, and project/ID. Empty queries apply only that latter
// ordering. Archived notes are always excluded. Input and nested slices are never
// modified. Limits are bytes, matching memory.Input, not rune counts.
func rankedNotes(ctx context.Context, accessible []memory.Note, query string) ([]memory.Note, error) {
	if ctx == nil {
		return nil, memory.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validRetrievalText(query, 2000, true, false) {
		return nil, memory.ErrInvalid
	}
	q, err := lexicalize(ctx, query)
	if err != nil {
		return nil, err
	}
	type rankedNote struct {
		note  memory.Note
		score int
	}
	ranked := []rankedNote{}
	for _, n := range accessible {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if n.Archived {
			continue
		}
		score := 0
		if strings.TrimSpace(query) != "" {
			if len(q.terms) == 0 {
				continue
			}
			for i, text := range []string{n.Title, n.Content, n.Source} {
				field, err := lexicalize(ctx, text)
				if err != nil {
					return nil, err
				}
				weight := []int{6, 2, 1}[i]
				for term := range q.terms {
					if _, ok := field.terms[term]; ok {
						score += weight
					}
				}
				// Boundary-aware phrase bonus, not a substring of an identifier.
				if strings.Contains(" "+field.phrase+" ", " "+q.phrase+" ") {
					score += weight
				}
			}
			if score == 0 {
				continue
			}
		}
		ranked = append(ranked, rankedNote{n, score})
	}
	sort.Slice(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.note.IsProjectSummary != b.note.IsProjectSummary {
			return a.note.IsProjectSummary
		}
		if (a.note.Kind == "handoff") != (b.note.Kind == "handoff") {
			return a.note.Kind != "handoff"
		}
		if a.note.UpdatedAt != b.note.UpdatedAt {
			return a.note.UpdatedAt > b.note.UpdatedAt
		}
		if a.note.Project != b.note.Project {
			return a.note.Project < b.note.Project
		}
		if a.note.ID != b.note.ID {
			return a.note.ID < b.note.ID
		}
		return a.note.Revision > b.note.Revision
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := make([]memory.Note, len(ranked))
	for i, n := range ranked {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out[i] = n.note
	}
	return out, nil
}

// lexicalDice uses set overlap: 2*intersection/(len(a)+len(b)). Empty fields
// contribute no evidence, even when both are empty. Integer overlap makes the
// result independent of Go's map iteration order.
func lexicalDice(a, b lexicalText) float64 {
	if len(a.terms) == 0 || len(b.terms) == 0 {
		return 0
	}
	intersection := 0
	for term := range a.terms {
		if _, ok := b.terms[term]; ok {
			intersection++
		}
	}
	return 2 * float64(intersection) / float64(len(a.terms)+len(b.terms))
}

// duplicateCandidates returns at most ten advisory matches from an already
// authorized snapshot, never reading storage or writing/merging notes. Project
// must be explicit: global is not an implicit fallback. Only nonarchived notes
// in that exact project are compared. Title/content limits and UTF-8/control
// validation match the write contract (content may be empty and multiline).
// Candidates need score >= .60: max(.70*titleDice+.30*contentDice,
// .90*contentDice). The second term catches copied content with a different
// title. Scores are rounded to four decimals; ties use project/ID/revision, not
// snapshot order or recency. Only a bounded top-ten metadata slice is retained.
func duplicateCandidates(ctx context.Context, accessible []memory.Note, project, title, content string) ([]duplicateCandidate, error) {
	if ctx == nil {
		return nil, memory.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validRetrievalProject(project) || !validRetrievalText(title, 200, false, true) ||
		!validRetrievalText(content, 65536, true, false) {
		return nil, memory.ErrInvalid
	}
	wantedTitle, err := lexicalize(ctx, title)
	if err != nil {
		return nil, err
	}
	wantedContent, err := lexicalize(ctx, content)
	if err != nil {
		return nil, err
	}
	out := []duplicateCandidate{}
	for _, n := range accessible {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if n.Archived || n.Project != project {
			continue
		}
		noteTitle, err := lexicalize(ctx, n.Title)
		if err != nil {
			return nil, err
		}
		noteContent, err := lexicalize(ctx, n.Content)
		if err != nil {
			return nil, err
		}
		titleScore, contentScore := lexicalDice(wantedTitle, noteTitle), lexicalDice(wantedContent, noteContent)
		score := math.Max(.7*titleScore+.3*contentScore, .9*contentScore)
		if score < .6 {
			continue
		}
		reason := "Kemiripan token judul dan/atau konten; perlu tinjauan manual."
		sameTitle := wantedTitle.phrase != "" && wantedTitle.phrase == noteTitle.phrase
		sameContent := wantedContent.phrase != "" && wantedContent.phrase == noteContent.phrase
		switch {
		case sameTitle && sameContent:
			reason = "Judul dan konten sama setelah normalisasi leksikal; perlu tinjauan manual."
		case sameTitle:
			reason = "Judul sama setelah normalisasi leksikal; perlu tinjauan manual."
		case sameContent:
			reason = "Konten sama setelah normalisasi leksikal; perlu tinjauan manual."
		}
		candidate := duplicateCandidate{n.ID, n.Title, n.Project, n.Revision, math.Round(score*10000) / 10000, reason}
		// A current-revision snapshot normally has unique IDs. Still suppress
		// duplicate rows defensively without an unbounded ID map.
		for i, old := range out {
			if old.ID == candidate.ID {
				if duplicateBefore(candidate, old) {
					out = append(out[:i], out[i+1:]...)
				} else {
					candidate.ID = ""
				}
				break
			}
		}
		if candidate.ID == "" {
			continue
		}
		i := sort.Search(len(out), func(i int) bool { return duplicateBefore(candidate, out[i]) })
		if i == duplicateCandidateLimit {
			continue
		}
		if len(out) < duplicateCandidateLimit {
			out = append(out, duplicateCandidate{})
		}
		copy(out[i+1:], out[i:len(out)-1])
		out[i] = candidate
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func duplicateBefore(a, b duplicateCandidate) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if a.Project != b.Project {
		return a.Project < b.Project
	}
	if a.ID != b.ID {
		return a.ID < b.ID
	}
	if a.Revision != b.Revision {
		return a.Revision > b.Revision
	}
	if a.Title != b.Title {
		return a.Title < b.Title
	}
	return a.Reason < b.Reason
}
