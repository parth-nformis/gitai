# gitai Documentation

gitai analyzes your git changes and generates **commit messages**, **code
reviews**, and **PR descriptions** using any OpenAI-compatible LLM
(vLLM, Ollama, OpenAI, LiteLLM, ...).

This folder documents the system one concern per file, so you can trace
any behavior from the docs straight into the code. Every diagram node is
labeled with the Go function that implements it.

| Document | What it covers |
|---|---|
| [architecture.md](architecture.md) | Package layout, the end-to-end request flow, where everything lives |
| [commit-message.md](commit-message.md) | Commit message generation: single-call and hierarchical chunked paths |
| [code-review.md](code-review.md) | Code review flow and output format |
| [pull-request.md](pull-request.md) | PR description flow (branch diff) |
| [diff-preprocessing.md](diff-preprocessing.md) | How raw diffs are cleaned before the model sees them |
| [api-client.md](api-client.md) | The API client: streaming, thinking mode, fallbacks, error handling |
| [reasoning.md](reasoning.md) | Muse Glimmer reasoning mode: the strength dial, model matching, CoT handling |
| [configuration.md](configuration.md) | Config file, environment variables, priority rules, custom prompts |
| [extending.md](extending.md) | How to add a new gitai feature (3 touch points) |
| [prepare-commit-msg-hook.md](prepare-commit-msg-hook.md) | The auto-commit-message git hook: design + implementation |

## How to read these docs

1. Start with [architecture.md](architecture.md) for the big picture.
2. Open the document for the behavior you care about.
3. Each document lists the exact functions involved — open them in your
   editor and follow the same order as the diagram.

## Quick orientation: the one-line pipeline

```
git diff  →  handler chooses diff source  →  (optional) diff cleanup  →  LLM  →  output
```

Everything in this repo is a detail of that line.
