// Package commands defines the Handler interface and its implementations for each
// gitai feature. Each handler receives a git diff, loads the appropriate system
// prompt, and calls the AI client to generate its output (commit message, review, etc.).
package commands

import (
	"context"

	"github.com/parthdande/gitai/client"
)

// Handler defines the contract for all gitai features.
// Each feature (commit, review, etc.) implements this interface.
type Handler interface {
	// Name returns the command name (e.g. "commit", "review").
	// It doubles as the key for per-task config (<task>.model,
	// <task>.thinking) and system prompt overrides (<task>.md).
	Name() string

	// Diff returns the git diff this feature operates on. Each handler
	// declares its own diff source (staged-only, stage-all, branch diff),
	// so callers never have to guess which diff a handler needs.
	Diff(ctx context.Context) (string, error)

	// Run executes the feature's workflow.
	//
	//   - ctx:       context for cancellation (Ctrl+C) and timeouts
	//   - cli:       the API client (holds api_base, api_key, global model fallback)
	//   - diff:      the staged git diff to analyze
	//   - model:     which model to use for this call (may override client.Model)
	//   - thinking:  whether to enable extended thinking mode
	//   - configDir: path to ~/.gitai (used to find system prompt files)
	//
	// Returns the AI's response text and any error.
	Run(ctx context.Context, cli *client.Client, diff string, model string, thinking bool, configDir string) (string, error)
}
