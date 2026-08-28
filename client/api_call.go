package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/parthdande/gitai/spinner"
)

// defaultHTTPTimeout is the maximum time an API call is allowed to take.
// Large diffs on local models can be slow, so this is generous; a hung
// server should still not block a terminal forever.
const defaultHTTPTimeout = 120 * time.Second

// chatCompletionRequest mirrors the OpenAI /chat/completions request body.
// Only the fields gitai needs are declared — extra response fields are
// simply ignored by encoding/json.
type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	// Stream selects SSE delivery. We always try streaming first and
	// fall back to a normal response (see doGenerate).
	Stream bool `json:"stream"`

	// ChatTemplateKwargs passes options straight through to the chat
	// template on vLLM-style servers. It is a *pointer* with omitempty
	// so that when thinking is off the field is omitted entirely —
	// strict providers reject unknown top-level keys, so "send it as
	// null" is not an option; the key must not exist at all.
	ChatTemplateKwargs *struct {
		EnableThinking bool `json:"enable_thinking"`
	} `json:"chat_template_kwargs,omitempty"`
}

// chatMessage is one entry in the conversation. Roles are the OpenAI
// convention: "system" (behavior instructions) then "user" (the actual
// input).
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionResponse mirrors the relevant parts of a non-streamed
// API response.
type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// streamChunk mirrors a single SSE event from a streaming response.
// Unlike the non-streamed shape, the text arrives incrementally in
// "delta" — each event carries the next fragment of the answer.
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		// FinishReason (e.g. "stop", "length") tells us why the
		// model stopped; read but not acted on.
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// httpClient returns the HTTP client to use for API calls.
// Reuses the embedded client if set, otherwise lazily creates one with
// the default timeout.
func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	// Lazy-init a client with a 120s timeout so we never hang forever.
	c.HTTPClient = &http.Client{Timeout: defaultHTTPTimeout}
	return c.HTTPClient
}

// Generate sends the prompt to the configured OpenAI-compatible endpoint.
//
//   - ctx:        context for cancellation (e.g. Ctrl+C) and timeouts
//   - model:      which model to use for this call (overrides client.Model if different)
//   - thinking:   whether to enable extended thinking mode
//   - reasoning: Muse Glimmer reasoning strength (low|medium|high|xhigh); empty means default
//
// If thinking is true and the API returns an error, Generate automatically
// retries one time with thinking disabled and returns that result instead.
// This way the user never sees a hard failure just because the model doesn't
// support thinking mode.
func (c *Client) Generate(ctx context.Context, prompt, systemPrompt, model string, thinking bool, reasoning string) (string, error) {
	if model == "" {
		model = c.Model // fallback to client-level model
	}
	if model == "" {
		return "", fmt.Errorf("model is required (set model in config or MODEL env var)")
	}
	if c.APIBase == "" {
		return "", fmt.Errorf("api_base is required (set api_base in config or API_BASE env var)")
	}

	// Model-specific handling for Muse Glimmer: reasoning strength is injected
	// into the system prompt (the only control the model reads) and
	// enable_thinking is never sent (always-on reasoning; the kwarg is a dead
	// knob for it). The prompt is mutated here so both the primary call and
	// the fallback retry below carry the same directive; for other models the
	// reasoning value is deliberately ignored.
	if IsMuseGlimmer(model) {
		if thinking && reasoning == "" {
			reasoning = "high"
		}
		thinking = false
		systemPrompt, _ = withReasoningStrength(systemPrompt, reasoning)
	}

	// Estimate prompt size to provide better diagnostics
	promptSize := len(prompt) + len(systemPrompt)

	// Try with the requested thinking setting.
	result, err := c.doGenerate(ctx, prompt, systemPrompt, model, thinking)

	// If thinking was ON and it failed, retry silently with thinking OFF.
	// For Muse Glimmer this gate never fires: thinking is forced to false
	// above, so a muse error surfaces directly instead of burning a second call.
	if err != nil && thinking {
		spinner.Note("Thinking mode failed (%v), falling back to non-thinking...", err)
		return c.doGenerate(ctx, prompt, systemPrompt, model, false)
	}

	// If the error mentions context length, provide a helpful suggestion.
	if err != nil && isContextLengthError(err) {
		return "", fmt.Errorf(
			"context length exceeded (prompt ~%d bytes). "+
				"This diff is too large for a single API call. "+
				"gitai will automatically chunk it into smaller pieces on the next run.\n\n"+
				"To reduce diff size:\n"+
				"  - Commit smaller changes more frequently\n"+
				"  - Avoid including build artifacts (dist/, node_modules/)\n"+
				"  - Avoid committing lock files or auto-generated code",
			promptSize,
		)
	}

	// If timeout, suggest the issue might be a large diff.
	if err != nil && isTimeoutError(err) {
		return "", fmt.Errorf(
			"request timed out (prompt ~%d bytes). "+
				"The diff may be too large — try committing smaller batches of changes.",
			promptSize,
		)
	}

	return result, err
}

