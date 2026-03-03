//go:build linux

package autoroute

import (
"fmt"
"log/slog"
"net"
"os/exec"
"strings"

"github.com/v2rayA/nanotun/internal/netaddr"
)

// nanotunMarkHex is the netfilter socket mark (0x6e74 == "nt") that the DNS
// relay stamps on its own upstream sockets.  Both nft and iptables rules
// exclude packets carrying this mark so the relay can reach the real upstream
// without being looped back to the gateway.
const nanotunMarkHex = "0x6e74"

// Apply installs a default IPv4/IPv6 route via the nanotun gateway and
// redirects all DNS queries (UDP/TCP port 53) to the built-in DNS relay.
//
// proxyAddr is the raw proxy URL (e.g. "socks5://127.0.0.1:1080").  Before
// setting the new default route, Apply captures the current default gateway
// and adds specific /32 host routes for the proxy server so that the proxy
// process can reach its remote peer without looping through the tunnel.
//
// DNS redirect rules exclude packets whose SO_MARK equals nanotunMarkHex so
// that the DNS relay's own upstream queries are never re-intercepted.
//
// DNS is redirected using nftables when available, falling back to iptables.
// The returned cleanup func tears down every rule added by Apply.
func Apply(tunName, proxyAddr string, log *slog.Logger) (func(), error) {
var cleanups []func()
revert := func() {
for i := len(cleanups) - 1; i >= 0; i-- {
cleanups[i]()
}
}

gw4 := netaddr.GatewayIPv4.String()
gw6 := netaddr.GatewayIPv6.String()

// ── Save current default gateway for proxy bypass ──────────────────────
origGW4, origDev4, err := defaultGateway4()
if err != nil {
return nop, fmt.Errorf("autoroute: detect current default gateway: %w", err)
}
log.Debug("autoroute: original gateway", "gw4", origGW4, "dev", origDev4)

// ── Add bypass routes for the proxy server BEFORE touching the default ─
proxyIPs := resolveProxyHost(proxyAddr, log)
for _, ip := range proxyIPs {
cidr := ip + "/32"
if err := run("ip", "route", "add", cidr, "via", origGW4, "dev", origDev4); err != nil {
log.Warn("autoroute: add proxy bypass route", "cidr", cidr, "err", err)
continue
}
log.Info("autoroute: proxy bypass route added", "cidr", cidr, "via", origGW4)
ipCopy := cidr
cleanups = append(cleanups, func() {
if err := run("ip", "route", "del", ipCopy, "via", origGW4, "dev", origDev4); err != nil {
log.Warn("autoroute: remove proxy bypass route", "cidr", ipCopy, "err", err)
}
})
}

// ── IPv4 default route ─────────────────────────────────────────────────
if err := run("ip", "route", "add", "default", "via", gw4, "dev", tunName); err != nil {
revert()
return nop, fmt.Errorf("autoroute: add ipv4 default route: %w", err)
}
cleanups = append(cleanups, func() {
if err := run("ip", "route", "del", "default", "via", gw4, "dev", tunName); err != nil {
log.Warn("autoroute: remove ipv4 default route", "err", err)
}
})

// ── IPv6 default route ─────────────────────────────────────────────────
if origGW6, origDev6, e6 := defaultGateway6(); e6 == nil && origGW6 != "" {
for _, ip := range proxyIPs {
_ = run("ip", "-6", "route", "add", ip+"/128", "via", origGW6, "dev", origDev6)
}
}
if err := run("ip", "-6", "route", "add", "default", "via", gw6, "dev", tunName); err != nil {
log.Warn("autoroute: add ipv6 default route", "err", err)
} else {
cleanups = append(cleanups, func() {
if err := run("ip", "-6", "route", "del", "default", "via", gw6, "dev", tunName); err != nil {
log.Warn("autoroute: remove ipv6 default route", "err", err)
}
})
}

// ── DNS redirect ───────────────────────────────────────────────────────
switch {
case toolExists("nft"):
if err := applyNft(gw4, gw6); err != nil {
log.Warn("autoroute: nft DNS redirect failed, DNS not redirected", "err", err)
} else {
log.Info("autoroute: DNS redirect via nftables")
cleanups = append(cleanups, func() {
if err := cleanupNft(); err != nil {
log.Warn("autoroute: cleanup nft tables", "err", err)
}
})
}
case toolExists("iptables"):
if err := applyIPTables(gw4, gw6); err != nil {
log.Warn("autoroute: iptables DNS redirect failed, DNS not redirected", "err", err)
} else {
log.Info("autoroute: DNS redirect via iptables")
cleanups = append(cleanups, func() {
if err := cleanupIPTables(gw4, gw6); err != nil {
log.Warn("autoroute: cleanup iptables rules", "err", err)
}
})
}
default:
log.Warn("autoroute: neither nft nor iptables found; set system DNS to " + gw4 + " manually")
}

log.Info("autoroute: default routes installed",
"gateway4", gw4, "gateway6", gw6, "dev", tunName)
return revert, nil
}

