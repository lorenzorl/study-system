// Package llm is the OpenRouter chat-completions client used by the ingestion
// pipeline.
//
// It knows how to reach one endpoint and nothing about what the text means: the
// prompt and the page model live in the context that needs them, and this package
// satisfies rag/domain.PageTranscriber implicitly.
package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
)

const (
	// DefaultTimeout bounds one attempt. A vision call that takes longer than
	// this is not going to succeed on a retry either.
	DefaultTimeout = 120 * time.Second

	// DefaultRetryBase is the first backoff delay. Each further retry doubles it.
	DefaultRetryBase = 2 * time.Second

	// maxAttempts is the total number of tries, including the first. A rate limit
	// in the middle of a 359-page run must not cost a page a manual re-run, but an
	// unbounded retry would stall the run instead.
	maxAttempts = 3

	// maxResponseBytes bounds a response read so a misbehaving gateway cannot
	// exhaust memory.
	maxResponseBytes = 8 << 20

	// maxErrorBodyBytes bounds how much of an unparseable error body reaches the
	// message. The full body can be a whole HTML error page.
	maxErrorBodyBytes = 512

	// imageDataPrefix is the data-URL prefix the API expects for an inline PNG.
	imageDataPrefix = "data:image/png;base64,"
)

// Options carries the injectable seams. The zero value is the production choice
// for every field.
type Options struct {
	// HTTPClient, when set, is used as-is and Timeout is ignored. Tests point it
	// at an httptest server, so no request ever leaves the process.
	HTTPClient *http.Client

	// Timeout bounds one attempt when HTTPClient is nil.
	Timeout time.Duration

	// RetryBase is the first backoff delay, doubled on each further retry.
	RetryBase time.Duration
}

// Client calls OpenRouter's chat-completions endpoint. It satisfies
// rag/domain.PageTranscriber without importing the ingestion context.
type Client struct {
	apiKey     string
	endpoint   string
	model      string
	httpClient *http.Client
	retryBase  time.Duration
}

// NewClient builds a client for baseURL. A missing or blank key is rejected
// here, naming the variable that supplies it, so a misconfigured run fails before
// it reads a page.
func NewClient(apiKey, baseURL, model string, options Options) (*Client, error) {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil, fmt.Errorf("%s is not set: the ingestion run cannot transcribe pages without it: %w",
			config.EnvOpenRouterAPIKey, apperror.ErrInvalidInput)
	}

	identifier := strings.TrimSpace(model)
	if identifier == "" {
		return nil, fmt.Errorf("model must not be empty: %w", apperror.ErrInvalidInput)
	}

	endpoint, err := chatCompletionsURL(baseURL)
	if err != nil {
		return nil, err
	}

	httpClient := options.HTTPClient
	if httpClient == nil {
		timeout := options.Timeout
		if timeout <= 0 {
			timeout = DefaultTimeout
		}
		httpClient = &http.Client{Timeout: timeout}
	}

	retryBase := options.RetryBase
	if retryBase <= 0 {
		retryBase = DefaultRetryBase
	}

	return &Client{
		apiKey:     key,
		endpoint:   endpoint,
		model:      identifier,
		httpClient: httpClient,
		retryBase:  retryBase,
	}, nil
}

// Transcribe sends one page's PNG render with prompt and returns the model's
// Markdown.
//
// A 429 or a 5xx is retried up to maxAttempts with exponential backoff. Another
// 4xx is returned immediately: a 400, a 401, or a 402 is a defect in the request
// or the account, and retrying it only wastes the run. The backoff waits on the
// context, so cancelling a run stops it during the wait rather than after it.
func (c *Client) Transcribe(ctx context.Context, prompt string, imagePNG []byte) (string, error) {
	if len(imagePNG) == 0 {
		return "", fmt.Errorf("page render is empty: %w", apperror.ErrInvalidInput)
	}

	payload, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{{
			Role: "user",
			Content: []contentPart{
				{Type: "text", Text: prompt},
				{Type: "image_url", ImageURL: &imageURL{
					URL: imageDataPrefix + base64.StdEncoding.EncodeToString(imagePNG),
				}},
			},
		}},
	})
	if err != nil {
		return "", fmt.Errorf("encode request: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		content, retryable, attemptErr := c.attempt(ctx, payload)
		if attemptErr == nil {
			return content, nil
		}

		if !retryable {
			return "", attemptErr
		}
		lastErr = attemptErr

		// The context is checked before waiting so a cancelled run reports the
		// cancellation rather than "gave up after 3 attempts".
		if ctx.Err() != nil {
			return "", fmt.Errorf("transcribe page: %w", ctx.Err())
		}
		if attempt == maxAttempts {
			break
		}
		if err := c.waitBeforeRetry(ctx, attempt+1); err != nil {
			return "", err
		}
	}

	return "", fmt.Errorf("transcribe page: gave up after %d attempts: %w", maxAttempts, lastErr)
}

// attempt performs one request. It reports whether the failure is worth
// retrying, which is decided here because only the status code knows.
func (c *Client) attempt(ctx context.Context, payload []byte) (string, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", false, fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return "", false, fmt.Errorf("call openrouter: %w", ctx.Err())
		}
		// A transport failure carries no status, so it is not one of the
		// transient statuses this client retries.
		return "", false, fmt.Errorf("call openrouter: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return "", false, fmt.Errorf("read openrouter response: %w", err)
	}

	if response.StatusCode < 200 || response.StatusCode > 299 {
		retryable := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		return "", retryable, apiError(response.StatusCode, body)
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", false, fmt.Errorf("parse openrouter response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		// An empty choices array is never an empty page: an empty page would be
		// recorded as if the model had transcribed nothing.
		return "", false, errors.New("openrouter response has no choices")
	}

	return parsed.Choices[0].Message.Content, false, nil
}

// waitBeforeRetry sleeps for the exponential backoff before the given attempt,
// returning early when the context is done.
func (c *Client) waitBeforeRetry(ctx context.Context, attempt int) error {
	delay := c.retryBase * time.Duration(1<<(attempt-2))

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return fmt.Errorf("wait to retry: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

// apiError describes a non-2xx response. It prefers the API's own message,
// because "no credits" or "invalid model" is what the user can act on, and falls
// back to a truncated body so a gateway that answered HTML still says something.
func apiError(status int, body []byte) error {
	var parsed struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil &&
		parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return fmt.Errorf("openrouter returned %d: %s", status, strings.TrimSpace(parsed.Error.Message))
	}

	return fmt.Errorf("openrouter returned %d: %s", status, truncate(body))
}

// truncate reduces a response body to a bounded, single-line excerpt.
func truncate(body []byte) string {
	text := strings.Join(strings.Fields(string(body)), " ")
	if text == "" {
		return "(empty response body)"
	}
	if len(text) <= maxErrorBodyBytes {
		return text
	}
	return text[:maxErrorBodyBytes] + "..."
}

// chatCompletionsURL joins the API root with the endpoint path and checks the
// result is absolute, so a mistyped base URL fails at construction.
func chatCompletionsURL(baseURL string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return "", fmt.Errorf("base URL must not be empty: %w", apperror.ErrInvalidInput)
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("base URL %q must be absolute: %w", baseURL, apperror.ErrInvalidInput)
	}

	return trimmed + "/chat/completions", nil
}

// chatRequest is the request body. The field order is the order the verified
// payload documents, which keeps a captured body readable next to the spec.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string        `json:"role"`
	Content []contentPart `json:"content"`
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

// chatResponse is only the part of the response this client consumes.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}
