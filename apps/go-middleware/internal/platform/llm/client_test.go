package llm_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/llm"
)

// pngBytes is a minimal byte slice that starts with the PNG signature, so the
// base64 round trip is checked on real-looking content rather than on text.
func pngBytes() []byte {
	return []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x01, 0x02, 0x03, 0xff}
}

// okBody is a well-formed success response.
func okBody(content string) string {
	encoded, err := json.Marshal(map[string]any{
		"choices": []any{
			map[string]any{"message": map[string]any{"content": content}},
		},
	})
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// newTestClient builds a client pointed at a test server, which is the whole
// point of the injectable base URL: no test reaches the network.
func newTestClient(t *testing.T, baseURL string, options llm.Options) *llm.Client {
	t.Helper()

	client, err := llm.NewClient("test-key", baseURL, "test/model", options)
	require.NoError(t, err)
	return client
}

// --- request shape ---

func TestClient_TranscribeSendsTheVerifiedRequest(t *testing.T) {
	var method, path, auth, contentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		contentType = r.Header.Get("Content-Type")
		_, _ = io.WriteString(w, okBody("page text"))
	}))
	defer server.Close()

	text, err := newTestClient(t, server.URL, llm.Options{}).Transcribe(t.Context(), "prompt", pngBytes())
	require.NoError(t, err)
	assert.Equal(t, "page text", text)

	assert.Equal(t, http.MethodPost, method)
	assert.Equal(t, "/chat/completions", path)
	assert.Equal(t, "Bearer test-key", auth)
	assert.Equal(t, "application/json", contentType)
}

func TestClient_TranscribeJoinsABaseURLWithAPathAndTrailingSlash(t *testing.T) {
	var path string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = io.WriteString(w, okBody("ok"))
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL+"/gateway/v1/", llm.Options{}).
		Transcribe(t.Context(), "prompt", pngBytes())
	require.NoError(t, err)

	assert.Equal(t, "/gateway/v1/chat/completions", path,
		"a trailing slash on the base URL must not produce a doubled separator")
}

func TestClient_TranscribeSendsTheVerifiedBodyShape(t *testing.T) {
	image := pngBytes()
	var raw []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, okBody("ok"))
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL, llm.Options{}).
		Transcribe(t.Context(), "convert this page", image)
	require.NoError(t, err)

	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))

	assert.Equal(t, "test/model", body["model"], "the configured model is always sent")

	messages, ok := body["messages"].([]any)
	require.True(t, ok)
	require.Len(t, messages, 1)

	message, ok := messages[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "user", message["role"])

	parts, ok := message["content"].([]any)
	require.True(t, ok)
	require.Len(t, parts, 2)

	textPart, ok := parts[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "text", textPart["type"])
	assert.Equal(t, "convert this page", textPart["text"])

	imagePart, ok := parts[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "image_url", imagePart["type"])

	imageURL, ok := imagePart["image_url"].(map[string]any)
	require.True(t, ok)
	url, ok := imageURL["url"].(string)
	require.True(t, ok)
	require.True(t, strings.HasPrefix(url, "data:image/png;base64,"),
		"the image must go in as an inline PNG data URL, got %q", url)

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(url, "data:image/png;base64,"))
	require.NoError(t, err)
	assert.Equal(t, image, decoded, "the PNG must survive the payload round trip byte for byte")
}

func TestClient_TranscribeRejectsAnEmptyImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the client must not call the API with no image")
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL, llm.Options{}).Transcribe(t.Context(), "prompt", nil)
	require.Error(t, err)
}

// --- response parsing ---

func TestClient_TranscribeParsesTheFirstChoiceContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, okBody("first choice"))
	}))
	defer server.Close()

	text, err := newTestClient(t, server.URL, llm.Options{}).
		Transcribe(t.Context(), "prompt", pngBytes())
	require.NoError(t, err)
	assert.Equal(t, "first choice", text)
}

// An empty choices array must never be read as an empty page: an empty page would
// be recorded as though the model had transcribed nothing.
func TestClient_EmptyChoicesIsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL, llm.Options{}).
		Transcribe(t.Context(), "prompt", pngBytes())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "choices")
}

// --- error mapping ---

func TestClient_APIErrorCarriesTheStatusAndMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"No auth credentials found","code":401}}`)
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL, llm.Options{}).
		Transcribe(t.Context(), "prompt", pngBytes())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401", "the status must be carried")
	assert.Contains(t, err.Error(), "No auth credentials found", "the API's own message must be carried")
}

func TestClient_NonJSONErrorBodyFallsBackToATruncatedBody(t *testing.T) {
	body := strings.Repeat("x", 4096)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, body)
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL, llm.Options{RetryBase: time.Millisecond}).
		Transcribe(t.Context(), "prompt", pngBytes())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "502")

	// The whole body must not fit in the message: a gateway can answer with a
	// multi-kilobyte HTML page.
	assert.Less(t, len(err.Error()), len(body)/2, "the fallback body must be truncated, got %d bytes", len(err.Error()))
}

// --- retry policy ---

func TestClient_RetriesOnRateLimitAndSucceeds(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"message":"rate limited"}}`)
			return
		}
		_, _ = io.WriteString(w, okBody("after retry"))
	}))
	defer server.Close()

	text, err := newTestClient(t, server.URL, llm.Options{RetryBase: time.Millisecond}).
		Transcribe(t.Context(), "prompt", pngBytes())
	require.NoError(t, err)
	assert.Equal(t, "after retry", text)
	assert.Equal(t, int32(2), atomic.LoadInt32(&attempts), "a 429 must be retried exactly once here")
}

func TestClient_RetriesOnServerErrorAndSucceeds(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, okBody("recovered"))
	}))
	defer server.Close()

	text, err := newTestClient(t, server.URL, llm.Options{RetryBase: time.Millisecond}).
		Transcribe(t.Context(), "prompt", pngBytes())
	require.NoError(t, err)
	assert.Equal(t, "recovered", text)
	assert.Equal(t, int32(2), atomic.LoadInt32(&attempts))
}

func TestClient_DoesNotRetryAnotherClientError(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"invalid model"}}`)
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL, llm.Options{RetryBase: time.Millisecond}).
		Transcribe(t.Context(), "prompt", pngBytes())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid model")
	assert.Equal(t, int32(1), atomic.LoadInt32(&attempts), "a 400 is not transient and must not be retried")
}

func TestClient_GivesUpAfterThreeAttempts(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL, llm.Options{RetryBase: time.Millisecond}).
		Transcribe(t.Context(), "prompt", pngBytes())
	require.Error(t, err)
	assert.Equal(t, int32(3), atomic.LoadInt32(&attempts), "the retry budget is three attempts in total")
}

func TestClient_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		cancel()
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	// A retry delay far beyond the test: only a client that watches the context
	// can return at all.
	_, err := newTestClient(t, server.URL, llm.Options{RetryBase: time.Hour}).
		Transcribe(ctx, "prompt", pngBytes())

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int32(1), atomic.LoadInt32(&attempts), "a cancelled run must not start another attempt")
}

// --- construction ---

func TestNewClient_RequiresAnAPIKey(t *testing.T) {
	for _, key := range []string{"", "   "} {
		_, err := llm.NewClient(key, "https://openrouter.ai/api/v1", "test/model", llm.Options{})

		require.Error(t, err, "key %q must be rejected", key)
		assert.Contains(t, err.Error(), "STUDY_OPENROUTER_API_KEY",
			"the failure must name the variable, so it is actionable at startup")
	}
}

func TestNewClient_RejectsAnEmptyModel(t *testing.T) {
	_, err := llm.NewClient("test-key", "https://openrouter.ai/api/v1", "  ", llm.Options{})
	require.Error(t, err)
}

func TestNewClient_RejectsARelativeBaseURL(t *testing.T) {
	_, err := llm.NewClient("test-key", "/api/v1", "test/model", llm.Options{})
	require.Error(t, err)
}

func TestClient_UsesTheInjectedHTTPClient(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		_, _ = io.WriteString(w, okBody("late"))
	}))
	defer server.Close()
	defer close(release)

	// The injected client owns the timeout, which is what makes this seam
	// testable without waiting two minutes on the default.
	client := newTestClient(t, server.URL, llm.Options{
		HTTPClient: &http.Client{Timeout: 5 * time.Millisecond},
	})

	_, err := client.Transcribe(t.Context(), "prompt", pngBytes())
	require.Error(t, err)
}
