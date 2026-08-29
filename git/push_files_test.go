package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// gitRepo builds a throwaway repo with two commits: main with main.go +
// util.go, and a feature branch that adds a.go, edits main.go, and deletes
// util.go (the deletion must be excluded from the push file list). It
// switches the test's CWD into the repo (PushFiles resolves git from the
// process CWD, exactly like the hook does) and returns the two commit
// shas (main, feature). -c flags mean no git config is written anywhere.
func gitRepo(t *testing.T) (string, string) {
	t.Helper()
	repo := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Errorf("restore chdir: %v", err)
		}
	})
	commit := func(name, msg string, mutate func()) {
		if mutate != nil {
			mutate()
		}
		args := []string{"-C", repo, "-c", "user.email=t@t", "-c", "user.name=t"}
		out, err := exec.Command("git", append(args, "commit", "-m", msg)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git commit (%s): %v\n%s", name, err, out)
		}
	}
	out, err := exec.Command("git", "init", "-b", "main", repo).CombinedOutput()
	if err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	write := func(rel, content string) {
		if err := os.WriteFile(filepath.Join(repo, rel), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
		exec.Command("git", "-C", repo, "add", "-A").Run()
	}
	write("main.go", "package main\n")
	write("util.go", "package main\n")
	commit("one", "initial", nil)
	mainSHA := revSHA(t, repo)

	if out, err := exec.Command("git", "-C", repo, "checkout", "-b", "feature").CombinedOutput(); err != nil {
		t.Fatalf("branch: %v\n%s", err, out)
	}
	write("a.go", "package main\n")
	write("main.go", "package main\n// edited\n")
	if err := os.Remove(filepath.Join(repo, "util.go")); err != nil {
		t.Fatalf("remove util.go: %v", err)
	}
	exec.Command("git", "-C", repo, "add", "-A").Run()
	commit("two", "feature", nil)
	featureSHA := revSHA(t, repo)

	return mainSHA, featureSHA
}

// revSHA returns the HEAD sha with Output()'s trailing newline trimmed —
// an untrimmed sha in a diff arg makes git silently fail and the whole
// test would read as "no files in the push".
func revSHA(t *testing.T, repo string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func TestPushFilesExistingRemoteRef(t *testing.T) {
	mainSHA, featureSHA := gitRepo(t)
	files, err := PushFiles([]RefPair{{LocalSHA: featureSHA, RemoteSHA: mainSHA}}, "origin")
	if err != nil {
		t.Fatalf("PushFiles: %v", err)
	}
	// git diff --name-only prints in path order.
	if !reflect.DeepEqual(files, []string{"a.go", "main.go"}) {
		t.Errorf("files = %v, want [a.go main.go]", files)
	}
}

func TestPushFilesRefDeletion(t *testing.T) {
	mainSHA, _ := gitRepo(t)
	files, err := PushFiles([]RefPair{{LocalSHA: zeroSHA, RemoteSHA: mainSHA}}, "origin")
	if err != nil {
		t.Fatalf("PushFiles: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("deletion should check no files, got %v", files)
	}
}

func TestPushFilesNewRemoteRef(t *testing.T) {
	_, featureSHA := gitRepo(t)
	files, err := PushFiles([]RefPair{{LocalSHA: featureSHA, RemoteSHA: zeroSHA}}, "origin")
	if err != nil {
		t.Fatalf("PushFiles: %v", err)
	}
	// New ref is judged against the merge-base with main.
	if !reflect.DeepEqual(files, []string{"a.go", "main.go"}) {
		t.Errorf("files = %v, want [a.go main.go]", files)
	}
}

func TestPushFilesUnionDedupes(t *testing.T) {
	mainSHA, featureSHA := gitRepo(t)
	files, err := PushFiles([]RefPair{
		{LocalSHA: featureSHA, RemoteSHA: mainSHA},
		{LocalSHA: featureSHA, RemoteSHA: mainSHA},
	}, "origin")
	if err != nil {
		t.Fatalf("PushFiles: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("union should dedupe, got %v", files)
	}
}
