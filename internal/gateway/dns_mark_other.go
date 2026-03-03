//go:build !linux

package gateway

import "net"

const nanotunMark = 0

// markDialer returns a plain dialer on non-Linux platforms where SO_MARK is
// not needed (DNS redirect is done via system DNS settings, not iptables).
func markDialer() *net.Dialer {
	return &net.Dialer{}
}
