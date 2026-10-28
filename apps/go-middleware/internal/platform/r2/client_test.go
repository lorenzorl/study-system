package r2_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/r2"
)

// testSecret is the example credential published in the AWS Signature Version 4
// documentation, not a live key. It is deliberately distinctive: every error
// path in this file asserts this exact string is absent, so a redaction
// regression fails on the value rather than on a shape.
const (
	testAccessKeyID = "AKIAIOSFODNN7EXAMPLE"
	testSecret      = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	testBucket      = "bucket"
	testPrefix      = "prefix"
)

// testNow is the injected clock. A real clock would make the signature
// untestable, which is why the client accepts one.
var testNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// request is what the double saw, recorded so assertions read as facts rather
// than as expectations about the client's internals.
type request struct {
	method string
	path   string
	query  url.Values
	host   string
	header http.Header
	body   []byte
}

// baseConfig is a fully configured destination pointing at endpoint.
func baseConfig(endpoint string) r2.Config {
	return r2.Config{
		Endpoint:        endpoint,
		Bucket:          testBucket,
		Prefix:          testPrefix,
		AccessKeyID:     testAccessKeyID,
		SecretAccessKey: testSecret,
	}
}

// mustNewClient builds a client with the injected clock.
func mustNewClient(t *testing.T, cfg r2.Config) *r2.Client {
	t.Helper()
	client, err := r2.New(cfg, r2.WithClock(func() time.Time { return testNow }))
	require.NoError(t, err)
	return client
}

// newClient starts an S3 double and returns a client bound to it. mutate can
// adjust the configuration before the client is built.
func newClient(t *testing.T, handler http.HandlerFunc, mutate func(*r2.Config)) *r2.Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	cfg := baseConfig(server.URL)
	if mutate != nil {
		mutate(&cfg)
	}
	return mustNewClient(t, cfg)
}

func record(r *http.Request) request {
	body, _ := io.ReadAll(r.Body)
	return request{
		method: r.Method,
		path:   r.URL.EscapedPath(),
		query:  r.URL.Query(),
		host:   r.Host,
		header: r.Header.Clone(),
		body:   body,
	}
}

// --- Put ---

func TestPut_SignsAndSendsTheBody(t *testing.T) {
	var seen request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = record(r)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client := mustNewClient(t, baseConfig(server.URL))

	body := "snapshot bytes"
	require.NoError(t, client.Put(context.Background(), "db/study.db", strings.NewReader(body), int64(len(body)), "application/x-sqlite3"))

	// Path-style addressing: endpoint + "/" + bucket + "/" + prefix + "/" + key.
	assert.Equal(t, http.MethodPut, seen.method)
	assert.Equal(t, "/bucket/prefix/db/study.db", seen.path)
	assert.Equal(t, body, string(seen.body))
	assert.Equal(t, "14", seen.header.Get("Content-Length"))
	assert.Equal(t, "application/x-sqlite3", seen.header.Get("Content-Type"))

	// The four headers the contract names are present, and the signed host is the
	// one addressed by the endpoint.
	assert.Equal(t, strings.TrimPrefix(server.URL, "http://"), seen.host)
	assert.Equal(t, "UNSIGNED-PAYLOAD", seen.header.Get("X-Amz-Content-Sha256"))
	assert.Equal(t, "20260101T120000Z", seen.header.Get("X-Amz-Date"))
	authorization := seen.header.Get("Authorization")
	require.NotEmpty(t, authorization)

	// The credential scope carries the region, the service and the terminator.
	assert.True(t, strings.HasPrefix(authorization, "AWS4-HMAC-SHA256 "), "got %q", authorization)
	assert.Contains(t, authorization, "Credential="+testAccessKeyID+"/20260101/auto/s3/aws4_request")
	assert.Contains(t, authorization, "SignedHeaders=host;x-amz-content-sha256;x-amz-date")

	// The secret is never sent, in any header.
	for name, values := range seen.header {
		for _, value := range values {
			assert.NotContains(t, value, testSecret, "header %s must not carry the secret", name)
		}
	}
}

