package git

import (
	"strings"
	"testing"
)

func TestSanitizeCommitMessage(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain text passes through",
			input: "feat: add feature\n\nbody line",
			want:  "feat: add feature\n\nbody line",
		},
		{
			// Only control bytes are stripped (pre-move behavior): the ESC
			// byte of an ANSI sequence goes, but the printable "[31m" text
			// that follows it stays.
			name:  "control characters stripped, newlines kept",
			input: "feat: bad\x00char\x1b[31m\nsecond line\r\nend",
			want:  "feat: badchar[31m\nsecond line\r\nend",
		},
		{
			name:  "surrounding whitespace trimmed",
			input: "  \nfeat: trimmed\n  ",
			want:  "feat: trimmed",
		},
		{
			name:  "empty input stays empty",
			input: "   ",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeCommitMessage(tt.input); got != tt.want {
				t.Errorf("SanitizeCommitMessage() = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("truncated to maxCommitMsgLen", func(t *testing.T) {
		got := SanitizeCommitMessage(strings.Repeat("a", maxCommitMsgLen+100))
		if len(got) != maxCommitMsgLen {
			t.Errorf("len(SanitizeCommitMessage()) = %d, want %d", len(got), maxCommitMsgLen)
		}
	})
}
