package client

import "testing"

func TestIsMuseGlimmer(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"muse-glimmer", true},
		{"muse-glimmer:30b", true},
		{"Meta/Muse-Glimmer-30B", true},
		{"muse-glimmer-30b-instruct", true},
		{"gpt-4o", false},
		{"qwen3-32b", false},
		{"muse", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsMuseGlimmer(tt.model); got != tt.want {
			t.Errorf("IsMuseGlimmer(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

func TestWithReasoningStrength(t *testing.T) {
	tests := []struct {
		name        string
		prompt      string
		level       string
		wantPrompt  string
		wantChanged bool
	}{
		{
			name:        "appends to existing prompt",
			prompt:      "You are a helpful assistant.",
			level:       "low",
			wantPrompt:  "You are a helpful assistant.\n\nReasoning strength: low",
			wantChanged: true,
		},
		{
			name:        "trims surrounding whitespace first",
			prompt:      "  You are a helpful assistant.  ",
			level:       "high",
			wantPrompt:  "You are a helpful assistant.\n\nReasoning strength: high",
			wantChanged: true,
		},
		{
			name:        "empty prompt becomes just the directive",
			prompt:      "",
			level:       "medium",
			wantPrompt:  "Reasoning strength: medium",
			wantChanged: true,
		},
		{
			name:        "whitespace-only prompt becomes just the directive",
			prompt:      "   ",
			level:       "xhigh",
			wantPrompt:  "Reasoning strength: xhigh",
			wantChanged: true,
		},
		{
			name:        "empty level is a no-op",
			prompt:      "You are a helpful assistant.",
			level:       "",
			wantPrompt:  "You are a helpful assistant.",
			wantChanged: false,
		},
		{
			name:        "existing directive wins, no duplicate line",
			prompt:      "Follow the instructions.\nReasoning strength: low",
			level:       "high",
			wantPrompt:  "Follow the instructions.\nReasoning strength: low",
			wantChanged: false,
		},
		{
			name:        "existing directive match is case-insensitive",
			prompt:      "Reasoning Strength: low",
			level:       "high",
			wantPrompt:  "Reasoning Strength: low",
			wantChanged: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPrompt, gotChanged := withReasoningStrength(tt.prompt, tt.level)
			if gotPrompt != tt.wantPrompt {
				t.Errorf("withReasoningStrength() prompt = %q, want %q", gotPrompt, tt.wantPrompt)
			}
			if gotChanged != tt.wantChanged {
				t.Errorf("withReasoningStrength() changed = %v, want %v", gotChanged, tt.wantChanged)
			}
		})
	}
}
