//go:build darwin

// Package tunconf configures the OS-level TUN network interface.
package tunconf

import (
	"fmt"
	"os/exec"
	"strconv"

	"github.com/v2rayA/nanotun/internal/netaddr"
)

// Configure assigns the well-known gateway addresses to the named TUN
// interface and brings the link up.
// On macOS, TUN interfaces are point-to-point; we use the gateway IP as both
// the local and the remote ("destination") end, which is the standard trick
// for tunnel interfaces.
// IPv6 configuration failure is silently ignored.
func Configure(name string) error {
	gw4 := netaddr.GatewayIPv4.String()
	// ifconfig <iface> inet <local> <dest> up
	if err := run("ifconfig", name, "inet", gw4, gw4, "up"); err != nil {
		return fmt.Errorf("tunconf: set ipv4 on %s: %w", name, err)
	}
	gw6 := netaddr.GatewayIPv6.String()
	prefixLen := strconv.Itoa(netaddr.PrefixIPv6.Bits())
	_ = run("ifconfig", name, "inet6", gw6, "prefixlen", prefixLen, "up")
	return nil
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", name, out)
	}
	return nil
}
