package commands

import (
	"context"

	"github.com/parthdande/gitai/git"
)

// Hook reuses the commit-message pipeline but reads the staged diff with no
// side effects. It exists for the prepare-commit-msg hook, which fires inside
// the user's own `git commit` — a context where CommitAllDiff's `git add -A`
// would mutate the index and change what actually gets committed.
type Hook struct{ Commit }

func (h *Hook) Name() string { return "hook" }

func (h *Hook) Diff(ctx context.Context) (string, error) {
	return git.StagedDiff(ctx)
}
