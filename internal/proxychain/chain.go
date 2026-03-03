package proxychain

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"

	"github.com/xjasonlyu/tun2socks/v2/metadata"
	"github.com/xjasonlyu/tun2socks/v2/proxy"

	"github.com/v2rayA/nanotun/internal/exclude"
)

var ErrProcessExcluded = errors.New("connection rejected by process exclusion rules")

// Options configures the behavior of a Chain wrapper.
type Options struct {
	Matcher        *exclude.Matcher
	DNSOverride    netip.AddrPort
	HasDNSOverride bool
	Logger         *slog.Logger
}

// Chain augments a proxy.Proxy with process exclusion and DNS overrides.
type Chain struct {
	upstream proxy.Proxy
	opts     Options
}

// New builds a chain around an upstream proxy.
func New(upstream proxy.Proxy, opts Options) *Chain {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Chain{upstream: upstream, opts: opts}
}

func (c *Chain) DialContext(ctx context.Context, md *metadata.Metadata) (net.Conn, error) {
	if err := c.filter(md); err != nil {
		return nil, err
	}
	target := c.applyOverrides(md)
	return c.upstream.DialContext(ctx, target)
}

func (c *Chain) DialUDP(md *metadata.Metadata) (net.PacketConn, error) {
	if err := c.filter(md); err != nil {
		return nil, err
	}
	target := c.applyOverrides(md)
	return c.upstream.DialUDP(target)
}

func (c *Chain) filter(md *metadata.Metadata) error {
	if c.opts.Matcher == nil {
		return nil
	}
	if c.opts.Matcher.ShouldSkip(md) {
		c.opts.Logger.Debug("drop flow due to exclusion", "src", md.SourceAddress(), "dst", md.DestinationAddress())
		return ErrProcessExcluded
	}
	return nil
}

func (c *Chain) applyOverrides(md *metadata.Metadata) *metadata.Metadata {
	if !c.opts.HasDNSOverride {
		return md
	}
	if md.Network != metadata.UDP || md.DstPort != 53 {
		return md
	}
	clone := *md
	clone.DstIP = c.opts.DNSOverride.Addr()
	clone.DstPort = c.opts.DNSOverride.Port()
	return &clone
}
