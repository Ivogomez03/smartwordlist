// Package ollama provides an HTTP client for the Ollama REST API using only
// the standard library. It supports health checks, embedding generation, and
// streaming text generation with timeout and retry controls.
//
// The client never imports an Ollama SDK — it wraps three simple endpoints:
//
//	GET  /api/tags     — health check
//	POST /api/embed    — vector embedding
//	POST /api/generate — text generation (streaming via NDJSON)
package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// DefaultTimeout is the fallback timeout when the constructor receives 0.
// Set to 600s (10 min) because loading an 8B+ model on constrained hardware
// (MacBook Air, low RAM) can take 5-8 minutes on first run. The
// http.Client.Timeout fires BEFORE the context deadline, so a short timeout
// kills the request before the model loads. 600s matches the default
// pipelineTimeout in main.go.
const DefaultTimeout = 600 * time.Second

// DefaultMaxRetries is the number of retry attempts for 5xx responses.
const DefaultMaxRetries = 1

// Client is a thin net/http wrapper around the Ollama REST API.
// All methods accept a context.Context so callers control cancellation.
type Client struct {
	baseURL    string
	httpClient *http.Client
	maxRetries int
}

// NewClient returns a Client targeting the given baseURL (e.g. "http://localhost:11434").
// If timeout is 0, DefaultTimeout (600s) is used.
// The client uses a 10s DialContext timeout so connection establishment never
// hangs for too long, while the overall request gets the full timeout budget.
func NewClient(baseURL string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout: 10 * time.Second,
				}).DialContext,
			},
		},
		maxRetries: DefaultMaxRetries,
	}
}

// SetMaxRetries overrides the default retry count for 5xx responses.
// Set to 0 to disable retries entirely.
func (c *Client) SetMaxRetries(n int) {
	if n < 0 {
		n = 0
	}
	c.maxRetries = n
}

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

// Health checks whether the Ollama instance is reachable by calling GET /api/tags.
// It returns nil on success or an error if the server is unreachable.
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return fmt.Errorf("ollama health: build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama health: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("ollama health: HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ---------------------------------------------------------------------------
// Embed
// ---------------------------------------------------------------------------

// embedRequest is the JSON body sent to POST /api/embed.
type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// embedResponse is the JSON body returned by /api/embed.
type embedResponse struct {
	Model      string      `json:"model"`
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed sends one or more texts to the /api/embed endpoint and returns the
// corresponding embedding vectors. Each vector has the dimensionality of the
// requested model (e.g. 768 for nomic-embed-text).
func (c *Client) Embed(ctx context.Context, model string, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	body := embedRequest{Model: model, Input: texts}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(backoff(attempt))
		}

		resp, doErr := c.doEmbed(ctx, payload)
		if doErr == nil {
			return resp, nil
		}

		lastErr = doErr

		// Do not retry 4xx client errors.
		if isClientError(doErr) {
			break
		}
	}

	return nil, lastErr
}

func (c *Client) doEmbed(ctx context.Context, payload []byte) ([][]float32, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/embed", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("ollama embed: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama embed: decode response: %w", err)
	}

	return result.Embeddings, nil
}

// ---------------------------------------------------------------------------
// Generate
// ---------------------------------------------------------------------------

// generateRequest is the JSON body sent to POST /api/generate.
type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
	// Format forces structured output. Use the string "json" for raw JSON
	// mode, or pass a JSON-Schema object (map[string]any) to constrain the
	// response shape. When nil/omitted the model returns free text.
	Format any `json:"format,omitempty"`
	// Think controls the thinking/tool-use mode for models that support it.
	// nil (default) omits the field entirely; &false disables thinking so the
	// model puts output in "response" instead of "thinking". Must be *bool
	// with omitempty — a plain bool with omitempty would drop &false.
	Think *bool `json:"think,omitempty"`
}

