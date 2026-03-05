//go:build windows

// Package autoroute automatically installs OS-level default routes and DNS
// redirect rules so that all traffic on the host is forwarded through the
// nanotun virtual gateway without manual configuration.
package autoroute

import (
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"strings"

	"github.com/v2rayA/nanotun/internal/netaddr"
)

// Apply installs default IPv4/IPv6 routes via the nanotun gateway and
// configures DNS for the TUN interface to point at the built-in relay.
//
// proxyAddr is the raw proxy URL.  Before replacing the default route, Apply
// records the current default gateway and adds specific host routes for the
// proxy server so that it can reach its peer without looping through the tunnel.
//
// The returned cleanup func reverses all changes.
func Apply(tunName, proxyAddr string, log *slog.Logger) (func(), error) {
	var cleanups []func()
	revert := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
		// Flush DNS cache so that the Windows DNS Client service
		// immediately picks up the restored DNS configuration.
		_ = run("ipconfig", "/flushdns")
	}

	gw4 := netaddr.GatewayIPv4.String()
	gw6 := netaddr.GatewayIPv6.String()

	// ── Save current default gateway for proxy bypass ─────────────────────
	origGW4, err := defaultWindowsGateway4()
	if err != nil {
		return nop, fmt.Errorf("autoroute: detect current default gateway: %w", err)
	}
	log.Debug("autoroute: original gateway", "gw4", origGW4)

	// ── Add bypass routes for the proxy server BEFORE touching the default ─
	proxyIPs := resolveProxyHost(proxyAddr, log)
	for _, ip := range proxyIPs {
		if err := addIPv4HostRoutePS(ip, origGW4); err != nil {
			log.Warn("autoroute: add proxy bypass route", "ip", ip, "err", err)
			continue
		}
		log.Info("autoroute: proxy bypass route added", "ip", ip, "via", origGW4)
		ipCopy := ip
		cleanups = append(cleanups, func() {
			if err := removeIPv4HostRoutePS(ipCopy, origGW4); err != nil {
				log.Warn("autoroute: remove proxy bypass route", "ip", ipCopy, "err", err)
			}
		})
	}

	// Ensure local loopback keeps the most specific route while tunnel is active.
	// This prevents localhost traffic from being caught by the split default routes.
	if err := addLocalhostRoutePS(); err != nil {
		log.Warn("autoroute: add localhost route", "err", err)
	} else {
		cleanups = append(cleanups, func() {
			if err := removeLocalhostRoutePS(); err != nil {
				log.Warn("autoroute: remove localhost route", "err", err)
			}
		})
	}

	// ── IPv4 default route (split routing) ───────────────────────────────
	// Use two /1 routes instead of a single /0 route so that the original
	// default route (0.0.0.0/0) on the physical NIC is never displaced.
	// The /1 routes are more specific and take precedence over any /0 route,
	// and removing them cleanly restores the original routing table.
	if err := run("netsh", "interface", "ipv4", "add", "route",
		"0.0.0.0/1", tunName, gw4, "metric=1", "store=active"); err != nil {
		return nop, fmt.Errorf("autoroute: add ipv4 default route (0/1): %w", err)
	}
	cleanups = append(cleanups, func() {
		if err := run("netsh", "interface", "ipv4", "delete", "route",
			"0.0.0.0/1", tunName, gw4); err != nil {
			log.Warn("autoroute: remove ipv4 default route (0/1)", "err", err)
		}
	})
	if err := run("netsh", "interface", "ipv4", "add", "route",
		"128.0.0.0/1", tunName, gw4, "metric=1", "store=active"); err != nil {
		revert()
		return nop, fmt.Errorf("autoroute: add ipv4 default route (128/1): %w", err)
	}
	cleanups = append(cleanups, func() {
		if err := run("netsh", "interface", "ipv4", "delete", "route",
			"128.0.0.0/1", tunName, gw4); err != nil {
			log.Warn("autoroute: remove ipv4 default route (128/1)", "err", err)
		}
	})

	// ── IPv6 default route (split routing) ───────────────────────────────
	if err := run("netsh", "interface", "ipv6", "add", "route",
		"::/1", tunName, gw6, "metric=1", "store=active"); err != nil {
		log.Warn("autoroute: add ipv6 default route (::/1)", "err", err)
	} else {
		cleanups = append(cleanups, func() {
			if err := run("netsh", "interface", "ipv6", "delete", "route",
				"::/1", tunName, gw6); err != nil {
				log.Warn("autoroute: remove ipv6 default route (::/1)", "err", err)
			}
		})
	}
	if err := run("netsh", "interface", "ipv6", "add", "route",
		"8000::/1", tunName, gw6, "metric=1", "store=active"); err != nil {
		log.Warn("autoroute: add ipv6 default route (8000::/1)", "err", err)
	} else {
		cleanups = append(cleanups, func() {
			if err := run("netsh", "interface", "ipv6", "delete", "route",
				"8000::/1", tunName, gw6); err != nil {
				log.Warn("autoroute: remove ipv6 default route (8000::/1)", "err", err)
			}
		})
	}

	// ── DNS ──────────────────────────────────────────────────────────────
	// Capture current DNS setting for restore.
	oldDNS := currentDNS(tunName)
	if err := run("netsh", "interface", "ipv4", "set", "dnsservers",
		"name="+tunName, "source=static", "address="+gw4, "validate=no"); err != nil {
		log.Warn("autoroute: set ipv4 DNS", "err", err)
	} else {
		log.Info("autoroute: DNS set", "interface", tunName, "dns", gw4)
		cleanups = append(cleanups, func() {
			if oldDNS == "" {
				_ = run("netsh", "interface", "ipv4", "set", "dnsservers",
					"name="+tunName, "source=dhcp")
			} else {
				_ = run("netsh", "interface", "ipv4", "set", "dnsservers",
					"name="+tunName, "source=static", "address="+oldDNS, "validate=no")
			}
		})
	}

	log.Info("autoroute: default routes installed",
		"gateway4", gw4, "gateway6", gw6, "interface", tunName)
	return revert, nil
}