func TestPut_EscapesEachPathSegment(t *testing.T) {
	var seen request
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = record(r)
		w.WriteHeader(http.StatusOK)
	}, nil)

	require.NoError(t, client.Put(context.Background(), "db/estudio 2026/ñandú.db", strings.NewReader("x"), 1, ""))
	assert.Equal(t, "/bucket/prefix/db/estudio%202026/%C3%B1and%C3%BA.db", seen.path)
}

func TestPut_WithoutPrefixAddressesTheBucketRoot(t *testing.T) {
	var seen request
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = record(r)
		w.WriteHeader(http.StatusOK)
	}, func(cfg *r2.Config) { cfg.Prefix = "" })

	require.NoError(t, client.Put(context.Background(), "db/study.db", strings.NewReader("x"), 1, ""))
	assert.Equal(t, "/bucket/db/study.db", seen.path)
}

// --- Get ---

func TestGet_ReturnsTheBody(t *testing.T) {
	var seen request
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = record(r)
		_, _ = io.WriteString(w, "restored snapshot")
	}, nil)

	body, err := client.Get(context.Background(), "db/study.db")
	require.NoError(t, err)
	defer body.Close()

	content, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, "restored snapshot", string(content))
	assert.Equal(t, http.MethodGet, seen.method)
	assert.Equal(t, "/bucket/prefix/db/study.db", seen.path)
	assert.Equal(t, "UNSIGNED-PAYLOAD", seen.header.Get("X-Amz-Content-Sha256"))
}

func TestGet_ReportsAMissingObject(t *testing.T) {
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}, nil)

	body, err := client.Get(context.Background(), "db/absent.db")
	require.Error(t, err)
	assert.Nil(t, body)
	assert.Contains(t, err.Error(), "404")
	assert.NotContains(t, err.Error(), testSecret)
}

func TestGet_RespectsACancelledContext(t *testing.T) {
	var calls int
	var mu sync.Mutex
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Get(ctx, "db/study.db")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), testSecret)

	mu.Lock()
	defer mu.Unlock()
	assert.Zero(t, calls, "a cancelled context must not reach the server")
}

// --- Head ---

func TestHead_MissingObjectIsNotAnError(t *testing.T) {
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}, nil)

	object, found, err := client.Head(context.Background(), "db/absent.db")

	require.NoError(t, err, "a 404 is an answer, not a failure")
	assert.False(t, found)
	assert.Equal(t, r2.Object{}, object)
}

func TestHead_ReturnsTheObjectMetadata(t *testing.T) {
	var seen request
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = record(r)
		w.Header().Set("Content-Length", "4096")
		w.Header().Set("ETag", `"abcdef123456"`)
		w.WriteHeader(http.StatusOK)
	}, nil)

	object, found, err := client.Head(context.Background(), "db/study.db")

	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "db/study.db", object.Key)
	assert.Equal(t, int64(4096), object.Size)
	assert.Equal(t, "abcdef123456", object.ETag)
	assert.Equal(t, http.MethodHead, seen.method)
}

// --- List ---

