# gitai

AI-assisted git commits and code reviews. Analyzes your staged changes and generates conventional commit messages or code reviews using **any OpenAI-compatible API** (vLLM, Ollama, OpenAI, LiteLLM, etc.).

---

## Installation

### Automated (Linux & macOS)

Run the install script to compile and place the binary in `/usr/local/bin`:

```bash
bash <(curl -s https://raw.githubusercontent.com/parthdande/gitai/main/install.sh)
```

The script clones the latest code, builds it in a temp directory, and moves the binary to `/usr/local/bin`. Your `~/.gitai/` config is preserved on subsequent runs.

### Manual

```bash
git clone https://github.com/parthdande/gitai.git
cd gitai
go build -o gitai cmd/main.go
sudo mv gitai /usr/local/bin/
```

---

## Configuration

GitAI works with any OpenAI-compatible API endpoint.

### Environment Variables

```bash
# Required: Base URL of your API server
export API_BASE="http://localhost:8000/v1"

# Optional: API key (not needed for local vLLM/Ollama)
export API_KEY="your-key-here"

# Optional: Model name (global fallback)
export MODEL="Qwen/Qwen3-32B"
```

### Config File

Created automatically at `~/.gitai/gitai.json` on first install:

```json
{
  "api_base": "http://localhost:8000/v1",
  "api_key": "",
  "model": "Qwen/Qwen3-32B"
}
```

### Per-task Model, Thinking, and Reasoning

Use different models for commit messages vs code reviews:

```json
{
  "api_base": "https://api.openai.com/v1",
  "api_key": "sk-...",
  "commit": { "model": "gpt-4o-mini", "thinking": false },
  "review": { "model": "gpt-4o", "thinking": true }
}
```

Priority (highest to lowest):
1. `--reason` CLI flag (reasoning only)
2. Per-task config (`commit.model`, `commit.thinking`, `commit.reasoning`, `review.model`, ...)
3. Global config (`model`, `reasoning`)
4. Environment variables (`MODEL`, `API_BASE`, `API_KEY`)

Note: for thinking, a per-task `<task>.thinking` value overrides the
`--think` flag when both are present.

### Example Configurations

**vLLM (self-hosted):**
```json
{
  "api_base": "http://localhost:8000/v1",
  "model": "meta-llama/Llama-3.3-70B-Instruct"
}
```

**Ollama:**
```json
{
  "api_base": "http://localhost:11434/v1",
  "model": "qwen3:32b"
}
```

**OpenAI:**
```json
{
  "api_base": "https://api.openai.com/v1",
  "api_key": "sk-...",
  "model": "gpt-4o-mini"
}
```

---

## Usage

Navigate to any Git repository and run `gitai` with the following flags:

### Generate Commit Message

Prints a suggested commit message from your staged changes:

```bash
gitai -commitmsg
```

### Auto-commit

Generates a commit message and commits all staged changes:

```bash
gitai -commit
```

### Code Review

Reviews staged changes for security, quality, and best practices:

```bash
gitai -review
```

### Thinking Mode

Enable extended thinking for models that support it (e.g. DeepSeek). Falls back automatically if the model doesn't support it:

```bash
gitai -commit -think
gitai -review -think
```

Or enable permanently in `~/.gitai/gitai.json`:

```json
{
  "commit": { "thinking": true },
  "review": { "thinking": false }
}
```

### Reasoning Mode (Muse Glimmer)

Muse Glimmer is an always-on reasoning model: it cannot be switched
off, only tuned. `-reason` sets the reasoning strength
(`low | medium | high | xhigh`) and gitai injects it into the system
prompt; it also stops sending the `enable_thinking` kwarg that the
model ignores. On any other model, `-reason` prints a one-time warning
and is ignored:

```bash
gitai -commitmsg -reason low
```

Or permanently in `~/.gitai/gitai.json`:

```json
{
  "model": "Meta/Muse-Glimmer-30B",
  "reasoning": "medium",
  "commit": { "reasoning": "low" }
}
```

For best results serve Muse Glimmer with `--reasoning-parser
muse_glimmer` (vLLM) so the private chain of thought stays out of the
output.

### Custom System Prompts

Override the built-in prompts by placing `.md` files in `~/.gitai/system_prompts/`:

```
~/.gitai/system_prompts/
├── commit.md   # Overrides commit message generation prompt
└── review.md   # Overrides code review prompt
```

Edit the files and the next `gitai` run picks them up — no rebuild needed. If a file is missing, gitai falls back to its built-in default.

### Git hooks

`gitai hook install` installs **two** hooks in the current repo (existing
hooks are backed up to `.bak` first). Both are off by default and toggle
per repo, stored in the repo's `.git/` (never in git config):

```bash
gitai hook install         # install both hooks (generic, per repo)

# 1) AI commit messages (prepare-commit-msg hook)
gitai commit-msg enable    # turn AI commit messages ON for this repo
git commit                 # editor opens pre-filled

# 2) Push checks (pre-push hook)
gitai push-check enable    # the next `git push` runs the check pipeline
```

**AI commit messages** only act for a bare `git commit` — they bail when a
message was given (`-m`, merge, template) or when the message file already
holds a real (non-comment) line (git pre-fills its default `#` comment
block, so the guard tests for actual content, not mere non-emptiness).
Any failure (no API, no staged changes, feature off) exits 0 so a commit
is never blocked. The hook uses the staged diff only (it never runs
`git add -A`). Remove it with `rm .git/hooks/prepare-commit-msg`.

