# nano-tun

nano-tun is a lightweight tun2socks-style forwarder written in Go. It keeps the entire data path in user space while offering two stack implementations: the full gVisor netstack and a simplified stack optimized for low-overhead forwarding. The daemon accepts traffic from a TUN interface and forwards it through a configurable upstream proxy.

## Features
- Dual packet engines (gVisor and simplified gVisor-based stack) selectable per run.
- **Virtual gateway addresses** auto-assigned to the TUN device; no manual `ip addr` required.
- **Built-in DNS relay** running on the virtual gateway IP — point your OS DNS at `198.18.0.1` and nanotun forwards queries to a configurable upstream resolver.
- Responds to **ICMP echo (ping)** directed at the gateway address.
- Mandatory upstream proxy with optional DNS-override for proxied UDP/53 flows.
- Process-based exclusion list that bypasses the proxy for selected executables.
- Configurable UDP session timeout, MTU, and TUN device name across platforms.
- YAML configuration merged with CLI flags for reproducible deployments.
- Structured logging via `slog` with adjustable verbosity levels.

## Prerequisites
- Go 1.22 or newer on Linux, Windows, or macOS.
- Permission to create and configure TUN/TAP interfaces (often requires Administrator/root).
- An upstream SOCKS5 or HTTP proxy reachable from the host.

## Building
```bash
git clone https://github.com/v2rayA/nanotun.git
cd nano-tun
go mod tidy
go build ./cmd/nanotun
```
The `go mod tidy` step pulls all referenced libraries (tun2socks core, gVisor, gopsutil, etc.).

## Network Layout

nanotun automatically programs the following addresses on the TUN device at startup:

| Family | Subnet | Gateway / DNS listener |
|--------|--------|------------------------|
| IPv4 | `198.18.0.0/30` | `198.18.0.1` |
| IPv6 | `fdfe:dcba:9876::/126` | `fdfe:dcba:9876::1` |

Both addresses are fixed. After nanotun starts:

1. Add default routes into the TUN device so traffic enters the tunnel:

   **Linux**
   ```bash
   ip route add default via 198.18.0.1 dev nano0
   ip -6 route add default via fdfe:dcba:9876::1 dev nano0
   ```

   **macOS**
   ```bash
   sudo route add -net 0.0.0.0/0 198.18.0.1
   sudo route add -inet6 ::/0 fdfe:dcba:9876::1
   ```

   **Windows** (Administrator PowerShell)
   ```powershell
   # Find the interface index of the TUN device first
   $idx = (Get-NetAdapter | Where-Object { $_.Name -like "*nano*" }).ifIndex
   New-NetRoute -InterfaceIndex $idx -DestinationPrefix "0.0.0.0/0"  -NextHop "198.18.0.1"
   New-NetRoute -InterfaceIndex $idx -DestinationPrefix "::/0"       -NextHop "fdfe:dcba:9876::1"
   ```

2. Point your system DNS resolver at the gateway so nanotun can relay queries:

   **Linux** (systemd-resolved or /etc/resolv.conf)
   ```bash
   echo "nameserver 198.18.0.1" | sudo tee /etc/resolv.conf
   ```

   **macOS** (System Settings → Network → DNS, or via `networksetup`)
   ```bash
   # Replace "Wi-Fi" with your active service name
   sudo networksetup -setdnsservers "Wi-Fi" 198.18.0.1
   ```

   **Windows** (Administrator PowerShell)
   ```powershell
   Set-DnsClientServerAddress -InterfaceIndex $idx -ServerAddresses "198.18.0.1"
   ```

## Quick Start
1. Create a configuration file (or copy `config.example.yaml`):
   ```yaml
   tunName: nano0
   mtu: 1500
   stackMode: gvisor
   proxy: socks5://127.0.0.1:1080
   dns: 8.8.8.8   # where nanotun forwards intercepted DNS queries
   udpTimeout: 1m
   logLevel: info
   ```
2. Start the daemon (root required for TUN creation and interface configuration):
   ```bash
   sudo ./nanotun --config ./nanotun.yaml
   ```
