package memory

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// CodeReference is a citation, not an authorization to open or index a file.
// Commit may be omitted when a stable revision is not known; readers must
// confirm the current source before relying on such a reference.
type CodeReference struct {
	Repository string `json:"repository"`
	Path       string `json:"path"`
	Commit     string `json:"commit,omitempty"`
	Symbol     string `json:"symbol,omitempty"`
	Line       int    `json:"line,omitempty"`
}

var codeCommitPattern = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)

func validateCodeReferences(refs []CodeReference) error {
	if len(refs) > 10 {
		return fmt.Errorf("%w: at most 10 code references", ErrInvalid)
	}
	seen := make(map[CodeReference]bool, len(refs))
	for _, ref := range refs {
		if !slugPattern.MatchString(ref.Repository) || !validText(ref.Path, 300, true) ||
			strings.Contains(ref.Path, "\\") || strings.HasPrefix(ref.Path, "/") || path.Clean(ref.Path) != ref.Path ||
			ref.Path == "." || strings.HasPrefix(ref.Path, "../") || strings.Contains(ref.Path, "/../") ||
			strings.HasPrefix(ref.Path, "./") || strings.Contains(ref.Path, "/./") ||
			(ref.Commit != "" && !codeCommitPattern.MatchString(ref.Commit)) ||
			!validText(ref.Symbol, 160, false) || ref.Line < 0 || ref.Line > 1000000 || seen[ref] {
			return fmt.Errorf("%w: invalid or duplicate code reference", ErrInvalid)
		}
		for _, segment := range strings.Split(ref.Path, "/") {
			if segment == "" || segment == "." || segment == ".." {
				return fmt.Errorf("%w: invalid code reference path", ErrInvalid)
			}
		}
		seen[ref] = true
	}
	return nil
}
