// Package client provides the HTTP client for OpenAI-compatible APIs.
package client

import "net/http"

// Client holds API configuration for OpenAI-compatible endpoints.
type Client struct {
	// APIBase is the server root, e.g. "http://localhost:8000/v1".
	APIBase string

	// APIKey is sent as a Bearer token; may be empty for local servers.
	APIKey string

	// Model is the global fallback model name.
	Model string

	// HTTPClient is reused across calls; nil means lazy init with default timeout.
	// gitai is single-threaded, so lazy init needs no locking.
	HTTPClient *http.Client
}
