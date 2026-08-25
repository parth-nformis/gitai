package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/parthdande/gitai/client"
	"github.com/parthdande/gitai/diffprep"
	"github.com/parthdande/gitai/git"
	"github.com/parthdande/gitai/prompts"
)

// Commit implements the commit message generation workflow.
//
// Strategy depends on diff size (the threshold lives in
// diffprep.ShouldChunk):
//
// Small diffs (<500 lines) are sent whole in a single call — one
// round trip, and the model sees the full context at once.
//
// Large diffs would either blow past the model's context window or
// come back shallow (the model's attention spreads too thin over
// thousands of lines), so they go through a two-stage hierarchical
// summarization pipeline instead:
//   - Stage 1: each chunk of the diff is summarized independently
//     (1-2 sentences per chunk)
//   - Stage 2: all summaries are synthesized into the final message
//
// The cost is several API calls; the benefit is that every part of a
// big diff actually gets read by the model.
type Commit struct{}

func (c *Commit) Name() string { return "commit" }

// Diff stages all working tree changes and returns the staged diff, so the
// commit message covers everything, not just what is already staged.
func (c *Commit) Diff(ctx context.Context) (string, error) {
	return git.CommitAllDiff(ctx)
}

func (c *Commit) Run(ctx context.Context, cli *client.Client, diff string, model string, thinking bool, configDir string) (string, error) {
	// Load the system prompt from ~/.gitai/system_prompts/commit.md (or use default).
	systemPrompt := prompts.LoadSystemPrompt("commit", configDir)

	// Preprocess the diff — filter noise files, compute stats, prepare content.
	prepared := diffprep.Process(diff)

	// Decide: single call or hierarchical chunking?
	if diffprep.ShouldChunk(diff) {
		return c.runHierarchical(ctx, cli, diff, prepared, model, thinking, systemPrompt)
	}

	// Small diff — send everything in one shot.
	prompt := fmt.Sprintf("Analyze this git diff and write a commit message:\n\n%s", prepared.Content)
	return cli.Generate(ctx, prompt, systemPrompt, model, thinking)
}

// runHierarchical uses a two-stage pipeline for large diffs:
// Stage 1: Summarize each chunk independently
// Stage 2: Synthesize all summaries into the final commit message
func (c *Commit) runHierarchical(ctx context.Context, cli *client.Client, rawDiff string, prepared *diffprep.PreparedDiff, model string, thinking bool, systemPrompt string) (string, error) {
	// 300 lines per chunk is the balance: small enough that each
	// summary fits easily in context, large enough that the number of
	// chunks (and therefore API calls) stays low.
	chunks := diffprep.ChunkDiff(rawDiff, 300)

	stage1Prompt := prompts.DefaultSummarizeChunkPrompt()

	var summaries []string
	for i, chunk := range chunks {
		chunkDiff := diffprep.ChunkToDiff(chunk)
		chunkFileNames := chunkFileNames(chunk)

		// Naming the files in the prompt gives the model an orientation
		// ("this chunk touches the auth layer") without re-deriving it
		// from the raw hunks.
		prompt := fmt.Sprintf(
			"Chunk %d/%d (files: %s):\n\n%s",
			i+1, len(chunks), chunkFileNames, chunkDiff,
		)

		// Stage 1 calls run with thinking OFF: chunk summarization is
		// a short, mechanical task, and enabling extended thinking
		// here would multiply latency and cost across every chunk for
		// no visible quality gain. The user's thinking choice is
		// honored on the stage-2 synthesis call, which is where the
		// actual message is written.
		summary, err := cli.Generate(ctx, prompt, stage1Prompt, model, false)
		if err != nil {
			return "", fmt.Errorf("chunk summarization failed for chunk %d: %w", i+1, err)
		}
		summaries = append(summaries, strings.TrimSpace(summary))
	}

	// Stage 2: Synthesize all summaries into the final commit message.
	// The prepared diff summary (file list, +/- counts) is attached so
	// the model knows the scale of the change even though it never
	// saw the full diff itself.
	synthesisPrompt := fmt.Sprintf(
		"Here is a summary of all the changes in this commit:\n\n%s\n\nBased on these summaries, write a comprehensive conventional commit message.\n\nDiff metadata:\n%s",
		strings.Join(summaries, "\n\n"),
		prepared.Summary,
	)

	return cli.Generate(ctx, synthesisPrompt, systemPrompt, model, thinking)
}

func chunkFileNames(files []diffprep.FileStats) string {
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Filename
	}
	return strings.Join(names, ", ")
}
