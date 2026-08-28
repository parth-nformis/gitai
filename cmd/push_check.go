package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/parthdande/gitai/config"
	"github.com/parthdande/gitai/git"
	"github.com/parthdande/gitai/pipeline"
)

// runPrePush is the -prepush mode, invoked from the pre-push hook that
// `gitai -install-hook` installs. git passes the remote name and URL as
// hook arguments and writes the refs being pushed on stdin, one line per
// ref: "<local ref> <local object> <remote ref> <remote object>".
//
// The exit code decides the push: 0 lets it proceed, 1 blocks it. gitai's
// own failures never block — only a real check failure does, so a broken
// gitai can never trap a user's push.
func runPrePush() {
	if !isFeatureOn("pushcheck") {
		os.Exit(0)
	}
	pairs, err := readPushRefs(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gitai push-check: %v\n", err)
		os.Exit(0)
	}
	files, err := git.PushFiles(pairs, hookRemoteName())
	if err != nil {
		fmt.Fprintf(os.Stderr, "gitai push-check: %v\n", err)
		os.Exit(0)
	}
	if len(files) == 0 {
		os.Exit(0) // ref deletion or no changed files
	}

	opts := loadPushCheckOptions()
	steps := pipeline.Run(files, opts)
	if len(steps) == 0 {
		os.Exit(0) // no supported language in the push
	}
	if pipeline.Blocked(steps, opts) {
		printPushReport(files, steps, opts, true)
		os.Exit(1)
	}
	printPushReport(files, steps, opts, false)
	os.Exit(0)
}

// readPushRefs parses the pre-push ref list (r is the hook's stdin). git
// writes one line per pushed ref:
//
//	<local ref> SP <local object> SP <remote ref> SP <remote object>
//
// A brand-new remote ref carries the all-zero remote object; a ref deletion
// carries "(delete)" as the local ref and the all-zero local object.
func readPushRefs(r io.Reader) ([]git.RefPair, error) {
	var pairs []git.RefPair
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 4 {
			continue
		}
		pairs = append(pairs, git.RefPair{LocalSHA: f[1], RemoteSHA: f[3]})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return pairs, nil
}

// hookRemoteName is the remote the push targets, passed to the hook script
// as $1 by git and forwarded as a positional arg to gitai.
func hookRemoteName() string {
	remote := "origin"
	for i, a := range os.Args {
		if a == "-prepush" && i+1 < len(os.Args) && !strings.HasPrefix(os.Args[i+1], "-") {
			remote = os.Args[i+1]
			break
		}
	}
	return remote
}

// loadPushCheckOptions reads the pushchecks block of gitai.json; a broken
// or missing config falls back to defaults rather than blocking the push.
func loadPushCheckOptions() pipeline.Options {
	opts := pipeline.DefaultOptions()
	v, _, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gitai push-check: config not loaded (%v); using defaults\n", err)
		return opts
	}
	if v.IsSet("pushchecks.threshold") {
		opts.Threshold = v.GetInt("pushchecks.threshold")
	}
	if v.IsSet("pushchecks.format") {
		opts.Format = v.GetBool("pushchecks.format")
	}
	if v.IsSet("pushchecks.lint") {
		opts.Lint = v.GetBool("pushchecks.lint")
	}
	for _, lang := range pipeline.AllLanguages() {
		for _, kind := range []string{"format", "lint"} {
			key := "pushchecks.tools." + string(lang) + "." + kind
			if v.IsSet(key) {
				if opts.Tools == nil {
					opts.Tools = map[pipeline.Language]map[string]string{}
				}
				if opts.Tools[lang] == nil {
					opts.Tools[lang] = map[string]string{}
				}
				opts.Tools[lang][kind] = v.GetString(key)
			}
		}
	}
	return opts
}

// printPushReport renders the per-step report. blocked is passed so the
// summary line matches the exit decision made in runPrePush.
func printPushReport(files []string, steps []pipeline.Step, opts pipeline.Options, blocked bool) {
	langs := pipeline.DetectLanguages(files)
	names := make([]string, len(langs))
	for i, l := range langs {
		names[i] = string(l)
	}
	unit := "files"
	if len(files) == 1 {
		unit = "file"
	}
	fmt.Printf("gitai push-checks (%d %s: %s)\n", len(files), unit, strings.Join(names, ", "))

	skipped := 0
	for _, s := range steps {
		switch s.Status {
		case pipeline.StatusPass:
			fmt.Printf("  ✓ %s %s (%s)\n", s.Kind, s.Tool, dur(s.Duration))
		case pipeline.StatusFail:
			line := fmt.Sprintf("  ✗ %s %s (%s)", s.Kind, s.Tool, dur(s.Duration))
			if s.Kind == "lint" {
				line += fmt.Sprintf(" — %d of %d checked files have findings", s.Affected, s.Total)
			}
			fmt.Println(line)
			for _, l := range capLines(s.Output, 20) {
				fmt.Printf("      %s\n", l)
			}
			if s.Kind == "format" {
				fmt.Println("      format failure blocks the push (fix the files or set pushchecks.format to false)")
			} else if !pipeline.LintBlocked(s.Affected, s.Total, opts.Threshold) {
				fmt.Printf("      below the %d%% threshold — warning only\n", opts.Threshold)
			}
		case pipeline.StatusSkipMissing:
			skipped++
			fmt.Printf("  ⚠ %s %s not installed — skipped (install it to enable this step)\n", s.Kind, s.Tool)
		case pipeline.StatusToolError:
			skipped++
			fmt.Printf("  ⚠ %s %s failed to run — tool error, not counted\n", s.Kind, s.Tool)
			for _, l := range capLines(strings.TrimRight(s.Output, "\n"), 5) {
				fmt.Printf("      %s\n", l)
			}
		}
	}

	lintWarned := false
	for _, s := range steps {
		if s.Kind == "lint" && s.Status == pipeline.StatusFail && !pipeline.LintBlocked(s.Affected, s.Total, opts.Threshold) {
			lintWarned = true
		}
	}

	fmt.Println()
	switch {
	case blocked:
		fmt.Println("Push blocked: fix the findings above and push again.")
	case lintWarned:
		if skipped > 0 {
			fmt.Printf("Push allowed: lint findings below the %d%% threshold (%d steps skipped).\n", opts.Threshold, skipped)
		} else {
			fmt.Printf("Push allowed: lint findings below the %d%% threshold.\n", opts.Threshold)
		}
	case skipped > 0:
		fmt.Printf("Checks passed (%d steps skipped).\n", skipped)
	default:
		fmt.Println("All checks passed.")
	}
}

func dur(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return d.Round(time.Millisecond).String()
}

// capLines trims output to n lines for the terminal report, noting how much
// was cut so nothing looks silently swallowed.
func capLines(s string, n int) []string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) == 0 {
		return nil
	}
	if len(lines) > n {
		out := append(lines[:n], fmt.Sprintf("… (%d more lines)", len(lines)-n))
		return out
	}
	return lines
}
