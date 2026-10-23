package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
)

const (
	// maxFetchRedirects is how many redirects one fetch follows. It is three
	// because that is enough for the ordinary hops a document link takes — a
	// moved page, its canonical form, a language redirect — and small enough that
	// a redirection loop costs three round trips instead of the whole deadline.
	maxFetchRedirects = 3

	// fetchTimeout bounds one URL ingest from the first DNS query to the last
	// byte of the body.
	//
	// It is one deadline for the whole fetch, not one per step: a server that
	// answers a byte a second would otherwise hold the request forever, because
	// every intermediate read would look healthy. Twenty seconds is long enough
	// for a text document over a slow link, including its redirects, and short
	// enough that a hanging host cannot pin a request slot indefinitely.
	fetchTimeout = 20 * time.Second

	// urlFetchUserAgent identifies who is fetching. A site that logs its traffic
	// should be able to see that the request came from a local study app rather
	// than from a browser the user believes they are using.
	urlFetchUserAgent = "study-system-url-ingest"

	// maxResponseHeaderBytes caps one response's header block. The body is
	// bounded by maxBodyBytes, but the headers are read before that bound
	// applies, so without this an origin could answer with an endless header
	// block and turn a text fetch into a memory problem. One mebibyte is far more
	// than any page needs and small enough to be harmless.
	maxResponseHeaderBytes = 1 << 20
)

// acceptedContentTypes is the allowlist of upstream payloads this endpoint reads.
//
// It is an allowlist rather than a blocklist because the corpus stores text: a
// PDF, an HTML page and an image all need a different extraction stage than the
// one this route runs, so anything not named here is something the caller has to
// bring through the route that can read it.
var acceptedContentTypes = map[string]struct{}{
	"text/plain":       {},
	"text/markdown":    {},
	"application/json": {},
}

// fetchRefusal is a refusal with the status code the fetch decided on.
//
// It exists so the decision stays where the evidence is: the fetch knows whether
// the URL was wrong (400), the upstream was wrong (502), or the payload was one
// this corpus cannot read (415/413), and ServeHTTP only has to write it.
type fetchRefusal struct {
	status  int
	message string
}

func (r *fetchRefusal) Error() string { return r.message }

// refuse builds a fetchRefusal. The format and its arguments become the message
// the client reads, so they must not carry anything the client does not own.
func refuse(status int, format string, args ...any) *fetchRefusal {
	return &fetchRefusal{status: status, message: fmt.Sprintf(format, args...)}
}

// IngestURLHandler handles POST /api/rag/documents/url.
//
// It fetches a text the user named by URL and hands it to the same
// IngestTextUseCase the paste route uses, so the corpus keeps one write path:
// identity, the single page, the index and the vectors are decided by exactly one
// piece of code regardless of where the text came from. What this route adds is
// only the fetching, and the fetching is the security surface — a server that
// follows a user-supplied URL is a proxy into its own network.
type IngestURLHandler struct {
	useCase IngestTextUseCase

	// rule and client are the address guarantee. The client is built once, in the
	// constructor, so a handler cannot be created without the dial-time rule
	// applied: there is no code path that fetches with a plain client.
	rule   AddressRule
	client *http.Client

	// maxBodyBytes is the download ceiling, the same STUDY_MAX_UPLOAD_BYTES the
	// artifact store applies. An upstream body larger than it would be refused by
	// the store after it had already been read into memory, so it is refused
	// while it is being read instead.
	maxBodyBytes int64

	// timeout bounds one fetch from the first DNS query to the last byte. It is a
	// field only so a test can run the deadline with a value it can wait for;
	// production always builds the handler with fetchTimeout.
	timeout time.Duration
}

// NewIngestURLHandler builds the production handler.
//
// The address rule is not a parameter here on purpose: this is the only
// constructor production code calls, so no wiring can ship a fetch that reaches
// the machine's own network by omitting the rule or by passing a permissive one.
// Tests that need fixtures on 127.0.0.1 — which the rule refuses, and must
// refuse — use NewIngestURLHandlerWithRule.
func NewIngestURLHandler(useCase IngestTextUseCase, maxBodyBytes int64) *IngestURLHandler {
	return NewIngestURLHandlerWithRule(useCase, maxBodyBytes, PublicAddressRule)
}

// NewIngestURLHandlerWithRule creates an IngestURLHandler over an explicit
// address policy. rule is enforced twice: once on the URL being fetched and once
// on the address the dialer is about to connect to.
//
// It is the tests' constructor: the fixtures they need live on an address the
// production rule refuses because that rule is the whole guarantee.
func NewIngestURLHandlerWithRule(
	useCase IngestTextUseCase, maxBodyBytes int64, rule AddressRule,
) *IngestURLHandler {
	return newIngestURLHandler(useCase, maxBodyBytes, rule, fetchTimeout)
}