// doGenerate performs a single API call (no fallback logic).
//
// It attempts streaming first and falls back to non-streaming if
// streaming fails. The reason for this order: streaming gives the user
// live feedback on long generations and avoids the "is it still
// working?" problem on slow local models, but not every
// OpenAI-compatible server supports it (some return an error for
// stream:true), so the non-streamed path must remain available.
func (c *Client) doGenerate(ctx context.Context, prompt, systemPrompt, model string, thinking bool) (string, error) {
	baseURL := c.buildURL()
	url := baseURL + "chat/completions"

	messages := c.buildMessages(prompt, systemPrompt)
	templateKwargs := c.buildTemplateKwargs(thinking)

	body, err := json.Marshal(chatCompletionRequest{
		Model:              model,
		Messages:           messages,
		Stream:             true, // Try streaming first
		ChatTemplateKwargs: templateKwargs,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Try streaming response first
	result, streamErr := c.doStreamedRequest(ctx, url, body)
	if streamErr == nil {
		return result, nil
	}

	// Streaming failed — fall back to non-streaming (some APIs don't support it)
	spinner.Note("Streaming not supported, falling back to non-streamed response...")

	bodyNonStream, _ := json.Marshal(chatCompletionRequest{
		Model:              model,
		Messages:           messages,
		Stream:             false,
		ChatTemplateKwargs: templateKwargs,
	})

	return c.doNonStreamedRequest(ctx, url, bodyNonStream)
}

// buildURL normalizes the API base URL.
//
// Users commonly write the base URL without a trailing slash or without
// the /v1 segment (e.g. "http://localhost:8000"). The OpenAI-compatible
// chat endpoint lives at <base>/v1/chat/completions, so buildURL makes
// sure both the trailing slash and the /v1 segment are present exactly
// once, regardless of how the URL was typed.
func (c *Client) buildURL() string {
	baseURL := c.APIBase
	if baseURL[len(baseURL)-1] != '/' {
		baseURL += "/"
	}
	if baseURL[len(baseURL)-4:] != "/v1/" && baseURL[len(baseURL)-4:] != "/v1" {
		baseURL += "v1/"
	}
	return baseURL
}

// buildMessages constructs the message list from system + user prompts.
func (c *Client) buildMessages(prompt, systemPrompt string) []chatMessage {
	messages := make([]chatMessage, 0, 2)
	if systemPrompt != "" {
		messages = append(messages, chatMessage{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, chatMessage{Role: "user", Content: prompt})
	return messages
}

// buildTemplateKwargs returns the thinking mode kwargs, or nil to omit
// the field from the JSON payload.
//
// Thinking mode is exposed to the model through the chat template
// (vLLM-style servers read chat_template_kwargs.enable_thinking). It is
// not a standard OpenAI field, so it must only be sent to servers that
// understand it — returning nil keeps it out of the request for
// everything else. The auto-fallback in Generate handles servers that
// reject it.
func (c *Client) buildTemplateKwargs(thinking bool) *struct {
	EnableThinking bool `json:"enable_thinking"`
} {
	if thinking {
		return &struct {
			EnableThinking bool `json:"enable_thinking"`
		}{EnableThinking: true}
	}
	return nil
}

// doStreamedRequest sends the request and reads an SSE stream response.
func (c *Client) doStreamedRequest(ctx context.Context, url string, body []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("streaming request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Read the SSE stream line by line. The wire format is plain text:
	// each event is a "data: <json>" line, terminated by a
	// "data: [DONE]" line.
	var result strings.Builder
	scanner := bufio.NewScanner(resp.Body)

	// Grow the scanner buffer well beyond the 64KB default: a single
	// event for a long token line can exceed it, and bufio silently
	// errors out on over-long lines otherwise.
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// SSE frames can carry comments, heartbeats, and other
		// non-data lines — only "data: " lines contain payload, so
		// everything else is skipped.
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// "[DONE]" is the standard sentinel marking end of stream.
		if data == "[DONE]" {
			break
		}

		// Parse the JSON chunk. Malformed chunks are skipped rather
		// than failing the whole call — a single bad event should not
		// discard a long, otherwise-successful generation.
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		// Each event carries a delta (the next slice of text);
		// accumulate all slices in order to reconstruct the answer.
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				result.WriteString(choice.Delta.Content)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading stream: %w", err)
	}

	return result.String(), nil
}

// doNonStreamedRequest sends a standard non-streaming request.
func (c *Client) doNonStreamedRequest(ctx context.Context, url string, body []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Handle non-200 responses with better diagnostics.
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		bodyStr := string(respBody)

		// Try to extract a more specific error from common API formats
		switch resp.StatusCode {
		case http.StatusRequestEntityTooLarge:
			return "", fmt.Errorf("request body too large (%d bytes). Your diff exceeds the API's payload limit. Try committing smaller batches", len(body))
		case http.StatusRequestTimeout:
			return "", fmt.Errorf("request timeout from server. The diff may be too large for the configured timeout")
		case http.StatusTooManyRequests:
			return "", fmt.Errorf("rate limited by the API. Please wait and try again")
		default:
			return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, bodyStr)
		}
	}

	// Decode and return the model's response.
	var result chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("API returned no choices")
	}

	return result.Choices[0].Message.Content, nil
}

// isContextLengthError heuristically checks if the error is related to
// exceeding the model's context limits.
//
// OpenAI-compatible servers do not share a stable machine-readable error
// code for this case, so we pattern-match the phrases they actually emit
// ("context length", "max_tokens", "too long", ...). Matching on text is
// deliberately loose: a false positive only changes the wording of the
// error message, it never changes what happens.
func isContextLengthError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "context") && strings.Contains(msg, "length") ||
		strings.Contains(msg, "model's max context length") ||
		strings.Contains(msg, "exceeds") && strings.Contains(msg, "context") ||
		strings.Contains(msg, "max_tokens") ||
		strings.Contains(msg, "too long")
}

// isTimeoutError checks if the error is a timeout, whether it came from
// our own client deadline or from the server. Same loose text-matching
// approach as isContextLengthError — see that function's comment.
func isTimeoutError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "i/o timeout")
}
