package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/parthdande/gitai/client"
	"github.com/parthdande/gitai/commands"
	"github.com/parthdande/gitai/config"
)

const (
	// maxCommitMsgLen caps generated commit messages. Git itself does
	// not hard-limit commit message size, but anything beyond a few KB
	// is almost certainly a model runaway and useless in `git log`, so
	// we truncate well short of where messages become unmanageable.
	maxCommitMsgLen = 7200

	// gitTimeout bounds the whole run (diff fetch + every API call).
	// Hierarchical summarization makes multiple calls for large diffs
	// and local models can be slow, so this needs real headroom.
	gitTimeout = 5 * time.Minute
)

func main() {
	// --- Read CLI flags ---
	commitMsgFlag := flag.Bool("commitmsg", false, "Generate a commit message from git diff and print it")
	commitFlag := flag.Bool("commit", false, "Generate a commit message and automatically commit all changes")
	reviewFlag := flag.Bool("review", false, "Review git diff for security, quality, and best practices")
	pullreqFlag := flag.Bool("pullreq", false, "Generate a PR description from branch diff")
	prFlag := flag.Bool("pr", false, "Alias for --pullreq")
	updateFlag := flag.Bool("update", false, "Update gitai to the latest version")
	uninstallFlag := flag.Bool("uninstall", false, "Uninstall gitai from the system")
	thinkFlag := flag.Bool("think", false, "Enable extended thinking mode (overrides config)")
	branchFlag := flag.String("branch", "main", "Base branch for PR diff (also -b)")
	// -b is registered only so that passing it does not produce a
	// "flag provided but not defined" error; the long form carries
	// the value, so this result is intentionally discarded.
	_ = flag.String("b", "main", "Short alias for --branch")

	flag.Usage = func() {
		fmt.Println(`gitai - AI-assisted git commits, messages, PRs, and code reviews

Usage:
  gitai [flags]

Flags:`)
		flag.PrintDefaults()
		fmt.Println(`
Config (~/.gitai/gitai.json):
  {
    "api_base": "https://...",
    "api_key": "sk-...",
    "commit": { "model": "...", "thinking": true },
    "review": { "model": "...", "thinking": false }
  }

System prompts: ~/.gitai/system_prompts/<command>.md
  (e.g. commit.md, review.md, pullreq.md) - edit for hot-reload`)
	}
	flag.Parse()

	// --- Special commands (no config needed) ---
	if *uninstallFlag {
		uninstall()
		return
	}
	if *updateFlag {
		doUpdate()
		return
	}

	// --- Load config from ~/.gitai/gitai.json (with env var overrides) ---
	v, configDir, err := config.Load()
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}

	// --- Figure out which task to run ---
	var handler commands.Handler
	switch {
	case *commitMsgFlag, *commitFlag:
		handler = &commands.Commit{}
	case *reviewFlag:
		handler = &commands.Review{}
	case *pullreqFlag, *prFlag:
		handler = &commands.PullReq{Base: *branchFlag}
	default:
		flag.Usage()
		return
	}

	// --- Pick the right model and thinking setting for this task ---
	// Priority (highest to lowest):
	//   1. --think flag (only for thinking, not model)
	//   2. gitai.json <task>.model and <task>.thinking
	//   3. gitai.json model (or MODEL env var)

	taskName := handler.Name()
	model := v.GetString("model")
	thinking := *thinkFlag // --think flag is the lowest-level thinking default

	// Override with per-task config if set.
	if taskModel := v.GetString(taskName + ".model"); taskModel != "" {
		model = taskModel
	}
	if v.IsSet(taskName + ".thinking") {
		if tv, ok := v.Get(taskName + ".thinking").(bool); ok {
			thinking = tv
		}
	}

	// --- Run the handler ---
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	cli := &client.Client{
		APIBase: v.GetString("api_base"),
		APIKey:  v.GetString("api_key"),
		Model:   model,
	}

	result, err := run(ctx, cli, handler, thinking, configDir)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// --- Print the result ---
	fmt.Println()
	fmt.Println("───────────────────────────────────────")
	fmt.Println(result)
	fmt.Println("───────────────────────────────────────")

	// --- Auto-commit if --commit was used ---
	if *commitFlag {
		sanitized := sanitizeForGit(result)
		fmt.Println("\nCommitting changes...")
		cmd := exec.CommandContext(ctx, "git", "commit", "-m", sanitized)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Failed to commit: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Successfully committed!")
	}
}

// run is the shared pipeline for every AI task:
//
//  1. ask the handler which diff it operates on (h.Diff),
//  2. refuse to run when there is nothing to analyze,
//  3. hand the diff to the handler (h.Run) and return its output.
//
// Keeping the diff source inside each handler means main never has to
// know the difference between a staged diff and a branch diff.
func run(ctx context.Context, cli *client.Client, h commands.Handler, thinking bool, configDir string) (string, error) {
	fmt.Printf("Running '%s' (model=%s, thinking=%v)...\n", h.Name(), cli.Model, thinking)

	diff, err := h.Diff(ctx)
	if err != nil {
		return "", err
	}
	if diff == "" {
		return "", fmt.Errorf("no changes detected - working tree is clean")
	}

	return h.Run(ctx, cli, diff, cli.Model, thinking, configDir)
}

// sanitizeForGit prepares raw model output for use as a commit message.
//
// Models occasionally emit control characters (unicode escapes, stray
// ANSI codes, null bytes) that are legal in a string but wrong inside
// a commit message — `git log` output gets mangled and some git tools
// reject such messages outright. We keep only printable ASCII plus the
// line break characters, then trim and truncate to maxCommitMsgLen.
func sanitizeForGit(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 0x20 && r <= 0x7E) || r == '\n' || r == '\r' {
			b.WriteRune(r)
		}
	}
	result := strings.TrimSpace(b.String())
	if len(result) > maxCommitMsgLen {
		result = result[:maxCommitMsgLen]
	}
	return result
}
