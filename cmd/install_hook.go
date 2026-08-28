package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// hookScript returns the shell script installed as prepare-commit-msg.
// binPath is the absolute path to the gitai binary so the hook keeps working
// even if $PATH changes between install time and commit time.
//
// The two guards make the hook a no-op except for a plain `git commit` with
// no message: $2 is the "source" (message/merge/template/...), which is only
// empty for a bare commit, and $1 is the message file. Note that git
// pre-fills $1 with its default "# ..." comment block before running this
// hook, so the file guard must test for *real* (non-comment) content, not
// just non-emptiness.
// Every exit path is 0 — a hook failure must never block a real commit.
func hookScript(binPath, hookPath string) string {
	return fmt.Sprintf(`#!/bin/sh
# gitai prepare-commit-msg hook - auto-generates a commit message.
# Installed by `+"`gitai -install-hook`"+`. Remove with: rm %s
[ -n "$2" ] && exit 0        # source set (-m/merge/template) -> leave alone
# git pre-fills $1 with its default "# ..." comment block, so bail only
# when the file already holds a real, non-comment line.
grep -qEv '^[[:space:]]*(#|$)' "$1" 2>/dev/null && exit 0
# stderr stays attached (no 2>/dev/null): gitai's live spinner and its
# "gitai hook: ..." warnings are written there, so the user sees the
# work happening instead of a silent wait. When stderr is not a TTY,
# gitai degrades to a single static line.
"%s" -hook "$1" || exit 0
exit 0
`, hookPath, binPath)
}

// prePushHookScript returns the shell script installed as pre-push. git
// passes the remote name and URL as $1/$2 and the refs being pushed on
// stdin; the remote name is forwarded so gitai can resolve that remote's
// default branch. A non-zero exit from gitai aborts the push — that is the
// whole point of the hook, so there is no `|| exit 0` here.
func prePushHookScript(binPath, hookPath string) string {
	return fmt.Sprintf(`#!/bin/sh
# gitai pre-push hook - runs the check pipeline on the pushed files.
# Installed by `+"`gitai -install-hook`"+`. Remove with: rm %s
exec "%s" -prepush "$1"
`, hookPath, binPath)
}

// installHook writes both gitai hooks (prepare-commit-msg and pre-push)
// into the current repo's .git/hooks. Requires a git repository and an
// installed gitai binary. Both features are off by default per repo; the
// hooks stay installed as shared infrastructure.
func installHook() {
	// Works from subdirectories of the repo too. Error means no .git found.
	absGitDir, err := gitDirPath()
	if err != nil {
		fmt.Printf("ERROR: not a git repository (no .git folder found here)\n")
		os.Exit(1)
	}

	binPath, err := os.Executable()
	if err != nil {
		fmt.Printf("ERROR: could not determine gitai binary path: %v\n", err)
		os.Exit(1)
	}

	hooksDir := filepath.Join(absGitDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		fmt.Printf("ERROR: could not create %s: %v\n", hooksDir, err)
		os.Exit(1)
	}

	// A configured core.hooksPath shadows .git/hooks entirely — warn so a
	// successful install doesn't turn out to be a silent no-op.
	if out, err := exec.Command("git", "config", "core.hooksPath").Output(); err == nil {
		if p := strings.TrimSpace(string(out)); p != "" {
			fmt.Printf("warning: core.hooksPath is set to %s; git will not use the hook in %s\n", p, hooksDir)
		}
	}

	hooks := []struct {
		name string
		body string
	}{
		{"prepare-commit-msg", hookScript(binPath, filepath.Join(hooksDir, "prepare-commit-msg"))},
		{"pre-push", prePushHookScript(binPath, filepath.Join(hooksDir, "pre-push"))},
	}
	for _, h := range hooks {
		hookPath := filepath.Join(hooksDir, h.name)
		backupHook(hookPath)
		if err := os.WriteFile(hookPath, []byte(h.body), 0o755); err != nil {
			fmt.Printf("ERROR: could not write %s hook: %v\n", h.name, err)
			os.Exit(1)
		}
		fmt.Printf("Installed %s hook at %s\n", h.name, hookPath)
	}

	fmt.Println()
	fmt.Println("Both features are OFF by default for this repo:")
	fmt.Println("  AI commit messages: gitai -commitmsg-on  /  gitai -commitmsg-off")
	fmt.Println("  Push checks:        gitai push-check enable  /  gitai push-check disable")
}

// backupHook preserves a pre-existing hook under .bak so a hand-written
// one isn't lost silently. A pre-existing .bak is kept: it may hold a
// hand-written hook from an earlier install, which overwriting would lose.
func backupHook(hookPath string) {
	if _, err := os.Stat(hookPath); err != nil {
		return
	}
	backup := hookPath + ".bak"
	if _, err := os.Stat(backup); err == nil {
		fmt.Printf("Existing %s hook found; keeping previous backup %s\n", filepath.Base(hookPath), backup)
		return
	}
	if err := copyFile(hookPath, backup); err != nil {
		fmt.Printf("ERROR: could not back up existing hook to %s: %v\n", backup, err)
		os.Exit(1)
	}
	fmt.Printf("Existing %s hook backed up to %s\n", filepath.Base(hookPath), backup)
}

// gitDirPath resolves the absolute path to the .git directory of the repo
// containing the current working directory (works from subdirectories too).
func gitDirPath() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--git-dir").Output()
	if err != nil {
		return "", err
	}
	// Output() leaves the trailing newline — strip it or the path below
	// would contain one and the hook would be written to a garbage dir.
	return filepath.Abs(strings.TrimSpace(string(out)))
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