// currentDNS returns the first DNS server currently set on the interface.
func currentDNS(ifaceName string) string {
	out, err := exec.Command("netsh", "interface", "ipv4", "show", "dnsservers",
		"name="+ifaceName).Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		// Line is like:  "Statically Configured DNS Servers:  8.8.8.8"
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			addr := strings.TrimSpace(parts[len(parts)-1])
			if addr != "" && addr != "None" && !strings.Contains(addr, " ") {
				return addr
			}
		}
	}
	return ""
}

// ── helpers ────────────────────────────────────────────────────────────────

// defaultWindowsGateway4 returns the current default IPv4 gateway.
//
// Strategy:
//  1. PowerShell Get-NetRoute  — locale-independent, available on Win8+.
//  2. route.exe print fallback — parses any line whose first two fields are
//     both "0.0.0.0", regardless of section headers (avoids locale issues).
func defaultWindowsGateway4() (string, error) {
	// ── 1. PowerShell (preferred) ──────────────────────────────────────────
	psScript := `(Get-NetRoute -DestinationPrefix '0.0.0.0/0' -ErrorAction SilentlyContinue | ` +
		`Sort-Object RouteMetric | Select-Object -First 1).NextHop`
	if out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive",
		"-Command", psScript).Output(); err == nil {
		gw := strings.TrimSpace(string(out))
		if gw != "" && gw != "::" {
			return gw, nil
		}
	}

	// ── 2. route.exe fallback ──────────────────────────────────────────────
	out, err := exec.Command("route", "print", "0.0.0.0", "mask", "0.0.0.0").Output()
	if err != nil {
		return "", fmt.Errorf("route print: %w", err)
	}
	// Look for any data line whose first two fields are both "0.0.0.0".
	// This works regardless of section header language:
	//   Network Destination  Netmask      Gateway      Interface  Metric
	//         0.0.0.0        0.0.0.0   192.168.1.1  192.168.1.10    35
	if gw := parseDefaultGatewayFromRoutePrint(string(out)); gw != "" {
		return gw, nil
	}
	return "", fmt.Errorf("no default route found (tried PowerShell Get-NetRoute and route print)")
}

// resolveProxyHost strips scheme/user-info from proxyAddr and resolves
// the hostname to IPv4 addresses.
func resolveProxyHost(proxyAddr string, log *slog.Logger) []string {
	if proxyAddr == "" {
		return nil
	}
	if idx := strings.Index(proxyAddr, "://"); idx >= 0 {
		proxyAddr = proxyAddr[idx+3:]
	}
	if idx := strings.LastIndex(proxyAddr, "@"); idx >= 0 {
		proxyAddr = proxyAddr[idx+1:]
	}
	host, _, err := net.SplitHostPort(proxyAddr)
	if err != nil {
		host = proxyAddr
	}
	addrs, err := net.LookupHost(host)
	if err != nil {
		log.Warn("autoroute: cannot resolve proxy host", "host", host, "err", err)
		return nil
	}
	var v4 []string
	for _, a := range addrs {
		if ip := net.ParseIP(a); ip != nil && ip.To4() != nil {
			v4 = append(v4, ip.String())
		}
	}
	return v4
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %s", name, args, out)
	}
	return nil
}

func runPowerShell(script string) error {
	return run("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
}

func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func addIPv4HostRoutePS(ip, nextHop string) error {
	script := "New-NetRoute -AddressFamily IPv4 " +
		"-DestinationPrefix " + psQuote(ip+"/32") + " " +
		"-NextHop " + psQuote(nextHop) + " " +
		"-RouteMetric 1 -PolicyStore ActiveStore -ErrorAction Stop"
	return runPowerShell(script)
}

func removeIPv4HostRoutePS(ip, nextHop string) error {
	script := "Remove-NetRoute -AddressFamily IPv4 " +
		"-DestinationPrefix " + psQuote(ip+"/32") + " " +
		"-NextHop " + psQuote(nextHop) + " " +
		"-PolicyStore ActiveStore -Confirm:$false -ErrorAction Stop"
	return runPowerShell(script)
}

func addLocalhostRoutePS() error {
	const (
		loopbackAddr   = "127.0.0.1"
		loopbackPrefix = "127.0.0.0/8"
		nextHopOnLink  = "0.0.0.0"
	)
	script := "$ifIndex = (Get-NetIPAddress -AddressFamily IPv4 -IPAddress " + psQuote(loopbackAddr) + " -ErrorAction Stop | " +
		"Select-Object -First 1).InterfaceIndex; " +
		"New-NetRoute -AddressFamily IPv4 -DestinationPrefix " + psQuote(loopbackPrefix) + " " +
		"-InterfaceIndex $ifIndex -NextHop " + psQuote(nextHopOnLink) + " -RouteMetric 1 -PolicyStore ActiveStore -ErrorAction Stop"
	return runPowerShell(script)
}

func removeLocalhostRoutePS() error {
	const (
		loopbackPrefix = "127.0.0.0/8"
		nextHopOnLink  = "0.0.0.0"
	)
	script := "Remove-NetRoute -AddressFamily IPv4 -DestinationPrefix " + psQuote(loopbackPrefix) + " " +
		"-NextHop " + psQuote(nextHopOnLink) + " " +
		"-PolicyStore ActiveStore -Confirm:$false -ErrorAction Stop"
	return runPowerShell(script)
}

func nop() {}
