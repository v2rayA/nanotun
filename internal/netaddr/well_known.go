// Package netaddr defines the well-known virtual addresses used by nanotun.
//
// Address layout (minimal, two addresses per family):
//
//	IPv4  198.18.0.0/30  → gateway/DNS = 198.18.0.1  (from RFC 2544 benchmarking range)
//	IPv6  fdfe:dcba:9876::/126  → gateway/DNS = fdfe:dcba:9876::1  (ULA range)
package netaddr

import "net/netip"

var (
	// GatewayIPv4 is assigned to the TUN interface and acts as IPv4 default
	// gateway and local DNS listener for the tunnel.
	GatewayIPv4 = netip.MustParseAddr("198.18.0.1")

	// PrefixIPv4 is the /30 subnet that contains GatewayIPv4.
	// It holds exactly two usable addresses (198.18.0.1 and 198.18.0.2).
	PrefixIPv4 = netip.MustParsePrefix("198.18.0.1/30")

	// GatewayIPv6 is assigned to the TUN interface and acts as IPv6 default
	// gateway and local DNS listener for the tunnel.
	GatewayIPv6 = netip.MustParseAddr("fdfe:dcba:9876::1")

	// PrefixIPv6 is the /126 subnet that contains GatewayIPv6.
	// It holds exactly two usable addresses (::1 and ::2).
	PrefixIPv6 = netip.MustParsePrefix("fdfe:dcba:9876::1/126")

	// DefaultUpstreamDNS is the fallback upstream DNS server used by the
	// built-in DNS relay when no explicit upstream is configured.
	DefaultUpstreamDNS = netip.MustParseAddrPort("8.8.8.8:53")
)
