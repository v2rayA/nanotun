package proxychain

import (
	"context"
	"log/slog"
	"net"
	"net/netip"

	"github.com/xjasonlyu/tun2socks/v2/metadata"
	"github.com/xjasonlyu/tun2socks/v2/proxy"

	"github.com/v2rayA/nanotun/internal/netaddr"
)

// Options configures the behavior of a Chain wrapper.
type Options struct {
	Matcher interface {
		ShouldSkip(*metadata.Metadata) bool
	}
	// ExcludedPrefixes are IP prefixes whose traffic is routed via a direct
	// connection instead of the upstream proxy.  IANA-reserved ranges are
	// always treated as direct regardless of this list.
	ExcludedPrefixes []netip.Prefix
	Logger           *slog.Logger
}

// Chain augments a proxy.Proxy with process exclusion, reserved-IP bypass,
// and user-defined IP exclusion.
type Chain struct {
	upstream proxy.Proxy
	direct   proxy.Proxy
	opts     Options
}

// New builds a chain around an upstream proxy.
func New(upstream proxy.Proxy, opts Options) *Chain {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Chain{
		upstream: upstream,
		direct:   proxy.NewDirect(),
		opts:     opts,
	}
}

func (c *Chain) DialContext(ctx context.Context, md *metadata.Metadata) (net.Conn, error) {
	if c.shouldBypassProcess(md) {
		c.opts.Logger.Debug("chain: direct (excluded process)",
			"src", md.SourceAddress(), "dst", md.DestinationAddress())
		return c.direct.DialContext(ctx, md)
	}
	if c.isDirect(md) {
		c.opts.Logger.Debug("chain: direct (reserved/excluded IP)",
			"dst", md.DestinationAddress())
		return c.direct.DialContext(ctx, md)
	}
	return c.upstream.DialContext(ctx, md)
}

func (c *Chain) DialUDP(md *metadata.Metadata) (net.PacketConn, error) {
	if c.shouldBypassProcess(md) {
		c.opts.Logger.Debug("chain: direct (excluded process)",
			"src", md.SourceAddress(), "dst", md.DestinationAddress())
		return c.direct.DialUDP(md)
	}
	if c.isDirect(md) {
		c.opts.Logger.Debug("chain: direct (reserved/excluded IP)",
			"dst", md.DestinationAddress())
		return c.direct.DialUDP(md)
	}
	return c.upstream.DialUDP(md)
}

// isDirect returns true when the destination should bypass the upstream proxy:
//   - IANA-reserved addresses (private, loopback, link-local, multicast, …)
//   - Any prefix in opts.ExcludedPrefixes
func (c *Chain) isDirect(md *metadata.Metadata) bool {
	dst := md.DstIP.Unmap()
	if !dst.IsValid() {
		return false
	}
	if netaddr.IsReserved(dst) {
		return true
	}
	for _, p := range c.opts.ExcludedPrefixes {
		if p.Contains(dst) {
			return true
		}
	}
	return false
}

func (c *Chain) shouldBypassProcess(md *metadata.Metadata) bool {
	if c.opts.Matcher == nil {
		return false
	}
	return c.opts.Matcher.ShouldSkip(md)
}
