package main

import (
	"os"
	"os/exec"
	"testing"
)

// chdirToTempGitRepo creates a fresh temporary git repository, switches the
// test's working directory into it, and returns a cleanup func that restores
// the original CWD. Tests that use it must not run in parallel (they mutate
// the process CWD).
func chdirToTempGitRepo(t *testing.T) func() {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init %s: %v\n%s", dir, err, out)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	return func() {
		if err := os.Chdir(orig); err != nil {
			t.Errorf("restore chdir: %v", err)
		}
	}
}

func TestCommitMsgToggleOnOff(t *testing.T) {
	cleanup := chdirToTempGitRepo(t)
	defer cleanup()

	if isCommitMsgOn() {
		t.Fatal("expected feature OFF in a fresh repo")
	}

	if err := setCommitMsgToggle(true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !isCommitMsgOn() {
		t.Fatal("expected feature ON after enabling")
	}
	path, err := commitMsgTogglePath()
	if err != nil {
		t.Fatalf("toggle path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("marker missing after enable: %v", err)
	}

	if err := setCommitMsgToggle(false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if isCommitMsgOn() {
		t.Fatal("expected feature OFF after disabling")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected marker removed, stat err=%v", err)
	}
}

func TestCommitMsgToggleIdempotent(t *testing.T) {
	cleanup := chdirToTempGitRepo(t)
	defer cleanup()

	for i := range 2 {
		if err := setCommitMsgToggle(true); err != nil {
			t.Fatalf("enable #%d: %v", i+1, err)
		}
	}
	if !isCommitMsgOn() {
		t.Fatal("expected feature ON")
	}
	for i := range 2 {
		if err := setCommitMsgToggle(false); err != nil {
			t.Fatalf("disable #%d: %v", i+1, err)
		}
	}
	if isCommitMsgOn() {
		t.Fatal("expected feature OFF")
	}
}

func TestCommitMsgToggleOutsideRepo(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(orig)

	// Not a git repo: the feature reads as OFF and enabling must error.
	if isCommitMsgOn() {
		t.Fatal("expected feature OFF outside a repo")
	}
	if err := setCommitMsgToggle(true); err == nil {
		t.Fatal("expected an error enabling outside a repo")
	}
}
