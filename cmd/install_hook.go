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

// installHook writes the prepare-commit-msg hook into the current repo's
// .git/hooks. Requires a git repository and an installed gitai binary.
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

	hookPath := filepath.Join(hooksDir, "prepare-commit-msg")

	// A configured core.hooksPath shadows .git/hooks entirely — warn so a
	// successful install doesn't turn out to be a silent no-op.
	if out, err := exec.Command("git", "config", "core.hooksPath").Output(); err == nil {
		if p := strings.TrimSpace(string(out)); p != "" {
			fmt.Printf("warning: core.hooksPath is set to %s; git will not use the hook in %s\n", p, hooksDir)
		}
	}

	// Back up an existing hook so a hand-written one isn't lost silently.
	// A pre-existing .bak is kept: it may hold a hand-written hook from an
	// earlier install, which overwriting would lose.
	if _, err := os.Stat(hookPath); err == nil {
		backup := hookPath + ".bak"
		if _, err := os.Stat(backup); err == nil {
			fmt.Printf("Existing hook found; keeping previous backup %s\n", backup)
		} else if copyErr := copyFile(hookPath, backup); copyErr != nil {
			fmt.Printf("ERROR: could not back up existing hook to %s: %v\n", backup, copyErr)
			os.Exit(1)
		} else {
			fmt.Printf("Existing hook backed up to %s\n", backup)
		}
	}

	if err := os.WriteFile(hookPath, []byte(hookScript(binPath, hookPath)), 0o755); err != nil {
		fmt.Printf("ERROR: could not write hook: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Installed prepare-commit-msg hook at %s\n", hookPath)
	fmt.Println("AI commit messages are OFF by default for this repo.")
	fmt.Println("Enable them with:  gitai -commitmsg-on")
	fmt.Println("Disable them with: gitai -commitmsg-off")
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
