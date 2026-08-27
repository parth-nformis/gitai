package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookScript(t *testing.T) {
	script := hookScript("/usr/local/bin/gitai", "/repo/.git/hooks/prepare-commit-msg")

	for _, want := range []string{
		"#!/bin/sh",
		`[ -n "$2" ] && exit 0`, // bail when the user gave -m / merge / template
		`[ -s "$1" ] && exit 0`, // bail when the message file is not empty
		`"/usr/local/bin/gitai" -hook "$1"`,
		"rm /repo/.git/hooks/prepare-commit-msg",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("hookScript missing %q (full script:\n%s)", want, script)
		}
	}

	if !strings.HasSuffix(script, "exit 0\n") {
		t.Errorf("hookScript must end with exit 0 so a failure never blocks a commit (got tail: %q)", tail(script, 40))
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func TestGitDirPath(t *testing.T) {
	repo := t.TempDir()
	if err := exec.Command("git", "init", "-q", repo).Run(); err != nil {
		t.Skipf("git not available: %v", err)
	}
	t.Chdir(repo)

	got, err := gitDirPath()
	if err != nil {
		t.Fatalf("gitDirPath() error: %v", err)
	}
	want := filepath.Join(repo, ".git")
	if got != want {
		t.Errorf("gitDirPath() = %q, want %q", got, want)
	}
	if strings.ContainsAny(got, "\n\r") {
		t.Errorf("gitDirPath() = %q must not contain newlines (regression: untrimmed rev-parse output)", got)
	}

	// Outside any repo this must fail, not produce a path.
	t.Chdir(t.TempDir())
	if _, err := gitDirPath(); err == nil {
		t.Error("gitDirPath() outside a repo: expected error, got nil")
	}
}
