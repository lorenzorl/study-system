// Package r2 is a stdlib-only, S3-compatible object client for Cloudflare R2.
//
// It exists because the module's dependency posture is stdlib plus uuid,
// testify and modernc.org/sqlite, and R2 speaks S3: access is AWS Signature
// Version 4 over net/http, so no AWS SDK is needed or wanted (S7).
//
// Two decisions are load-bearing and deliberately boring:
//
//   - Every request is signed with x-amz-content-sha256: UNSIGNED-PAYLOAD. A
//     snapshot or an artifact can be up to 100 MiB, cannot be buffered to be
//     hashed, and must not be read twice, so the body is streamed exactly once
//     and its hash is never part of the signature.
//   - The secret access key never leaves this package in text. It is not in any
//     error, it is not in the Authorization header's echoed form, and error
//     details taken from an upstream body are truncated and scrubbed (S9).
//
// The client is context-aware: every call takes a context and cancels the
// underlying request with it.
package r2

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultRegion is used when Config.Region is empty. R2 accepts "auto" for every
// bucket, and the region is part of the credential scope, so it has to be
// decided somewhere; deciding it here keeps callers from having to.
const DefaultRegion = "auto"

// maxErrorBodyBytes bounds how much of an upstream error body reaches an error
// message. An S3 error can be a whole HTML page from a proxy, and none of it is
// more useful than its first few hundred bytes.
const maxErrorBodyBytes = 512

// redacted replaces every occurrence of a credential inside diagnostic text.
const redacted = "[REDACTED]"

// Config is where the client writes and how it authenticates.
//
// The zero value is not usable: New validates it. It deliberately carries no
// String method debt from platform/config -- but it does implement fmt.Stringer
// for the same reason that package does: without it, one log.Printf("%+v", cfg)
// writes the bucket key to disk.
type Config struct {
	// Endpoint is the S3 endpoint, for example
	// "https://<account>.r2.cloudflarestorage.com" or the URL of a local double.
	Endpoint string

	// Bucket is the bucket every request addresses, using path-style URLs.
	Bucket string

	// AccessKeyID and SecretAccessKey are the R2 token's key pair.
	AccessKeyID     string
	SecretAccessKey string

	// Prefix namespaces every key this client reads or writes. Key arguments and
	// the Object.Key values that come back are relative to it; empty means the
	// bucket root.
	Prefix string

	// Region is the signing region. Empty means DefaultRegion.
	Region string
}

// String renders the configuration with the credential replaced by its presence.
//
// This is the method that keeps a secret out of a log line: fmt honours a
// Stringer on a nested field, so a Config printed inside another struct is
// covered too.
func (c Config) String() string {
	secret := "unset"
	if c.SecretAccessKey != "" {
		secret = redacted
	}
	region := c.Region
	if region == "" {
		region = DefaultRegion
	}
	return fmt.Sprintf("r2.Config{Endpoint:%s Bucket:%s Prefix:%s Region:%s AccessKeyID:%s SecretAccessKey:%s}",
		c.Endpoint, c.Bucket, c.Prefix, region, c.AccessKeyID, secret)
}

// GoString makes %#v print the redacted form as well.
func (c Config) GoString() string {
	return c.String()
}

// Object is one stored object.
type Object struct {
	// Key is the full key, relative to Config.Prefix.
	Key string

	// Size is the stored byte count.
	Size int64

	// ETag is the entity tag without its surrounding quotes.
	ETag string
}

// Option customises a Client without widening the constructor's signature.
type Option func(*Client)

// WithClock replaces the source of "now", which is what makes the signature
// deterministic in tests. A nil function is ignored.
func WithClock(now func() time.Time) Option {
	return func(c *Client) {
		if now != nil {
			c.now = now
		}
	}
}

// Client is an S3-compatible client bound to one bucket and one credential.
// It is safe for concurrent use.
type Client struct {
	config Config

	// base is the endpoint with any trailing slash removed and no query.
	base string

	// basePath is the escaped path prefix the endpoint itself carries, usually
	// empty.
	basePath string

	// prefixPath is the escaped form of Config.Prefix, empty when unset.
	prefixPath string

	httpClient *http.Client
	now        func() time.Time
}

