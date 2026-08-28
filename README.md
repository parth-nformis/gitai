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

### Git hook (auto commit message)

Install gitai as your repo's `prepare-commit-msg` hook and a plain
`git commit` (no `-m`) opens the editor with an AI-generated message
already in the file. Tweak it if you want, save, done:

```bash
gitai -install-hook
git commit        # editor opens pre-filled
```

The hook only acts for a bare `git commit` — it bails when a message was
given (`-m`, merge, template) or when the message file already holds a
real (non-comment) line. Note that git pre-fills the file with its default
`#` comment block before the hook runs, so the guard tests for actual
content, not mere non-emptiness. Any failure (no API, no staged changes)
just exits 0 so a commit is never blocked. The hook uses the staged diff only (it never runs `git add -A`).
Remove it with: `rm .git/hooks/prepare-commit-msg`.

Per-run settings can be tuned via a `hook` block in
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
│   └── install_hook.go  # `gitai -install-hook`: writes the prepare-commit-msg hook
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
│   └── sanitize.go      # SanitizeCommitMessage: control-char strip, length cap
├── diffprep/            # Diff filtering, stats, and chunking before the LLM call
│   └── preprocess.go    # Noise filtering, truncation, hierarchical chunking
├── prompts/             # System prompts
│   ├── loader.go        # Loads custom prompts from disk, falls back to defaults
│   ├── commit.go        # Default commit system prompt
│   ├── review.go        # Default review system prompt
│   └── pullreq.go       # Default pullreq system prompt
└── install.sh           # Automated install: clone, build, install to /usr/local/bin
```

Adding a new feature = one `commands/<feature>.go` file (implement `Name`, `Diff`, `Run`),
one default prompt in `prompts/`, and one flag in `cmd/main.go`.

## License

MIT