func TestList_FollowsContinuationTokensToTheEnd(t *testing.T) {
	var mu sync.Mutex
	var pages []request

	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		pages = append(pages, record(r))
		page := len(pages)
		mu.Unlock()

		switch page {
		case 1:
			assert.Equal(t, "2", r.URL.Query().Get("list-type"))
			assert.Equal(t, "prefix/db", r.URL.Query().Get("prefix"))
			assert.Empty(t, r.URL.Query().Get("continuation-token"))
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult>
  <IsTruncated>true</IsTruncated>
  <Contents><Key>prefix/db/a.db</Key><Size>10</Size><ETag>"etag-a"</ETag></Contents>
  <Contents><Key>prefix/db/b.db</Key><Size>20</Size><ETag>"etag-b"</ETag></Contents>
  <NextContinuationToken>page-2</NextContinuationToken>
</ListBucketResult>`)
		case 2:
			assert.Equal(t, "page-2", r.URL.Query().Get("continuation-token"))
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult>
  <IsTruncated>false</IsTruncated>
  <Contents><Key>prefix/db/c.db</Key><Size>30</Size><ETag>"etag-c"</ETag></Contents>
</ListBucketResult>`)
		default:
			t.Errorf("unexpected page %d: the client must stop when IsTruncated is false", page)
		}
	}, nil)

	objects, err := client.List(context.Background(), "db")
	require.NoError(t, err)

	assert.Equal(t, []r2.Object{
		{Key: "db/a.db", Size: 10, ETag: "etag-a"},
		{Key: "db/b.db", Size: 20, ETag: "etag-b"},
		{Key: "db/c.db", Size: 30, ETag: "etag-c"},
	}, objects)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, pages, 2, "both pages must be requested")
	assert.Equal(t, http.MethodGet, pages[0].method)
	assert.Equal(t, "/bucket", pages[0].path)
}

func TestList_RefusesARepeatedContinuationToken(t *testing.T) {
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `<ListBucketResult>
  <IsTruncated>true</IsTruncated>
  <NextContinuationToken>same-token</NextContinuationToken>
</ListBucketResult>`)
	}, nil)

	_, err := client.List(context.Background(), "db")
	require.Error(t, err, "a server that repeats its token must not loop forever")
	assert.Contains(t, err.Error(), "continuation token repeated")
	assert.NotContains(t, err.Error(), testSecret)
}

func TestList_RefusesATruncatedPageWithoutAToken(t *testing.T) {
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `<ListBucketResult><IsTruncated>true</IsTruncated></ListBucketResult>`)
	}, nil)

	_, err := client.List(context.Background(), "db")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no continuation token")
}

// --- Delete ---

func TestDelete_MissingObjectIsNotAnError(t *testing.T) {
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}, nil)

	require.NoError(t, client.Delete(context.Background(), "db/old.db"),
		"retention must not fail because a previous run already removed the object")
}

func TestDelete_RemovesTheObject(t *testing.T) {
	var seen request
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = record(r)
		w.WriteHeader(http.StatusNoContent)
	}, nil)

	require.NoError(t, client.Delete(context.Background(), "db/old.db"))
	assert.Equal(t, http.MethodDelete, seen.method)
	assert.Equal(t, "/bucket/prefix/db/old.db", seen.path)
}

func TestDelete_ReportsAServerFailure(t *testing.T) {
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}, nil)

	err := client.Delete(context.Background(), "db/old.db")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

// --- S9: the secret never leaves ---

func TestForbidden_NamesTheStatusAndRedactsTheCredential(t *testing.T) {
	var authorization string

	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		// A hostile or chatty upstream: a full error page that echoes the request
		// headers back and repeats the credential. Nothing of it may reach the
		// caller unredacted.
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprintf(w, "<html>\n<body>\nsecret=%s\nauth=%s\n%s\n</body>\n</html>",
			testSecret, authorization, strings.Repeat("diagnostic padding ", 200))
	}, nil)

	err := client.Put(context.Background(), "db/study.db", strings.NewReader("x"), 1, "")
	require.Error(t, err)

	message := err.Error()
	assert.Contains(t, message, "403", "the status must be named: without it the operator cannot tell a bad key from a bad bucket")
	assert.NotContains(t, message, testSecret, "the secret must never appear in an error")
	assert.NotContains(t, message, authorization, "the Authorization header value must never appear in an error")
	assert.NotContains(t, message, "\n", "an error body must be flattened to one line")
	assert.LessOrEqual(t, len(message), 700, "an upstream HTML page must be truncated, got %d bytes", len(message))
}

