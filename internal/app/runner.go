package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/xjasonlyu/tun2socks/v2/proxy"
	"github.com/xjasonlyu/tun2socks/v2/tunnel"
	"github.com/xjasonlyu/tun2socks/v2/tunnel/statistic"

	"github.com/v2rayA/nanotun/internal/config"
	"github.com/v2rayA/nanotun/internal/exclude"
	"github.com/v2rayA/nanotun/internal/proxychain"
	"github.com/v2rayA/nanotun/internal/stack"
	gvisor "github.com/v2rayA/nanotun/internal/stack/gvisor"
	"github.com/v2rayA/nanotun/internal/stack/simple"
)

// Run wires every subsystem together and blocks until ctx is cancelled.
func Run(ctx context.Context, cfg config.Runtime, log *slog.Logger) error {
	upstream, err := buildProxy(cfg.Proxy)
	if err != nil {
		return err
	}

	matcher := exclude.New(cfg.ExcludedProcesses, cfg.ExcludeRefresh, log)
	if matcher != nil {
		matcher.Start(ctx)
	}

	chain := proxychain.New(upstream, proxychain.Options{
		Matcher:        matcher,
		HasDNSOverride: cfg.HasDNSOverride,
		DNSOverride:    cfg.DNSAddr,
		Logger:         log,
	})

	tun := tunnel.New(chain, statistic.DefaultManager)
	defer tun.Close()
	tun.SetUDPTimeout(cfg.UDPTimeout)
	tun.ProcessAsync()

	driver, err := createDriver(cfg, tun, log)
	if err != nil {
		return err
	}

	return driver.Run(ctx)
}

func buildProxy(raw string) (proxy.Proxy, error) {
	if !strings.Contains(raw, "://") {
		raw = "socks5://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse proxy: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	addr := u.Host
	user := u.User.Username()
	pass, _ := u.User.Password()
	switch scheme {
	case "direct":
		return proxy.NewDirect(), nil
	case "reject":
		return proxy.NewReject(), nil
	case "http", "https":
		return proxy.NewHTTP(addr, user, pass)
	case "socks4":
		return proxy.NewSocks4(addr, user)
	case "socks5", "socks5h":
		if addr == "" {
			addr = u.Path // socks5 over UDS
		}
		return proxy.NewSocks5(addr, user, pass)
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q", scheme)
	}
}

func createDriver(cfg config.Runtime, handler *tunnel.Tunnel, log *slog.Logger) (stack.Driver, error) {
	switch cfg.StackMode {
	case config.StackModeGVisor:
		return gvisor.New(gvisor.Options{
			TunName: cfg.TunName,
			MTU:     cfg.MTU,
			Handler: handler,
			Logger:  log,
		})
	case config.StackModeSimple:
		return simple.New(simple.Options{
			TunName:    cfg.TunName,
			MTU:        cfg.MTU,
			Handler:    handler,
			Logger:     log,
			EnableIPv6: true,
		})
	default:
		return nil, fmt.Errorf("unsupported stack mode %q", cfg.StackMode)
	}
}
