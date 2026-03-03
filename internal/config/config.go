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
	TunName           string        `yaml:"tunName" json:"tunName"`
	MTU               int           `yaml:"mtu" json:"mtu"`
	StackMode         string        `yaml:"stackMode" json:"stackMode"`
	Proxy             string        `yaml:"proxy" json:"proxy"`
	DNSServer         string        `yaml:"dnsServer" json:"dnsServer"`
	UpstreamDNS       string        `yaml:"upstreamDNS" json:"upstreamDNS"`
	UDPTimeout        time.Duration `yaml:"udpTimeout" json:"udpTimeout"`
	ExcludedProcesses []string      `yaml:"excludedProcesses" json:"excludedProcesses"`
	ExcludeRefresh    time.Duration `yaml:"excludeRefresh" json:"excludeRefresh"`
	LogLevel          string        `yaml:"logLevel" json:"logLevel"`
}

// Runtime includes the validated form of Config together with derived values.
type Runtime struct {
	TunName        string
	MTU            int
	StackMode      StackMode
	Proxy          string
	DNSAddr        netip.AddrPort
	HasDNSOverride bool
	// UpstreamDNS is where the built-in gateway DNS relay forwards queries.
	// Defaults to 8.8.8.8:53 when not configured by the user.
	UpstreamDNS       netip.AddrPort
	UDPTimeout        time.Duration
	ExcludedProcesses []string
	ExcludeRefresh    time.Duration
	LogLevel          string
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
	if override.DNSServer != "" {
		c.DNSServer = override.DNSServer
	}
	if override.UpstreamDNS != "" {
		c.UpstreamDNS = override.UpstreamDNS
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
	if override.LogLevel != "" {
		c.LogLevel = override.LogLevel
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

	run := Runtime{
		TunName:           base.TunName,
		MTU:               base.MTU,
		StackMode:         mode,
		Proxy:             base.Proxy,
		UDPTimeout:        base.UDPTimeout,
		ExcludedProcesses: normalizeProcessList(base.ExcludedProcesses),
		ExcludeRefresh:    base.ExcludeRefresh,
		LogLevel:          base.LogLevel,
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

	if strings.TrimSpace(base.DNSServer) != "" {
		addr, err := parseAddrPort(base.DNSServer)
		if err != nil {
			return Runtime{}, fmt.Errorf("dnsServer: %w", err)
		}
		run.DNSAddr = addr
		run.HasDNSOverride = true
	}

	// Upstream DNS for the built-in gateway relay.
	if strings.TrimSpace(base.UpstreamDNS) != "" {
		addr, err := parseAddrPort(base.UpstreamDNS)
		if err != nil {
			return Runtime{}, fmt.Errorf("upstreamDNS: %w", err)
		}
		run.UpstreamDNS = addr
	} else {
		run.UpstreamDNS = netip.MustParseAddrPort("8.8.8.8:53")
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

func normalizeProcessList(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	seen := make(map[string]struct{})
	for _, name := range names {
		cleaned := strings.ToLower(strings.TrimSpace(name))
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
