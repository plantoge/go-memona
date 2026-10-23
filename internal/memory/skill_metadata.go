package memory

import "fmt"

// SkillMetadata is descriptive, untrusted text. It neither grants publication
// approval nor loads packages, checks compatibility, or executes instructions.
type SkillMetadata struct {
	Trigger       string   `json:"trigger,omitempty"`
	Version       string   `json:"version,omitempty"`
	Compatibility []string `json:"compatibility,omitempty"`
	TestEvidence  string   `json:"test_evidence,omitempty"`
}

const MaxSkillMetadataBytes = 4000

func validateSkillMetadata(kind string, metadata *SkillMetadata) error {
	if metadata == nil {
		return nil
	}
	if kind != "skill" {
		return fmt.Errorf("%w: skill_metadata is only allowed for skills", ErrInvalid)
	}
	if !validText(metadata.Trigger, 1000, false) || !validText(metadata.Version, 100, false) ||
		!validText(metadata.TestEvidence, 2000, false) || len(metadata.Compatibility) > 8 {
		return fmt.Errorf("%w: invalid skill_metadata text or field bounds", ErrInvalid)
	}
	total := len(metadata.Trigger) + len(metadata.Version) + len(metadata.TestEvidence)
	for _, compatibility := range metadata.Compatibility {
		if !validText(compatibility, MaxSkillMetadataBytes, false) {
			return fmt.Errorf("%w: invalid skill_metadata compatibility text", ErrInvalid)
		}
		total += len(compatibility)
	}
	if total > MaxSkillMetadataBytes {
		return fmt.Errorf("%w: skill_metadata text must total at most 4000 bytes", ErrInvalid)
	}
	return nil
}

// cloneSkillMetadata isolates the pointer and compatibility backing array. Empty
// metadata normalizes to nil on notes, but raw Input intent is kept for receipts.
func cloneSkillMetadata(metadata *SkillMetadata) *SkillMetadata {
	if metadata == nil || (metadata.Trigger == "" && metadata.Version == "" && metadata.TestEvidence == "" && len(metadata.Compatibility) == 0) {
		return nil
	}
	copy := *metadata
	copy.Compatibility = append([]string(nil), metadata.Compatibility...)
	return &copy
}
