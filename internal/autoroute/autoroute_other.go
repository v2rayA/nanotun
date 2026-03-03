//go:build !linux && !darwin && !windows

// Package autoroute automatically installs OS-level default routes and DNS
// redirect rules so that all traffic on the host is forwarded through the
// nanotun virtual gateway without manual configuration.
package autoroute

import (
	"fmt"
	"log/slog"
)

// Apply is not implemented on this platform and returns an error.
func Apply(tunName, proxyAddr string, log *slog.Logger) (func(), error) {
	return func() {}, fmt.Errorf("autoDefaultRoute is not supported on this platform")
}
