package simple

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/xjasonlyu/tun2socks/v2/core/adapter"
	"github.com/xjasonlyu/tun2socks/v2/core/device/tun"
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

	stack, err := buildStack(dev, d.opts.Handler, d.opts.EnableIPv6)
	if err != nil {
		return err
	}
	defer func() {
		stack.Close()
		stack.Wait()
	}()

	d.opts.Logger.Info("simple backend ready", "device", dev.Name(), "mtu", d.opts.MTU, "ipv6", d.opts.EnableIPv6)

	<-ctx.Done()
	d.opts.Logger.Info("simple backend stopping")
	return ctx.Err()
}