// ── gateway detection ──────────────────────────────────────────────────────

func defaultGateway4() (gw, dev string, err error) {
out, err := exec.Command("ip", "route", "show", "default").Output()
if err != nil {
return "", "", fmt.Errorf("ip route show default: %w", err)
}
// Example: "default via 192.168.1.1 dev eth0 proto dhcp metric 100"
for _, line := range strings.Split(string(out), "\n") {
fields := strings.Fields(line)
for i, f := range fields {
if f == "via" && i+1 < len(fields) {
gw = fields[i+1]
}
if f == "dev" && i+1 < len(fields) {
dev = fields[i+1]
}
}
if gw != "" && dev != "" {
return gw, dev, nil
}
gw, dev = "", ""
}
return "", "", fmt.Errorf("no default route found")
}

func defaultGateway6() (gw, dev string, err error) {
out, err := exec.Command("ip", "-6", "route", "show", "default").Output()
if err != nil {
return "", "", err
}
for _, line := range strings.Split(string(out), "\n") {
fields := strings.Fields(line)
for i, f := range fields {
if f == "via" && i+1 < len(fields) {
gw = fields[i+1]
}
if f == "dev" && i+1 < len(fields) {
dev = fields[i+1]
}
}
if gw != "" && dev != "" {
return gw, dev, nil
}
gw, dev = "", ""
}
return "", "", fmt.Errorf("no ipv6 default route")
}

// resolveProxyHost strips scheme/user-info from proxyAddr, extracts the
// hostname, and returns its resolved IPv4 addresses.
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

// ── nftables ───────────────────────────────────────────────────────────────

const (
nftTable4 = "nanotun_dns"
nftTable6 = "nanotun_dns6"
)

func applyNft(gw4, gw6 string) error {
cmds := [][]string{
// IPv4 table + chain + rules
{"nft", "add", "table", "ip", nftTable4},
{"nft", "add", "chain", "ip", nftTable4, "output",
`{ type nat hook output priority -100 ; policy accept ; }`},
// Exclude packets already marked by the DNS relay itself (SO_MARK nanotunMarkHex).
{"nft", "add", "rule", "ip", nftTable4, "output",
"udp", "dport", "53", "ip", "daddr", "!=", gw4,
"meta", "mark", "!=", nanotunMarkHex, "dnat", "to", gw4},
{"nft", "add", "rule", "ip", nftTable4, "output",
"tcp", "dport", "53", "ip", "daddr", "!=", gw4,
"meta", "mark", "!=", nanotunMarkHex, "dnat", "to", gw4},
// IPv6 table + chain + rules
{"nft", "add", "table", "ip6", nftTable6},
{"nft", "add", "chain", "ip6", nftTable6, "output",
`{ type nat hook output priority -100 ; policy accept ; }`},
{"nft", "add", "rule", "ip6", nftTable6, "output",
"udp", "dport", "53", "ip6", "daddr", "!=", gw6,
"meta", "mark", "!=", nanotunMarkHex, "dnat", "to", gw6},
{"nft", "add", "rule", "ip6", nftTable6, "output",
"tcp", "dport", "53", "ip6", "daddr", "!=", gw6,
"meta", "mark", "!=", nanotunMarkHex, "dnat", "to", gw6},
}
for _, args := range cmds {
if err := run(args[0], args[1:]...); err != nil {
_ = cleanupNft()
return err
}
}
return nil
}