// String renders the client without its credential.
//
// Config.String alone is not enough: fmt cannot call a method on an unexported
// struct field, so log.Printf("%+v", client) would print Config's fields directly
// and write the secret out. This method is what actually closes that path.
func (c *Client) String() string {
	if c == nil {
		return "r2.Client(nil)"
	}
	return "r2.Client{" + c.config.String() + "}"
}

// GoString makes %#v print the redacted form as well.
func (c *Client) GoString() string {
	return c.String()
}

// New validates cfg and returns a client for it.
//
// It rejects an incomplete destination, an endpoint that is not an http or https
// URL with a host, and an endpoint that carries credentials of its own. No error
// it returns contains the secret.
func New(cfg Config, opts ...Option) (*Client, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, errors.New("r2: endpoint is required")
	}
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, errors.New("r2: bucket is required")
	}
	if cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		// The message names the two fields and never their values.
		return nil, errors.New("r2: credentials are required: both the access key id and the secret access key must be set")
	}

	parsed, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("r2: endpoint %q is not a usable URL", redactEndpoint(cfg.Endpoint))
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("r2: endpoint %q must use http or https", redactEndpoint(cfg.Endpoint))
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("r2: endpoint %q has no host", redactEndpoint(cfg.Endpoint))
	}
	if parsed.User != nil {
		// A key pair in the URL is a second place a credential lives, outside the
		// field the redaction covers. The repository has already paid for this
		// class of bug once (4b92c33), so it is refused rather than tolerated.
		return nil, errors.New("r2: endpoint must not carry credentials; set the access key fields instead")
	}

	cfg.Prefix = strings.Trim(cfg.Prefix, "/")
	if cfg.Region == "" {
		cfg.Region = DefaultRegion
	}

	client := &Client{
		config:     cfg,
		base:       parsed.Scheme + "://" + parsed.Host,
		basePath:   strings.Trim(parsed.Path, "/"),
		prefixPath: escapePath(cfg.Prefix),
		httpClient: &http.Client{},
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(client)
	}
	return client, nil
}

// Put uploads body to key.
//
// size is the exact number of bytes body will produce and becomes the
// Content-Length: S3 rejects a chunked upload, so an unknown size is not an
// error this client can paper over. contentType may be empty, in which case no
// Content-Type header is sent; it is not a signed header, so a proxy is free to
// leave it alone.
func (c *Client) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	if size < 0 {
		return errors.New("r2: put " + strconv.Quote(key) + ": size must not be negative")
	}
	if body == nil {
		body = http.NoBody
	}
	req, err := c.newRequest(ctx, http.MethodPut, c.objectPath(key), key, body, size, contentType, nil)
	if err != nil {
		return err
	}
	resp, err := c.send(req, "put", key)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if !isSuccess(resp.StatusCode) {
		return c.statusError(req, "put", key, resp)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// Get returns the object's body, which the caller must close.
func (c *Client) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	req, err := c.newRequest(ctx, http.MethodGet, c.objectPath(key), key, nil, 0, "", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.send(req, "get", key)
	if err != nil {
		return nil, err
	}
	if !isSuccess(resp.StatusCode) {
		defer resp.Body.Close()
		return nil, c.statusError(req, "get", key, resp)
	}
	return resp.Body, nil
}

// Head reports the object's metadata and whether it exists. A missing object is
// not an error: it is (Object{}, false, nil), because "not there yet" is a
// normal answer when deciding whether an upload can be skipped.
func (c *Client) Head(ctx context.Context, key string) (Object, bool, error) {
	req, err := c.newRequest(ctx, http.MethodHead, c.objectPath(key), key, nil, 0, "", nil)
	if err != nil {
		return Object{}, false, err
	}
	resp, err := c.send(req, "head", key)
	if err != nil {
		return Object{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return Object{}, false, nil
	}
	if !isSuccess(resp.StatusCode) {
		return Object{}, false, c.statusError(req, "head", key, resp)
	}

	size := resp.ContentLength
	if size < 0 {
		size = 0
	}
	return Object{
		Key:  key,
		Size: size,
		ETag: trimETag(resp.Header.Get("ETag")),
	}, true, nil
}

// List returns every object whose key starts with prefix, following
// ListObjectsV2 continuation tokens until the bucket says there are no more.
//
// prefix is relative to Config.Prefix, like every other key in this package.
func (c *Client) List(ctx context.Context, prefix string) ([]Object, error) {
	requestedPrefix := c.fullPrefix(prefix)
	objects := make([]Object, 0, 16)
	seenTokens := make(map[string]struct{})
	token := ""

	for {
		query := url.Values{"list-type": {"2"}}
		if requestedPrefix != "" {
			query.Set("prefix", requestedPrefix)
		}
		if token != "" {
			query.Set("continuation-token", token)
		}

		req, err := c.newRequest(ctx, http.MethodGet, c.bucketPath(), "bucket="+c.config.Bucket, nil, 0, "", query)
		if err != nil {
			return nil, err
		}
		resp, err := c.send(req, "list", "bucket="+c.config.Bucket)
		if err != nil {
			return nil, err
		}

		if !isSuccess(resp.StatusCode) {
			pageErr := c.statusError(req, "list", "bucket="+c.config.Bucket, resp)
			resp.Body.Close()
			return nil, pageErr
		}

		var page listBucketResult
		decodeErr := xml.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&page)
		resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("r2: list %q: decode response: %w", requestedPrefix, decodeErr)
		}

		for _, item := range page.Contents {
			key := c.relativeKey(item.Key)
			objects = append(objects, Object{
				Key:  key,
				Size: item.Size,
				ETag: trimETag(item.ETag),
			})
		}

		if !page.IsTruncated {
			return objects, nil
		}
		next := page.NextContinuationToken
		if next == "" {
			// Truncated without a token would otherwise loop on page one forever.
			return nil, fmt.Errorf("r2: list %q: response is truncated but carries no continuation token", requestedPrefix)
		}
		if _, repeated := seenTokens[next]; repeated {
			return nil, fmt.Errorf("r2: list %q: continuation token repeated; refusing to loop", requestedPrefix)
		}
		seenTokens[next] = struct{}{}
		token = next
	}
}

