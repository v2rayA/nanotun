// Package gateway provides a transport-handler wrapper that intercepts
// connections destined for the virtual gateway address and handles them
// locally (e.g. DNS relay), without forwarding them through the upstream proxy.
// It also silently drops LAN-only protocols (NetBIOS, SMB, mDNS) that must
// never be forwarded to a remote proxy.
package gateway

import (
	"log/slog"
	"net/netip"

	"gvisor.dev/gvisor/pkg/tcpip"

	"github.com/xjasonlyu/tun2socks/v2/core/adapter"

	"github.com/v2rayA/nanotun/internal/netaddr"
)

// lanOnlyTCPPorts lists TCP ports that carry LAN-local protocols and must
// never be forwarded to a remote proxy.
//
//	139 – NetBIOS Session Service
//	445 – SMB/CIFS (also used by Samba)
var lanOnlyTCPPorts = map[uint16]bool{
	139: true,
	445: true,
}

// lanOnlyUDPPorts lists UDP ports that carry LAN-local protocols.
//
//	137 – NetBIOS Name Service
//	138 – NetBIOS Datagram Service
//	445 – SMB-over-UDP
//	5353 – Multicast DNS (mDNS)
var lanOnlyUDPPorts = map[uint16]bool{
	137:  true,
	138:  true,
	445:  true,
	5353: true,
}

// Handler wraps an adapter.TransportHandler and intercepts traffic destined
// for the well-known gateway IP addresses.
//
//   - TCP to gateway: dropped immediately (no service runs on TCP at the GW).
//   - TCP on LAN-only ports (NetBIOS/SMB): dropped.
//   - UDP/53 to gateway: handled by the built-in DNS relay.
//   - UDP on LAN-only ports (NetBIOS/mDNS): dropped.
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
// TCP connections to the gateway IP or on LAN-only ports are silently dropped.
func (h *Handler) HandleTCP(conn adapter.TCPConn) {
	id := conn.ID()
	dst := addrFromTCPIP(id.LocalAddress)
	if dst == h.gw4 || dst == h.gw6 {
		_ = conn.Close()
		return
	}
	if lanOnlyTCPPorts[id.LocalPort] {
		h.log.Debug("gateway: drop LAN-only TCP", "port", id.LocalPort, "dst", dst)
		_ = conn.Close()
		return
	}
	h.upstream.HandleTCP(conn)
}

// HandleUDP implements adapter.TransportHandler.
// UDP datagrams to gateway-port-53 are proxied by the DNS relay;
// datagrams on LAN-only ports are dropped;
// all other UDP is forwarded to the upstream handler.
func (h *Handler) HandleUDP(conn adapter.UDPConn) {
	id := conn.ID()
	dst := addrFromTCPIP(id.LocalAddress)
	if (dst == h.gw4 || dst == h.gw6) && id.LocalPort == 53 {
		go h.dns.Handle(conn)
		return
	}
	if lanOnlyUDPPorts[id.LocalPort] {
		h.log.Debug("gateway: drop LAN-only UDP", "port", id.LocalPort, "dst", dst)
		_ = conn.Close()
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
