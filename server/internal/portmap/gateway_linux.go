//go:build linux

package portmap

import (
	"bufio"
	"net/netip"
	"os"
)

// DefaultGateways gives the IPv4 default gateways from /proc/net/route (no command is run).
func DefaultGateways() []netip.Addr {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return nil
	}
	defer f.Close()
	return parseProcRoute(bufio.NewScanner(f))
}
