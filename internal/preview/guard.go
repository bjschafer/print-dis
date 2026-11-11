package preview

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"syscall"
	"time"
)

// ErrBlocked is returned when a connection would reach a refused address.
var ErrBlocked = errors.New("preview: address not allowed")

var blockedPrefixes = mustPrefixes(
	"0.0.0.0/8",       // "this network"
	"100.64.0.0/10",   // CGNAT
	"192.0.0.0/24",    // IETF protocol assignments
	"192.0.2.0/24",    // TEST-NET-1
	"198.18.0.0/15",   // benchmarking
	"198.51.100.0/24", // TEST-NET-2
	"203.0.113.0/24",  // TEST-NET-3
	"240.0.0.0/4",     // reserved, incl. broadcast
	"fc00::/7",        // ULA
	"2001:db8::/32",   // documentation
)

func mustPrefixes(ss ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(ss))
	for i, s := range ss {
		out[i] = netip.MustParsePrefix(s)
	}
	return out
}

var (
	nat64  = netip.MustParsePrefix("64:ff9b::/96")
	sixTo4 = netip.MustParsePrefix("2002::/16")
)

// classify reports why ip must not be connected to, or "" when it is a
// public address. IPv4-mapped IPv6 and addresses that embed an IPv4 address
// (NAT64, 6to4) are judged by the embedded IPv4 address.
func classify(ip netip.Addr) string {
	ip = ip.WithZone("").Unmap()
	switch {
	case !ip.IsValid():
		return "invalid"
	case nat64.Contains(ip):
		b := ip.As16()
		return classify(netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}))
	case sixTo4.Contains(ip):
		b := ip.As16()
		return classify(netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]}))
	case ip.IsLoopback():
		return "loopback"
	case ip.IsUnspecified():
		return "unspecified"
	case ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast():
		return "link-local"
	case ip.IsMulticast(), ip.IsInterfaceLocalMulticast():
		return "multicast"
	case ip.IsPrivate():
		return "private"
	}
	for _, p := range blockedPrefixes {
		if p.Contains(ip) {
			return "reserved"
		}
	}
	return ""
}

// guardedDialer returns a DialContext whose Control hook checks the address
// actually being connected to, after DNS resolution. That covers redirects
// (each hop dials afresh), DNS rebinding (the check sees the IP the socket
// uses, not one looked up earlier) and literal IPs in URLs.
//
// Allowlisting is by the host name this dial was asked for. Each redirect hop
// dials its own URL's host, so an allowlisted host that redirects to a
// non-allowlisted one is checked strictly.
func guardedDialer(c *config) func(context.Context, string, string) (net.Conn, error) {
	mk := func(allowPrivate bool) *net.Dialer {
		return &net.Dialer{
			Timeout: 5 * time.Second,
			Control: func(_, address string, _ syscall.RawConn) error {
				ap, err := netip.ParseAddrPort(address)
				if err != nil {
					return fmt.Errorf("%w: %w", ErrBlocked, err)
				}
				if allowPrivate {
					return nil
				}
				why := classify(ap.Addr())
				if why == "loopback" && c.allowLoopback {
					return nil
				}
				if why != "" {
					return fmt.Errorf("%w (%s)", ErrBlocked, why)
				}
				return nil
			},
		}
	}
	strict, lax := mk(false), mk(true)
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		if c.allowHosts[strings.ToLower(host)] {
			return lax.DialContext(ctx, network, addr)
		}
		return strict.DialContext(ctx, network, addr)
	}
}
