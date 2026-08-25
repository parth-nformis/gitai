// Package git wraps the git commands gitai needs. Each diff source is an
// explicit function so callers can pick the one they want — in particular,
// StagedDiff has no side effects, which matters for hooks and other flows
// that must not modify the index.
package git

import (
	"context"
	"fmt"
	"os/exec"
)

// StagedDiff returns the diff of currently staged changes
// (`git diff --cached`: index vs HEAD), without modifying the index.
//
// The no-side-effect guarantee is important: callers may run in
// contexts where touching the user's staging area would be wrong —
// notably the upcoming prepare-commit-msg hook, which fires in the
// middle of the user's own `git commit`.
func StagedDiff(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "diff", "--cached").Output()
	if err != nil {
		return "", fmt.Errorf("could not fetch staged diff: %w", err)
	}
	return string(out), nil
}

// CommitAllDiff stages all working tree changes (`git add -A`, which
// covers modified, untracked, and deleted files), then returns the
// staged diff. Used by flows whose output should cover the entire
// working tree, not just what the user happened to stage.
//
// Note the deliberate difference from StagedDiff: this one HAS a side
// effect (it rewrites the index), so only use it for top-level user
// commands, never from hooks.
func CommitAllDiff(ctx context.Context) (string, error) {
	if err := exec.CommandContext(ctx, "git", "add", "-A").Run(); err != nil {
		return "", fmt.Errorf("could not stage changes: %w", err)
	}
	return StagedDiff(ctx)
}

// BranchDiff returns the diff between the base branch and the current
// working tree, including both committed and uncommitted changes
// (`git diff <base>` compares the branch commit against the tree, not
// just HEAD). This is what a PR diff must show: everything that would
// land when the branch is merged.
func BranchDiff(ctx context.Context, base string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "diff", base).Output()
	if err != nil {
		// The most common cause of failure here is a typo'd or
		// not-yet-fetched base branch, so say that explicitly.
		return "", fmt.Errorf("could not fetch branch diff: %w (ensure branch '%s' exists)", err, base)
	}
	return string(out), nil
}
