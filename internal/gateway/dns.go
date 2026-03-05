package gateway

import (
	"encoding/binary"
	"io"
	"log/slog"
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
	//
	// markDialer() stamps the socket with nanotunMark (SO_MARK on Linux) so
	// that the nft/iptables port-53 redirect rules can exempt this connection
	// and avoid a forwarding loop.
	upConn, err := markDialer().Dial("udp", r.upstream.String())
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

// HandleTCP processes one DNS query over a TCP connection (DNS-over-TCP).
// DNS over TCP frames each message with a 2-byte big-endian length prefix.
// The conn is always closed when HandleTCP returns.
func (r *DNSRelay) HandleTCP(conn adapter.TCPConn) {
	defer conn.Close()

	// --- read the 2-byte length prefix from the client ---
	conn.SetReadDeadline(time.Now().Add(dnsReadTimeout))
	var lenBuf [2]byte
	if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
		r.log.Debug("dns relay tcp: read length prefix", "err", err)
		return
	}
	msgLen := int(binary.BigEndian.Uint16(lenBuf[:]))
	if msgLen == 0 || msgLen > dnsBufSize {
		r.log.Debug("dns relay tcp: invalid message length", "len", msgLen)
		return
	}

	// --- read the DNS query ---
	query := make([]byte, msgLen)
	if _, err := io.ReadFull(conn, query); err != nil {
		r.log.Debug("dns relay tcp: read query", "err", err)
		return
	}

	// --- forward to upstream DNS via TCP, bypassing the tunnel ---
	upConn, err := markDialer().Dial("tcp", r.upstream.String())
	if err != nil {
		r.log.Debug("dns relay tcp: dial upstream", "upstream", r.upstream, "err", err)
		return
	}
	defer upConn.Close()
	upConn.SetDeadline(time.Now().Add(dnsForwardTimeout))

	// Write length-prefixed query to upstream.
	wire := make([]byte, 2+msgLen)
	binary.BigEndian.PutUint16(wire[:2], uint16(msgLen))
	copy(wire[2:], query)
	if _, err := upConn.Write(wire); err != nil {
		r.log.Debug("dns relay tcp: write query to upstream", "err", err)
		return
	}

	// --- read the length-prefixed response from upstream ---
	var respLenBuf [2]byte
	if _, err := io.ReadFull(upConn, respLenBuf[:]); err != nil {
		r.log.Debug("dns relay tcp: read response length", "err", err)
		return
	}
	respLen := int(binary.BigEndian.Uint16(respLenBuf[:]))
	if respLen == 0 || respLen > dnsBufSize {
		r.log.Debug("dns relay tcp: invalid response length", "len", respLen)
		return
	}
	resp := make([]byte, respLen)
	if _, err := io.ReadFull(upConn, resp); err != nil {
		r.log.Debug("dns relay tcp: read response", "err", err)
		return
	}

	// --- write the length-prefixed response back to the client ---
	conn.SetWriteDeadline(time.Now().Add(dnsReadTimeout))
	respWire := make([]byte, 2+respLen)
	binary.BigEndian.PutUint16(respWire[:2], uint16(respLen))
	copy(respWire[2:], resp)
	if _, err := conn.Write(respWire); err != nil {
		r.log.Debug("dns relay tcp: write response to client", "err", err)
	}
}
