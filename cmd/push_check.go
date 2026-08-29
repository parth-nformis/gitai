package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/parthdande/gitai/config"
	"github.com/parthdande/gitai/git"
	"github.com/parthdande/gitai/pipeline"
)

// runPrePush is the -pre-push mode, invoked from the pre-push hook that
// `gitai hook install` installs. git passes the remote name and URL as
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
	target := pushTarget(pairs, hookRemoteName())
	if pipeline.Blocked(steps, opts) {
		printPushReport(os.Stdout, steps, opts.Threshold, true, target)
		os.Exit(1)
	}
	printPushReport(os.Stdout, steps, opts.Threshold, false, target)
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
		pairs = append(pairs, git.RefPair{LocalRef: f[0], LocalSHA: f[1], RemoteRef: f[2], RemoteSHA: f[3]})
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
		if a == "-pre-push" && i+1 < len(os.Args) && !strings.HasPrefix(os.Args[i+1], "-") {
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

// printPushReport renders the compact check report: a verdict that answers
// "did the push go out?" on line 1, a one-line check table, and a count
// line for each blocking failure. Raw tool output is never shown — the
// user can re-run the tool for details.
func printPushReport(w io.Writer, steps []pipeline.Step, threshold int, blocked bool, target string) {
	verdict := "✓ Push passed"
	switch {
	case blocked:
		verdict = "✗ Push blocked"
	case lintUnderThreshold(steps, threshold):
		verdict = "⚠ Push allowed with findings"
	}
	if target != "" {
		verdict += " · " + target
	}
	fmt.Fprintln(w, verdict)

	entries := make([]string, 0, len(steps))
	for _, s := range steps {
		entries = append(entries, tableEntry(s, threshold))
	}
	fmt.Fprintf(w, "  %s\n", strings.Join(entries, "   "))

	for _, s := range steps {
		switch {
		case s.Status == pipeline.StatusFail && s.Kind == "format":
			fmt.Fprintf(w, "  %s: unformatted files found\n", s.Tool)
		case s.Status == pipeline.StatusFail && pipeline.LintBlocked(s.Affected, s.Total, threshold):
			fmt.Fprintf(w, "  %s: %d of %d files have findings\n", s.Tool, s.Affected, s.Total)
		case s.Status == pipeline.StatusToolError:
			fmt.Fprintf(w, "  %s: tool error, not counted\n", s.Tool)
		}
	}
}

// tableEntry renders one check-table cell: the tool name plus its outcome
// mark.
func tableEntry(s pipeline.Step, threshold int) string {
	switch s.Status {
	case pipeline.StatusPass:
		return s.Tool + " ✓"
	case pipeline.StatusFail:
		if s.Kind == "format" || pipeline.LintBlocked(s.Affected, s.Total, threshold) {
			return s.Tool + " ✗"
		}
		return fmt.Sprintf("%s ⚠ (%d of %d files)", s.Tool, s.Affected, s.Total)
	case pipeline.StatusToolError:
		return s.Tool + " error"
	default:
		return s.Tool + " skip"
	}
}

// lintUnderThreshold reports whether a lint step failed but stayed below
// the blocking threshold — the amber "allowed with findings" verdict.
func lintUnderThreshold(steps []pipeline.Step, threshold int) bool {
	for _, s := range steps {
		if s.Kind == "lint" && s.Status == pipeline.StatusFail && !pipeline.LintBlocked(s.Affected, s.Total, threshold) {
			return true
		}
	}
	return false
}

// pushTarget renders the "test → origin/test" target shown after the
// verdict. Multiple refs collapse to the bare remote name; deletion refs
// contribute nothing.
func pushTarget(pairs []git.RefPair, remote string) string {
	var refs []git.RefPair
	for _, p := range pairs {
		if p.LocalRef != "" && p.LocalRef != "(delete)" {
			refs = append(refs, p)
		}
	}
	if len(refs) == 0 {
		return ""
	}
	if len(refs) == 1 {
		return fmt.Sprintf("%s → %s/%s", shortRef(refs[0].LocalRef), remote, shortRef(refs[0].RemoteRef))
	}
	names := make([]string, len(refs))
	for i, p := range refs {
		names[i] = shortRef(p.LocalRef)
	}
	return fmt.Sprintf("%s → %s", strings.Join(names, ", "), remote)
}

// shortRef strips a ref's namespace: refs/heads/test → test,
// refs/tags/v1.0 → v1.0.
func shortRef(ref string) string {
	rest := strings.TrimPrefix(ref, "refs/")
	if i := strings.Index(rest, "/"); i > 0 {
		return rest[i+1:]
	}
	return rest
}