// newIngestURLHandler is the one both constructors go through, and it exists so a
// test can bound the fetch with a deadline it can afford to wait for. Production
// always passes fetchTimeout.
func newIngestURLHandler(
	useCase IngestTextUseCase, maxBodyBytes int64, rule AddressRule, timeout time.Duration,
) *IngestURLHandler {
	return &IngestURLHandler{
		useCase:      useCase,
		rule:         rule,
		client:       newFetchClient(rule, timeout),
		maxBodyBytes: maxBodyBytes,
		timeout:      timeout,
	}
}

// newFetchClient builds the HTTP client one fetch uses.
//
// It is deliberately not http.DefaultClient or any shared client: the address
// rule lives in the transport, so a client without it is a client that fetches
// anything the URL says.
func newFetchClient(rule AddressRule, timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		// Timeout is the dial's own ceiling, so a black-holed connection fails
		// before the whole-fetch deadline has to expire to save it.
		Timeout: timeout,
		Control: newDialControl(rule),
	}

	transport := &http.Transport{
		DialContext: dialer.DialContext,
		// Proxy stays nil on purpose. Honouring HTTP_PROXY would make the dial
		// target the proxy, so the rule would inspect the proxy's address — which
		// is public — while the proxy itself reached 127.0.0.1 or the metadata
		// service on this machine. The rule must see the address the request
		// finally goes to, so this client ignores proxies entirely.
		Proxy:               nil,
		TLSHandshakeTimeout: timeout,
		// A response's headers are read before any byte of the body is bounded by
		// maxBodyBytes, so an upstream that answers with an endless header block
		// would otherwise be bounded only by the deadline. The cap keeps a
		// hostile origin from turning a text fetch into a memory problem.
		MaxResponseHeaderBytes: maxResponseHeaderBytes,
	}

	return &http.Client{
		Transport: transport,
		// Redirects are followed by this handler and not by the client, because
		// every hop has to pass the scheme and address rules before it is
		// fetched. Returning ErrUseLastResponse makes that the only behaviour
		// available: a redirect answer is returned to the caller instead of being
		// chased by the transport, and the loop below validates each hop itself.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// fetchedResource is one successfully downloaded text and where it came from.
type fetchedResource struct {
	// url is the URL that was finally fetched, after any redirects. It is what
	// the document records as its origin.
	url *url.URL

	// text is the downloaded payload, valid UTF-8 and trimmed at the edges.
	text string
}

func (h *IngestURLHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var request IngestURLRequest
	if !readJSONBody(w, r, h.maxBodyBytes, &request) {
		return
	}

	rawURL := strings.TrimSpace(request.URL)
	if rawURL == "" {
		httpx.WriteError(w, http.StatusBadRequest, "url is required")
		return
	}

	title := strings.TrimSpace(request.Title)
	// The bound is the text route's, so the two creation paths cannot disagree
	// about how long a document's name may be.
	if utf8.RuneCountInString(title) > MaxTextDocumentTitleRunes {
		httpx.WriteError(w, http.StatusBadRequest,
			fmt.Sprintf("title must be at most %d characters", MaxTextDocumentTitleRunes))
		return
	}

	fetched, err := h.fetch(r.Context(), rawURL)
	if err != nil {
		h.writeFetchError(w, err)
		return
	}

	// The ingest gets the request's own context, not the fetch deadline: the
	// download being bounded says nothing about how long the corpus may take to
	// index and embed what it received.
	report, err := h.useCase.Execute(r.Context(), app.IngestTextRequest{
		Title:     titleFor(title, fetched.url),
		Text:      fetched.text,
		SourceURI: fetched.url.String(),
	})
	if err != nil {
		if errors.Is(err, apperror.ErrConflict) {
			httpx.WriteError(w, http.StatusConflict, duplicateTextMessage)
			return
		}
		httpx.WriteErrorFrom(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, IngestURLResponse{
		IngestTextResponse: IngestTextResponse{
			DocumentID: report.DocumentID,
			State:      string(report.State),
			Chunks:     report.Chunks,
			Embedded:   report.Embedded,
		},
		SourceURI: fetched.url.String(),
	})
}

// fetch downloads the text raw names, validating every hop before it is fetched.
//
// The loop is the redirect policy: the client never follows one, so each hop is
// parsed and checked by checkTarget first, and a hop that names a refused scheme
// or address is answered as the client's mistake rather than being chased.
func (h *IngestURLHandler) fetch(ctx context.Context, raw string) (fetchedResource, error) {
	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	target, err := url.Parse(raw)
	if err != nil {
		return fetchedResource{}, refuse(http.StatusBadRequest, "the URL cannot be parsed: %v", err)
	}

	current := target
	for hop := 0; ; hop++ {
		if err := h.checkTarget(ctx, current); err != nil {
			return fetchedResource{}, err
		}

		request, err := http.NewRequestWithContext(ctx, http.MethodGet, current.String(), nil)
		if err != nil {
			return fetchedResource{}, refuse(http.StatusBadRequest, "the URL cannot be requested: %v", err)
		}
		request.Header.Set("User-Agent", urlFetchUserAgent)
		request.Header.Set("Accept", "text/plain, text/markdown, application/json")

		response, err := h.client.Do(request)
		if err != nil {
			return fetchedResource{}, h.transportRefusal(current, err)
		}

		if isRedirect(response.StatusCode) {
			response.Body.Close()

			// The bound is on hops followed, so a chain of exactly
			// maxFetchRedirects redirects still succeeds and the next one is
			// refused. Counting the answers instead would refuse a legitimate
			// last hop one step early.
			if hop >= maxFetchRedirects {
				return fetchedResource{}, refuse(http.StatusBadRequest,
					"the URL redirects more than %d times", maxFetchRedirects)
			}

			// Location resolves a relative target against the URL that answered,
			// so the next hop is an absolute URL the loop can validate.
			next, err := response.Location()
			if err != nil {
				return fetchedResource{}, refuse(http.StatusBadRequest,
					"the redirect from %s does not name a usable location: %v", current.Redacted(), err)
			}
			current = next
			continue
		}

		text, err := h.readText(current, response)
		if err != nil {
			return fetchedResource{}, err
		}

		return fetchedResource{url: current, text: text}, nil
	}
}

// checkTarget applies the scheme and address rules to one hop before it is
// fetched.
//
// Both rules live here rather than in the URL parser alone because this is the
// one place every hop passes through — the first request and every redirect.
func (h *IngestURLHandler) checkTarget(ctx context.Context, target *url.URL) error {
	// The scheme allowlist is what keeps file://, data: and javascript: out. A
	// client that hands this endpoint a local path or an inline payload is asking
	// the server to read its own filesystem; only the two fetchable schemes are
	// accepted, and the message names the one that was refused.
	switch scheme := strings.ToLower(target.Scheme); scheme {
	case "http", "https":
	case "":
		return refuse(http.StatusBadRequest,
			"the URL has no scheme; only http and https are fetched")
	default:
		return refuse(http.StatusBadRequest,
			"the URL scheme %q is not allowed; only http and https are fetched", scheme)
	}

	host := target.Hostname()
	if host == "" {
		return refuse(http.StatusBadRequest, "the URL has no host")
	}

	addresses, err := lookupAddresses(ctx, host)
	if err != nil {
		return refuse(http.StatusBadRequest,
			"the URL host %q could not be resolved: %v", host, err)
	}

	// Every answer the name gives must pass, not just one: a name that answers
	// with a public and a private address is a name a retry can dial into the
	// private one, so accepting it because one answer looked public would be the
	// bypass this check exists to close.
	for _, addr := range addresses {
		if err := h.rule(addr); err != nil {
			return refuse(http.StatusBadRequest,
				"%s resolves to %s, an address the server will not fetch", target.Redacted(), addr)
		}
	}

	return nil
}

// transportRefusal turns a failed round trip into the client's or the upstream's
// refusal.
//
// A refusal the dialer raised is the client's: the URL resolved to an address
// this server does not fetch, which is a 400 and must not be reported as an
// upstream failure. Everything else — a dead host, a TLS failure, the deadline —
// is the upstream's, so it is a 502 that says what happened.
func (h *IngestURLHandler) transportRefusal(target *url.URL, err error) error {
	if errors.Is(err, errAddressRefused) {
		return refuse(http.StatusBadRequest,
			"%s was refused when the connection was opened: it is not an address the server fetches",
			target.Redacted())
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return refuse(http.StatusBadGateway,
			"the URL did not answer within %s", h.timeout)
	}
	return refuse(http.StatusBadGateway, "the URL could not be fetched: %v", err)
}

// readText validates and reads the payload of a non-redirect response.
//
// The body is read through a LimitReader, so the ceiling applies to the bytes
// actually received rather than to what the upstream claimed: a chunked or
// compressed response has no honest Content-Length to trust.
func (h *IngestURLHandler) readText(target *url.URL, response *http.Response) (string, error) {
	defer response.Body.Close()

	// A non-2xx is the upstream's answer, so it is reported as one, with the
	// status the client needs to explain it to a user.
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return "", refuse(http.StatusBadGateway, "%s answered %s",
			target.Redacted(), responseStatus(response))
	}

	contentType := response.Header.Get("Content-Type")
	if contentType == "" {
		// Guessing would mean sniffing bytes, and sniffed bytes are how an HTML
		// page becomes "text" the corpus stores as markup. The caller is told
		// which route can read what this one cannot.
		return "", refuse(http.StatusUnsupportedMediaType,
			"the URL answered no content type, so it is not known to be text; "+
				"download the file and upload it through POST /api/rag/documents")
	}

	// The media type is compared without its parameters, because text/plain;
	// charset=utf-8 is still text/plain; a charset that is not UTF-8 is caught by
	// the UTF-8 check below rather than by being refused as a different type.
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", refuse(http.StatusUnsupportedMediaType,
			"the URL answered an unreadable content type %q; "+
				"download the file and upload it through POST /api/rag/documents", contentType)
	}
	if _, ok := acceptedContentTypes[mediaType]; !ok {
		return "", refuse(http.StatusUnsupportedMediaType,
			"the URL answered %q, which is not a text payload: download the file and "+
				"upload it through POST /api/rag/documents, or use a text URL "+
				"(an HTML page is not supported yet)", mediaType)
	}

	// One byte past the ceiling separates "exactly at the limit" from "over it":
	// reading exactly maxBodyBytes would leave a truncated payload that looks
	// whole, and the caller would never learn that the rest was dropped.
	payload, err := io.ReadAll(io.LimitReader(response.Body, h.maxBodyBytes+1))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", refuse(http.StatusBadGateway,
				"%s did not finish answering within %s", target.Redacted(), h.timeout)
		}
		return "", refuse(http.StatusBadGateway,
			"the body of %s could not be read: %v", target.Redacted(), err)
	}
	if int64(len(payload)) > h.maxBodyBytes {
		return "", refuse(http.StatusRequestEntityTooLarge,
			"%s answered more than %d bytes, which is the largest material this corpus accepts",
			target.Redacted(), h.maxBodyBytes)
	}

	// The corpus stores text and the index tokenises it. Invalid UTF-8 would be
	// stored as replacement characters, so the material would be silently
	// altered and its content hash would describe bytes nobody can read.
	if !utf8.Valid(payload) {
		return "", refuse(http.StatusBadGateway,
			"%s answered bytes that are not valid UTF-8", target.Redacted())
	}

	text := strings.TrimSpace(string(payload))
	if text == "" {
		return "", refuse(http.StatusBadRequest,
			"%s answered no text to ingest", target.Redacted())
	}
	return text, nil
}

