package http

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"syscall"
)

// AddressRule decides whether this server may open a connection to addr.
//
// The rule is a parameter of the URL-ingest handler rather than a constant for
// two reasons. The handler tests serve their fixtures from 127.0.0.1, which the
// production rule refuses — and it must refuse it, because the address rule is
// the only thing separating "fetch the document the user named" from "fetch any
// resource this machine can reach on the user's behalf". Injecting the rule is
// what lets the tests exercise the fetch path without weakening the rule they
// ship next to.
type AddressRule func(addr netip.Addr) error

// errAddressRefused marks a refusal produced by an AddressRule.
//
// The mark exists so the transport can answer 400 — the URL the client sent is
// one this server does not fetch — instead of the 502 an upstream failure gets,
// even when the refusal arrives from the dialer rather than from the pre-flight
// check.
var errAddressRefused = errors.New("address refused")

// carrierGradeNATPrefix is 100.64.0.0/10, the shared address space RFC 6598
// reserves for carrier-grade NAT.
//
// It is spelled out because the standard library has no predicate for it, and a
// deployment inside a carrier NAT would otherwise reach its neighbours: the
// addresses are not RFC 1918 private, so every other test in PublicAddressRule
// lets them through.
var carrierGradeNATPrefix = netip.MustParsePrefix("100.64.0.0/10")

// reservedPrefixes are the remaining IPv4 ranges that are well-formed and still
// never a document a person means to read. Each one is a hole when it is missing:
//
//   - 0.0.0.0/8 is "this network", and a dial to an address inside it lands on the
//     local host on Linux — while `IsUnspecified` only sees 0.0.0.0 itself.
//   - 192.0.0.0/24 is IETF protocol assignment space.
//   - 198.18.0.0/15 is the benchmarking range.
//   - 240.0.0.0/4 is reserved, and 255.255.255.255 (limited broadcast) lives in it.
var reservedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"),
}

// The two IPv6 prefixes that embed an IPv4 address in their low bits.
var (
	// nat64Prefix is the well-known prefix of RFC 6052: bytes 12..15 are the IPv4
	// address an IPv6-only host reaches through a NAT64 gateway.
	nat64Prefix = netip.MustParsePrefix("64:ff9b::/96")

	// sixToFourPrefix is the 6to4 prefix of RFC 3056: bytes 2..5 are the IPv4
	// address of the tunnel endpoint.
	sixToFourPrefix = netip.MustParsePrefix("2002::/16")
)

// embeddedIPv4 returns the IPv4 address an IPv6 address points at, and false for
// an address that is not one of the two carrier forms.
func embeddedIPv4(addr netip.Addr) (netip.Addr, bool) {
	if !addr.Is6() || addr.Is4In6() {
		return netip.Addr{}, false
	}

	b := addr.As16()

	switch {
	case nat64Prefix.Contains(addr):
		return netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}), true
	case sixToFourPrefix.Contains(addr):
		return netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]}), true
	}

	return netip.Addr{}, false
}

// PublicAddressRule refuses every address a server-side fetch must not reach: the
// host's own loopback, its private and link-local networks, and the special
// ranges that mean "this host" or "nobody".
//
// A server that fetches a user-supplied URL is a proxy into its own network. The
// cloud metadata service (169.254.169.254) hands out credentials, a service on
// 127.0.0.1 hands out whatever it trusts its own machine to ask, and the private
// ranges reach every device behind the router. None of them are what a user means
// by "read this page", so all of them are refused here rather than being left to
// whatever happens to be listening.
func PublicAddressRule(addr netip.Addr) error {
	// An IPv4-mapped IPv6 address reaches the IPv4 address it embeds, so every
	// class below is asked of the unmapped form. Refusing the mapped spelling
	// only would let ::ffff:127.0.0.1 through as if it were a different address.
	if addr.Is4In6() {
		addr = addr.Unmap()
	}

	// Two more IPv6 forms carry an IPv4 address inside them, and on a host with
	// either route the embedding form reaches the embedded address: NAT64
	// (64:ff9b::/96) on an IPv6-only network, 6to4 (2002::/16) on a tunnel. An
	// address that is really 127.0.0.1 or 169.254.169.254 has to be judged as that
	// address, so the rule recurses on the embedded IPv4 — which terminates, since
	// the embedded value is always IPv4.
	if embedded, ok := embeddedIPv4(addr); ok {
		return PublicAddressRule(embedded)
	}

	switch {
	case !addr.IsValid():
		return fmt.Errorf("%w: not a usable address", errAddressRefused)
	case addr.IsUnspecified():
		// 0.0.0.0 and :: are dialled as "this host", which makes them the most
		// local address there is rather than the least.
		return fmt.Errorf("%w: %s is the unspecified address", errAddressRefused, addr)
	case addr.IsLoopback():
		return fmt.Errorf("%w: %s is a loopback address", errAddressRefused, addr)
	case addr.IsPrivate():
		// RFC 1918 for IPv4 and RFC 4193 (fc00::/7) for IPv6, which is every
		// address on the machine's own network.
		return fmt.Errorf("%w: %s is a private address", errAddressRefused, addr)
	case addr.IsLinkLocalUnicast(), addr.IsLinkLocalMulticast():
		// 169.254/16 and fe80::/10: the neighbours on the same link, including
		// the metadata service at 169.254.169.254.
		return fmt.Errorf("%w: %s is a link-local address", errAddressRefused, addr)
	case addr.IsMulticast():
		return fmt.Errorf("%w: %s is a multicast address", errAddressRefused, addr)
	case carrierGradeNATPrefix.Contains(addr):
		return fmt.Errorf("%w: %s is carrier-grade NAT space", errAddressRefused, addr)
	}

	for _, prefix := range reservedPrefixes {
		if prefix.Contains(addr) {
			return fmt.Errorf("%w: %s is in the reserved range %s", errAddressRefused, addr, prefix)
		}
	}

	return nil
}

// newDialControl builds the net.Dialer.Control that applies rule to the address
// the connection is about to be opened to.
//
// This is the enforceable half of the address rule, and the reason the check on
// the parsed URL is not enough on its own: a name can answer with a public
// address when it is validated and with a private one a moment later — DNS
// rebinding — and only the address handed to the dialer is the one that decides
// where the request actually goes. The pre-flight check exists for a readable
// error and a refused request before any connection; this control is what makes
// the guarantee hold regardless of what the name says next.
//
// Control runs after the resolution and before the connect, so refusing here
// cannot be raced by the resolver changing its answer again.
func newDialControl(rule AddressRule) func(network, address string, conn syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("%w: %q is not a host:port address", errAddressRefused, address)
		}
		addr, err := netip.ParseAddr(host)
		if err != nil {
			// The dialer hands this func a resolved address. Anything else means
			// the rule cannot be applied to what is being dialled, and a rule
			// that cannot be applied is a refusal, never a pass.
			return fmt.Errorf("%w: %q is not an IP address", errAddressRefused, host)
		}
		return rule(addr)
	}
}
