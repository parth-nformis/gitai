package commands

import (
	"context"
	"fmt"

	"github.com/parthdande/gitai/client"
	"github.com/parthdande/gitai/git"
	"github.com/parthdande/gitai/prompts"
)

// PullReq implements the PR description generation workflow.
//
// Its diff source is different from commit/review: instead of the
// staged snapshot, it diffs the whole branch (committed and uncommitted
// changes) against Base, because a PR description has to describe
// everything the PR will contain — most of which is already committed.
type PullReq struct {
	// Base is the base branch to diff against (e.g. "main"). Set by
	// the CLI flag -branch at startup.
	Base string
}

func (p *PullReq) Name() string { return "pullreq" }

// Diff returns all changes on the current branch that are not on the base
// branch, including uncommitted working tree changes.
func (p *PullReq) Diff(ctx context.Context) (string, error) {
	return git.BranchDiff(ctx, p.Base)
}

func (p *PullReq) Run(ctx context.Context, cli *client.Client, diff string, model string, thinking bool, reasoning string, configDir string) (string, error) {
	systemPrompt := prompts.LoadSystemPrompt("pullreq", configDir)

	prompt := fmt.Sprintf("Generate a PR description for these changes:\n\n%s", diff)
	return cli.Generate(ctx, prompt, systemPrompt, model, thinking, reasoning)
}