// generateChunk is a single NDJSON line from the streaming /api/generate response.
type generateChunk struct {
	Response string `json:"response"`
	// Thinking captures output that thinking-enabled models route to the
	// "thinking" field. We surface it so even if a model ignores think:false,
	// we still recover the JSON content.
	Thinking string `json:"thinking,omitempty"`
	Done     bool   `json:"done"`
	Error    string `json:"error,omitempty"`
}

// ErrModelNotFound is a sentinel error returned when Ollama replies 404
// because the requested model has not been pulled.
type ErrModelNotFound struct {
	Model string
}

func (e *ErrModelNotFound) Error() string {
	return fmt.Sprintf("model not found: %s", e.Model)
}

// Is reports whether target is the same ErrModelNotFound error.
func (e *ErrModelNotFound) Is(target error) bool {
	_, ok := target.(*ErrModelNotFound)
	return ok
}

// Generate sends a prompt to /api/generate and returns a channel of token
// strings. When stream is true the channel receives partial tokens as they
// arrive (NDJSON). When stream is false the channel receives a single
// aggregated response and then closes.
//
// The optional format parameter is sent as the "format" field in the request
// body. Pass the string "json" to force raw JSON output, or a JSON-Schema
// object (map[string]any) to constrain the response shape. Pass nil to
// omit the field (free-text mode).
//
// The think parameter controls the thinking/tool-use mode for models that
// support it. Pass nil to omit the field; pass &false to disable thinking
// (ensures output goes to "response", not "thinking").
//
// The returned error covers only request-setup failures; streaming errors
// (including ErrModelNotFound) cause the channel to close early.  Callers
// should range over the channel until it closes.
func (c *Client) Generate(ctx context.Context, model, prompt string, stream bool, format any, think *bool) (<-chan string, error) {
	body := generateRequest{
		Model:  model,
		Prompt: prompt,
		Stream: stream,
		Format: format,
		Think:  think,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ollama generate: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("ollama generate: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama generate: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, &ErrModelNotFound{Model: model}
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("ollama generate: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan string, 16)
	go c.streamGenerate(ctx, resp.Body, ch)

	return ch, nil
}

// streamGenerate reads NDJSON lines from the response body and sends each
// "response" field token to the channel. It closes the channel when the
// stream ends or the context is cancelled.
//
// Thinking-model output (the "thinking" field) is also surfaced: even when
// think:false is sent, a model might still route output to "thinking".
// We emit both fields to the caller so the parser sees the full content.
func (c *Client) streamGenerate(ctx context.Context, body io.ReadCloser, ch chan<- string) {
	defer close(ch)
	defer body.Close()

	scanner := bufio.NewScanner(body)
	// Increase buffer size for large tokens.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var chunk generateChunk
		if err := json.Unmarshal(line, &chunk); err != nil {
			// Malformed line — skip it rather than kill the stream.
			continue
		}

		if chunk.Error != "" {
			// Ollama returned an error mid-stream.
			return
		}

		// Emit Response first, then Thinking — both may carry content.
		if chunk.Response != "" {
			select {
			case ch <- chunk.Response:
			case <-ctx.Done():
				return
			}
		}
		if chunk.Thinking != "" {
			select {
			case ch <- chunk.Thinking:
			case <-ctx.Done():
				return
			}
		}

		if chunk.Done {
			return
		}
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		// Scanner error — stream interrupted.
		return
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// backoff returns a linear backoff duration for the given retry attempt.
func backoff(attempt int) time.Duration {
	return time.Duration(attempt) * 500 * time.Millisecond
}

// isClientError returns true when err is an HTTP 4xx error.
// We use a simple heuristic: 4xx errors are not retryable.
func isClientError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// HTTP 4xx codes in the range 400..499.
	for code := 400; code < 500; code++ {
		if strings.Contains(msg, fmt.Sprintf("HTTP %d", code)) {
			return true
		}
	}
	return false
}
