package server

import (
	"memona/internal/access"
	"strings"
)

// Existing bounded audit evidence, not live-connection telemetry.
type keyUsage struct {
	LastRecordedAt string `json:"last_recorded_at"`
	Action         string `json:"action"`
}

func recordedKeyUsage(keys []access.Key, events []access.Event) map[string]keyUsage {
	actors := make(map[string]string, len(keys))
	for _, key := range keys {
		actors[key.Actor()] = key.ID
	}
	result := map[string]keyUsage{}
	// ActivityContext is newest-first, bounded to 200 audit events.
	for _, event := range events {
		id, ok := actors[event.Actor]
		if !ok || !(strings.HasPrefix(event.Action, "memory.") || strings.HasPrefix(event.Action, "skill.")) {
			continue
		}
		if _, exists := result[id]; !exists {
			result[id] = keyUsage{event.Time, event.Action}
		}
	}
	return result
}
