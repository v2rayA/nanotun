package gvisor

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/xjasonlyu/tun2socks/v2/core"
	"github.com/xjasonlyu/tun2socks/v2/core/adapter"
	"github.com/xjasonlyu/tun2socks/v2/core/device/tun"
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

	stack, err := core.CreateStack(&core.Config{
		LinkEndpoint:     dev,
		TransportHandler: d.opts.Handler,
	})
	if err != nil {
		return fmt.Errorf("create stack: %w", err)
	}
	defer func() {
		stack.Close()
		stack.Wait()
	}()

	d.opts.Logger.Info("gvisor backend ready", "device", dev.Name(), "mtu", d.opts.MTU)

	<-ctx.Done()
	d.opts.Logger.Info("gvisor backend stopping")
	return ctx.Err()
}