// Delete removes key. A key that is already gone is success: retention deletes
// snapshots after an upload, and a previous run that stopped halfway must not
// turn the next run into a failure over an object that is already absent.
func (c *Client) Delete(ctx context.Context, key string) error {
	req, err := c.newRequest(ctx, http.MethodDelete, c.objectPath(key), key, nil, 0, "", nil)
	if err != nil {
		return err
	}
	resp, err := c.send(req, "delete", key)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBodyBytes))
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if !isSuccess(resp.StatusCode) {
		return c.statusError(req, "delete", key, resp)
	}
	return nil
}

// newRequest builds a signed path-style request for canonicalURI. target is the
// object key or bucket the request names, used only to make an error message
// say what was attempted; query, when set, is the raw parameter set, which this
// function canonicalises for both the URL and the signature.
func (c *Client) newRequest(ctx context.Context, method, canonicalURI, target string, body io.Reader, contentLength int64, contentType string, query url.Values) (*http.Request, error) {
	canonicalQuery := canonicalQueryString(query)

	rawURL := c.base + canonicalURI
	if canonicalQuery != "" {
		rawURL += "?" + canonicalQuery
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, fmt.Errorf("r2: %s %q: build request: %w", strings.ToLower(method), target, err)
	}
	if body != nil {
		switch {
		case contentLength > 0:
			req.ContentLength = contentLength
		case contentLength == 0:
			// An explicit zero keeps S3 from refusing a chunked PUT of an empty
			// object.
			req.Body = http.NoBody
			req.ContentLength = 0
		}
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// The clock is read once. Two reads could straddle a second boundary and sign
	// a different timestamp from the one the x-amz-date header carries, which the
	// service rejects with SignatureDoesNotMatch.
	now := c.now().UTC()
	amzDate := now.Format(amzTimeLayout)
	signed := signV4(signedRequest{
		Method:         method,
		CanonicalURI:   canonicalURI,
		CanonicalQuery: canonicalQuery,
		HeaderValues: map[string]string{
			headerHost:        req.URL.Host,
			headerContentHash: unsignedPayload,
			headerAmzDate:     amzDate,
		},
		SignedHeaders:   signedHeaderNames,
		PayloadHash:     unsignedPayload,
		Time:            now,
		Region:          c.config.Region,
		Service:         serviceS3,
		AccessKeyID:     c.config.AccessKeyID,
		SecretAccessKey: c.config.SecretAccessKey,
	})

	req.Header.Set(headerContentHash, unsignedPayload)
	req.Header.Set(headerAmzDate, amzDate)
	req.Header.Set("Authorization", signed.Authorization)
	return req, nil
}

// bucketPath renders the path-style bucket URL: the endpoint's own path, then
// the bucket. A bucket-level operation must not carry the object prefix in its
// path -- that is a query parameter for List, and a path segment here would
// address something other than the bucket.
func (c *Client) bucketPath() string {
	segments := make([]string, 0, 2)
	if c.basePath != "" {
		segments = append(segments, escapePath(c.basePath))
	}
	segments = append(segments, awsEscape(c.config.Bucket))
	return "/" + strings.Join(segments, "/")
}

// objectPath renders the path-style object URL: bucket, configured prefix, key,
// each segment encoded but the separators kept.
func (c *Client) objectPath(key string) string {
	path := c.bucketPath()
	if c.prefixPath != "" {
		path += "/" + c.prefixPath
	}
	if key = strings.TrimPrefix(key, "/"); key != "" {
		path += "/" + escapePath(key)
	}
	return path
}

// fullPrefix joins Config.Prefix and prefix for the List request.
func (c *Client) fullPrefix(prefix string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	switch {
	case c.config.Prefix == "":
		return prefix
	case prefix == "":
		return c.config.Prefix
	default:
		return c.config.Prefix + "/" + prefix
	}
}

// relativeKey strips Config.Prefix from a key the bucket returned, so the values
// handed back to the caller are in the same namespace as the keys it passed in.
func (c *Client) relativeKey(key string) string {
	if c.config.Prefix == "" {
		return key
	}
	return strings.TrimPrefix(key, c.config.Prefix+"/")
}

// send issues the request and wraps a transport failure with what was attempted.
// The URL in a transport error never contains a credential: the credential lives
// in a header and in the signature, not in the URL.
func (c *Client) send(req *http.Request, operation, target string) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("r2: %s %q: %w", operation, target, err)
	}
	return resp, nil
}

