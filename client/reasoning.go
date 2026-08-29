package client

import (
	"strings"
)

// IsMuseGlimmer reports whether the (per-call) model is Muse Glimmer.
// Case-insensitive substring; matches "muse-glimmer", "muse-glimmer:30b",
// "meta/muse-glimmer-30b", etc.
func IsMuseGlimmer(model string) bool {
	return strings.Contains(strings.ToLower(model), "muse-glimmer")
}

// ValidReasoningStrength reports whether s is a supported Muse Glimmer
// reasoning strength. Kept next to the injection helpers so the set of
// accepted values lives with the mechanism that consumes it.
func ValidReasoningStrength(s string) bool {
	switch s {
	case "low", "medium", "high", "xhigh":
		return true
	}
	return false
}

// withReasoningStrength appends "Reasoning strength: <level>" to the system
// prompt. No-op when level=="" or the prompt already contains a
// "Reasoning strength:" line (case-insensitive) so a user's own directive wins
// and we never emit two conflicting lines. Trims the prompt first so an empty
// custom prompt becomes just the directive. Returns the prompt and whether it
// changed.
func withReasoningStrength(systemPrompt, level string) (string, bool) {
	if level == "" {
		return systemPrompt, false
	}

	trimmed := strings.TrimSpace(systemPrompt)
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "reasoning strength:") {
		return systemPrompt, false
	}

	line := "Reasoning strength: " + level
	if trimmed == "" {
		return line, true
	}
	return trimmed + "\n\n" + line, true
}
