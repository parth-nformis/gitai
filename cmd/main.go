package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/parthdande/gitai/client"
	"github.com/parthdande/gitai/commands"
	"github.com/parthdande/gitai/config"
	"github.com/parthdande/gitai/git"
	"github.com/parthdande/gitai/spinner"
)

const (
	// Version is the release version of gitai. Bump manually when
	// releasing: MAJOR.MINOR.PATCH.
	Version = "0.1.0"

	// gitTimeout bounds the whole run (diff fetch + every API call).
	// Hierarchical summarization makes multiple calls for large diffs
	// and local models can be slow, so this needs real headroom.
	gitTimeout = 5 * time.Minute
)

func main() {
	// --- Repo-scoped subcommands (handled before flag parsing) ---
	// `gitai <feature> <verb>` manages this repo's state (hook install,
	// per-repo feature toggles). Handled here so the bare words are never
	// parsed as flags, and so they need no API config.
	if len(os.Args) >= 3 {
		switch os.Args[1] {
		case "hook":
			if os.Args[2] == "install" {
				installHook()
				return
			}
			fmt.Fprintln(os.Stderr, "Usage: gitai hook install")
			os.Exit(1)
		case "commit-msg", "push-check":
			name := "commitmsg"
			if os.Args[1] == "push-check" {
				name = "pushcheck"
			}
			if os.Args[2] == "enable" {
				toggleFeature(name, true)
				return
			}
			if os.Args[2] == "disable" {
				toggleFeature(name, false)
				return
			}
			fmt.Fprintf(os.Stderr, "Usage: gitai %s enable|disable\n", os.Args[1])
			os.Exit(1)
		}
	}

	// --- Read CLI flags ---
	commitMsgFlag := flag.Bool("commitmsg", false, "Generate a commit message from git diff and print it")
	commitFlag := flag.Bool("commit", false, "Generate a commit message and automatically commit all changes")
	reviewFlag := flag.Bool("review", false, "Review git diff for security, quality, and best practices")
	pullreqFlag := flag.Bool("pullreq", false, "Generate a PR description from branch diff")
	prFlag := flag.Bool("pr", false, "Alias for --pullreq")
	updateFlag := flag.Bool("update", false, "Update gitai to the latest version")
	uninstallFlag := flag.Bool("uninstall", false, "Uninstall gitai from the system")
	hookFile := flag.String("commit-msg-file", "", "Hook mode: generate a commit message from the staged diff and write it to this file (internal, used by the installed hook)")
	prePushFlag := flag.Bool("pre-push", false, "Pre-push hook mode: run the check pipeline on the pushed files (internal, used by the installed hook)")
	thinkFlag := flag.Bool("think", false, "Enable extended thinking mode (overrides config)")
	reasonFlag := flag.String("reason", "", "Muse Glimmer reasoning strength: low|medium|high|xhigh")
	branchFlag := flag.String("branch", "main", "Base branch for PR diff (also -b)")
	versionFlag := flag.Bool("version", false, "Print version and exit")
	// -b is registered only so that passing it does not produce a
	// "flag provided but not defined" error; the long form carries
	// the value, so this result is intentionally discarded.
	_ = flag.String("b", "main", "Short alias for --branch")

	flag.Usage = func() {
		fmt.Printf("gitai version %s\n\n", Version)
		fmt.Println(`gitai - AI-assisted git commits, messages, PRs, and code reviews

Repo commands (per-repo state):
  gitai hook install                Install both git hooks in this repo
  gitai commit-msg enable|disable   AI commit messages for this repo
  gitai push-check enable|disable   Pre-push check pipeline for this repo

Usage:
  gitai [flags]

Flags:`)
		flag.PrintDefaults()
		fmt.Println(`
Config (~/.gitai/gitai.json):
  {
    "api_base": "https://...",
    "api_key": "sk-...",
    "reasoning": "high",
    "commit": { "model": "...", "thinking": true, "reasoning": "medium" },
    "review": { "model": "...", "thinking": false }
  }

System prompts: ~/.gitai/system_prompts/<command>.md
  (e.g. commit.md, review.md, pullreq.md) - edit for hot-reload`)
	}
	flag.Parse()

	if *versionFlag {
		fmt.Println(Version)
		return
	}

	// --- Special commands (no config needed) ---
	if *uninstallFlag {
		uninstall()
		return
	}
	if *updateFlag {
		doUpdate()
		return
	}
	// Pre-push hook mode: its own config load (with defaults fallback) and
	// its own exit-code contract — 0 lets the push through, 1 blocks it.
	if *prePushFlag {
		runPrePush()
		return
	}

	// --- Load config from ~/.gitai/gitai.json (with env var overrides) ---
	v, configDir, err := config.Load()
	if err != nil {
		if *hookFile != "" {
			// Hook mode must never block a commit: report and exit 0 so
			// git opens the (empty) message file and the user writes one.
			fmt.Fprintf(os.Stderr, "gitai hook: %v\n", err)
			os.Exit(0)
		}
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}

	// --- Figure out which task to run ---
	var handler commands.Handler
	// Hook mode is checked first: if it ever lost precedence to -commit
	// or -commitmsg, that handler's Diff would run `git add -A` inside the
	// user's own commit and change what gets committed.
	switch {
	case *hookFile != "":
		handler = &commands.Hook{}
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

	// Hook mode: AI commit messages are a per-repo toggle. When it is off
	// for this repo, no-op silently — the hook must never block a commit.
	if *hookFile != "" && !isFeatureOn("commitmsg") {
		os.Exit(0)
	}

	// --- Pick the right model, thinking, and reasoning for this task ---
	// Priority (highest to lowest):
	//   1. CLI flags (--think, --reason)
	//   2. gitai.json <task>.model, <task>.thinking, <task>.reasoning
	//   3. gitai.json model, reasoning (or MODEL env var)

	taskName := handler.Name()
	model := v.GetString("model")
	thinking := *thinkFlag // flag is baseline; per-task config will override if set
	reasoning := *reasonFlag

	// Override with per-task config if set.
	if taskModel := v.GetString(taskName + ".model"); taskModel != "" {
		model = taskModel
	}
	if v.IsSet(taskName + ".thinking") {
		if tv, ok := v.Get(taskName + ".thinking").(bool); ok {
			thinking = tv
		}
	}
	if reasoning == "" {
		if v.IsSet(taskName + ".reasoning") {
			reasoning = v.GetString(taskName + ".reasoning")
		} else if v.IsSet("reasoning") {
			reasoning = v.GetString("reasoning")
		}
	}

	// Validate reasoning strength if set.
	if reasoning != "" {
		if !client.ValidReasoningStrength(reasoning) {
			if *hookFile != "" {
				// Hook mode must never block a commit: drop the invalid
				// value and generate with the default strength instead.
				fmt.Fprintf(os.Stderr, "gitai hook: ignoring invalid reasoning strength %q\n", reasoning)
				reasoning = ""
			} else {
				fmt.Printf("ERROR: invalid reasoning strength '%s'. Must be one of: low, medium, high, xhigh\n", reasoning)
				os.Exit(1)
			}
		}
	}

	// Model-aware notice: reasoning strength is only meaningful for Muse Glimmer.
	if reasoning != "" && !client.IsMuseGlimmer(model) {
		fmt.Fprintf(os.Stderr, "warning: reasoning strength '%s' is set but ignored for model '%s' (only Muse Glimmer uses it)\n", reasoning, model)
	}

	// --- Run the handler ---
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	cli := &client.Client{
		APIBase: v.GetString("api_base"),
		APIKey:  v.GetString("api_key"),
		Model:   model,
	}

	result, err := run(ctx, cli, handler, thinking, reasoning, configDir)
	if err != nil {
		if *hookFile != "" {
			// Hook mode must never block a commit: report and exit 0 so
			// git opens the (empty) message file and the user writes one.
			fmt.Fprintf(os.Stderr, "gitai hook: %v\n", err)
			os.Exit(0)
		}
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if *hookFile != "" {
		// Hook mode: write the message straight into the file git will
		// open in the editor. No banner, no auto-commit — the user sees
		// and approves the message in their own editor.
		sanitized := git.SanitizeCommitMessage(result)
		if err := os.WriteFile(*hookFile, []byte(sanitized+"\n"), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "gitai hook: could not write message file: %v\n", err)
			os.Exit(0)
		}
		os.Exit(0)
	}

	// --- Print the result ---
	fmt.Println()
	fmt.Println("───────────────────────────────────────")
	fmt.Println(result)
	fmt.Println("───────────────────────────────────────")

	// --- Auto-commit if --commit was used ---
	if *commitFlag {
		sanitized := git.SanitizeCommitMessage(result)
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
func run(ctx context.Context, cli *client.Client, h commands.Handler, thinking bool, reasoning string, configDir string) (string, error) {
	// Show a live spinner for the whole run so a slow model doesn't look
	// frozen; it animates on stderr and is cleared when run returns.
	sp := spinner.Start(labelFor(h, cli.Model))
	defer sp.Stop()

	diff, err := h.Diff(ctx)
	if err != nil {
		return "", err
	}
	if diff == "" {
		return "", fmt.Errorf("no changes detected - working tree is clean")
	}

	return h.Run(ctx, cli, diff, cli.Model, thinking, reasoning, configDir)
}

// labelFor builds the spinner label for a task: a short verb plus the model,
// so the user sees both what gitai is doing and which model it is using.
func labelFor(h commands.Handler, model string) string {
	verb := "Working"
	switch h.Name() {
	case "commit", "hook":
		verb = "Generating commit message"
	case "review":
		verb = "Reviewing changes"
	case "pullreq":
		verb = "Drafting PR description"
	}
	return fmt.Sprintf("%s (%s)", verb, model)
}
