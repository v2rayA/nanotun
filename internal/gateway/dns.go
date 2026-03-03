package gateway

import (
	"log/slog"
	"net"
	"net/netip"
	"time"

	"github.com/xjasonlyu/tun2socks/v2/core/adapter"
)

const (
	dnsReadTimeout    = 5 * time.Second
	dnsForwardTimeout = 5 * time.Second
	// Maximum DNS message size over UDP (4096 to support EDNS0).
	dnsBufSize = 4096
)

// DNSRelay forwards DNS queries arriving on the virtual gateway address to a
// real upstream DNS server over the host's native network (not the tunnel).
type DNSRelay struct {
	upstream netip.AddrPort
	log      *slog.Logger
}

func newDNSRelay(upstream netip.AddrPort, log *slog.Logger) *DNSRelay {
	return &DNSRelay{upstream: upstream, log: log}
}

// Handle processes one DNS query over the provided UDPConn from the gVisor
// stack. It reads a single UDP datagram, forwards it to the upstream resolver,
// and writes the response back. The conn is always closed when Handle returns.
func (r *DNSRelay) Handle(conn adapter.UDPConn) {
	defer conn.Close()

	// --- read query from gVisor-side client ---
	buf := make([]byte, dnsBufSize)
	conn.SetReadDeadline(time.Now().Add(dnsReadTimeout))
	n, err := conn.Read(buf)
	if err != nil {
		r.log.Debug("dns relay: read query", "err", err)
		return
	}
	query := buf[:n]

	// --- forward to upstream DNS via host OS network (bypasses TUN) ---
	upConn, err := net.DialTimeout("udp", r.upstream.String(), dnsForwardTimeout)
	if err != nil {
		r.log.Debug("dns relay: dial upstream", "upstream", r.upstream, "err", err)
		return
	}
	defer upConn.Close()
	upConn.SetDeadline(time.Now().Add(dnsForwardTimeout))

	if _, err := upConn.Write(query); err != nil {
		r.log.Debug("dns relay: write query to upstream", "err", err)
		return
	}

	resp := make([]byte, dnsBufSize)
	m, err := upConn.Read(resp)
	if err != nil {
		r.log.Debug("dns relay: read response from upstream", "err", err)
		return
	}

	// --- write response back to gVisor-side client ---
	conn.SetWriteDeadline(time.Now().Add(dnsReadTimeout))
	if _, err := conn.Write(resp[:m]); err != nil {
		r.log.Debug("dns relay: write response to client", "err", err)
	}
}
