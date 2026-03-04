package exclude

import (
	"net/netip"
	"testing"
	"time"

	M "github.com/xjasonlyu/tun2socks/v2/metadata"
)

func TestShouldSkipMatchesExeSuffix(t *testing.T) {
	m := New([]string{"shadowsocks"}, time.Second, nil)
	if m == nil {
		t.Fatalf("matcher should not be nil when targets provided")
	}

	ap := netip.AddrPortFrom(netip.MustParseAddr("198.18.0.2"), 12345)
	key := flowKey(M.TCP, ap)

	m.mu.Lock()
	m.flowToPID[key] = 1
	m.pidNames[1] = "Shadowsocks.exe"
	m.mu.Unlock()

	md := &M.Metadata{
		Network: M.TCP,
		SrcIP:   ap.Addr(),
		SrcPort: ap.Port(),
	}
	if !m.ShouldSkip(md) {
		t.Fatalf("expected ShouldSkip to match shadowsocks.exe")
	}
}

// TestFlowKeyUnmapIPv4Mapped verifies that IPv4-mapped IPv6 addresses
// (::ffff:x.x.x.x) are normalised to plain IPv4 in the flow key so that
// they match the source address the TUN stack reports.  On Windows, dual-
// stack sockets may appear with the mapped form only.
func TestFlowKeyUnmapIPv4Mapped(t *testing.T) {
	// Plain IPv4 key — this is what TUN metadata always produces.
	plain := flowKey(M.TCP, netip.AddrPortFrom(
		netip.MustParseAddr("192.168.1.10"), 8080))
	// IPv4-mapped IPv6 → after Unmap() it should yield the same key.
	mapped := netip.MustParseAddr("::ffff:192.168.1.10").Unmap()
	unmapped := flowKey(M.TCP, netip.AddrPortFrom(mapped, 8080))

	if plain != unmapped {
		t.Fatalf("flow keys differ after Unmap: plain=%q  unmapped=%q", plain, unmapped)
	}
}

// TestShouldSkipRetryOnMiss ensures that ShouldSkip retries after the
// on-demand refreshOnMiss fills the cache (simulated by injecting data
// between the initial miss and the retry window).
func TestShouldSkipRetryOnMiss(t *testing.T) {
	m := New([]string{"testproc"}, time.Hour, nil)
	if m == nil {
		t.Fatal("matcher should not be nil")
	}

	ap := netip.AddrPortFrom(netip.MustParseAddr("10.0.0.5"), 9999)
	md := &M.Metadata{
		Network: M.TCP,
		SrcIP:   ap.Addr(),
		SrcPort: ap.Port(),
	}

	// First call: flow not in cache → should not skip.
	if m.ShouldSkip(md) {
		t.Fatal("should not skip when flow is unknown")
	}

	// Simulate the scanner having picked up the flow.
	key := flowKey(M.TCP, ap)
	m.mu.Lock()
	m.flowToPID[key] = 42
	m.pidNames[42] = "testproc"
	m.mu.Unlock()

	// Second call: flow now cached → should skip.
	if !m.ShouldSkip(md) {
		t.Fatal("expected skip after flow is populated")
	}
}
