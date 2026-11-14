//go:build !windows && !linux

package portmap

import "net/netip"

// DefaultGateways: unknown on this system (UPnP, which finds the router by itself, still works).
func DefaultGateways() []netip.Addr { return nil }