// statusError turns a non-2xx response into an error that names the operation,
// the target and the HTTP status, plus a bounded and scrubbed excerpt of the
// body. It never includes the request's Authorization header.
func (c *Client) statusError(req *http.Request, operation, target string, resp *http.Response) error {
	detail := errorBodyDetail(resp.Body)
	message := fmt.Sprintf("r2: %s %q: %s", operation, target, resp.Status)
	if detail != "" {
		message += ": " + detail
	}
	return errors.New(c.scrub(req, message))
}

// errorBodyDetail reads at most maxErrorBodyBytes of an error body and flattens
// it to one line, because an S3 error can be an HTML page and a multi-line error
// message is unusable in a log.
func errorBodyDetail(body io.Reader) string {
	raw, err := io.ReadAll(io.LimitReader(body, maxErrorBodyBytes))
	if err != nil || len(raw) == 0 {
		return ""
	}
	flattened := strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ", "\t", " ").Replace(string(raw))
	return strings.TrimSpace(flattened)
}

// scrub is the last line of defence for S9: even if a proxy echoes the request
// back in its error page, neither the secret access key nor the Authorization
// header value reaches the caller's error text.
func (c *Client) scrub(req *http.Request, message string) string {
	if c.config.SecretAccessKey != "" {
		message = strings.ReplaceAll(message, c.config.SecretAccessKey, redacted)
	}
	if req != nil {
		if auth := req.Header.Get("Authorization"); auth != "" {
			message = strings.ReplaceAll(message, auth, redacted)
		}
	}
	return message
}

// redactEndpoint strips the URL userinfo from an endpoint before it is echoed in
// an error. When the value cannot be parsed, nothing of it is echoed: there is
// no way to tell whether an unparseable string carries a credential.
func redactEndpoint(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "<unparseable>"
	}
	parsed.User = nil
	return parsed.String()
}

func isSuccess(status int) bool {
	return status >= 200 && status < 300
}

func trimETag(etag string) string {
	return strings.Trim(strings.TrimSpace(etag), `"`)
}

// listBucketResult is the subset of a ListObjectsV2 response this client reads.
type listBucketResult struct {
	XMLName               xml.Name          `xml:"ListBucketResult"`
	Contents              []listBucketEntry `xml:"Contents"`
	IsTruncated           bool              `xml:"IsTruncated"`
	NextContinuationToken string            `xml:"NextContinuationToken"`
}

type listBucketEntry struct {
	Key  string `xml:"Key"`
	Size int64  `xml:"Size"`
	ETag string `xml:"ETag"`
}
