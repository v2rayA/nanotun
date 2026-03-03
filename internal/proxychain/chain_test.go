package proxychain

import (
	"context"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/xjasonlyu/tun2socks/v2/metadata"
	"github.com/xjasonlyu/tun2socks/v2/proxy/proto"
)

func TestChainBypassesExcludedProcess(t *testing.T) {
	upstream := &stubProxy{}
	direct := &stubProxy{}
	matcher := &stubMatcher{skip: true}

	chain := New(upstream, Options{Matcher: matcher})
	chain.direct = direct

	md := &metadata.Metadata{
		Network: metadata.TCP,
		SrcIP:   netip.MustParseAddr("198.18.0.2"),
		SrcPort: 12345,
		DstIP:   netip.MustParseAddr("1.1.1.1"),
		DstPort: 443,
	}

	if _, err := chain.DialContext(context.Background(), md); err != nil {
		t.Fatalf("DialContext returned error: %v", err)
	}

	if direct.dialCount != 1 {
		t.Fatalf("expected direct dial once, got %d", direct.dialCount)
	}
	if upstream.dialCount != 0 {
		t.Fatalf("expected upstream not to be used, got %d dials", upstream.dialCount)
	}
}

func TestChainProxiesWhenNotExcluded(t *testing.T) {
	upstream := &stubProxy{}
	direct := &stubProxy{}
	matcher := &stubMatcher{skip: false}

	chain := New(upstream, Options{Matcher: matcher})
	chain.direct = direct

	md := &metadata.Metadata{
		Network: metadata.TCP,
		SrcIP:   netip.MustParseAddr("198.18.0.2"),
		SrcPort: 12345,
		DstIP:   netip.MustParseAddr("1.1.1.1"),
		DstPort: 443,
	}

	if _, err := chain.DialContext(context.Background(), md); err != nil {
		t.Fatalf("DialContext returned error: %v", err)
	}

	if upstream.dialCount != 1 {
		t.Fatalf("expected upstream dial once, got %d", upstream.dialCount)
	}
	if direct.dialCount != 0 {
		t.Fatalf("expected direct unused, got %d dials", direct.dialCount)
	}
}

type stubMatcher struct {
	skip bool
}

func (m *stubMatcher) ShouldSkip(*metadata.Metadata) bool { return m.skip }

type stubProxy struct {
	dialCount int
	udpCount  int
}

func (s *stubProxy) DialContext(context.Context, *metadata.Metadata) (net.Conn, error) {
	s.dialCount++
	c1, c2 := net.Pipe()
	_ = c2.Close()
	return c1, nil
}

func (s *stubProxy) DialUDP(*metadata.Metadata) (net.PacketConn, error) {
	s.udpCount++
	return &dummyPacketConn{}, nil
}

func (s *stubProxy) Addr() string       { return "" }
func (s *stubProxy) Proto() proto.Proto { return proto.Direct }

type dummyPacketConn struct{}

func (*dummyPacketConn) ReadFrom([]byte) (int, net.Addr, error) { return 0, nil, io.EOF }
func (*dummyPacketConn) WriteTo([]byte, net.Addr) (int, error)  { return 0, nil }
func (*dummyPacketConn) Close() error                           { return nil }
func (*dummyPacketConn) LocalAddr() net.Addr                    { return &net.IPAddr{} }
func (*dummyPacketConn) SetDeadline(time.Time) error            { return nil }
func (*dummyPacketConn) SetReadDeadline(time.Time) error        { return nil }
func (*dummyPacketConn) SetWriteDeadline(time.Time) error       { return nil }
