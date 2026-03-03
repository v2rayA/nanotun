package config

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// StackMode enumerates the available packet processing backends.
type StackMode string

const (
	StackModeGVisor StackMode = "gvisor"
	StackModeSimple StackMode = "simple"
)

// Config represents the user facing configuration as loaded from disk or CLI.
type Config struct {
	TunName   string `yaml:"tunName" json:"tunName"`
	MTU       int    `yaml:"mtu" json:"mtu"`
	StackMode string `yaml:"stackMode" json:"stackMode"`
	Proxy     string `yaml:"proxy" json:"proxy"`
	// DNS is the upstream resolver the built-in gateway relay forwards queries
	// to after intercepting all port-53 traffic via the TUN gateway address.
	DNS               string        `yaml:"dns" json:"dns"`
	UDPTimeout        time.Duration `yaml:"udpTimeout" json:"udpTimeout"`
	ExcludedProcesses []string      `yaml:"excludedProcesses" json:"excludedProcesses"`
	ExcludeRefresh    time.Duration `yaml:"excludeRefresh" json:"excludeRefresh"`
	// ExcludedIPs lists CIDR prefixes (e.g. "10.8.0.0/16") whose traffic is
	// routed directly, bypassing the upstream proxy.  IANA-reserved ranges are
	// always bypassed regardless of this list.
	ExcludedIPs []string `yaml:"excludedIPs" json:"excludedIPs"`
	LogLevel    string   `yaml:"logLevel" json:"logLevel"`
	// AutoDefaultRoute, when true, causes nanotun to automatically install
	// OS-level default routes and DNS redirect rules at startup and remove
	// them cleanly on exit.
	AutoDefaultRoute bool `yaml:"autoDefaultRoute" json:"autoDefaultRoute"`
}

// Runtime includes the validated form of Config together with derived values.
type Runtime struct {
	TunName   string
	MTU       int
	StackMode StackMode
	Proxy     string
	// DNS is where the built-in gateway DNS relay forwards queries.
	// Defaults to 8.8.8.8:53 when not configured by the user.
	DNS               netip.AddrPort
	UDPTimeout        time.Duration
	ExcludedProcesses []string
	ExcludeRefresh    time.Duration
	// ExcludedIPs are the validated prefix form of Config.ExcludedIPs.
	ExcludedIPs      []netip.Prefix
	LogLevel         string
	AutoDefaultRoute bool
}

// Default returns the baseline configuration.
func Default() Config {
	return Config{
		TunName:        "nano0",
		MTU:            1500,
		StackMode:      string(StackModeGVisor),
		UDPTimeout:     time.Minute,
		ExcludeRefresh: 15 * time.Second,
		LogLevel:       "info",
	}
}

// LoadFile parses a YAML configuration file. Empty paths yield an empty config.
func LoadFile(path string) (Config, error) {
	if strings.TrimSpace(path) == "" {
		return Config{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Merge overrides the receiver with the provided values when they are non-zero.
func (c *Config) Merge(override Config) {
	if override.TunName != "" {
		c.TunName = override.TunName
	}
	if override.MTU != 0 {
		c.MTU = override.MTU
	}
	if override.StackMode != "" {
		c.StackMode = strings.ToLower(override.StackMode)
	}
	if override.Proxy != "" {
		c.Proxy = override.Proxy
	}
	if override.DNS != "" {
		c.DNS = override.DNS
	}
	if override.UDPTimeout != 0 {
		c.UDPTimeout = override.UDPTimeout
	}
	if len(override.ExcludedProcesses) > 0 {
		c.ExcludedProcesses = slices.Clone(override.ExcludedProcesses)
	}
	if override.ExcludeRefresh != 0 {
		c.ExcludeRefresh = override.ExcludeRefresh
	}
	if len(override.ExcludedIPs) > 0 {
		c.ExcludedIPs = slices.Clone(override.ExcludedIPs)
	}
	if override.LogLevel != "" {
		c.LogLevel = override.LogLevel
	}
	if override.AutoDefaultRoute {
		c.AutoDefaultRoute = true
	}
}

// Finalize validates the configuration and produces a runtime view.
func (c Config) Finalize() (Runtime, error) {
	base := Default()
	base.Merge(c)

	if base.Proxy == "" {
		return Runtime{}, errors.New("proxy address is required")
	}

	mode, err := parseStackMode(base.StackMode)
	if err != nil {
		return Runtime{}, err
	}

	excludedPrefixes, err := parsePrefixList(base.ExcludedIPs)
	if err != nil {
		return Runtime{}, fmt.Errorf("excludedIPs: %w", err)
	}

	run := Runtime{
		TunName:           base.TunName,
		MTU:               base.MTU,
		StackMode:         mode,
		Proxy:             base.Proxy,
		UDPTimeout:        base.UDPTimeout,
		ExcludedProcesses: normalizeProcessList(base.ExcludedProcesses),
		ExcludeRefresh:    base.ExcludeRefresh,
		ExcludedIPs:       excludedPrefixes,
		LogLevel:          base.LogLevel,
		AutoDefaultRoute:  base.AutoDefaultRoute,
	}

	if base.UDPTimeout == 0 {
		run.UDPTimeout = time.Minute
	}
	if base.ExcludeRefresh == 0 {
		run.ExcludeRefresh = 15 * time.Second
	}
	if base.MTU < 0 {
		return Runtime{}, fmt.Errorf("invalid MTU: %d", base.MTU)
	}

	// DNS upstream resolver for the built-in gateway relay.
	if strings.TrimSpace(base.DNS) != "" {
		addr, err := parseAddrPort(base.DNS)
		if err != nil {
			return Runtime{}, fmt.Errorf("dns: %w", err)
		}
		run.DNS = addr
	} else {
		run.DNS = netip.MustParseAddrPort("8.8.8.8:53")
	}

	return run, nil
}

func parseStackMode(v string) (StackMode, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", string(StackModeGVisor):
		return StackModeGVisor, nil
	case string(StackModeSimple):
		return StackModeSimple, nil
	default:
		return "", fmt.Errorf("unknown stack mode %q", v)
	}
}

func parseAddrPort(raw string) (netip.AddrPort, error) {
	if !strings.Contains(raw, ":") {
		raw = raw + ":53"
	}
	ap, err := netip.ParseAddrPort(raw)
	if err != nil {
		return netip.AddrPort{}, err
	}
	return ap, nil
}

func parsePrefixList(raw []string) ([]netip.Prefix, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]netip.Prefix, 0, len(raw))
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		// Allow bare IPs (treat as /32 or /128).
		if !strings.Contains(s, "/") {
			ip, err := netip.ParseAddr(s)
			if err != nil {
				return nil, fmt.Errorf("invalid IP/prefix %q", s)
			}
			if ip.Is4() {
				s = s + "/32"
			} else {
				s = s + "/128"
			}
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("invalid prefix %q: %w", s, err)
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

func normalizeProcessList(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	seen := make(map[string]struct{})
	for _, name := range names {
		cleaned := normalizeProcessName(name)
		if cleaned == "" {
			continue
		}
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		out = append(out, cleaned)
	}
	slices.Sort(out)
	return out
}

func normalizeProcessName(name string) string {
	cleaned := strings.ToLower(strings.TrimSpace(name))
	if strings.HasSuffix(cleaned, ".exe") {
		cleaned = strings.TrimSuffix(cleaned, ".exe")
	}
	return cleaned
}
