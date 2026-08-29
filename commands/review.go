package commands

import (
	"context"
	"fmt"

	"github.com/parthdande/gitai/client"
	"github.com/parthdande/gitai/git"
	"github.com/parthdande/gitai/prompts"
)

// Review implements the code review workflow (security, quality,
// best practices).
//
// Unlike Commit, Review sends the raw diff in a single call: a review
// needs to see actual code lines to point at problems, so the
// hierarchical chunking that commit uses for very large diffs does not
// apply here. The review prompt (see prompts.DefaultReviewSystem)
// forces a fixed output structure — verdict, then SECURITY / QUALITY /
// BEST PRACTICES sections — so results are easy to scan and, if ever
// needed, parse.
type Review struct{}

func (r *Review) Name() string { return "review" }

// Diff stages all working tree changes and returns the staged diff, so the
// review covers the entire working tree.
func (r *Review) Diff(ctx context.Context) (string, error) {
	return git.CommitAllDiff(ctx)
}

func (r *Review) Run(ctx context.Context, cli *client.Client, diff string, model string, thinking bool, reasoning string, configDir string) (string, error) {
	// Load the system prompt from ~/.gitai/system_prompts/review.md (or use default).
	systemPrompt := prompts.LoadSystemPrompt("review", configDir)

	prompt := fmt.Sprintf("Review this git diff for security vulnerabilities, code quality, and best practices:\n\n%s", diff)
	return cli.Generate(ctx, prompt, systemPrompt, model, thinking, reasoning)
}
