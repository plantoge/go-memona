package memory

import (
	"fmt"
	"strings"
	"time"
)

// Review metadata is self-declared by the caller, not a server verification or
// an inference of conflict/replacement. Validation is independent of wall time
// so immutable history remains valid when the clock or environment changes.
func validateReviewMetadata(in Input) error {
	if in.Kind == "skill" && (in.VerifiedAt != nil || in.NeedsReview != nil) {
		return fmt.Errorf("%w: verified_at and needs_review are for non-skill notes only, including explicit clears", ErrInvalid)
	}
	if in.VerifiedAt == nil || *in.VerifiedAt == "" {
		return nil
	}
	if in.Status != "verified" || strings.TrimSpace(in.Source) == "" {
		return fmt.Errorf("%w: verified_at requires verified status and evidence in source", ErrInvalid)
	}
	value := *in.VerifiedAt
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.UTC().Format(time.RFC3339Nano) != value {
		return fmt.Errorf("%w: verified_at must be canonical UTC RFC3339Nano with Z", ErrInvalid)
	}
	return nil
}

func applyReviewMetadata(n *Note, in Input) {
	if in.VerifiedAt != nil {
		n.VerifiedAt = *in.VerifiedAt
	} else if n.Status != "verified" {
		// Leaving verified status clears an inherited timestamp; returning to
		// verified later does not resurrect it. Explicit nonempty input is
		// rejected by validation rather than silently discarded.
		n.VerifiedAt = ""
	}
	if in.NeedsReview != nil {
		n.NeedsReview = *in.NeedsReview
	}
}