**Push checks** run a quality pipeline on `git push`: only the files in
the push are checked, one formatter + one linter per detected language
(go, python, node, shell, html, yaml), each step shown live with the
spinner. A format failure blocks the push immediately; lint blocks only
when findings affect ≥ 50% of the checked files (configurable, `0` = any
finding blocks). Missing tools warn and skip — they never block — and
gitai's own failures never block a push. Tune it via the `pushchecks`
block in `~/.gitai/gitai.json` (threshold, format/lint toggles, tool
overrides). See [docs/push-pipeline.md](docs/push-pipeline.md) for the
full design. Remove it with `rm .git/hooks/pre-push` (or just
`gitai push-check disable`).

Per-run AI settings can be tuned via a `hook` block in
`~/.gitai/gitai.json` (`hook.model`, `hook.thinking`, `hook.reasoning`),
same as any other task.

### Update to Latest Version

```bash
gitai -update
```

Downloads and runs the install script to replace the binary. Config at `~/.gitai/` is preserved.

### Uninstall

Removes the `gitai` binary (requires sudo):

```bash
sudo gitai -uninstall
```

---

## Documentation

In-depth documentation with diagrams lives in [`docs/`](docs/README.md) —
architecture, per-feature flows, diff preprocessing, the API client,
configuration, and how to extend gitai with new features.

## Architecture

```
gitai/
├── cmd/                 # CLI entry point
│   ├── main.go          # Flags, task dispatch, model resolution, auto-commit
│   ├── update.go        # Self-update handler
│   ├── uninstall.go     # Uninstall handler
│   ├── install_hook.go  # `gitai hook install`: writes both hooks (prepare-commit-msg + pre-push)
│   ├── hook_toggle.go   # Per-repo feature toggles (`commit-msg`/`push-check` enable|disable)
│   └── push_check.go    # `-pre-push` mode: ref parsing, option loading, step report
├── client/              # HTTP client for OpenAI-compatible APIs
│   ├── client.go        # Client struct (api_base, api_key, model, http client)
│   ├── api_call.go      # API call logic, thinking + reasoning mode, auto-fallback
│   └── reasoning.go     # Muse Glimmer: model matching + strength injection
├── commands/            # Feature handlers (commit, review, pullreq, hook)
│   ├── handler.go       # Handler interface (Name + Diff + Run)
│   ├── commit.go        # Commit message generation
│   ├── review.go        # Code review generation
│   ├── pullreq.go       # PR description generation
│   └── hook.go          # Hook mode: commit pipeline + staged diff (no side effects)
├── config/              # Config loading (~/.gitai/gitai.json + env overrides)
│   └── config.go        # Load() with validation
├── git/                 # Git plumbing
│   ├── git.go           # Diff sources: StagedDiff, CommitAllDiff, BranchDiff
│   ├── sanitize.go      # SanitizeCommitMessage: control-char strip, length cap
│   └── push_files.go    # PushFiles: which files a push changes (diff / merge-base / deletions)
├── pipeline/            # The pre-push check pipeline (push-checks)
│   ├── detect.go        # Language detection by file extension (go, python, node, shell, html, yaml)
│   ├── tools.go         # Default formatter + linter per language, FailOnOut semantics
│   └── run.go           # Step runner (spinner + timeout), threshold blocking, affected-file counting
├── diffprep/            # Diff filtering, stats, and chunking before the LLM call
│   └── preprocess.go    # Noise filtering, truncation, hierarchical chunking
├── prompts/             # System prompts
│   ├── loader.go        # Loads custom prompts from disk, falls back to defaults
│   ├── commit.go        # Default commit system prompt
│   ├── review.go        # Default review system prompt
│   └── pullreq.go       # Default pullreq system prompt
├── spinner/             # Purple braille-dot loading spinner shown while the AI runs
│   └── spinner.go       # Start/Stop/Note; TTY-aware, writes to stderr only
└── install.sh           # Automated install: clone, build, install to /usr/local/bin
```

Adding a new feature = one `commands/<feature>.go` file (implement `Name`, `Diff`, `Run`),
one default prompt in `prompts/`, and one flag in `cmd/main.go`.

## Design notes (scalability)

The package graph is layered and acyclic — nothing imports "upward":

```
cmd  →  commands  →  client / diffprep / git / prompts  →  spinner, config
```

- **New AI feature** = handler in `commands/` + default prompt in `prompts/`
  + one flag (see [docs/extending.md](docs/extending.md)).
- **New hook feature** = per-repo marker in `<git-dir>/gitai/` + hook script
  + a mode function; the toggle plumbing (`isFeatureOn`, marker helpers in
  `cmd/hook_toggle.go`) is name-parameterized, so a third feature adds no
  shared code.
- **Watch points (no action needed yet):** `cmd/` accumulates the shared
  hook plumbing (marker paths, script writing, `.bak` backups) — at a third
  hook feature, extract that into a `hook/` package. `loadPushCheckOptions`
  reads the `pushchecks` config block by string keys — worth a typed struct
  if per-language tool config grows.

## License

MIT
