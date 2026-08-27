# Configuration

All resolution happens in one place: `config.Load()`
(`config/config.go`). Callers never re-derive precedence.

## Sources

| Source | Where | Notes |
|---|---|---|
| Config file | `~/.gitai/gitai.json` | Created on first install |
| Env vars | `API_BASE`, `API_KEY`, `MODEL` | Supported names |
| Legacy env vars | `GEMINI_API_BASE`, `GEMINI_API_KEY` | Kept for backward compatibility only |
| CLI flags | `--think`, `--reason`, `--branch` | Per-invocation overrides |

## Precedence rules

| Setting | Rule | Why |
|---|---|---|
| `api_base` / `api_key` | config file wins; env vars only fill in when the file has no value | A team's committed config stays stable even on machines with env vars set for other tools |
| model (global) | `MODEL` env var always overrides the config file | "Try a different model once" needs no config edit |
| per-task model | `<task>.model` in the config file | Lets commit use a small fast model and review use a strong one |
| thinking | `--think` seeds the value; `<task>.thinking` wins if set (default: off) | Per-task config is the persistent, task-specific setting; the flag covers "just this run" when no per-task value exists |
| reasoning | `--reason` > `<task>.reasoning` > top-level `reasoning` > unset | The flag is a deliberate per-invocation choice and must beat the file; Muse Glimmer only |

Per-task keys are looked up generically (`<task>.model`,
`<task>.thinking`, `<task>.reasoning`) — adding a new task needs
**no change** to `config.Load`, which is why the config file is the
extensibility point ([extending.md](extending.md)). The
[prepare-commit-msg hook](prepare-commit-msg-hook.md) is a task too: a
`hook` block (`hook.model`, `hook.reasoning`, ...) tunes the hook and is
resolved by the same generic code.

## Example config

```json
{
  "api_base": "http://localhost:8000/v1",
  "api_key": "",
  "model": "Qwen/Qwen3-32B",
  "commit":  { "model": "qwen3-8b",   "thinking": false },
  "review":  { "model": "qwen3-32b", "thinking": true }
}
```

Muse Glimmer uses a reasoning strength instead of a thinking toggle:

```json
{
  "api_base": "http://localhost:8000/v1",
  "model": "Meta/Muse-Glimmer-30B",
  "reasoning": "medium",
  "commit": { "reasoning": "low" }
}
```

Validation: `api_base` must resolve to something, and at least one
model must exist (global or per-task). Otherwise gitai exits with a
pointed-at-file error before doing anything.

## Custom system prompts (hot-reload)

```
~/.gitai/system_prompts/
├── commit.md    # overrides the commit-message prompt
├── review.md    # overrides the code-review prompt
└── pullreq.md   # overrides the PR-description prompt
```

`prompts.LoadSystemPrompt` reads the file on every run; missing file →
built-in default. Edit the `.md`, re-run gitai — no rebuild, no
restart. This is the fastest lever on output quality.
