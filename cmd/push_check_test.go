package main

import (
	"os"
	"strings"
	"testing"
)

func TestReadPushRefs(t *testing.T) {
	// pre-push contract: one line per ref, four fields —
	// "<local ref> <local object> <remote ref> <remote object>". Ref 1 is an
	// existing remote branch, ref 2 a brand-new one (zero remote object),
	// ref 3 a deletion ("(delete)" local ref, zero local object).
	in := strings.NewReader(
		"refs/heads/main abc123 refs/heads/main def456\n" +
			"refs/heads/new f00f00 refs/heads/new 0000000000000000000000000000000000000000\n" +
			"(delete) 0000000000000000000000000000000000000000 refs/heads/gone 999999\n")

	pairs, err := readPushRefs(in)
	if err != nil {
		t.Fatalf("readPushRefs: %v", err)
	}
	if len(pairs) != 3 {
		t.Fatalf("expected 3 pairs, got %d: %+v", len(pairs), pairs)
	}
	if pairs[0].LocalSHA != "abc123" || pairs[0].RemoteSHA != "def456" {
		t.Errorf("pair 0 = %+v", pairs[0])
	}
	if pairs[1].LocalSHA != "f00f00" || pairs[1].RemoteSHA != "0000000000000000000000000000000000000000" {
		t.Errorf("pair 1 = %+v", pairs[1])
	}
	if pairs[2].LocalSHA != "0000000000000000000000000000000000000000" || pairs[2].RemoteSHA != "999999" {
		t.Errorf("pair 2 (deletion) = %+v", pairs[2])
	}
}

func TestReadPushRefsEmpty(t *testing.T) {
	pairs, err := readPushRefs(strings.NewReader(""))
	if err != nil {
		t.Fatalf("readPushRefs: %v", err)
	}
	if len(pairs) != 0 {
		t.Errorf("expected no pairs, got %+v", pairs)
	}
}

func TestHookRemoteName(t *testing.T) {
	old := os.Args
	defer func() { os.Args = old }()

	os.Args = []string{"gitai", "-prepush", "upstream"}
	if got := hookRemoteName(); got != "upstream" {
		t.Errorf("hookRemoteName() = %q, want upstream", got)
	}

	// No forward: default to origin.
	os.Args = []string{"gitai", "-prepush"}
	if got := hookRemoteName(); got != "origin" {
		t.Errorf("hookRemoteName() = %q, want origin", got)
	}
}

func TestCapLines(t *testing.T) {
	in := "a\nb\nc\nd"
	out := capLines(in, 2)
	if len(out) != 3 || out[0] != "a" || out[1] != "b" || out[2] != "… (2 more lines)" {
		t.Errorf("capLines = %v", out)
	}
	if out := capLines("a\nb", 5); len(out) != 2 {
		t.Errorf("short input should pass through, got %v", out)
	}
}
