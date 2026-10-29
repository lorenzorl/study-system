package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
)

// T4/S4: every address class a server-side fetch must never reach is refused.
// The table is the rule's specification: an address that reaches one of these
// classes is a request from this process into its own network, so the answer
// must be no for each of them and not only for the one that was reported first.
func TestPublicAddressRule_RefusesAddressesTheServerMustNotReach(t *testing.T) {
	for _, raw := range []string{
		"127.0.0.1",          // loopback
		"::1",                // loopback, IPv6
		"::ffff:127.0.0.1",   // the IPv4-mapped spelling of loopback
		"10.0.0.1",           // private, 10/8
		"172.16.0.1",         // private, 172.16/12
		"192.168.1.1",        // private, 192.168/16
		"169.254.169.254",    // link-local, the metadata service
		"0.0.0.0",            // unspecified: "this host" to a dialer
		"100.64.0.1",         // CGNAT, 100.64/10
		"fd00::1",            // unique-local, fc00::/7
		"fe80::1",            // link-local, fe80::/10
		"0.1.2.3",            // 0/8 is "this network"; Linux routes it to the local host
		"192.0.0.1",          // IETF protocol assignments, 192.0.0/24
		"198.18.0.1",         // benchmarking range, 198.18/15
		"240.0.0.1",          // reserved, 240/4; broadcast lives here too
		"255.255.255.255",    // limited broadcast
		"64:ff9b::7f00:1",    // NAT64-embedded 127.0.0.1
		"64:ff9b::a9fe:a9fe", // NAT64-embedded 169.254.169.254
		"2002:7f00:1::",      // 6to4-embedded 127.0.0.1
	} {
		t.Run(raw, func(t *testing.T) {
			addr, err := netip.ParseAddr(raw)
			require.NoError(t, err)

			require.Error(t, PublicAddressRule(addr), "%s must be refused", raw)
		})
	}
}

// T4/S4: the rule is an allowlist of classes, not a list of known-bad hosts, so
// ordinary public addresses still pass. Without this the previous test would
// also pass for a rule that refuses everything.
func TestPublicAddressRule_AcceptsPublicAddresses(t *testing.T) {
	for _, raw := range []string{
		"8.8.8.8",
		"2606:4700::1111",
		"::ffff:8.8.8.8",   // the IPv4-mapped spelling of a public address
		"64:ff9b::808:808", // NAT64-embedded 8.8.8.8: still reachable on a NAT64 network
		"2002:808:808::",   // 6to4-embedded 8.8.8.8: still reachable through the tunnel
	} {
		t.Run(raw, func(t *testing.T) {
			addr, err := netip.ParseAddr(raw)
			require.NoError(t, err)

			assert.NoError(t, PublicAddressRule(addr), "%s must be allowed", raw)
		})
	}
}

// T4/S4: the rule is applied to the address the connection is about to be opened
// to, after the name was resolved. This is the half of the guarantee DNS
// rebinding cannot pass: the pre-flight check sees the first answer, this sees
// the address actually dialled.
func TestNewDialControl_RefusesTheAddressBeingDialled(t *testing.T) {
	control := newDialControl(PublicAddressRule)

	for _, address := range []string{
		"10.0.0.1:80",
		"[::1]:80",
		"[::ffff:10.0.0.1]:443",
		"169.254.169.254:80",
	} {
		t.Run(address, func(t *testing.T) {
			err := control("tcp", address, nil)
			require.Error(t, err, "a private target must never reach the connect")
			assert.ErrorIs(t, err, errAddressRefused,
				"the refusal is marked, so the handler can answer 400 instead of 502")
		})
	}

	t.Run("public address", func(t *testing.T) {
		require.NoError(t, control("tcp", "8.8.8.8:443", nil))
	})

	t.Run("unreadable address", func(t *testing.T) {
		// A rule that cannot be applied is a refusal: an address the control
		// could not parse is not one it may allow.
		require.Error(t, control("tcp", "not-an-address", nil))
	})
}

// deadlineUseCase is never reached by the timeout test: the deadline fires while
// the fetch is still waiting, which is exactly what stops a payload from reaching
// the corpus.
type deadlineUseCase struct{ calls atomic.Int32 }

func (u *deadlineUseCase) Execute(context.Context, app.IngestTextRequest) (app.IngestTextReport, error) {
	u.calls.Add(1)
	return app.IngestTextReport{}, nil
}

// T4/S4 — one deadline bounds the whole fetch, not each step, so an origin that
// answers a header and then drips cannot hold the request open. The timeout is
// injected here so the test waits 50ms instead of the production 20s.
func TestIngestURLHandler_TheDeadlineBoundsTheWholeFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	useCase := &deadlineUseCase{}
	handler := newIngestURLHandler(
		useCase, 1<<20, func(netip.Addr) error { return nil }, 50*time.Millisecond,
	)

	body, err := json.Marshal(IngestURLRequest{URL: srv.URL + "/slow"})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(
		http.MethodPost, "/api/rag/documents/url", bytes.NewReader(body)))

	require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "did not finish answering within")
	require.Zero(t, useCase.calls.Load(), "a timed-out fetch never reaches the corpus")
}
