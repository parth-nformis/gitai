package git

import (
	"strings"
)

// maxCommitMsgLen caps AI-generated commit messages. Git itself does not
// hard-limit commit message size, but anything beyond a few KB is almost
// certainly a model runaway and useless in `git log`, so we truncate well
// short of where messages become unmanageable.
const maxCommitMsgLen = 7200

// maxCommitMsgLineLen caps individual lines so the message the hook writes
// into the editor stays readable in the traditional 80-column width: the
// user sees and edits this in a plain text editor, and unbounded model
// output otherwise renders as one giant unbroken paragraph.
const maxCommitMsgLineLen = 80

// SanitizeCommitMessage prepares raw model output for use as a commit
// message.
//
// Models occasionally emit control characters (unicode escapes, stray
// ANSI codes, null bytes) that are legal in a string but wrong inside a
// commit message — `git log` output gets mangled and some git tools
// reject such messages outright. We keep only printable ASCII plus the
// line break characters, then trim, wrap long lines to
// maxCommitMsgLineLen, and truncate to maxCommitMsgLen.
func SanitizeCommitMessage(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 0x20 && r <= 0x7E) || r == '\n' || r == '\r' {
			b.WriteRune(r)
		}
	}
	result := strings.TrimSpace(b.String())
	if len(result) > maxCommitMsgLen {
		result = result[:maxCommitMsgLen]
	}
	return wrapLines(result, maxCommitMsgLineLen)
}

// wrapLines re-wraps each line of s to at most width characters, breaking
// at word boundaries: a word is moved to the next line when it would push
// the current line past the width. A single token longer than width
// (e.g. a URL the model emitted) is hard-broken so no line ever exceeds
// the limit. Lines already within the width are returned unchanged.
func wrapLines(s string, width int) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = wrapLine(line, width)
	}
	return strings.Join(lines, "\n")
}

// wrapLine greedily packs space-separated words onto lines of at most
// width characters.
func wrapLine(s string, width int) string {
	if len(s) <= width {
		return s
	}
	var out []string
	cur := ""
	for w := range strings.FieldsSeq(s) {
		if len(w) > width {
			// The token cannot fit on any line: flush what we have,
			// emit full-width chunks, and keep the remainder as the
			// start of the next line.
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			for len(w) > width {
				out = append(out, w[:width])
				w = w[width:]
			}
			cur = w
			continue
		}
		if cur == "" {
			cur = w
		} else if len(cur)+1+len(w) <= width {
			cur += " " + w
		} else {
			out = append(out, cur)
			cur = w
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return strings.Join(out, "\n")
}
