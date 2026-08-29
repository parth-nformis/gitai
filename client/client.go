// Package client provides the HTTP client for talking to OpenAI-compatible APIs.
//
// The Client struct holds connection settings (api_base, api_key, model) and a
// reused HTTP client with connection pooling. Generate sends a prompt to the
// chat completions endpoint and supports optional thinking mode with auto-fallback.
package client

import "net/http"

// Client holds the configuration needed to make API calls to any
// OpenAI-compatible endpoint (vLLM, Ollama, OpenAI, LiteLLM, etc.).
//
// Only the /chat/completions endpoint is used, which every
// OpenAI-compatible server implements — that is the entire contract
// gitai relies on.
//
// Model is the global fallback — individual calls may override it
// with a per-task model (see config: commit.model, review.model, ...).
type Client struct {
	// APIBase is the server root, e.g. "http://localhost:8000/v1".
	// Trailing-slash and /v1 normalization happens in buildURL, so
	// users can write "http://localhost:8000" and it still works.
	APIBase string

	// APIKey is sent as a Bearer token. May be empty — local servers
	// (Ollama, vLLM without auth) do not require one, in which case
	// the Authorization header is simply omitted.
	APIKey string

	// Model is the global fallback model name. Per-task models
	// (resolved in cmd/main.go) take precedence when set.
	Model string

	// HTTPClient is reused across calls so TCP connections are pooled.
	// nil = lazily created on first call with a default 120s timeout.
	// gitai is single-threaded, so the lazy init needs no locking.
	HTTPClient *http.Client
}
