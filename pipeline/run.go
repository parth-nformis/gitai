package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/parthdande/gitai/spinner"
)

// Status is the outcome state of one check step.
type Status string

const (
	StatusPass         Status = "pass"
	StatusFail         Status = "fail"
	StatusSkipMissing  Status = "skip-missing"  // tool not installed
	StatusSkipDisabled Status = "skip-disabled" // switched off in config
	StatusToolError    Status = "tool-error"    // linter crashed without naming any file
)

// stepTimeout caps a single tool run. A hung tool must not hang a push
// forever; a timed-out step is treated as a failure so it is visible.
const stepTimeout = 5 * time.Minute

// Step is one executed (or skipped) check.
type Step struct {
	Language Language
	Kind     string // "format" or "lint"
	Tool     string
	Status   Status
	Duration time.Duration
	Output   string // raw tool output; the report caps what it shows
	Affected int    // lint only: distinct files with at least one finding
	Total    int    // files the step checked
}

// Options controls which steps run and how lint failures gate the push.
type Options struct {
	Format bool
	Lint   bool
	// Threshold is the percentage of checked files with findings at which
	// the push is blocked; 0 means any finding blocks.
	Threshold int
	// Tools optionally overrides whole commands:
	// lang → kind("format"/"lint") → command line (may include "{files}").
	Tools map[Language]map[string]string
}

// DefaultOptions is the behavior when gitai.json has no pushchecks block.
func DefaultOptions() Options {
	return Options{Format: true, Lint: true, Threshold: 50}
}

// Run executes the pipeline for the given files and returns the ordered
// step results. Each step starts the purple spinner with its own label,
// so a long push reads as "checking go… checking python…" in real time.
func Run(files []string, opts Options) []Step {
	var steps []Step
	for _, lang := range DetectLanguages(files) {
		langFiles := FilesFor(files, lang)
		checks, ok := checksFor(lang)
		if !ok {
			continue
		}
		steps = append(steps,
			runStep(lang, "format", checks.Formatter, langFiles, opts),
			runStep(lang, "lint", checks.Linter, langFiles, opts),
		)
	}
	return steps
}

func runStep(lang Language, kind string, tool Tool, files []string, opts Options) (step Step) {
	enabled := opts.Format
	if kind == "lint" {
		enabled = opts.Lint
	}
	if !enabled {
		return Step{Language: lang, Kind: kind, Tool: tool.Name, Status: StatusSkipDisabled, Total: len(files)}
	}

	// A config override replaces the command entirely; the first field
	// becomes the executable to look up. The built-in tool's FailOnOut
	// semantics survive the override, since they describe how that kind
	// of step reports failure (gofmt-style listers exit 0 with findings).
	if cmd, ok := opts.Tools[lang][kind]; ok {
		if fields := strings.Fields(cmd); len(fields) > 0 {
			tool.Bin = fields[0]
			tool.Args = fields[1:]
		}
	}

	step = Step{Language: lang, Kind: kind, Tool: tool.Name, Total: len(files)}
	if _, err := exec.LookPath(tool.Bin); err != nil {
		step.Status = StatusSkipMissing
		return step
	}

	label := stepLabel(lang, kind, tool.Name)
	sp := spinner.Start(label)
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	var out []byte
	var runErr error
	if tool.PerDirectory {
		out, runErr = runPerDirectory(ctx, tool, files)
	} else {
		out, runErr = exec.CommandContext(ctx, tool.Bin, resolveArgs(tool.Args, files)...).CombinedOutput()
	}
	cancel()
	sp.Stop()
	step.Duration = time.Since(start)
	step.Output = string(out)

	timedOut := runErr != nil && ctx.Err() == context.DeadlineExceeded
	failed := timedOut || runErr != nil || (tool.FailOnOut && strings.TrimSpace(step.Output) != "")
	if failed {
		if timedOut {
			step.Output = fmt.Sprintf("step timed out after %s", stepTimeout)
		}
		if kind == "lint" {
			step.Affected = AffectedFiles(step.Output, lang)
			// A linter that exits non-zero without naming a single file
			// crashed (bad config, version mismatch) instead of reporting
			// findings. Inventing "one affected file" could block the push
			// at high shares, so a token-less lint failure is a tool error:
			// visible, never blocking.
			if step.Affected == 0 {
				step.Status = StatusToolError
				return step
			}
		}
		step.Status = StatusFail
	} else {
		step.Status = StatusPass
	}
	return step
}

