# Design: `prepare-commit-msg` Hook

Status: **implemented** (see the "Implementation" section at the bottom for
how the built version maps onto this design).

## Goal

Type `git commit` (no `-m`) and your editor opens with an
AI-generated commit message already in the file. Tweak it if you want,
save, and the commit proceeds. No new command in the user's muscle
memory; the feature is invisible until they use it.

## How git hooks fit

Git runs `.git/hooks/prepare-commit-msg` right before opening the
commit message editor, with:

| Arg | Meaning |
|---|---|
| `$1` | Path to the commit message file (e.g. `.git/COMMIT_EDITMSG`) |
| `$2` | Source: `message`, `template`, `merge`, `copy` — **empty for a plain `git commit`** |
| `$3` | SHA (only when committing an amend) |

The hook must **only act when `$2` is empty** — that is the one case
where the user typed `git commit` with no message and no `-m`.

## Flow

```mermaid
sequenceDiagram
    participant U as User
    participant G as git
    participant H as hook script
    participant A as gitai
    participant L as LLM

    U->>G: git commit (no -m)
    G->>H: prepare-commit-msg (msgfile, source="")
    H->>H: skip if source != "" or file non-empty
    H->>A: gitai -hook <msgfile>
    A->>A: git.StagedDiff (NO side effects)
    A->>L: generate commit message
    L-->>A: message text
    A->>H: message written to <msgfile>
    G->>U: editor opens with pre-filled message
    U->>G: save (edits optional)
    G->>G: commit completes
```

## Critical constraint: no side effects on the index

At hook time the user may have **deliberately staged a subset** of
their changes. The shared commit path uses
`git.CommitAllDiff` (which runs `git add -A`) — that is exactly wrong
inside a hook. The hook must use `git.StagedDiff`
(`git/git.go`), which reads the staged diff without touching the index.

This is the specific reason `git.StagedDiff` exists in the codebase
today even though no caller uses it yet.

## Implementation sketch

| Piece | Shape |
|---|---|
| `gitai -install-hook` | New flag: writes a small shell script to `.git/hooks/prepare-commit-msg`, `chmod +x`, per repository |
| Hook script | `[ -n "$2" ] && exit 0; [ -s "$1" ] && exit 0; gitai -hook "$1" 2>/dev/null \|\| exit 0` — failures must never block a commit |
| `gitai -hook <file>` | New mode: `Commit` handler + `git.StagedDiff` + write result to the file instead of stdout (no decorative banners, no auto-commit) |
| Empty staged diff | Hook exits silently — normal `git commit` flow continues |

## Safety properties

- Never modifies the index or working tree.
- Never blocks a commit: any error (no API, no model, no staged
  changes) → exit 0, git proceeds normally.
- Never overwrites: only fills an empty message file from an empty
  source.
- User always sees the message in their editor before it is committed.

## Implementation (as built)

| Piece | Where | Notes |
|---|---|---|
| `gitai -install-hook` | `cmd/install_hook.go` | Resolves the git dir via `git rev-parse --git-dir` (works from subdirs); embeds the **absolute** gitai path (`os.Executable`) in the script so it survives `$PATH` changes; backs up an existing `prepare-commit-msg` to `.bak` (a pre-existing `.bak` is **kept**, never clobbered — it may hold a hand-written hook); `chmod 0755`; warns when `core.hooksPath` is set, since that shadows `.git/hooks`. Early-returns in `main` before `config.Load` — installing needs no API. |
| `gitai -hook <file>` | `commands/hook.go` + `cmd/main.go` | `Hook` embeds `Commit` and overrides only `Diff` → `git.StagedDiff`. `Run` (prompt, `diffprep`, generate) is promoted from `Commit` unchanged. `main` writes `git.SanitizeCommitMessage(result)` into `<file>` and exits 0 on every path — error, success, or clean tree. Dispatch checks `-hook` before the task flags, so it can never fall back to `Commit` (whose `git add -A` would run mid-commit); config-load errors and invalid `reasoning` values also resolve to exit 0 (the value is dropped, generation proceeds). |
| Config | `hook.*` keys | `Name()=="hook"` makes the generic per-task resolution in `main` treat it like any other task (`hook.model`, `hook.thinking`, `hook.reasoning`), falling back to the global model. |
| Tests | `cmd/install_hook_test.go`, `commands/hook_test.go` | Script guards + resolved path are asserted; `Hook` is checked to satisfy `Handler` (compile-time) and to report `Name()=="hook"`. |

Deliberate deviations from the sketch above: the script embeds the
absolute binary path rather than bare `gitai`, an existing hook is
backed up to `.bak` rather than overwritten silently (and a pre-existing
`.bak` is kept on re-install), and install warns when `core.hooksPath`
is set. All small robustness wins that don't change the flow.
