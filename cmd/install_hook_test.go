package main

import (
	"os"
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
		`grep -qEv '^[[:space:]]*(#|$)' "$1" 2>/dev/null && exit 0`, // bail only on real (non-comment) content
		`"/usr/local/bin/gitai" -commit-msg-file "$1"`,
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

// TestHookScriptBailGuards runs the actual installed script (against a fake
// gitai) to pin the guard behavior. The comment-only case is the regression
// for the bug where git's pre-filled "# comment" template made the old
// `[ -s "$1" ]` guard bail on every real `git commit`.
func TestHookScriptBailGuards(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh not available: %v", err)
	}

	dir := t.TempDir()
	// Fake gitai: when invoked as "gitai -commit-msg-file <file>", write a marker.
	fakeBin := filepath.Join(dir, "gitai")
	// Fake gitai mirrors the real prepend behavior: write the generated
	// message above whatever the file already holds (git's comment block).
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\nif [ \"$1\" = \"-commit-msg-file\" ]; then\n  existing=\"$(cat \"$2\" 2>/dev/null)\"\n  printf 'generated message\\n\\n%s\\n' \"$existing\" > \"$2\"\n  exit 0\nfi\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(dir, "prepare-commit-msg")
	if err := os.WriteFile(hookPath, []byte(hookScript(fakeBin, hookPath)), 0o755); err != nil {
		t.Fatal(err)
	}

	run := func(fileContent, source string) string {
		msgfile := filepath.Join(dir, "msg")
		if err := os.WriteFile(msgfile, []byte(fileContent), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := exec.Command("sh", hookPath, msgfile, source).Run(); err != nil {
			t.Fatalf("hook exited non-zero (must never block a commit): %v", err)
		}
		data, err := os.ReadFile(msgfile)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	// git pre-fills the file with its default comment block -> generate,
	// with our message above git's preserved comment block.
	got := run("# Please enter the commit message for your changes.\n# On branch main\n# (initial commit)\n", "")
	if !strings.Contains(got, "generated message") {
		t.Errorf("comment-only file: want generated message present, got %q", got)
	}
	if !strings.Contains(got, "# On branch main") {
		t.Errorf("comment-only file: git's comment block must be preserved, got %q", got)
	}

	// An empty file (older git / direct invocation) -> generate.
	if got := run("", ""); !strings.Contains(got, "generated message") {
		t.Errorf("empty file: want generated message, got %q", got)
	}

	// A real (non-comment) line already present -> leave alone.
	if got := run("feat: hand-written message\n", ""); got != "feat: hand-written message\n" {
		t.Errorf("real content: want untouched file, got %q", got)
	}

	// Source set (-m / merge / template / amend) -> leave alone.
	if got := run("# just a comment\n", "message"); got != "# just a comment\n" {
		t.Errorf("source set: want untouched file, got %q", got)
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
