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

// StagedDiff returns the diff of currently staged changes, without
// modifying the index.
func StagedDiff(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "diff", "--cached").Output()
	if err != nil {
		return "", fmt.Errorf("could not fetch staged diff: %w", err)
	}
	return string(out), nil
}

// CommitAllDiff stages all working tree changes, then returns the staged
// diff. Used by flows whose output should cover the entire working tree,
// not just what the user happened to stage.
func CommitAllDiff(ctx context.Context) (string, error) {
	if err := exec.CommandContext(ctx, "git", "add", "-A").Run(); err != nil {
		return "", fmt.Errorf("could not stage changes: %w", err)
	}
	return StagedDiff(ctx)
}

// BranchDiff returns the diff between the base branch and the current
// working tree, including both committed and uncommitted changes.
func BranchDiff(ctx context.Context, base string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "diff", base).Output()
	if err != nil {
		return "", fmt.Errorf("could not fetch branch diff: %w (ensure branch '%s' exists)", err, base)
	}
	return string(out), nil
}
