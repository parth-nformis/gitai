# Configuration

All resolution happens in one place: `config.Load()`
(`config/config.go`). Callers never re-derive precedence.

## Sources

| Source | Where | Notes |
|---|---|---|
| Config file | `~/.gitai/gitai.json` | Created on first install |
| Env vars | `API_BASE`, `API_KEY`, `MODEL` | Supported names |
| Legacy env vars | `GEMINI_API_BASE`, `GEMINI_API_KEY` | Kept for backward compatibility only |
| CLI flags | `--think`, `--branch` | Per-invocation overrides |

## Precedence rules

| Setting | Rule | Why |
|---|---|---|
| `api_base` / `api_key` | config file wins; env vars only fill in when the file has no value | A team's committed config stays stable even on machines with env vars set for other tools |
| model (global) | `MODEL` env var always overrides the config file | "Try a different model once" needs no config edit |
| per-task model | `<task>.model` in the config file | Lets commit use a small fast model and review use a strong one |
| thinking | `--think` flag > `<task>.thinking` > (no default: off) | The flag is the loudest, most deliberate signal |

Per-task keys are looked up generically (`<task>.model`,
`<task>.thinking`) — adding a new task needs **no change** to
`config.Load`, which is why the config file is the extensibility point
([extending.md](extending.md)).

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
