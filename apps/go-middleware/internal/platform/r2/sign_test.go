package r2

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The credentials below are the example key pair published in the AWS Signature
// Version 4 documentation, not a live key. They are the pair the published
// signature vectors are computed with, so they cannot be replaced by a fixture.
const (
	vectorAccessKeyID     = "AKIAIOSFODNN7EXAMPLE"
	vectorSecretAccessKey = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	emptyPayloadSHA256    = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

func mustParseAMZTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(amzTimeLayout, value)
	require.NoError(t, err)
	return parsed
}

// TestSignV4_PublishedGetObjectVector reproduces the GET Object example from
// "Authenticating Requests (AWS Signature Version 4)". The expected signature is
// the value AWS publishes, so this pins the whole chain -- canonical request,
// string to sign, signing key -- against an authority outside this repository.
func TestSignV4_PublishedGetObjectVector(t *testing.T) {
	signed := signV4(signedRequest{
		Method:         "GET",
		CanonicalURI:   "/test.txt",
		CanonicalQuery: "",
		HeaderValues: map[string]string{
			"host":                 "examplebucket.s3.amazonaws.com",
			"range":                "bytes=0-9",
			"x-amz-content-sha256": emptyPayloadSHA256,
			"x-amz-date":           "20130524T000000Z",
		},
		SignedHeaders:   []string{"host", "range", "x-amz-content-sha256", "x-amz-date"},
		PayloadHash:     emptyPayloadSHA256,
		Time:            mustParseAMZTime(t, "20130524T000000Z"),
		Region:          "us-east-1",
		Service:         "s3",
		AccessKeyID:     vectorAccessKeyID,
		SecretAccessKey: vectorSecretAccessKey,
	})

	assert.Equal(t,
		"f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41",
		signed.Signature,
		"the published GET Object signature must be reproduced byte for byte; canonical request was:\n%s",
		signed.CanonicalRequest,
	)
	assert.Equal(t, "20130524/us-east-1/s3/aws4_request", signed.Scope)
	assert.True(t, strings.HasSuffix(signed.Authorization, "Signature="+signed.Signature),
		"the Authorization header must end with the signature, got %q", signed.Authorization)
}

// TestSignV4_UnsignedPayloadPutVector pins the same chain for the combination
// this client actually sends: UNSIGNED-PAYLOAD under the R2 "auto" region. A
// change to the region, the payload hash sentinel, or the signed-header list
// cannot pass unnoticed, because each of them moves this signature.
func TestSignV4_UnsignedPayloadPutVector(t *testing.T) {
	signed := signV4(signedRequest{
		Method:         "PUT",
		CanonicalURI:   "/bucket/prefix/db/study-20260101T120000Z.db",
		CanonicalQuery: "",
		HeaderValues: map[string]string{
			"host":                 "acct123.r2.cloudflarestorage.com",
			"x-amz-content-sha256": unsignedPayload,
			"x-amz-date":           "20260101T120000Z",
		},
		SignedHeaders:   signedHeaderNames,
		PayloadHash:     unsignedPayload,
		Time:            mustParseAMZTime(t, "20260101T120000Z"),
		Region:          "auto",
		Service:         "s3",
		AccessKeyID:     vectorAccessKeyID,
		SecretAccessKey: vectorSecretAccessKey,
	})

	assert.Equal(t,
		"d134ab5e8124b06d63f58dbd7aedf8005e98319db03c06ffb4bb7e7420d22355",
		signed.Signature,
		"the UNSIGNED-PAYLOAD signature under region auto must not drift; canonical request was:\n%s",
		signed.CanonicalRequest,
	)
	assert.Equal(t, "20260101/auto/s3/aws4_request", signed.Scope)
	assert.Contains(t, signed.Authorization, "SignedHeaders=host;x-amz-content-sha256;x-amz-date")
}

// TestCanonicalQueryString_IsSortedAndRFC3986Encoded covers the encoding that
// net/url cannot provide: a space is %20, never "+", and both sides of every
// pair are encoded.
func TestCanonicalQueryString_IsSortedAndRFC3986Encoded(t *testing.T) {
	assert.Equal(t, "", canonicalQueryString(nil))
	assert.Equal(t,
		"continuation-token=a%2Bb%2Fc&list-type=2&prefix=prefix%2Fdb%20x%2F",
		canonicalQueryString(map[string][]string{
			"prefix":             {"prefix/db x/"},
			"list-type":          {"2"},
			"continuation-token": {"a+b/c"},
		}),
	)
}

// TestEscapePath_EncodesEverySegmentAndKeepsSeparators guards the path-style URL
// shape: separators survive, everything else outside the unreserved set is
// encoded with upper-case hex.
func TestEscapePath_EncodesEverySegmentAndKeepsSeparators(t *testing.T) {
	assert.Equal(t, "a%20b/%C3%B1and%C3%BA/%25/%2A", escapePath("a b/ñandú/%/*"))
	assert.Equal(t, "plain/key-1.db", escapePath("plain/key-1.db"))
}
