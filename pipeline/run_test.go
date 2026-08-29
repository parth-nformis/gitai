package pipeline

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLintBlocked(t *testing.T) {
	tests := []struct {
		affected, total, threshold int
		want                       bool
	}{
		{0, 10, 50, false}, // no findings never block
		{1, 10, 50, false}, // 10% < 50%
		{5, 10, 50, true},  // 50% = 50%
		{6, 10, 50, true},  // 60% > 50%
		{1, 1, 50, true},   // single file with a finding
		{1, 4, 50, false},  // 25% < 50%
		{2, 4, 50, true},   // 50% = 50%
		{1, 10, 0, true},   // threshold 0 = block on any finding
		{0, 10, 0, false},  // ...but still not with zero findings
		{1, 0, 50, false},  // no checked files
	}
	for _, tt := range tests {
		got := LintBlocked(tt.affected, tt.total, tt.threshold)
		if got != tt.want {
			t.Errorf("LintBlocked(%d,%d,%d) = %v, want %v", tt.affected, tt.total, tt.threshold, got, tt.want)
		}
	}
}

func TestAffectedFiles(t *testing.T) {
	tests := []struct {
		name   string
		output string
		lang   Language
		want   int
	}{
		{
			"golangci-lint style",
			"main.go:12:3: unused variable\nmain.go:15:1: error return\nutil.go:7:1: shadowed var\n3 issues:\n",
			LangGo, 2,
		},
		{
			"ruff style",
			"app.py:10:5: F401 'os' imported but unused\napp.py:20:1: E501 line too long\nworker.py:1:1: F841 unused\nFound 3 errors.\n",
			LangPython, 2,
		},
		{
			"eslint style",
			"./src/a.js: 10:5  error  'x' is not defined  no-undef\n./src/b.ts: 1:1  error  missing semicolon  semi\n\n2 problems\n",
			LangNode, 2,
		},
		{
			"shellcheck style",
			"In run.sh line 3:\n  cmd $var\n     ^ SC2086: Double quote\n\nIn run.sh line 9:\n     ^ SC2155\nrun.sh: 2 errors\n",
			LangShell, 1,
		},
		{
			"yamllint style",
			"config.yml:1:0: [error] missing document start\nconfig.yml:4:12: [warning] line too long\n",
			LangYAML, 1,
		},
		{
			"no file tokens means zero",
			"fatal: internal tool error\n",
			LangGo, 0,
		},
		{
			"summary-only line counts its file",
			"Found 2 files with warnings: main.go, util.go\n",
			LangGo, 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AffectedFiles(tt.output, tt.lang); got != tt.want {
				t.Errorf("AffectedFiles() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestResolveArgs(t *testing.T) {
	if got := resolveArgs([]string{"-l", "{files}"}, []string{"a.go", "b.go"}); !reflect.DeepEqual(got, []string{"-l", "a.go", "b.go"}) {
		t.Errorf("placeholder replacement = %v", got)
	}
	if got := resolveArgs([]string{"run", "--all"}, []string{"a.go"}); !reflect.DeepEqual(got, []string{"run", "--all", "a.go"}) {
		t.Errorf("append when no placeholder = %v", got)
	}
}

func TestGroupFilesByDir(t *testing.T) {
	groups := groupFilesByDir([]string{"a/x.go", "b/y.go", "a/z.go", "top.go"})
	want := []dirGroup{
		{dir: "a", files: []string{"a/x.go", "a/z.go"}},
		{dir: "b", files: []string{"b/y.go"}},
		{dir: ".", files: []string{"top.go"}},
	}
	if !reflect.DeepEqual(groups, want) {
		t.Errorf("groupFilesByDir() = %+v, want %+v", groups, want)
	}
}

// fakeTool writes an executable script into dir under the given name and
// returns its path; the caller prepends dir to PATH for a test run.
func fakeTool(t *testing.T, dir, name, script string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// TestRunEndToEnd exercises Run() with fake tools on PATH: the format step
// passes, the lint step fails with output naming two files, and the
// skip-missing path is hit by a tool that does not exist.
func TestRunEndToEnd(t *testing.T) {
	dir := t.TempDir()
	// Silent: the go format entry is FailOnOut (gofmt semantics), so any
	// output from the formatter would fail the step.
	fakeTool(t, dir, "passtool", "#!/bin/sh\nexit 0\n")
	fakeTool(t, dir, "failtool", "#!/bin/sh\necho 'main.go:12:3: unused variable'\necho 'util.go:7:1: shadowed var'\nexit 1\n")

	orig := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(filepath.ListSeparator)+orig)

	opts := Options{Format: true, Lint: true, Threshold: 50}
	opts.Tools = map[Language]map[string]string{
		LangGo: {
			"format": "passtool {files}",
			"lint":   "failtool {files}",
		},
	}

	steps := Run([]string{"main.go", "util.go"}, opts)
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d: %+v", len(steps), steps)
	}
	if steps[0].Status != StatusPass || steps[0].Kind != "format" {
		t.Errorf("format step = %+v, want pass", steps[0])
	}
	if steps[1].Status != StatusFail || steps[1].Kind != "lint" {
		t.Errorf("lint step = %+v, want fail", steps[1])
	}
	if steps[1].Affected != 2 || steps[1].Total != 2 {
		t.Errorf("lint affected/total = %d/%d, want 2/2", steps[1].Affected, steps[1].Total)
	}
	if !Blocked(steps, opts) {
		t.Error("expected push blocked (2/2 lint-affected >= 50%)")
	}
}

// crashTool pins the tool-error path: a linter that exits 1 without naming
// any file (config error, version mismatch) must not be counted as a
// finding — even at threshold 0 it must not block.
func TestRunToolError(t *testing.T) {
	dir := t.TempDir()
	fakeTool(t, dir, "crashtool", "#!/bin/sh\necho \"Error: cannot load config: go version mismatch\"\nexit 1\n")
	orig := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(filepath.ListSeparator)+orig)

	opts := Options{Format: true, Lint: true, Threshold: 0}
	opts.Tools = map[Language]map[string]string{
		LangGo: {
			"format": "definitely-not-installed-tool {files}",
			"lint":   "crashtool {files}",
		},
	}
	steps := Run([]string{"main.go"}, opts)
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if steps[1].Status != StatusToolError {
		t.Errorf("lint step = %v, want tool-error", steps[1].Status)
	}
	if Blocked(steps, opts) {
		t.Error("a crashed linter must never block the push")
	}
}

func TestRunSkipMissing(t *testing.T) {
	opts := Options{Format: true, Lint: true, Threshold: 50}
	// Both steps point at a nonexistent binary so the test holds no matter
	// what linters the machine actually has installed.
	opts.Tools = map[Language]map[string]string{
		LangGo: {
			"format": "definitely-not-installed-tool {files}",
			"lint":   "also-not-installed-tool {files}",
		},
	}
	steps := Run([]string{"main.go"}, opts)
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	for _, s := range steps {
		if s.Status != StatusSkipMissing {
			t.Errorf("%s step = %v, want skip-missing", s.Kind, s.Status)
		}
	}
	if Blocked(steps, opts) {
		t.Error("missing tools must never block the push")
	}
}

func TestRunDisabledSteps(t *testing.T) {
	opts := Options{Format: false, Lint: false, Threshold: 50}
	steps := Run([]string{"main.go"}, opts)
	for _, s := range steps {
		if s.Status != StatusSkipDisabled {
			t.Errorf("%s step = %v, want skip-disabled", s.Kind, s.Status)
		}
	}
}

// failOnError pins the FailOnOut contract: a tool that exits 0 but prints
// findings must fail the step (gofmt / shfmt behave exactly this way).
func TestRunFailOnOut(t *testing.T) {
	dir := t.TempDir()
	fakeTool(t, dir, "listingtool", "#!/bin/sh\necho 'main.go'\nexit 0\n")
	orig := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(filepath.ListSeparator)+orig)

	tool := Tool{Name: "listingtool", Bin: "listingtool", Args: []string{"{files}"}, FailOnOut: true}
	step := runStep(LangGo, "format", tool, []string{"main.go"}, Options{Format: true})
	if step.Status != StatusFail {
		t.Errorf("FailOnOut step = %v, want fail (tool printed a filename but exited 0)", step.Status)
	}

	same := tool
	same.FailOnOut = false
	step = runStep(LangGo, "format", same, []string{"main.go"}, Options{Format: true})
	if step.Status != StatusPass {
		t.Errorf("exit-code-based step = %v, want pass", step.Status)
	}
}

// TestRunPerDirectory pins the per-directory contract: a PerDirectory tool
// given files spanning two directories runs once per directory, and each
// call receives only that directory's files.
func TestRunPerDirectory(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "calls.txt")
	fakeTool(t, dir, "logtool", "#!/bin/sh\necho \"CALL: $@\" >> \"$CALLLOG\"\nexit 0\n")

	orig := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(filepath.ListSeparator)+orig)
	t.Setenv("CALLLOG", logFile)

	tool := Tool{Name: "logtool", Bin: "logtool", Args: []string{"run", "{files}"}, PerDirectory: true}
	step := runStep(LangGo, "lint", tool, []string{"a/x.go", "b/y.go", "a/z.go"}, Options{Lint: true})
	if step.Status != StatusPass {
		t.Fatalf("step = %v, want pass", step.Status)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read call log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := []string{
		"CALL: run a/x.go a/z.go",
		"CALL: run b/y.go",
	}
	if !reflect.DeepEqual(lines, want) {
		t.Errorf("calls = %q, want %q", lines, want)
	}
}
