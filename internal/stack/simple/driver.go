package simple

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/xjasonlyu/tun2socks/v2/core/adapter"
	"github.com/xjasonlyu/tun2socks/v2/core/device/tun"

	"github.com/v2rayA/nanotun/internal/netaddr"
	"github.com/v2rayA/nanotun/internal/tunconf"
)

// Options controls the lightweight stack.
type Options struct {
	TunName    string
	MTU        int
	Handler    adapter.TransportHandler
	Logger     *slog.Logger
	EnableIPv6 bool
}

// Driver hosts the simplified netstack pipeline.
type Driver struct {
	opts Options
}

// New creates a new simple driver.
func New(opts Options) (*Driver, error) {
	if opts.Handler == nil {
		return nil, fmt.Errorf("transport handler is required")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Driver{opts: opts}, nil
}

// Run starts the stack until context cancellation.
func (d *Driver) Run(ctx context.Context) error {
	dev, err := tun.Open(d.opts.TunName, uint32(d.opts.MTU))
	if err != nil {
		return fmt.Errorf("open tun: %w", err)
	}
	defer dev.Close()

	if err := tunconf.Configure(dev.Name()); err != nil {
		return fmt.Errorf("configure tun: %w", err)
	}

	s, err := buildStack(dev, d.opts.Handler, d.opts.EnableIPv6)
	if err != nil {
		return err
	}
	defer func() {
		s.Close()
		waitWithTimeout(s, 2*time.Second)
	}()

	d.opts.Logger.Info("simple backend ready",
		"device", dev.Name(), "mtu", d.opts.MTU, "ipv6", d.opts.EnableIPv6,
		"gateway4", netaddr.GatewayIPv4,
		"gateway6", netaddr.GatewayIPv6,
	)

	<-ctx.Done()
	d.opts.Logger.Info("simple backend stopping")
	return ctx.Err()
}

// waitWithTimeout calls s.Wait() but gives up after the given duration so that
// long-lived proxy connections do not block clean shutdown.
func waitWithTimeout(s interface{ Wait() }, timeout time.Duration) {
	done := make(chan struct{})
	go func() { s.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}
