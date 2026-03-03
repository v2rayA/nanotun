package simple

import (
	"fmt"
	"time"

	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/icmp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"gvisor.dev/gvisor/pkg/waiter"

	"github.com/xjasonlyu/tun2socks/v2/core/adapter"
)

const (
	tcpWindowSize        = 1 << 20
	tcpMaxConnAttempts   = 512
	tcpKeepaliveIdle     = 60 * time.Second
	tcpKeepaliveInterval = 30 * time.Second
)

func buildStack(ep stack.LinkEndpoint, handler adapter.TransportHandler, enableIPv6 bool) (*stack.Stack, error) {
	netProtocols := []stack.NetworkProtocolFactory{ipv4.NewProtocol}
	icmpProtocols := []stack.TransportProtocolFactory{icmp.NewProtocol4}
	if enableIPv6 {
		netProtocols = append(netProtocols, ipv6.NewProtocol)
		icmpProtocols = append(icmpProtocols, icmp.NewProtocol6)
	}

	s := stack.New(stack.Options{
		NetworkProtocols: netProtocols,
		TransportProtocols: append([]stack.TransportProtocolFactory{
			tcp.NewProtocol,
			udp.NewProtocol,
		}, icmpProtocols...),
	})

	nicID := s.NextNICID()
	if err := s.CreateNIC(nicID, ep); err != nil {
		return nil, fmt.Errorf("create nic: %w", err)
	}
	if err := s.SetPromiscuousMode(nicID, true); err != nil {
		return nil, fmt.Errorf("promiscuous: %w", err)
	}

	routes := []tcpip.Route{{Destination: header.IPv4EmptySubnet, NIC: nicID}}
	if enableIPv6 {
		routes = append(routes, tcpip.Route{Destination: header.IPv6EmptySubnet, NIC: nicID})
	}
	s.SetRouteTable(routes)

	registerTCPForwarder(s, handler)
	registerUDPForwarder(s, handler)

	return s, nil
}

func registerTCPForwarder(s *stack.Stack, handler adapter.TransportHandler) {
	forwarder := tcp.NewForwarder(s, tcpWindowSize, tcpMaxConnAttempts, func(req *tcp.ForwarderRequest) {
		var wq waiter.Queue
		ep, err := req.CreateEndpoint(&wq)
		if err != nil {
			req.Complete(true)
			return
		}
		defer req.Complete(false)

		if err := setSocketOptions(s, ep); err != nil {
			ep.Close()
			return
		}

		conn := &tcpConn{TCPConn: gonet.NewTCPConn(&wq, ep), id: req.ID()}
		handler.HandleTCP(conn)
	})
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, forwarder.HandlePacket)
}

func registerUDPForwarder(s *stack.Stack, handler adapter.TransportHandler) {
	forwarder := udp.NewForwarder(s, func(req *udp.ForwarderRequest) {
		var wq waiter.Queue
		ep, err := req.CreateEndpoint(&wq)
		if err != nil {
			return
		}
		conn := &udpConn{UDPConn: gonet.NewUDPConn(&wq, ep), id: req.ID()}
		handler.HandleUDP(conn)
	})
	s.SetTransportProtocolHandler(udp.ProtocolNumber, forwarder.HandlePacket)
}

func setSocketOptions(_ *stack.Stack, ep tcpip.Endpoint) tcpip.Error {
	ep.SocketOptions().SetKeepAlive(true)

	idle := tcpip.KeepaliveIdleOption(tcpKeepaliveIdle)
	if err := ep.SetSockOpt(&idle); err != nil {
		return err
	}

	interval := tcpip.KeepaliveIntervalOption(tcpKeepaliveInterval)
	if err := ep.SetSockOpt(&interval); err != nil {
		return err
	}

	if err := ep.SetSockOptInt(tcpip.KeepaliveCountOption, 9); err != nil {
		return err
	}
	return nil
}

type tcpConn struct {
	*gonet.TCPConn
	id stack.TransportEndpointID
}

func (c *tcpConn) ID() *stack.TransportEndpointID {
	return &c.id
}

type udpConn struct {
	*gonet.UDPConn
	id stack.TransportEndpointID
}

func (c *udpConn) ID() *stack.TransportEndpointID {
	return &c.id
}
