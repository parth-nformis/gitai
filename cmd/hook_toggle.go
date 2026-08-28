package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// Per-repo feature toggles.
//
// The installed hooks are generic infrastructure; whether a given feature
// (today: AI commit messages, push checks) actually acts is a per-repo,
// per-feature decision stored as a marker inside the repo's .git folder.
// Keeping it out of git config and out of the working tree means it is
// gitai-managed, invisible to version control, and scoped to exactly this
// repo. Each hook feature gets its own marker here.

// gitaiToggleDir returns the per-repo gitai state directory
// (<git-dir>/gitai), creating nothing.
func gitaiToggleDir() (string, error) {
	gitDir, err := gitDirPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(gitDir, "gitai"), nil
}

// featureMarkerPath is the marker for a hook feature
// (<git-dir>/gitai/<name>): presence means the feature is ON for this repo.
func featureMarkerPath(name string) (string, error) {
	dir, err := gitaiToggleDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// setFeatureToggle turns a per-repo feature on or off. It is idempotent in
// both directions.
func setFeatureToggle(name string, on bool) error {
	path, err := featureMarkerPath(name)
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

// isFeatureOn reports whether a per-repo feature is ON. Any failure (not in
// a repo, unreadable marker, ...) reads as OFF so a hook path can never be
// blocked by a state error.
func isFeatureOn(name string) bool {
	path, err := featureMarkerPath(name)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// featureInfo carries the user-facing name of a hook feature and the hint
// printed when it is enabled.
var featureInfo = map[string]struct {
	label  string
	onHint string
}{
	"commitmsg": {
		label:  "AI commit messages",
		onHint: "Now run `git commit` (no -m) to get an AI-generated message.",
	},
	"pushcheck": {
		label:  "Push checks",
		onHint: "The next `git push` will run the check pipeline.",
	},
}

// toggleFeature is the shared CLI handler for the per-repo feature toggles
// (-commitmsg-on/-commitmsg-off, push-check enable/disable). It needs no
// API config, so it runs before config.Load like the other housekeeping
// commands.
func toggleFeature(name string, on bool) {
	info, ok := featureInfo[name]
	if !ok {
		info = struct {
			label  string
			onHint string
		}{label: name}
	}
	if err := setFeatureToggle(name, on); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}
	if on {
		fmt.Printf("%s enabled for this repo.\n", info.label)
		if info.onHint != "" {
			fmt.Println(info.onHint)
		}
	} else {
		fmt.Printf("%s disabled for this repo.\n", info.label)
	}
}
