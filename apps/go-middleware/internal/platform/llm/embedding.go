package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
)

const (
	// DefaultEmbeddingBatchSize is how many texts one request carries.
	//
	// Embedding is a network call, so one request per chunk would cost hundreds of
	// round trips for one book; one request for the whole book would hold a
	// response large enough to be worth bounding. A batch of 64 keeps each
	// response in the low megabytes.
	DefaultEmbeddingBatchSize = 64

	// embeddingsPath is the endpoint, relative to the API root.
	embeddingsPath = "/embeddings"
)

// EmbeddingClient calls OpenRouter's embeddings endpoint. It satisfies
// rag/domain.Embedder without importing the RAG context.
//
// It is a separate client from Client on purpose: the chat-completions path sends
// a prompt and an image and returns Markdown, while this one sends text and
// returns vectors. Sharing one type would put a model call and a bi-encoder call
// behind one method and one retry policy that each of them half-uses.
type EmbeddingClient struct {
	apiKey     string
	endpoint   string
	model      string
	httpClient *http.Client
	retryBase  time.Duration
	batchSize  int
}

// NewEmbeddingClient builds a client for baseURL. A missing or blank key is
// rejected here, naming the variable that supplies it, so a misconfigured run
// fails before it has embedded a document.
func NewEmbeddingClient(apiKey, baseURL, model string, options Options) (*EmbeddingClient, error) {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil, fmt.Errorf("%s is not set: the semantic stage cannot embed text without it: %w",
			config.EnvOpenRouterAPIKey, apperror.ErrInvalidInput)
	}

	identifier := strings.TrimSpace(model)
	if identifier == "" {
		return nil, fmt.Errorf("embedding model must not be empty: %w", apperror.ErrInvalidInput)
	}

	// apiURL and waitForRetry are the shared transport helpers in client.go: both
	// clients go through the one absolute-URL check and the one backoff policy.
	endpoint, err := apiURL(baseURL, embeddingsPath)
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

	return &EmbeddingClient{
		apiKey:     key,
		endpoint:   endpoint,
		model:      identifier,
		httpClient: httpClient,
		retryBase:  retryBase,
		batchSize:  DefaultEmbeddingBatchSize,
	}, nil
}

// Embed returns one vector per text, in the same order as the input, batching the
// inputs below this call.
//
// An empty input is an empty result and no request: there is nothing to embed,
// and a request with an empty input would be rejected by the API.
func (c *EmbeddingClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(texts))

	for start := 0; start < len(texts); start += c.batchSize {
		end := min(start+c.batchSize, len(texts))

		batch, err := c.embedBatch(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, batch...)
	}

	return vectors, nil
}

// embedBatch embeds one request's worth of text, retrying a transient failure the
// same way the chat client does.
func (c *EmbeddingClient) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	payload, err := json.Marshal(embeddingRequest{Model: c.model, Input: texts})
	if err != nil {
		return nil, fmt.Errorf("encode embedding request: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		vectors, retryable, attemptErr := c.attempt(ctx, payload, len(texts))
		if attemptErr == nil {
			return vectors, nil
		}
		if !retryable {
			return nil, attemptErr
		}
		lastErr = attemptErr

		if ctx.Err() != nil {
			return nil, fmt.Errorf("embed text: %w", ctx.Err())
		}
		if attempt == maxAttempts {
			break
		}
		if err := waitForRetry(ctx, c.retryBase, attempt+1); err != nil {
			return nil, err
		}
	}

	return nil, fmt.Errorf("embed text: gave up after %d attempts: %w", maxAttempts, lastErr)
}

// attempt performs one request and reports whether the failure is worth
// retrying, which only the status code knows.
func (c *EmbeddingClient) attempt(
	ctx context.Context,
	payload []byte,
	count int,
) ([][]float32, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, false, fmt.Errorf("build embedding request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, fmt.Errorf("call openrouter embeddings: %w", ctx.Err())
		}
		return nil, false, fmt.Errorf("call openrouter embeddings: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return nil, false, fmt.Errorf("read openrouter embedding response: %w", err)
	}

	if response.StatusCode < 200 || response.StatusCode > 299 {
		retryable := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		return nil, retryable, apiError(response.StatusCode, body)
	}

	var parsed embeddingResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, false, fmt.Errorf("parse openrouter embedding response: %w", err)
	}

	return placeVectors(parsed.Data, count)
}

// placeVectors puts each returned vector at the position its index names.
//
// Positional pairing would be enough for a provider that always answers in order,
// but the response carries an index precisely because it is not obliged to, and a
// silently swapped vector would make one chunk answer for another's text.
func placeVectors(items []embeddingItem, count int) ([][]float32, bool, error) {
	if len(items) != count {
		return nil, false, fmt.Errorf(
			"openrouter returned %d vectors for %d inputs", len(items), count)
	}

	vectors := make([][]float32, count)
	filled := make([]bool, count)

	for position, item := range items {
		index := position
		if item.Index != nil {
			index = *item.Index
		}
		if index < 0 || index >= count {
			return nil, false, fmt.Errorf("embedding index %d is outside 0-%d", index, count-1)
		}
		if filled[index] {
			return nil, false, fmt.Errorf("openrouter returned two vectors for input %d", index)
		}
		if len(item.Embedding) == 0 {
			return nil, false, fmt.Errorf("openrouter returned an empty vector for input %d", index)
		}

		vectors[index] = item.Embedding
		filled[index] = true
	}

	return vectors, false, nil
}

// embeddingRequest is the request body. The field order matches the verified
// payload, which keeps a captured body readable next to the spec.
type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// embeddingItem is one returned vector. Index is a pointer so an omitted index is
// distinguishable from a zero one.
type embeddingItem struct {
	Index     *int      `json:"index"`
	Embedding []float32 `json:"embedding"`
}

// embeddingResponse is only the part of the response this client consumes.
type embeddingResponse struct {
	Data []embeddingItem `json:"data"`
}
