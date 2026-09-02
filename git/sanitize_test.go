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
		// Truncation applies before wrapping, so the content (newlines
		// aside) must be exactly the cap; wrapping may add line breaks.
		content := strings.ReplaceAll(got, "\n", "")
		if len(content) != maxCommitMsgLen {
			t.Errorf("content length = %d, want %d", len(content), maxCommitMsgLen)
		}
	})

	t.Run("long lines wrapped to maxCommitMsgLineLen", func(t *testing.T) {
		got := SanitizeCommitMessage("feat: x\n" + strings.Repeat("word ", 40))
		for i, line := range strings.Split(got, "\n") {
			if len(line) > maxCommitMsgLineLen {
				t.Errorf("line %d has length %d, want <= %d:\n%q", i, len(line), maxCommitMsgLineLen, got)
			}
		}
	})
}

func TestWrapLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		width int
		want  string
	}{
		{
			name:  "line within width is unchanged",
			input: "feat: a short subject line",
			width: 80,
			want:  "feat: a short subject line",
		},
		{
			name:  "line exactly at width is unchanged",
			input: strings.Repeat("a", 80),
			width: 80,
			want:  strings.Repeat("a", 80),
		},
		{
			// "feat: " + 15 words fills the line to exactly 80; the
			// 16th word starts on the next line.
			name:  "wraps at word boundaries",
			input: "feat: " + strings.Repeat("word ", 15) + "word",
			width: 80,
			want:  "feat: " + strings.Repeat("word ", 14) + "word\nword",
		},
		{
			// A 100-char token cannot fit: the line starts with it
			// hard-broken into an 80-char chunk, remainder on line 3.
			name:  "token longer than width is hard-broken",
			input: "x " + strings.Repeat("a", 100),
			width: 80,
			want:  "x\n" + strings.Repeat("a", 80) + "\n" + strings.Repeat("a", 20),
		},
		{
			// An 80-char token fits on its own line but not after "x".
			name:  "token equal to width moves to next line",
			input: "x " + strings.Repeat("b", 80),
			width: 80,
			want:  "x\n" + strings.Repeat("b", 80),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wrapLine(tt.input, tt.width); got != tt.want {
				t.Errorf("wrapLine() = %q, want %q", got, tt.want)
			}
		})
	}
}
