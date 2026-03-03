package gvisor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	gvstack "gvisor.dev/gvisor/pkg/tcpip/stack"

	"github.com/xjasonlyu/tun2socks/v2/core"
	"github.com/xjasonlyu/tun2socks/v2/core/adapter"
	"github.com/xjasonlyu/tun2socks/v2/core/device/tun"

	"github.com/v2rayA/nanotun/internal/netaddr"
	"github.com/v2rayA/nanotun/internal/tunconf"
)

// Options holds the configuration for the gVisor-backed driver.
type Options struct {
	TunName string
	MTU     int

	Handler adapter.TransportHandler
	Logger  *slog.Logger
}

// Driver runs the fully featured gVisor netstack pipeline.
type Driver struct {
	opts Options
}

// New creates a gVisor driver instance.
func New(opts Options) (*Driver, error) {
	if opts.Handler == nil {
		return nil, fmt.Errorf("transport handler is required")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Driver{opts: opts}, nil
}

// Run blocks until the context is cancelled.
func (d *Driver) Run(ctx context.Context) error {
	dev, err := tun.Open(d.opts.TunName, uint32(d.opts.MTU))
	if err != nil {
		return fmt.Errorf("open tun: %w", err)
	}
	defer dev.Close()

	// Configure the OS-level TUN interface with the well-known gateway IPs.
	if err := tunconf.Configure(dev.Name()); err != nil {
		return fmt.Errorf("configure tun: %w", err)
	}

	gStack, err := core.CreateStack(&core.Config{
		LinkEndpoint:     dev,
		TransportHandler: d.opts.Handler,
	})
	if err != nil {
		return fmt.Errorf("create stack: %w", err)
	}
	defer func() {
		gStack.Close()
		waitWithTimeout(gStack, 2*time.Second)
	}()

	// Bind gateway addresses to the NIC so that gVisor responds to ICMP
	// echo requests (ping) directed at the virtual gateway.
	addGatewayAddresses(gStack)

	d.opts.Logger.Info("gvisor backend ready",
		"device", dev.Name(), "mtu", d.opts.MTU,
		"gateway4", netaddr.GatewayIPv4,
		"gateway6", netaddr.GatewayIPv6,
	)

	<-ctx.Done()
	d.opts.Logger.Info("gvisor backend stopping")
	return ctx.Err()
}

// addGatewayAddresses binds the virtual gateway IPs to the first NIC in the
// gVisor stack, enabling automatic ICMP echo replies.
func addGatewayAddresses(s *gvstack.Stack) {
	// Find the NIC created by core.CreateStack (always NIC 1).
	var nicID tcpip.NICID
	for id := range s.NICInfo() {
		nicID = id
		break
	}
	if nicID == 0 {
		return
	}

	_ = s.AddProtocolAddress(nicID, tcpip.ProtocolAddress{
		Protocol: ipv4.ProtocolNumber,
		AddressWithPrefix: tcpip.AddressWithPrefix{
			Address:   tcpip.AddrFrom4(netaddr.GatewayIPv4.As4()),
			PrefixLen: netaddr.PrefixIPv4.Bits(),
		},
	}, gvstack.AddressProperties{})

	_ = s.AddProtocolAddress(nicID, tcpip.ProtocolAddress{
		Protocol: ipv6.ProtocolNumber,
		AddressWithPrefix: tcpip.AddressWithPrefix{
			Address:   tcpip.AddrFrom16(netaddr.GatewayIPv6.As16()),
			PrefixLen: netaddr.PrefixIPv6.Bits(),
		},
	}, gvstack.AddressProperties{})
}

// waitWithTimeout calls s.Wait() but gives up after the given duration so that
// long-lived proxy connections do not block clean shutdown.
func waitWithTimeout(s *gvstack.Stack, timeout time.Duration) {
	done := make(chan struct{})
	go func() { s.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}
