package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// Per-repo feature toggles.
//
// The installed prepare-commit-msg hook is generic infrastructure; whether a
// given feature (today: AI commit messages) actually acts is a per-repo,
// per-feature decision stored as a marker inside the repo's .git folder.
// Keeping it out of git config and out of the working tree means it is
// gitai-managed, invisible to version control, and scoped to exactly this
// repo. Future hook features each get their own marker here.

// gitaiToggleDir returns the per-repo gitai state directory
// (<git-dir>/gitai), creating nothing.
func gitaiToggleDir() (string, error) {
	gitDir, err := gitDirPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(gitDir, "gitai"), nil
}

// commitMsgTogglePath is the marker for "use AI for commit messages":
// presence means the feature is ON for this repo.
func commitMsgTogglePath() (string, error) {
	dir, err := gitaiToggleDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "commitmsg"), nil
}

// setCommitMsgToggle turns the per-repo commit-message feature on or off.
// It is idempotent in both directions.
func setCommitMsgToggle(on bool) error {
	path, err := commitMsgTogglePath()
	if err != nil {
		return err
	}
	if on {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		// Presence is the state; the content is just a human-readable hint.
		return os.WriteFile(path, []byte("on\n"), 0o644)
	}
	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// isCommitMsgOn reports whether the per-repo commit-message feature is ON.
// Any failure (not in a repo, unreadable marker, ...) reads as OFF so the
// hook path can never be blocked by a state error.
func isCommitMsgOn() bool {
	path, err := commitMsgTogglePath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// toggleCommitMsg is the CLI handler for -commitmsg-on / -commitmsg-off.
// It needs no API config, so it runs before config.Load like the other
// housekeeping commands.
func toggleCommitMsg(on bool) {
	if err := setCommitMsgToggle(on); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}
	if on {
		fmt.Println("AI commit messages enabled for this repo.")
		fmt.Println("Now run `git commit` (no -m) to get an AI-generated message.")
	} else {
		fmt.Println("AI commit messages disabled for this repo.")
	}
}
