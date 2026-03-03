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
