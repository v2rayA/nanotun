package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/pflag"

	"github.com/v2rayA/nanotun/internal/app"
	"github.com/v2rayA/nanotun/internal/config"
	"github.com/v2rayA/nanotun/internal/logger"
)

func main() {
	runtimeCfg, err := loadRuntimeConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}

	log := logger.SetupWithDir(runtimeCfg.LogLevel, runtimeCfg.LogDir)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, runtimeCfg, log); err != nil && err != context.Canceled {
		log.Error("run failed", "error", err)
		os.Exit(1)
	}
}

func loadRuntimeConfig() (config.Runtime, error) {
	fs := pflag.NewFlagSet("nanotun", pflag.ExitOnError)

	configPath := fs.String("config", "", "Path to YAML configuration file")
	tunName := fs.String("tun", "", "Name of the TUN interface")
	mtu := fs.Int("mtu", 0, "MTU to enforce for the TUN device")
	stackMode := fs.String("stack", "", "Stack mode: gvisor | simple")
	proxyAddr := fs.String("proxy", "", "Proxy address, e.g. socks5://127.0.0.1:1080")
	dns := fs.String("dns", "", "Upstream DNS server the gateway relay forwards queries to (host or host:port, default 8.8.8.8:53)")
	autoRoute := fs.Bool("auto-route", false, "Automatically add default routes and redirect DNS via the TUN gateway")
	udpTimeout := fs.Duration("udp-timeout", 0, "UDP session timeout")
	excludeProcesses := fs.StringArray("exclude", nil, "Process name to bypass; repeatable")
	excludeRefresh := fs.Duration("exclude-refresh", 0, "Process table refresh interval")
	excludeIPs := fs.StringArray("exclude-ip", nil, "CIDR prefix or IP to route directly (bypass proxy); repeatable")
	logLevel := fs.String("log-level", "", "Log level: debug|info|warn|error")
	logDir := fs.String("log-dir", "", "Directory for log files (default: ./logs)")

	_ = fs.Parse(os.Args[1:])

	base := config.Default()
	fileCfg, err := config.LoadFile(*configPath)
	if err != nil {
		return config.Runtime{}, err
	}
	base.Merge(fileCfg)

	var overrides config.Config
	if fs.Changed("tun") {
		overrides.TunName = *tunName
	}
	if fs.Changed("mtu") {
		overrides.MTU = *mtu
	}
	if fs.Changed("stack") {
		overrides.StackMode = *stackMode
	}
	if fs.Changed("proxy") {
		overrides.Proxy = *proxyAddr
	}
	if fs.Changed("dns") {
		overrides.DNS = *dns
	}
	if fs.Changed("auto-route") {
		overrides.AutoDefaultRoute = *autoRoute
	}
	if fs.Changed("udp-timeout") {
		overrides.UDPTimeout = *udpTimeout
	}
	if fs.Changed("exclude") {
		overrides.ExcludedProcesses = *excludeProcesses
	}
	if fs.Changed("exclude-refresh") {
		overrides.ExcludeRefresh = *excludeRefresh
	}
	if fs.Changed("exclude-ip") {
		overrides.ExcludedIPs = *excludeIPs
	}
	if fs.Changed("log-level") {
		overrides.LogLevel = *logLevel
	}
	if fs.Changed("log-dir") {
		overrides.LogDir = *logDir
	}

	base.Merge(overrides)
	return base.Finalize()
}
