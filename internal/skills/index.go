package skills

import (
	"fmt"
	"strings"
)

type IndexEntry struct {
	Name        string `json:"name"`
	Version     int    `json:"version"`
	Trigger     string `json:"trigger"`
	Summary     string `json:"summary"`
	SHA256      string `json:"sha256"`
	Description string `json:"description"`
}

func FormatStableIndex(entries []IndexEntry) string {
	if len(entries) == 0 {
		return ""
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		lines = append(lines, fmt.Sprintf(
			"skill: %s\nversion: %d\ntrigger: %s\nsummary: %s\nsha256: %s",
			entry.Name,
			entry.Version,
			entry.Trigger,
			entry.Summary,
			entry.SHA256,
		))
	}
	return strings.Join(lines, "\n---\n")
}
