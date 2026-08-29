package git

import (
	"strings"
)

// maxCommitMsgLen caps AI-generated commit messages. Git itself does not
// hard-limit commit message size, but anything beyond a few KB is almost
// certainly a model runaway and useless in `git log`, so we truncate well
// short of where messages become unmanageable.
const maxCommitMsgLen = 7200

// SanitizeCommitMessage prepares raw model output for use as a commit
// message.
//
// Models occasionally emit control characters (unicode escapes, stray
// ANSI codes, null bytes) that are legal in a string but wrong inside a
// commit message — `git log` output gets mangled and some git tools
// reject such messages outright. We keep only printable ASCII plus the
// line break characters, then trim and truncate to maxCommitMsgLen.
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
	return result
}
