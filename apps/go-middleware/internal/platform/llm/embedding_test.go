package llm_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/llm"
)

// embeddingBody builds a well-formed embedding response from a vector per input.
func embeddingBody(vectors [][]float32) string {
	data := make([]map[string]any, 0, len(vectors))
	for i, vector := range vectors {
		data = append(data, map[string]any{"object": "embedding", "index": i, "embedding": vector})
	}

	encoded, err := json.Marshal(map[string]any{"data": data})
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// oneVectorBody is the common single-input response.
func oneVectorBody(vector []float32) string {
	return embeddingBody([][]float32{vector})
}

func newTestEmbeddingClient(t *testing.T, baseURL string, options llm.Options) *llm.EmbeddingClient {
	t.Helper()

	client, err := llm.NewEmbeddingClient("test-key", baseURL, "test/embedding", options)
	require.NoError(t, err)
	return client
}

// --- request shape ---

func TestEmbeddingClient_EmbedPostsToTheEmbeddingsEndpoint(t *testing.T) {
	var method, path, auth, contentType string
	var body map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		contentType = r.Header.Get("Content-Type")

		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)

		_, _ = io.WriteString(w, oneVectorBody([]float32{0.1, 0.2}))
	}))
	defer server.Close()

	vectors, err := newTestEmbeddingClient(t, server.URL, llm.Options{}).
		Embed(t.Context(), []string{"hola"})
	require.NoError(t, err)
	require.Len(t, vectors, 1)
	assert.Equal(t, []float32{0.1, 0.2}, vectors[0])

	assert.Equal(t, http.MethodPost, method)
	assert.Equal(t, "/embeddings", path, "the embedding client must never touch the chat-completions path")
	assert.Equal(t, "Bearer test-key", auth)
	assert.Equal(t, "application/json", contentType)

	assert.Equal(t, "test/embedding", body["model"])
	input, ok := body["input"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"hola"}, input)
}

func TestEmbeddingClient_EmbedJoinsABaseURLWithAPathAndTrailingSlash(t *testing.T) {
	var path string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = io.WriteString(w, oneVectorBody([]float32{1}))
	}))
	defer server.Close()

	_, err := newTestEmbeddingClient(t, server.URL+"/gateway/v1/", llm.Options{}).
		Embed(t.Context(), []string{"hola"})
	require.NoError(t, err)

	assert.Equal(t, "/gateway/v1/embeddings", path)
}

func TestEmbeddingClient_EmbedSendsTheWholeBatchInOneRequest(t *testing.T) {
	var inputs [][]string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []string `json:"input"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		inputs = append(inputs, body.Input)

		vectors := make([][]float32, len(body.Input))
		for i := range body.Input {
			vectors[i] = []float32{float32(i)}
		}
		_, _ = io.WriteString(w, embeddingBody(vectors))
	}))
	defer server.Close()

	texts := []string{"uno", "dos", "tres"}
	vectors, err := newTestEmbeddingClient(t, server.URL, llm.Options{}).Embed(t.Context(), texts)
	require.NoError(t, err)

	assert.Len(t, inputs, 1, "a short batch must be a single request")
	assert.Equal(t, texts, inputs[0])

	require.Len(t, vectors, 3)
	for i := range texts {
		assert.Equal(t, []float32{float32(i)}, vectors[i])
	}
}

// --- response parsing ---

// The API returns an index with each vector, and an order-independent read is
// what keeps vector i paired with text i.
func TestEmbeddingClient_EmbedPlacesVectorsByIndex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[
			{"index":1,"embedding":[9,9]},
			{"index":0,"embedding":[1,1]}
		]}`)
	}))
	defer server.Close()

	vectors, err := newTestEmbeddingClient(t, server.URL, llm.Options{}).
		Embed(t.Context(), []string{"primero", "segundo"})
	require.NoError(t, err)

	require.Len(t, vectors, 2)
	assert.Equal(t, []float32{1, 1}, vectors[0], "the vector with index 0 belongs to the first text")
	assert.Equal(t, []float32{9, 9}, vectors[1])
}

func TestEmbeddingClient_EmbedRejectsAMismatchedVectorCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, oneVectorBody([]float32{1, 2}))
	}))
	defer server.Close()

	_, err := newTestEmbeddingClient(t, server.URL, llm.Options{}).
		Embed(t.Context(), []string{"uno", "dos"})
	require.Error(t, err, "two texts with one vector must not be paired silently")
}

