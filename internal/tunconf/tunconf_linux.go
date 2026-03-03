//go:build linux

// Package tunconf configures the OS-level TUN network interface.
package tunconf

import (
	"fmt"
	"os/exec"

	"github.com/v2rayA/nanotun/internal/netaddr"
)

// Configure assigns the well-known gateway addresses to the named TUN
// interface and brings the link up.
// IPv4 configuration is mandatory; an IPv6 failure is silently ignored.
func Configure(name string) error {
	if err := run("ip", "addr", "add", netaddr.PrefixIPv4.String(), "dev", name); err != nil {
		return fmt.Errorf("tunconf: set ipv4 %s on %s: %w", netaddr.PrefixIPv4, name, err)
	}
	// IPv6 is best-effort — not all kernels/configurations support it.
	_ = run("ip", "-6", "addr", "add", netaddr.PrefixIPv6.String(), "dev", name)
	if err := run("ip", "link", "set", name, "up"); err != nil {
		return fmt.Errorf("tunconf: bring up %s: %w", name, err)
	}
	return nil
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", name, out)
	}
	return nil
}
