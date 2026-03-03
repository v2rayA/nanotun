//go:build darwin

// Package autoroute automatically installs OS-level default routes and DNS
// redirect rules so that all traffic on the host is forwarded through the
// nanotun virtual gateway without manual configuration.
package autoroute

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"strings"

	"github.com/v2rayA/nanotun/internal/netaddr"
)

// Apply installs default IPv4/IPv6 routes via the nanotun gateway and
// configures the active network service's DNS to the built-in gateway relay.
//
// proxyAddr is the raw proxy URL.  Before replacing the default route, Apply
// records the current default gateway and adds specific host routes for the
// proxy server so that it can reach its peer without looping through the tunnel.
//
// The returned cleanup func reverses all changes.
func Apply(tunName, proxyAddr string, log *slog.Logger) (func(), error) {
	_ = tunName // macOS route add does not need the tun interface name
	var cleanups []func()
	revert := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}

	gw4 := netaddr.GatewayIPv4.String()
	gw6 := netaddr.GatewayIPv6.String()

	// ── Save current default gateway for proxy bypass ─────────────────────
	origGW4, err := defaultDarwinGateway4()
	if err != nil {
		return nop, fmt.Errorf("autoroute: detect current default gateway: %w", err)
	}
	log.Debug("autoroute: original gateway", "gw4", origGW4)

	// ── Add bypass routes for the proxy server BEFORE touching the default ─
	proxyIPs := resolveProxyHost(proxyAddr, log)
	for _, ip := range proxyIPs {
		if err := run("route", "add", "-host", ip, origGW4); err != nil {
			log.Warn("autoroute: add proxy bypass route", "ip", ip, "err", err)
			continue
		}
		log.Info("autoroute: proxy bypass route added", "ip", ip, "via", origGW4)
		ipCopy := ip
		cleanups = append(cleanups, func() {
			if err := run("route", "delete", "-host", ipCopy, origGW4); err != nil {
				log.Warn("autoroute: remove proxy bypass route", "ip", ipCopy, "err", err)
			}
		})
	}

	// ── IPv4 default route ───────────────────────────────────────────────
	if err := run("route", "add", "-net", "0.0.0.0/1", gw4); err != nil {
		return nop, fmt.Errorf("autoroute: add ipv4 default route: %w", err)
	}
	cleanups = append(cleanups, func() {
		if err := run("route", "delete", "-net", "0.0.0.0/1", gw4); err != nil {
			log.Warn("autoroute: remove ipv4 default route", "err", err)
		}
	})
	if err := run("route", "add", "-net", "128.0.0.0/1", gw4); err != nil {
		revert()
		return nop, fmt.Errorf("autoroute: add ipv4 default route (128/1): %w", err)
	}
	cleanups = append(cleanups, func() {
		if err := run("route", "delete", "-net", "128.0.0.0/1", gw4); err != nil {
			log.Warn("autoroute: remove ipv4 default route (128/1)", "err", err)
		}
	})

	// ── IPv6 default route ───────────────────────────────────────────────
	if err := run("route", "add", "-inet6", "::/1", gw6); err != nil {
		log.Warn("autoroute: add ipv6 default route", "err", err)
	} else {
		cleanups = append(cleanups, func() {
			if err := run("route", "delete", "-inet6", "::/1", gw6); err != nil {
				log.Warn("autoroute: remove ipv6 default route", "err", err)
			}
		})
	}
	if err := run("route", "add", "-inet6", "8000::/1", gw6); err != nil {
		log.Warn("autoroute: add ipv6 default route (8000/1)", "err", err)
	} else {
		cleanups = append(cleanups, func() {
			if err := run("route", "delete", "-inet6", "8000::/1", gw6); err != nil {
				log.Warn("autoroute: remove ipv6 default route (8000/1)", "err", err)
			}
		})
	}

	// ── DNS via networksetup ─────────────────────────────────────────────
	svc, oldDNS, err := detectNetworkService(log)
	if err != nil {
		log.Warn("autoroute: DNS not configured (cannot detect active service)", "err", err)
	} else {
		if err := run("networksetup", "-setdnsservers", svc, gw4); err != nil {
			log.Warn("autoroute: set DNS", "service", svc, "err", err)
		} else {
			log.Info("autoroute: DNS set", "service", svc, "dns", gw4)
			cleanups = append(cleanups, func() {
				restoreDNS := oldDNS
				if len(restoreDNS) == 0 {
					restoreDNS = []string{"empty"}
				}
				args := append([]string{"-setdnsservers", svc}, restoreDNS...)
				if err := run("networksetup", args...); err != nil {
					log.Warn("autoroute: restore DNS", "service", svc, "err", err)
				}
			})
		}
	}

	log.Info("autoroute: default routes installed", "gateway4", gw4, "gateway6", gw6)
	return revert, nil
}

// detectNetworkService returns the active primary network service name and its
// current DNS servers (used for cleanup).
func detectNetworkService(log *slog.Logger) (string, []string, error) {
	// Use `route get 8.8.8.8` to find the active interface, then match via
	// `networksetup -listallhardwareports`.
	out, err := exec.Command("route", "get", "8.8.8.8").Output()
	if err != nil {
		return "", nil, fmt.Errorf("route get: %w", err)
	}
	iface := ""
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "interface:") {
			iface = strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
			break
		}
	}
	if iface == "" {
		return "", nil, fmt.Errorf("could not parse active interface from 'route get 8.8.8.8'")
	}

	svc, err := serviceForInterface(iface)
	if err != nil {
		return "", nil, err
	}

	// Read current DNS so we can restore it on cleanup.
	old, _ := currentDNS(svc, log)
	return svc, old, nil
}

// serviceForInterface finds the networksetup service name for a BSD interface.
func serviceForInterface(iface string) (string, error) {
	out, err := exec.Command("networksetup", "-listallhardwareports").Output()
	if err != nil {
		return "", fmt.Errorf("networksetup -listallhardwareports: %w", err)
	}
	var curDevice, curService string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Hardware Port:") {
			curService = strings.TrimSpace(strings.TrimPrefix(line, "Hardware Port:"))
		} else if strings.HasPrefix(line, "Device:") {
			curDevice = strings.TrimSpace(strings.TrimPrefix(line, "Device:"))
			if curDevice == iface && curService != "" {
				return curService, nil
			}
		}
	}
	return "", fmt.Errorf("no networksetup service found for interface %s", iface)
}

// currentDNS returns the current DNS servers for the given service.
func currentDNS(svc string, log *slog.Logger) ([]string, error) {
	out, err := exec.Command("networksetup", "-getdnsservers", svc).Output()
	if err != nil {
		return nil, err
	}
	var servers []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.Contains(line, "aren't any") {
			continue
		}
		servers = append(servers, line)
	}
	return servers, nil
}

// ── helpers ────────────────────────────────────────────────────────────────

// defaultDarwinGateway4 returns the current default IPv4 gateway address.
func defaultDarwinGateway4() (string, error) {
	out, err := exec.Command("route", "get", "8.8.8.8").Output()
	if err != nil {
		return "", fmt.Errorf("route get 8.8.8.8: %w", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "gateway:") {
			gw := strings.TrimSpace(strings.TrimPrefix(line, "gateway:"))
			if gw != "" {
				return gw, nil
			}
		}
	}
	return "", fmt.Errorf("gateway not found in 'route get 8.8.8.8' output")
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

func nop() {}
