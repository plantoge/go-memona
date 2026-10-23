package memory

import "fmt"

const MaxRelations = 20
const MaxSummaryBytes = 8000

func validateMetadata(in Input) error {
	if err := validateSkillMetadata(in.Kind, in.SkillMetadata); err != nil {
		return err
	}
	if in.IsProjectSummary != nil && *in.IsProjectSummary && (in.Kind != "knowledge" || len(in.Content) > MaxSummaryBytes) {
		return fmt.Errorf("%w: project summary must be knowledge with content at most 8000 bytes", ErrInvalid)
	}
	seen := map[string]bool{}
	for _, group := range [][]string{in.RelatedTo, in.Supersedes} {
		if len(group) > MaxRelations {
			return fmt.Errorf("%w: at most 20 IDs per relation type", ErrInvalid)
		}
		for _, id := range group {
			if !idPattern.MatchString(id) || seen[id] {
				return fmt.Errorf("%w: relation IDs must be valid and unique across relation types", ErrInvalid)
			}
			seen[id] = true
		}
	}
	return nil
}

func applyMetadata(n *Note, in Input) {
	if in.SkillMetadata != nil {
		n.SkillMetadata = cloneSkillMetadata(in.SkillMetadata)
	}
	if in.IsTesting != nil {
		n.IsTesting = *in.IsTesting
	}
	if in.IsProjectSummary != nil {
		n.IsProjectSummary = *in.IsProjectSummary
	}
	if in.RelatedTo != nil {
		n.RelatedTo = append([]string(nil), in.RelatedTo...)
	}
	if in.Supersedes != nil {
		n.Supersedes = append([]string(nil), in.Supersedes...)
	}
}

// Called with the writer mutex held. Inspect latest Markdown, not an independent
// database index. Archived edges still participate in cycle validation so restore
// cannot revive a cycle. Targets may be archived; retrieval marks them explicitly.
func (s *Store) validateConnections(candidate Note) error {
	return s.validateConnectionsBudget(candidate, nil)
}

func (s *Store) validateConnectionsBudget(candidate Note, budget *readBudget) error {
	if err := budget.check(); err != nil {
		return err
	}
	if !candidate.IsProjectSummary && len(candidate.RelatedTo)+len(candidate.Supersedes) == 0 {
		return nil
	}
	all, err := budget.list(s)
	if err != nil {
		return err
	}
	byID := make(map[string]Note, len(all)+1)
	for _, n := range all {
		if err := budget.check(); err != nil {
			return err
		}
		byID[n.ID] = n
		if candidate.IsProjectSummary && n.ID != candidate.ID && n.Project == candidate.Project && n.IsProjectSummary && !n.Archived {
			return fmt.Errorf("%w: project already has a nonarchived summary; clear its flag or archive it first", ErrConflict)
		}
	}
	byID[candidate.ID] = candidate
	for _, group := range [][]string{candidate.RelatedTo, candidate.Supersedes} {
		for _, id := range group {
			target, ok := byID[id]
			if !ok || id == candidate.ID || target.Project != candidate.Project {
				return fmt.Errorf("%w: relation target unavailable, self-referencing, or outside this project", ErrInvalid)
			}
		}
	}
	visited := map[string]bool{}
	var scanErr error
	var reachesCandidate func(string) bool
	reachesCandidate = func(id string) bool {
		if scanErr = budget.check(); scanErr != nil {
			return false
		}
		if id == candidate.ID {
			return true
		}
		if visited[id] {
			return false
		}
		visited[id] = true
		for _, next := range byID[id].Supersedes {
			if reachesCandidate(next) {
				return true
			}
			if scanErr != nil {
				return false
			}
		}
		return false
	}
	for _, id := range candidate.Supersedes {
		if reachesCandidate(id) {
			return fmt.Errorf("%w: supersedes must not contain a cycle", ErrInvalid)
		}
		if scanErr != nil {
			return scanErr
		}
	}
	return budget.check()
}