// --- batching ---

func TestEmbeddingClient_EmbedSplitsALargeBatch(t *testing.T) {
	var batchSizes []int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []string `json:"input"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		batchSizes = append(batchSizes, len(body.Input))

		vectors := make([][]float32, len(body.Input))
		for i, text := range body.Input {
			// The embedding carries the text's own number, so concatenation order
			// across batches is checked rather than assumed.
			number, err := strconv.Atoi(strings.TrimPrefix(text, "text-"))
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			vectors[i] = []float32{float32(number)}
		}
		_, _ = io.WriteString(w, embeddingBody(vectors))
	}))
	defer server.Close()

	total := llm.DefaultEmbeddingBatchSize + 1
	texts := make([]string, total)
	for i := range texts {
		texts[i] = fmt.Sprintf("text-%d", i)
	}

	vectors, err := newTestEmbeddingClient(t, server.URL, llm.Options{}).Embed(t.Context(), texts)
	require.NoError(t, err)
	require.Len(t, vectors, total)

	assert.Equal(t, []int{llm.DefaultEmbeddingBatchSize, 1}, batchSizes,
		"a batch larger than the limit must be split, and every text must still be sent")
	for i := range texts {
		require.NotEmpty(t, vectors[i])
		assert.Equal(t, float32(i), vectors[i][0],
			"vector %d must stay paired with text %d across batches", i, i)
	}
}

func TestEmbeddingClient_EmptyInputMakesNoRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("an empty input must not reach the API")
	}))
	defer server.Close()

	vectors, err := newTestEmbeddingClient(t, server.URL, llm.Options{}).Embed(t.Context(), nil)
	require.NoError(t, err)
	assert.NotNil(t, vectors)
	assert.Empty(t, vectors)
}

// --- error mapping and retry ---

func TestEmbeddingClient_APIErrorCarriesTheStatusAndMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"No auth credentials found"}}`)
	}))
	defer server.Close()

	_, err := newTestEmbeddingClient(t, server.URL, llm.Options{}).Embed(t.Context(), []string{"hola"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
	assert.Contains(t, err.Error(), "No auth credentials found")
}

func TestEmbeddingClient_RetriesOnRateLimitAndSucceeds(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"message":"rate limited"}}`)
			return
		}
		_, _ = io.WriteString(w, oneVectorBody([]float32{1}))
	}))
	defer server.Close()

	vectors, err := newTestEmbeddingClient(t, server.URL, llm.Options{RetryBase: time.Millisecond}).
		Embed(t.Context(), []string{"hola"})
	require.NoError(t, err)
	assert.Len(t, vectors, 1)
	assert.Equal(t, int32(2), atomic.LoadInt32(&attempts))
}

func TestEmbeddingClient_DoesNotRetryAnotherClientError(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"invalid model"}}`)
	}))
	defer server.Close()

	_, err := newTestEmbeddingClient(t, server.URL, llm.Options{RetryBase: time.Millisecond}).
		Embed(t.Context(), []string{"hola"})
	require.Error(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&attempts), "a 400 is not transient")
}

func TestEmbeddingClient_GivesUpAfterThreeAttempts(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := newTestEmbeddingClient(t, server.URL, llm.Options{RetryBase: time.Millisecond}).
		Embed(t.Context(), []string{"hola"})
	require.Error(t, err)
	assert.Equal(t, int32(3), atomic.LoadInt32(&attempts), "the retry budget is shared with the chat client")
}

// --- construction ---

func TestNewEmbeddingClient_RequiresAnAPIKey(t *testing.T) {
	_, err := llm.NewEmbeddingClient("   ", "https://openrouter.ai/api/v1", "test/embedding", llm.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "STUDY_OPENROUTER_API_KEY",
		"the failure must name the variable the user has to set")
}

func TestNewEmbeddingClient_RejectsAnEmptyModel(t *testing.T) {
	_, err := llm.NewEmbeddingClient("test-key", "https://openrouter.ai/api/v1", "  ", llm.Options{})
	require.Error(t, err)
}

func TestNewEmbeddingClient_RejectsARelativeBaseURL(t *testing.T) {
	_, err := llm.NewEmbeddingClient("test-key", "/api/v1", "test/embedding", llm.Options{})
	require.Error(t, err)
}
