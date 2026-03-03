// Package gateway provides a transport-handler wrapper that intercepts
// connections destined for the virtual gateway address and handles them
// locally (e.g. DNS relay), without forwarding them through the upstream proxy.
package gateway

import (
	"log/slog"
	"net/netip"

	"gvisor.dev/gvisor/pkg/tcpip"

	"github.com/xjasonlyu/tun2socks/v2/core/adapter"

	"github.com/v2rayA/nanotun/internal/netaddr"
)

// Handler wraps an adapter.TransportHandler and intercepts traffic destined
// for the well-known gateway IP addresses.
//
//   - TCP to gateway: dropped immediately (no service runs on TCP at the GW).
//   - UDP/53 to gateway: handled by the built-in DNS relay.
//   - Everything else: forwarded to the upstream handler unchanged.
type Handler struct {
	upstream adapter.TransportHandler
	dns      *DNSRelay
	gw4      netip.Addr
	gw6      netip.Addr
	log      *slog.Logger
}

// New creates a gateway Handler.
// upstream is the real transport handler (e.g. *tunnel.Tunnel).
// dnsUpstream is where DNS queries are forwarded; if zero, 8.8.8.8:53 is used.
func New(upstream adapter.TransportHandler, dnsUpstream netip.AddrPort, log *slog.Logger) *Handler {
	if !dnsUpstream.IsValid() {
		dnsUpstream = netaddr.DefaultUpstreamDNS
	}
	if log == nil {
		log = slog.Default()
	}
	return &Handler{
		upstream: upstream,
		dns:      newDNSRelay(dnsUpstream, log),
		gw4:      netaddr.GatewayIPv4,
		gw6:      netaddr.GatewayIPv6,
		log:      log,
	}
}

// HandleTCP implements adapter.TransportHandler.
// TCP connections to the gateway IP are silently dropped.
func (h *Handler) HandleTCP(conn adapter.TCPConn) {
	id := conn.ID()
	dst := addrFromTCPIP(id.LocalAddress)
	if dst == h.gw4 || dst == h.gw6 {
		_ = conn.Close()
		return
	}
	h.upstream.HandleTCP(conn)
}

// HandleUDP implements adapter.TransportHandler.
// UDP datagrams to gateway-port-53 are proxied by the DNS relay; all
// other UDP is forwarded to the upstream handler.
func (h *Handler) HandleUDP(conn adapter.UDPConn) {
	id := conn.ID()
	dst := addrFromTCPIP(id.LocalAddress)
	if (dst == h.gw4 || dst == h.gw6) && id.LocalPort == 53 {
		go h.dns.Handle(conn)
		return
	}
	h.upstream.HandleUDP(conn)
}

// addrFromTCPIP converts a gVisor tcpip.Address to a net/netip.Addr.
// IPv4-mapped IPv6 addresses are automatically unmapped.
func addrFromTCPIP(a tcpip.Address) netip.Addr {
	b := a.AsSlice()
	addr, ok := netip.AddrFromSlice(b)
	if !ok {
		return netip.Addr{}
	}
	return addr.Unmap()
}