func cleanupNft() error {
var last error
for _, args := range [][]string{
{"nft", "delete", "table", "ip", nftTable4},
{"nft", "delete", "table", "ip6", nftTable6},
} {
if err := run(args[0], args[1:]...); err != nil {
last = err
}
}
return last
}

// ── iptables ───────────────────────────────────────────────────────────────

func applyIPTables(gw4, gw6 string) error {
cmds := [][]string{
// Exclude packets already marked by the DNS relay (mark nanotunMarkHex).
{"iptables", "-t", "nat", "-I", "OUTPUT", "-p", "udp", "--dport", "53",
"!", "-d", gw4, "-m", "mark", "!", "--mark", nanotunMarkHex, "-j", "DNAT", "--to", gw4},
{"iptables", "-t", "nat", "-I", "OUTPUT", "-p", "tcp", "--dport", "53",
"!", "-d", gw4, "-m", "mark", "!", "--mark", nanotunMarkHex, "-j", "DNAT", "--to", gw4},
}
for _, args := range cmds {
if err := run(args[0], args[1:]...); err != nil {
_ = cleanupIPTables(gw4, gw6)
return err
}
}
// Best-effort IPv6
_ = run("ip6tables", "-t", "nat", "-I", "OUTPUT", "-p", "udp", "--dport", "53",
"!", "-d", gw6, "-m", "mark", "!", "--mark", nanotunMarkHex, "-j", "DNAT", "--to", gw6)
_ = run("ip6tables", "-t", "nat", "-I", "OUTPUT", "-p", "tcp", "--dport", "53",
"!", "-d", gw6, "-m", "mark", "!", "--mark", nanotunMarkHex, "-j", "DNAT", "--to", gw6)
return nil
}

func cleanupIPTables(gw4, gw6 string) error {
var last error
for _, args := range [][]string{
{"iptables", "-t", "nat", "-D", "OUTPUT", "-p", "udp", "--dport", "53",
"!", "-d", gw4, "-m", "mark", "!", "--mark", nanotunMarkHex, "-j", "DNAT", "--to", gw4},
{"iptables", "-t", "nat", "-D", "OUTPUT", "-p", "tcp", "--dport", "53",
"!", "-d", gw4, "-m", "mark", "!", "--mark", nanotunMarkHex, "-j", "DNAT", "--to", gw4},
{"ip6tables", "-t", "nat", "-D", "OUTPUT", "-p", "udp", "--dport", "53",
"!", "-d", gw6, "-m", "mark", "!", "--mark", nanotunMarkHex, "-j", "DNAT", "--to", gw6},
{"ip6tables", "-t", "nat", "-D", "OUTPUT", "-p", "tcp", "--dport", "53",
"!", "-d", gw6, "-m", "mark", "!", "--mark", nanotunMarkHex, "-j", "DNAT", "--to", gw6},
} {
if err := run(args[0], args[1:]...); err != nil {
last = err
}
}
return last
}

// ── helpers ────────────────────────────────────────────────────────────────

func toolExists(name string) bool {
_, err := exec.LookPath(name)
return err == nil
}

func run(name string, args ...string) error {
out, err := exec.Command(name, args...).CombinedOutput()
if err != nil {
return fmt.Errorf("%s %v: %s", name, args, out)
}
return nil
}

func nop() {}