// stepLabel is the spinner text for a step, e.g. "Linting go with golangci-lint".
func stepLabel(lang Language, kind, tool string) string {
	verb := "Linting"
	if kind == "format" {
		verb = "Formatting"
	}
	return fmt.Sprintf("%s %s with %s", verb, lang, tool)
}

// resolveArgs expands the "{files}" placeholder; when the args carry no
// placeholder the files are appended, so override commands still receive
// the file list.
func resolveArgs(args []string, files []string) []string {
	var out []string
	hadPlaceholder := false
	for _, a := range args {
		if a == "{files}" {
			out = append(out, files...)
			hadPlaceholder = true
			continue
		}
		out = append(out, a)
	}
	if !hadPlaceholder {
		out = append(out, files...)
	}
	return out
}

// dirGroup is one directory plus the pushed files that live in it. A
// per-directory tool gets one call per group, since such a tool can only
// check files that all share a directory in a single invocation.
type dirGroup struct {
	dir   string
	files []string
}

// groupFilesByDir groups file paths by their parent directory. Order is
// stable: directories appear in the order they are first encountered, and
// files within a group keep their input order.
func groupFilesByDir(files []string) []dirGroup {
	index := map[string]int{}
	var groups []dirGroup
	for _, f := range files {
		d := filepath.Dir(f)
		i, ok := index[d]
		if !ok {
			i = len(groups)
			index[d] = i
			groups = append(groups, dirGroup{dir: d})
		}
		groups[i].files = append(groups[i].files, f)
	}
	return groups
}

// runPerDirectory runs a per-directory tool once per distinct directory the
// files span, concatenating each run's output. It returns the combined
// output plus a non-nil error if any run exited non-zero, so the caller's
// existing failed/affected logic applies to the whole step as one unit.
// The shared ctx bounds the whole step (all directories together) to
// stepTimeout, so a large push can never push a per-directory step past it.
func runPerDirectory(ctx context.Context, tool Tool, files []string) ([]byte, error) {
	var buf bytes.Buffer
	var runErr error
	for _, g := range groupFilesByDir(files) {
		out, err := exec.CommandContext(ctx, tool.Bin, resolveArgs(tool.Args, g.files)...).CombinedOutput()
		buf.Write(out)
		buf.WriteByte('\n')
		if err != nil && runErr == nil {
			runErr = err
		}
	}
	return buf.Bytes(), runErr
}

// Blocked reports whether any step in steps blocks the push: a format
// failure blocks immediately; a lint failure only blocks when the share of
// affected files reaches the threshold.
func Blocked(steps []Step, opts Options) bool {
	for _, s := range steps {
		if s.Status != StatusFail {
			continue
		}
		if s.Kind == "format" || LintBlocked(s.Affected, s.Total, opts.Threshold) {
			return true
		}
	}
	return false
}

// LintBlocked reports whether findings on affected of total checked files
// reach threshold (a percentage; 0 blocks on any finding at all).
func LintBlocked(affected, total, threshold int) bool {
	if affected <= 0 || total <= 0 {
		return false
	}
	if threshold <= 0 {
		return true
	}
	return float64(affected)*100/float64(total) >= float64(threshold)
}

// AffectedFiles counts the distinct files with at least one lint finding by
// scanning the tool output for path tokens carrying this language's
// extension. Linters print "path:line:col: message" but details differ per
// tool, so a tolerant token scan beats a per-tool parser. It returns 0 when
// the output names no file; the caller turns that into a tool error rather
// than a finding.
func AffectedFiles(output string, lang Language) int {
	exts := langExts[lang]
	alts := make([]string, len(exts))
	for i, e := range exts {
		alts[i] = strings.TrimPrefix(regexp.QuoteMeta(e), `\.`)
	}
	re := regexp.MustCompile(`(?i)\S+?\.(` + strings.Join(alts, "|") + `)(?::\d+)?\b`)

	seen := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		for _, m := range re.FindAllString(line, -1) {
			// Cut off a trailing ":line" so the same file reports once.
			if i := strings.Index(m, ":"); i > 0 {
				m = m[:i]
			}
			seen[m] = true
		}
	}
	return len(seen)
}