// writeFetchError answers a failed fetch with the status the fetch chose.
func (h *IngestURLHandler) writeFetchError(w http.ResponseWriter, err error) {
	var refusal *fetchRefusal
	if errors.As(err, &refusal) {
		httpx.WriteError(w, refusal.status, refusal.message)
		return
	}

	// Anything else is a defect in this handler, not a fact about the URL.
	httpx.WriteErrorFrom(w, err)
}

// lookupAddresses resolves a URL host to the addresses a connection could go to.
//
// A literal address is answered without asking the resolver, so a URL naming
// 127.0.0.1 is checked even on a machine whose DNS is unreachable.
func lookupAddresses(ctx context.Context, host string) ([]netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{addr}, nil
	}
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

// isRedirect reports whether status is one a fetch may follow. The list is the
// one net/http follows; 300, 304 and 305 are answers, not moves.
func isRedirect(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

// responseStatus is the upstream's status line as text: the status the corpus
// user has to read, with the server's own reason when it sent one.
func responseStatus(response *http.Response) string {
	if response.Status != "" {
		return response.Status
	}
	return fmt.Sprintf("%d %s", response.StatusCode, http.StatusText(response.StatusCode))
}

// titleFor applies the title precedence: the caller's title, then the last path
// segment of the URL that was fetched, then its host.
//
// A corpus list of unnamed rows is a list nobody can choose from, so a fetch
// always produces a name; the fallbacks are the URL's own, which is the only
// other thing the caller told us about the material.
func titleFor(requested string, final *url.URL) string {
	if requested != "" {
		return requested
	}

	// Path is the percent-decoded form, so a segment read from here is the name
	// the server serves rather than its escaped spelling.
	if trimmed := strings.Trim(final.Path, "/"); trimmed != "" {
		segments := strings.Split(trimmed, "/")
		if last := strings.TrimSpace(segments[len(segments)-1]); last != "" {
			return last
		}
	}

	return final.Hostname()
}
