package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// RefPair is one ref being pushed, from the pre-push hook's stdin.
type RefPair struct {
	LocalSHA  string
	RemoteSHA string // all zeros when the remote ref is being created
}

// zeroSHA is git's "no such object yet" id, used for new remote refs.
const zeroSHA = "0000000000000000000000000000000000000000"

// PushFiles returns the union of files changed by the pushes described in
// pairs, excluding deleted files — the check tools read files from the
// working tree, so a file the push removes cannot be checked.
//
//   - existing remote ref: diff between the remote and local sha — exactly
//     what the push will add;
//   - new remote ref: diff against the merge-base with the default branch,
//     so a new branch is judged on what it adds over trunk;
//   - deletion (local sha zero): nothing.
//
// Plumbing failures are lenient (skip that ref) rather than fatal: the
// pipeline is a quality gate, not a hard dependency, and gitai must never
// block a push on git plumbing surprises.
func PushFiles(pairs []RefPair, remote string) ([]string, error) {
	seen := map[string]bool{}
	var files []string
	for _, p := range pairs {
		if p.LocalSHA == zeroSHA {
			continue
		}
		base := p.RemoteSHA
		if base == zeroSHA {
			b, err := newRefBase(remote, p.LocalSHA)
			if err != nil {
				continue
			}
			base = b
		}
		out, err := exec.Command("git", "diff", "--name-status", base, p.LocalSHA).Output()
		if err != nil {
			continue
		}
		addFiles(&files, seen, string(out))
	}
	return files, nil
}

// newRefBase finds where a new branch is judged against. In order:
// merge-base with the remote's default branch, then main, then master,
// then — as a conservative last resort — every tracked file, so a new
// ref can never escape the checks entirely when the trunk is unresolvable.
func newRefBase(remote, localSHA string) (string, error) {
	branches := []string{defaultBranch(remote), "main", "master"}
	for _, b := range branches {
		if out, err := exec.Command("git", "merge-base", b, localSHA).Output(); err == nil {
			if sha := strings.TrimSpace(string(out)); sha != "" {
				return sha, nil
			}
		}
	}
	// We need a diff base, not a list: diff against the empty tree so the
	// "files in the push" covers everything the ref contains.
	empty, err := exec.Command("git", "hash-object", "-t", "tree", "/dev/null").Output()
	if err != nil {
		return "", fmt.Errorf("could not build empty tree: %w", err)
	}
	return strings.TrimSpace(string(empty)), nil
}

// defaultBranch resolves the remote's default branch (refs/remotes/<remote>/HEAD),
// falling back to main when the remote has not advertised one.
func defaultBranch(remote string) string {
	if out, err := exec.Command("git", "symbolic-ref", "refs/remotes/"+remote+"/HEAD").Output(); err == nil {
		if b := strings.TrimPrefix(strings.TrimSpace(string(out)), "refs/remotes/"+remote+"/"); b != "" {
			return b
		}
	}
	return "main"
}

// addFiles appends the pushed file paths from git's --name-status output.
// Splitting on the first tab (not whitespace) keeps paths that contain
// spaces intact.
func addFiles(files *[]string, seen map[string]bool, out string) {
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		status, rest := parts[0], parts[1]
		var path string
		switch status[0] {
		case 'D':
			continue // deleted files cannot be checked
		case 'R', 'C':
			fields := strings.Split(rest, "\t")
			path = fields[len(fields)-1] // destination of the rename/copy
		default:
			path = rest
		}
		if !seen[path] {
			seen[path] = true
			*files = append(*files, path)
		}
	}
}
