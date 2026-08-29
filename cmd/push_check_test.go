package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/parthdande/gitai/git"
	"github.com/parthdande/gitai/pipeline"
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
	if pairs[0].LocalRef != "refs/heads/main" || pairs[0].LocalSHA != "abc123" || pairs[0].RemoteRef != "refs/heads/main" || pairs[0].RemoteSHA != "def456" {
		t.Errorf("pair 0 = %+v", pairs[0])
	}
	if pairs[1].LocalRef != "refs/heads/new" || pairs[1].LocalSHA != "f00f00" || pairs[1].RemoteRef != "refs/heads/new" || pairs[1].RemoteSHA != "0000000000000000000000000000000000000000" {
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

	os.Args = []string{"gitai", "-pre-push", "upstream"}
	if got := hookRemoteName(); got != "upstream" {
		t.Errorf("hookRemoteName() = %q, want upstream", got)
	}

	// No forward: default to origin.
	os.Args = []string{"gitai", "-pre-push"}
	if got := hookRemoteName(); got != "origin" {
		t.Errorf("hookRemoteName() = %q, want origin", got)
	}
}

func TestPushTarget(t *testing.T) {
	cases := []struct {
		name   string
		pairs  []git.RefPair
		remote string
		want   string
	}{
		{
			name:   "single branch",
			pairs:  []git.RefPair{{LocalRef: "refs/heads/test", RemoteRef: "refs/heads/test"}},
			remote: "origin",
			want:   "test → origin/test",
		},
		{
			name:   "tag",
			pairs:  []git.RefPair{{LocalRef: "refs/tags/v1.0", RemoteRef: "refs/tags/v1.0"}},
			remote: "origin",
			want:   "v1.0 → origin/v1.0",
		},
		{
			name: "multiple refs collapse to the remote name",
			pairs: []git.RefPair{
				{LocalRef: "refs/heads/test", RemoteRef: "refs/heads/test"},
				{LocalRef: "refs/heads/main", RemoteRef: "refs/heads/main"},
			},
			remote: "origin",
			want:   "test, main → origin",
		},
		{
			name:   "deletion ref contributes nothing",
			pairs:  []git.RefPair{{LocalRef: "(delete)", RemoteRef: "refs/heads/gone"}},
			remote: "origin",
			want:   "",
		},
		{
			name:   "no refs",
			pairs:  nil,
			remote: "origin",
			want:   "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pushTarget(tc.pairs, tc.remote); got != tc.want {
				t.Errorf("pushTarget() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPrintPushReport(t *testing.T) {
	step := func(kind, tool string, status pipeline.Status, affected, total int) pipeline.Step {
		return pipeline.Step{Kind: kind, Tool: tool, Status: status, Affected: affected, Total: total}
	}

	cases := []struct {
		name      string
		steps     []pipeline.Step
		threshold int
		blocked   bool
		target    string
		want      string
	}{
		{
			name: "clean push",
			steps: []pipeline.Step{
				step("format", "gofmt", pipeline.StatusPass, 0, 2),
				step("lint", "golangci-lint", pipeline.StatusPass, 0, 2),
				step("format", "prettier", pipeline.StatusSkipMissing, 0, 1),
				step("lint", "yamllint", pipeline.StatusSkipMissing, 0, 1),
			},
			threshold: 50,
			blocked:   false,
			target:    "test → origin/test",
			want: "✓ Push passed · test → origin/test\n" +
				"  gofmt ✓   golangci-lint ✓   prettier skip   yamllint skip\n",
		},
		{
			name: "lint findings under threshold",
			steps: []pipeline.Step{
				step("format", "gofmt", pipeline.StatusPass, 0, 5),
				step("lint", "golangci-lint", pipeline.StatusFail, 1, 5),
			},
			threshold: 50,
			blocked:   false,
			target:    "test → origin/test",
			want: "⚠ Push allowed with findings · test → origin/test\n" +
				"  gofmt ✓   golangci-lint ⚠ (1 of 5 files)\n",
		},
		{
			name: "blocked by lint",
			steps: []pipeline.Step{
				step("format", "gofmt", pipeline.StatusPass, 0, 5),
				step("lint", "golangci-lint", pipeline.StatusFail, 3, 5),
			},
			threshold: 50,
			blocked:   true,
			target:    "test → origin/test",
			want: "✗ Push blocked · test → origin/test\n" +
				"  gofmt ✓   golangci-lint ✗\n" +
				"  golangci-lint: 3 of 5 files have findings\n",
		},
		{
			name: "blocked by format",
			steps: []pipeline.Step{
				step("format", "gofmt", pipeline.StatusFail, 0, 2),
				step("lint", "golangci-lint", pipeline.StatusPass, 0, 2),
			},
			threshold: 50,
			blocked:   true,
			target:    "test → origin/test",
			want: "✗ Push blocked · test → origin/test\n" +
				"  gofmt ✗   golangci-lint ✓\n" +
				"  gofmt: unformatted files found\n",
		},
		{
			name: "tool error never blocks but stays visible",
			steps: []pipeline.Step{
				step("format", "gofmt", pipeline.StatusPass, 0, 2),
				step("lint", "golangci-lint", pipeline.StatusToolError, 0, 2),
			},
			threshold: 50,
			blocked:   false,
			target:    "test → origin/test",
			want: "✓ Push passed · test → origin/test\n" +
				"  gofmt ✓   golangci-lint error\n" +
				"  golangci-lint: tool error, not counted\n",
		},
		{
			name: "no target",
			steps: []pipeline.Step{
				step("format", "gofmt", pipeline.StatusPass, 0, 1),
			},
			threshold: 50,
			blocked:   false,
			target:    "",
			want: "✓ Push passed\n" +
				"  gofmt ✓\n",
		},
		{
			name: "one blocking lint and one under-threshold lint",
			steps: []pipeline.Step{
				step("format", "gofmt", pipeline.StatusPass, 0, 5),
				step("lint", "golangci-lint", pipeline.StatusFail, 3, 5),
				step("format", "prettier", pipeline.StatusPass, 0, 4),
				step("lint", "eslint", pipeline.StatusFail, 1, 4),
			},
			threshold: 50,
			blocked:   true,
			target:    "test → origin/test",
			want: "✗ Push blocked · test → origin/test\n" +
				"  gofmt ✓   golangci-lint ✗   prettier ✓   eslint ⚠ (1 of 4 files)\n" +
				"  golangci-lint: 3 of 5 files have findings\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			printPushReport(&buf, tc.steps, tc.threshold, tc.blocked, tc.target)
			if got := buf.String(); got != tc.want {
				t.Errorf("report =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}
