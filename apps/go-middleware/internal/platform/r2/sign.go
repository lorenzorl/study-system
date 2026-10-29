package r2

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// AWS Signature Version 4, in the exact spelling the specification requires.
// These are constants rather than literals because each one is part of the
// signed input: a typo in any of them produces a signature that is
// self-consistent and wrong, which the service rejects with a message that does
// not say which one moved.
const (
	algorithmSigV4    = "AWS4-HMAC-SHA256"
	serviceS3         = "s3"
	terminalRequest   = "aws4_request"
	unsignedPayload   = "UNSIGNED-PAYLOAD"
	amzTimeLayout     = "20060102T150405Z"
	amzDateLayout     = "20060102"
	headerHost        = "host"
	headerAmzDate     = "x-amz-date"
	headerContentHash = "x-amz-content-sha256"
	headerAuth        = "authorization"
)

// signedHeaderValues are the request headers this client signs. SigV4 requires
// the host header to be signed; the two x-amz headers are signed so that a
// replay cannot swap the payload hash or the timestamp.
var signedHeaderNames = []string{headerHost, headerContentHash, headerAmzDate}

// signedRequest is the part of an HTTP request that SigV4 covers, already in
// canonical form. Keeping it a value with no HTTP dependency is what makes the
// published AWS test vectors testable without a network.
type signedRequest struct {
	// Method is the upper-case HTTP method.
	Method string

	// CanonicalURI is the percent-encoded path, starting with "/". S3 signs the
	// path as sent, so it must not be encoded a second time.
	CanonicalURI string

	// CanonicalQuery is the sorted, percent-encoded query string, without the
	// leading "?". Empty when the request has no query.
	CanonicalQuery string

	// HeaderValues holds the canonical value of every signed header, keyed by
	// lower-case header name.
	HeaderValues map[string]string

	// SignedHeaders lists the signed header names, lower-case and sorted, which
	// is also the order they appear in the canonical request.
	SignedHeaders []string

	// PayloadHash is the x-amz-content-sha256 value. This client always sends
	// unsignedPayload, so bodies stream without being read to be hashed.
	PayloadHash string

	// Time is the request time. Only its UTC fields are used.
	Time time.Time

	// Region, Service, AccessKeyID and SecretAccessKey form the credential
	// scope and the signing key.
	Region          string
	Service         string
	AccessKeyID     string
	SecretAccessKey string
}

// signature is the result of signing, including the intermediate values so a
// failing test can show where the divergence was instead of only that the final
// hex differs.
type signature struct {
	// Signature is the hex-encoded HMAC-SHA256 signature.
	Signature string

	// Authorization is the full Authorization header value.
	Authorization string

	// Scope is the credential scope: date/region/service/aws4_request.
	Scope string

	// CanonicalRequest and StringToSign are the signed inputs.
	CanonicalRequest string
	StringToSign     string
}

// signV4 computes the SigV4 signature and Authorization header value for req.
func signV4(req signedRequest) signature {
	date := req.Time.UTC().Format(amzDateLayout)
	amzDate := req.Time.UTC().Format(amzTimeLayout)
	scope := strings.Join([]string{date, req.Region, req.Service, terminalRequest}, "/")

	canonicalRequest := req.canonicalRequest()
	canonicalHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{
		algorithmSigV4,
		amzDate,
		scope,
		hex.EncodeToString(canonicalHash[:]),
	}, "\n")

	key := signingKey(req.SecretAccessKey, date, req.Region, req.Service)
	signatureHex := hex.EncodeToString(hmacSHA256(key, stringToSign))

	return signature{
		Signature: signatureHex,
		Authorization: fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
			algorithmSigV4,
			req.AccessKeyID,
			scope,
			strings.Join(req.SignedHeaders, ";"),
			signatureHex,
		),
		Scope:            scope,
		CanonicalRequest: canonicalRequest,
		StringToSign:     stringToSign,
	}
}

// canonicalRequest renders the canonical request as the specification defines
// it, trailing blank line included.
func (r signedRequest) canonicalRequest() string {
	var b strings.Builder
	b.WriteString(r.Method)
	b.WriteString("\n")
	b.WriteString(r.CanonicalURI)
	b.WriteString("\n")
	b.WriteString(r.CanonicalQuery)
	b.WriteString("\n")
	for _, name := range r.SignedHeaders {
		b.WriteString(name)
		b.WriteString(":")
		b.WriteString(canonicalHeaderValue(r.HeaderValues[name]))
		b.WriteString("\n")
	}
	// The canonical headers block is terminated by an empty line.
	b.WriteString("\n")
	b.WriteString(strings.Join(r.SignedHeaders, ";"))
	b.WriteString("\n")
	b.WriteString(r.PayloadHash)
	return b.String()
}

// canonicalHeaderValue trims surrounding whitespace and collapses internal runs
// of whitespace to one space, which is how SigV4 canonicalises a header value.
func canonicalHeaderValue(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// signingKey derives the scoped signing key:
// HMAC(HMAC(HMAC(HMAC("AWS4"+secret, date), region), service), "aws4_request").
func signingKey(secret, date, region, service string) []byte {
	key := hmacSHA256([]byte("AWS4"+secret), date)
	key = hmacSHA256(key, region)
	key = hmacSHA256(key, service)
	return hmacSHA256(key, terminalRequest)
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

// canonicalQueryString renders query parameters the way SigV4 requires: keys
// sorted, both sides percent-encoded with the unreserved set only, and repeated
// values sorted within a key.
func canonicalQueryString(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	first := true
	for _, key := range keys {
		entries := append([]string(nil), values[key]...)
		sort.Strings(entries)
		for _, value := range entries {
			if !first {
				b.WriteString("&")
			}
			first = false
			b.WriteString(awsEscape(key))
			b.WriteString("=")
			b.WriteString(awsEscape(value))
		}
	}
	return b.String()
}

// unreserved is the RFC 3986 unreserved set, the only bytes SigV4 leaves alone.
const unreserved = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"

const upperHex = "0123456789ABCDEF"

// awsEscape percent-encodes every byte outside the unreserved set with upper-case
// hex digits. net/url cannot be used for this: QueryEscape encodes a space as
// "+", and PathEscape leaves different bytes alone in different Go versions,
// both of which change the signature.
func awsEscape(value string) string {
	needs := false
	for i := 0; i < len(value); i++ {
		if strings.IndexByte(unreserved, value[i]) < 0 {
			needs = true
			break
		}
	}
	if !needs {
		return value
	}

	var b strings.Builder
	b.Grow(len(value) * 3)
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if strings.IndexByte(unreserved, ch) >= 0 {
			b.WriteByte(ch)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(upperHex[ch>>4])
		b.WriteByte(upperHex[ch&0x0f])
	}
	return b.String()
}

// escapePath encodes a slash-separated path one segment at a time, leaving the
// separators themselves intact so the object key keeps its structure.
func escapePath(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = awsEscape(segment)
	}
	return strings.Join(segments, "/")
}
