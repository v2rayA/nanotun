//go:build linux

package gateway

import (
	"net"
	"syscall"
)

// nanotunMark is a netfilter socket mark used to prevent the DNS relay's own
// upstream queries from being re-intercepted by the nft/iptables rules that
// redirect port-53 traffic to the gateway.  The value 0x6e74 spells "nt"
// (nanotun) in ASCII.
const nanotunMark = 0x6e74

// markDialer returns a net.Dialer whose sockets are stamped with nanotunMark
// so that the redirect rules in autoroute_linux.go can exempt them.
func markDialer() *net.Dialer {
	return &net.Dialer{
		Control: func(network, address string, c syscall.RawConn) error {
			var setSockErr error
			err := c.Control(func(fd uintptr) {
				setSockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_MARK, nanotunMark)
			})
			if err != nil {
				return err
			}
			return setSockErr
		},
	}
}