3. Configure default routes and DNS as shown in [Network Layout](#network-layout), **or** set `autoDefaultRoute: true` in the config to let nanotun handle this automatically.

## CLI Flags
`cmd/nanotun` exposes the following switches (all optional when covered by the YAML config):

| Flag | Description |
| --- | --- |
| `--config` | Path to the YAML configuration file. |
| `--tun` | Override the TUN interface name. |
| `--mtu` | Override MTU value. |
| `--stack` | `gvisor` or `simple`. |
| `--proxy` | Upstream proxy URL (e.g., `socks5://127.0.0.1:1080`). |
| `--dns` | Upstream resolver for the built-in DNS relay (default `8.8.8.8:53`). Intercepted DNS queries are forwarded here; does not go through the proxy. |
| ` ` | Automatically install default routes and DNS redirect rules at startup; cleaned up on exit. |
| `--udp-timeout` | Duration before idle UDP flows are closed. |
| `--exclude` | Process name to bypass (repeatable). |
| `--exclude-refresh` | Interval for refreshing the process table. |
| `--log-level` | `debug`, `info`, `warn`, or `error`. |

CLI values always override YAML fields. See [cmd/nanotun/main.go](cmd/nanotun/main.go) for the authoritative flag wiring.

## Configuration Reference
The YAML schema handled by [internal/config/config.go](internal/config/config.go) understands:

| Field | Default | Description |
|-------|---------|-------------|
| `tunName` | `nano0` | Name of the TUN interface to open. |
| `mtu` | `1500` | MTU to program on the device. |
| `stackMode` | `gvisor` | `gvisor` for the full netstack, `simple` for the trimmed driver. |
| `proxy` | *(required)* | Upstream proxy URL. Supported schemes: `socks5://`, `socks5h://`, `socks4://`, `http://`, `direct://`, `reject://`. |
| `dns` | `8.8.8.8:53` | Upstream resolver for the built-in DNS relay. nanotun intercepts all DNS at the gateway address and forwards queries here, bypassing the proxy. |
| `autoDefaultRoute` | `false` | When `true`, auto-installs OS routes and DNS redirect rules and cleans up on exit. |
| `udpTimeout` | `1m` | Idle timeout for UDP sessions. |
| `excludedProcesses` | *(none)* | Lower-cased executables that should never transit the proxy. |
| `excludeRefresh` | `15s` | Scan interval for process table refresh. |
| `logLevel` | `info` | `debug` / `info` / `warn` / `error`. |

## Built-in Gateway
nanotun runs a virtual gateway on the TUN interface's first address:

- **ICMP echo (ping)** to `198.18.0.1` / `fdfe:dcba:9876::1` is answered by the gVisor stack — useful for confirming the tunnel is alive.
- **DNS relay** on UDP port 53 of the gateway address intercepts all DNS queries and forwards them to the configured `dns` server via the host OS network (bypassing the proxy). When `autoDefaultRoute` is enabled nftables/iptables automatically redirect all outbound port-53 traffic to this relay; otherwise, point your system DNS at `198.18.0.1` manually.
- **TCP** to the gateway address is silently dropped (no service listens there).

### Traffic loop prevention

When `autoDefaultRoute: true`, nanotun records the original default gateway, then adds a specific host route for the proxy server's IP address via that gateway **before** installing the TUN default route. This ensures the proxy client's own outbound traffic (to its remote peer) bypasses the TUN entirely and prevents any forwarding loop.

## Process Exclusion
The exclusion subsystem ([internal/exclude/matcher.go](internal/exclude/matcher.go)) periodically maps active flows back to process names using `gopsutil`. Any matching process causes the flow to be rejected before it hits the proxy, enabling per-application split tunneling. The process table is refreshed on a timer and the in-memory cache is **fully replaced** each cycle to prevent stale-PID accumulation.

The `--exclude` flag accepts multiple values and is case-insensitive. The `.exe` suffix is ignored on Windows.

## Stack Modes
- **gVisor** ([internal/stack/gvisor/driver.go](internal/stack/gvisor/driver.go)) uses the `tun2socks` `core` package and mirrors the behavior of upstream implementations. Choose this for maximum protocol coverage.
- **Simple** ([internal/stack/simple](internal/stack/simple)) builds a trimmed stack directly with gVisor primitives, registers TCP/UDP forwarders, and keeps optional IPv6 support. Promiscuous mode and spoofing are both enabled so the stack can receive and respond on behalf of arbitrary source addresses inside the tunnel.

Switch between stacks with `--stack gvisor` or `--stack simple` (or via `stackMode` in YAML).

## Development Tips
- Run `go test ./...` during development (ensure dependencies are downloaded first).
- Use `--log-level debug` to inspect per-flow events.
- The clean-shutdown timeout for active connections is 2 seconds; the process will always exit promptly after SIGINT/SIGTERM.

## Referenced Projects & Licenses
| Project | Purpose | License |
| --- | --- | --- |
| [xjasonlyu/tun2socks](https://github.com/xjasonlyu/tun2socks) | Core tunnel, proxy adapters, statistics | MIT |
| [gVisor](https://github.com/google/gvisor) | Network stack primitives | Apache-2.0 |
| [gopsutil](https://github.com/shirou/gopsutil) | Process and connection inspection for exclusions | BSD-3-Clause |
| [spf13/pflag](https://github.com/spf13/pflag) | POSIX/GNU-style CLI flag parsing | BSD-3-Clause |
| [go-yaml/yaml](https://github.com/go-yaml/yaml) | YAML parsing for configuration files | Apache-2.0 |

Each project retains its original license; nano-tun simply consumes the published modules. Review upstream repositories before redistributing or embedding them in proprietary products.

## License
nano-tun is distributed under the terms of the [MIT License](LICENSE), which is compatible with the referenced upstream projects.
