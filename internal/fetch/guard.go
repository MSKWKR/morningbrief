package fetch

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"syscall"
	"time"
)

// ErrBlockedAddress is returned when the dial policy refuses an address.
var ErrBlockedAddress = errors.New("fetch: address blocked by dial policy")

// Extra ranges that netip's Is* helpers do not cover but which are not
// publicly routable (or are transition mechanisms that can wrap private IPs).
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),     // "this network"
	netip.MustParsePrefix("100.64.0.0/10"), // CGNAT (RFC 6598)
	netip.MustParsePrefix("192.0.0.0/24"),  // IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"), // benchmarking
	netip.MustParsePrefix("240.0.0.0/4"),   // reserved, incl. 255.255.255.255
	netip.MustParsePrefix("64:ff9b::/96"),  // NAT64: can embed any IPv4
	netip.MustParsePrefix("2001:db8::/32"), // documentation
	// Added after review. Note ::ffff:0:0/96 (IPv4-mapped, RFC 4291) is handled
	// by Unmap() in IsPublic and judged as IPv4; the IPv4-translated range of
	// RFC 2765 (SIIT) is ::ffff:0:0:0/96, which is what is listed here.
	netip.MustParsePrefix("64:ff9b:1::/48"),  // local-use NAT64 (RFC 8215)
	netip.MustParsePrefix("2002::/16"),       // 6to4 (RFC 3056)
	netip.MustParsePrefix("2001::/32"),       // Teredo (RFC 4380)
	netip.MustParsePrefix("::/96"),           // IPv4-compatible, deprecated (RFC 4291)
	netip.MustParsePrefix("::ffff:0:0:0/96"), // IPv4-translated (RFC 2765)
	netip.MustParsePrefix("fec0::/10"),       // deprecated site-local (RFC 3879)
	netip.MustParsePrefix("100::/64"),        // discard-only (RFC 6666)
	netip.MustParsePrefix("192.88.99.0/24"),  // 6to4 relay anycast (RFC 7526)
}

// IsPublic reports whether addr is a globally routable unicast address that is
// safe to connect to from inside a private network.
func IsPublic(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	// ::ffff:127.0.0.1 must be judged as 127.0.0.1.
	addr = addr.Unmap()
	if addr.IsUnspecified() || addr.IsLoopback() || addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast() || addr.IsMulticast() {
		return false
	}
	for _, p := range blockedPrefixes {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

// defaultDialPolicy allows only public addresses on ports 80 and 443.
func defaultDialPolicy(ap netip.AddrPort) error {
	if !IsPublic(ap.Addr()) {
		return fmt.Errorf("%w: %s", ErrBlockedAddress, ap.Addr())
	}
	if p := ap.Port(); p != 80 && p != 443 {
		return fmt.Errorf("%w: port %d", ErrBlockedAddress, p)
	}
	return nil
}

// controlFunc adapts a policy to net.Dialer.Control. Control runs after DNS
// resolution with the literal IP:port about to be connected, for every dial
// (each redirect hop, each happy-eyeballs candidate), so DNS rebinding and
// redirects to internal hosts cannot bypass it.
func controlFunc(policy func(netip.AddrPort) error) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, _ syscall.RawConn) error {
		ap, err := netip.ParseAddrPort(address)
		if err != nil {
			return fmt.Errorf("%w: unparsable address", ErrBlockedAddress)
		}
		if err := policy(ap); err != nil {
			if errors.Is(err, ErrBlockedAddress) {
				return err
			}
			return fmt.Errorf("%w: %v", ErrBlockedAddress, err)
		}
		return nil
	}
}

func newDialer(policy func(netip.AddrPort) error) *net.Dialer {
	return &net.Dialer{Timeout: 10 * time.Second, Control: controlFunc(policy)}
}