func TestConfig_StringNeverPrintsTheSecret(t *testing.T) {
	cfg := r2.Config{
		Endpoint:        "https://acct123.r2.cloudflarestorage.com",
		Bucket:          testBucket,
		Prefix:          testPrefix,
		AccessKeyID:     testAccessKeyID,
		SecretAccessKey: testSecret,
	}

	for _, format := range []string{"%v", "%+v", "%s", "%#v"} {
		rendered := fmt.Sprintf(format, cfg)
		assert.NotContains(t, rendered, testSecret, "%s of a Config must not print the secret", format)
		assert.Contains(t, rendered, "AKIAIOSFODNN7EXAMPLE", "%s should still name the access key id", format)
	}

	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {}, nil)
	assert.NotContains(t, fmt.Sprintf("%+v", client), testSecret,
		"a nested redacted Config must survive fmt through the client")
}

// --- New ---

func TestNew_RejectsAnUnusableConfiguration(t *testing.T) {
	usable := r2.Config{
		Endpoint:        "https://acct123.r2.cloudflarestorage.com",
		Bucket:          testBucket,
		AccessKeyID:     testAccessKeyID,
		SecretAccessKey: testSecret,
	}

	cases := map[string]struct {
		mutate  func(*r2.Config)
		message string
	}{
		"missing endpoint":      {func(c *r2.Config) { c.Endpoint = "" }, "endpoint is required"},
		"blank endpoint":        {func(c *r2.Config) { c.Endpoint = "   " }, "endpoint is required"},
		"missing bucket":        {func(c *r2.Config) { c.Bucket = "" }, "bucket is required"},
		"missing access key id": {func(c *r2.Config) { c.AccessKeyID = "" }, "credentials are required"},
		"missing secret":        {func(c *r2.Config) { c.SecretAccessKey = "" }, "credentials are required"},
		"unparseable endpoint":  {func(c *r2.Config) { c.Endpoint = "://not-a-url" }, "not a usable URL"},
		"non-http scheme":       {func(c *r2.Config) { c.Endpoint = "ftp://acct123.example" }, "must use http or https"},
		"endpoint without host": {func(c *r2.Config) { c.Endpoint = "https://" }, "has no host"},
		"credentials in url":    {func(c *r2.Config) { c.Endpoint = "https://user:hunter2@acct123.example" }, "must not carry credentials"},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := usable
			testCase.mutate(&cfg)

			client, err := r2.New(cfg)
			require.Error(t, err)
			assert.Nil(t, client)
			assert.Contains(t, err.Error(), testCase.message)
			assert.NotContains(t, err.Error(), testSecret, "no validation error may print the secret")
		})
	}
}

// TestPut_ReadsTheClockOncePerRequest pins an invariant that is invisible until
// it fails in production: the timestamp in the x-amz-date header and the one
// that gets signed must come from the same read, or a request that straddles a
// second boundary is rejected with SignatureDoesNotMatch.
func TestPut_ReadsTheClockOncePerRequest(t *testing.T) {
	var mutex sync.Mutex
	calls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client, err := r2.New(baseConfig(server.URL), r2.WithClock(func() time.Time {
		mutex.Lock()
		defer mutex.Unlock()
		calls++
		return testNow.Add(time.Duration(calls-1) * 48 * time.Hour)
	}))
	require.NoError(t, err)

	require.NoError(t, client.Put(context.Background(), "db/study.db", strings.NewReader("x"), 1, ""))

	mutex.Lock()
	defer mutex.Unlock()
	assert.Equal(t, 1, calls, "one request must read the clock exactly once")
}

func TestNew_DefaultsTheRegionToAuto(t *testing.T) {
	var authorization string
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}, func(cfg *r2.Config) { cfg.Region = "" })

	require.NoError(t, client.Put(context.Background(), "db/study.db", strings.NewReader("x"), 1, ""))
	assert.Contains(t, authorization, "/20260101/auto/s3/aws4_request",
		"an empty region must sign as auto, got %q", authorization)
}

func TestNew_HonoursAnExplicitRegion(t *testing.T) {
	var authorization string
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}, func(cfg *r2.Config) { cfg.Region = "us-east-1" })

	require.NoError(t, client.Put(context.Background(), "db/study.db", strings.NewReader("x"), 1, ""))
	assert.Contains(t, authorization, "/20260101/us-east-1/s3/aws4_request")
}
