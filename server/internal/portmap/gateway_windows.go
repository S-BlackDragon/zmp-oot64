//go:build windows

package portmap

import (
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DefaultGateways gives the IPv4 gateways of the adapters that are up, from the system's own table
// (GetAdaptersAddresses; no command is run).
func DefaultGateways() []netip.Addr {
	size := uint32(16 << 10)
	var buf []byte
	for i := 0; i < 3; i++ {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_INET, windows.GAA_FLAG_INCLUDE_GATEWAYS|windows.GAA_FLAG_SKIP_ANYCAST|
			windows.GAA_FLAG_SKIP_MULTICAST|windows.GAA_FLAG_SKIP_DNS_SERVER, 0,
			(*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if err != windows.ERROR_BUFFER_OVERFLOW {
			return nil
		}
		buf = nil
	}
	if buf == nil {
		return nil
	}
	var out []netip.Addr
	for a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); a != nil; a = a.Next {
		if a.OperStatus != windows.IfOperStatusUp {
			continue
		}
		for g := a.FirstGatewayAddress; g != nil; g = g.Next {
			ip, ok := netip.AddrFromSlice(g.Address.IP())
			if ok && ip.Unmap().Is4() && !ip.IsUnspecified() {
				out = append(out, ip.Unmap())
			}
		}
	}
	return out
}
