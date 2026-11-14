package portmap

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"net/netip"
	"strings"
)

// parseProcRoute reads Linux's /proc/net/route: the gateways of the default routes (destination 00000000; the
// addresses are little-endian hex).
func parseProcRoute(sc *bufio.Scanner) []netip.Addr {
	var out []netip.Addr
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		f := strings.Fields(sc.Text())
		if len(f) < 4 || f[1] != "00000000" { // destination 0.0.0.0: a default route
			continue
		}
		b, err := hex.DecodeString(f[2])
		if err != nil || len(b) != 4 {
			continue
		}
		var a [4]byte
		binary.LittleEndian.PutUint32(a[:], binary.BigEndian.Uint32(b))
		if ip := netip.AddrFrom4(a); !ip.IsUnspecified() {
			out = append(out, ip)
		}
	}
	return out
}
