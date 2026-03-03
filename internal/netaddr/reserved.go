package netaddr

import "net/netip"

// reservedPrefixes lists IPv4/IPv6 address ranges that are IANA-reserved and
// must never be forwarded to a remote proxy.  The list complements the
// standard-library predicates (IsLoopback, IsPrivate, IsMulticast, …) with
// ranges those predicates do not cover.
var reservedPrefixes = func() []netip.Prefix {
	raw := []string{
		// ── IPv4 ────────────────────────────────────────────────────────────
		"0.0.0.0/8",          // "This" network (RFC 1122)
		"100.64.0.0/10",      // Shared Address Space (RFC 6598)
		"192.0.0.0/24",       // IETF Protocol Assignments (RFC 6890)
		"192.0.2.0/24",       // TEST-NET-1 / documentation (RFC 5737)
		"198.18.0.0/15",      // Benchmarking (RFC 2544) — also nanotun gateway subnet
		"198.51.100.0/24",    // TEST-NET-2 / documentation (RFC 5737)
		"203.0.113.0/24",     // TEST-NET-3 / documentation (RFC 5737)
		"240.0.0.0/4",        // Reserved for future use (RFC 1112)
		"255.255.255.255/32", // Limited broadcast
		// ── IPv6 ────────────────────────────────────────────────────────────
		"::ffff:0:0/96", // IPv4-mapped addresses (RFC 4291)
		"64:ff9b::/96",  // IPv4/IPv6 translation (RFC 6052)
		"100::/64",      // Discard prefix (RFC 6666)
		"2001::/23",     // IETF Protocol Assignments (covers 2001:db8:: etc.)
		"2002::/16",     // 6to4 (RFC 3056)
	}
	out := make([]netip.Prefix, len(raw))
	for i, s := range raw {
		out[i] = netip.MustParsePrefix(s)
	}
	return out
}()

// IsReserved reports whether addr is an IANA-reserved address that should
// never be forwarded to a remote proxy: private ranges, loopback, link-local,
// multicast, broadcast, documentation, benchmarking, and so on.
//
// IPv4-mapped IPv6 addresses are automatically unmapped before the check.
func IsReserved(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() {
		return false
	}
	// Standard-library predicates cover the most common ranges.
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsUnspecified() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() {
		return true
	}
	// Explicit checks for ranges not covered by the standard library.
	for _, p := range reservedPrefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
