//go:build windows

// Package tunconf configures the OS-level TUN network interface.
package tunconf

import (
	"fmt"
	"os/exec"

	"github.com/v2rayA/nanotun/internal/netaddr"
)

// Configure assigns the well-known gateway addresses to the named TUN
// interface on Windows. Requires Administrator privileges.
// IPv6 configuration failure is silently ignored.
func Configure(name string) error {
	// /30 → 255.255.255.252
	const mask4 = "255.255.255.252"
	gw4 := netaddr.GatewayIPv4.String()

	// "set address" replaces the existing address cleanly.
	if err := run("netsh", "interface", "ipv4", "set", "address",
		"name="+name, "source=static",
		"address="+gw4, "mask="+mask4); err != nil {
		return fmt.Errorf("tunconf: set ipv4 on %s: %w", name, err)
	}

	gw6 := netaddr.GatewayIPv6.String()
	prefixLen := fmt.Sprintf("%d", netaddr.PrefixIPv6.Bits())
	_ = run("netsh", "interface", "ipv6", "add", "address",
		"interface="+name, "address="+gw6+"/"+prefixLen)
	return nil
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", name, out)
	}
	return nil
}
