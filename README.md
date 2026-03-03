# nano-tun

nano-tun is a lightweight tun2socks-style forwarder written in Go. It keeps the entire data path in user space while offering two stack implementations: the full gVisor netstack and a simplified stack optimized for low-overhead forwarding. The daemon accepts traffic from a TUN interface and forwards it through a configurable upstream proxy.

## Features
- Dual packet engines (gVisor and simplified gVisor-based stack) selectable per run.
- Mandatory upstream proxy with optional DNS override for UDP/53.
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

## Quick Start
1. Create a configuration file:
   ```yaml
   tunName: nano0
   mtu: 1500
   stackMode: simple
   proxy: socks5://127.0.0.1:1080
   dnsServer: 1.1.1.1
   udpTimeout: 1m
   excludedProcesses:
     - chrome.exe
   excludeRefresh: 10s
   logLevel: info
   ```
2. Start the daemon:
   ```bash
   sudo ./nanotun --config ./nanotun.yaml
   ```
3. Configure your OS routing rules to direct the desired subnets into the `tunName` device.

## CLI Flags
`cmd/nanotun` exposes the following switches (all optional when covered by the YAML config):

| Flag | Description |
| --- | --- |
| `--config` | Path to the YAML configuration file. |
| `--tun` | Override the TUN interface name. |
| `--mtu` | Override MTU value. |
| `--stack` | `gvisor` or `simple`. |
| `--proxy` | Upstream proxy URL (e.g., `socks5://127.0.0.1:1080`). |
| `--dns` | DNS override target (`host` or `host:port`). Applied only to UDP/53. |
| `--udp-timeout` | Duration before idle UDP flows are closed. |
| `--exclude` | Process name to bypass (repeatable). |
| `--exclude-refresh` | Interval for refreshing the process table. |
| `--log-level` | `debug`, `info`, `warn`, or `error`. |

CLI values always override YAML fields. See [cmd/nanotun/main.go](cmd/nanotun/main.go) for the authoritative flag wiring.

## Configuration Reference
The YAML schema handled by [internal/config/config.go](internal/config/config.go) understands:
- `tunName`: Name of the TUN interface to open (default `nano0`).
- `mtu`: MTU to program on the device (default `1500`).
- `stackMode`: `gvisor` for the full netstack, `simple` for the trimmed driver.
- `proxy`: Required upstream proxy URL (SOCKS5/HTTP supported by tun2socks core).
- `dnsServer`: Optional DNS override in `host` or `host:port` form (defaults to port 53).
- `udpTimeout`: Idle timeout for UDP sessions (default `1m`).
- `excludedProcesses`: Lower-cased executables that should never transit the proxy.
- `excludeRefresh`: Scan interval for process table refresh (default `15s`).
- `logLevel`: `debug`/`info`/`warn`/`error` for slog.

## Process Exclusion
The exclusion subsystem ([internal/exclude/matcher.go](internal/exclude/matcher.go)) periodically maps active flows back to process names using `gopsutil`. Any matching process causes the flow to be rejected before it hits the proxy, allowing split tunneling per application. The `--exclude` flag accepts multiple values and is case-insensitive.

## Stack Modes
- **gVisor** ([internal/stack/gvisor/driver.go](internal/stack/gvisor/driver.go)) uses the `tun2socks` `core` package and mirrors the behavior of upstream implementations. Choose this for maximum protocol coverage.
- **Simple** ([internal/stack/simple](internal/stack/simple)) builds a trimmed stack directly with gVisor primitives, registers TCP/UDP forwarders, and keeps optional IPv6 support. It avoids extra layers for reduced overhead but sacrifices some advanced features.

Switch between stacks with `--stack gvisor` or `--stack simple` (or via `stackMode` in YAML).

## Development Tips
- Run `go test ./...` during development (ensure dependencies are downloaded first).
- Use `LOG_LEVEL=debug` or `--log-level debug` to inspect flow-level events.
- The tunnel driver expects you to manage OS routes and DNS settings externally.

## Referenced Projects & Licenses
| Project | Purpose | License |
| --- | --- | --- |
| [xjasonlyu/tun2socks](https://github.com/xjasonlyu/tun2socks) | Core tunnel, proxy adapters, statistics | MIT |
| [gVisor](https://github.com/google/gvisor) | Network stack primitives for the simple driver | Apache-2.0 |
| [gopsutil](https://github.com/shirou/gopsutil) | Process and connection inspection for exclusions | BSD-3-Clause |
| [spf13/pflag](https://github.com/spf13/pflag) | POSIX/GNU-style CLI flag parsing | BSD-3-Clause |
| [go-yaml/yaml](https://github.com/go-yaml/yaml) | YAML parsing for configuration files | Apache-2.0 |

Each project retains its original license; nano-tun simply consumes the published modules. Review upstream repositories before redistributing or embedding them in proprietary products.

## License
nano-tun is distributed under the terms of the [MIT License](LICENSE), which is compatible with the referenced upstream projects.
