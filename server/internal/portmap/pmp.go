package portmap

// NAT-PMP (RFC 6886) and PCP (RFC 6887): one UDP packet each way to the gateway's port 5351. Written here (about
// two hundred lines) because no maintained Go library does PCP, and the NAT-PMP one has its port fixed, which leaves
// no way to test it against a fake router on 127.0.0.1.

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"
)

type pmpMapper struct {
	gw   netip.Addr
	port int
	pcp  bool
}

func (p *pmpMapper) name() string {
	if p.pcp {
		return MethodPCP
	}
	return MethodNATPMP
}

// exchange sends req to the gateway and returns the first answer accepted by ok, resending after 250, 500 and
// 1000 ms (the RFCs' schedule, cut short) until ctx ends.
func (p *pmpMapper) exchange(ctx context.Context, req []byte, ok func([]byte) bool) ([]byte, netip.Addr, error) {
	if !p.gw.IsValid() {
		return nil, netip.Addr{}, errNoAnswer
	}
	conn, err := net.DialUDP("udp", nil, net.UDPAddrFromAddrPort(netip.AddrPortFrom(p.gw, uint16(p.port))))
	if err != nil {
		return nil, netip.Addr{}, fmt.Errorf("%w: %v", errNoAnswer, err)
	}
	defer conn.Close()
	local := netip.Addr{}
	if a, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		local = a.AddrPort().Addr().Unmap()
	}
	buf := make([]byte, 1100)
	wait := 250 * time.Millisecond
	for {
		if _, err := conn.Write(req); err != nil {
			return nil, local, fmt.Errorf("%w: %v", errNoAnswer, err)
		}
		deadline := time.Now().Add(wait)
		if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		_ = conn.SetReadDeadline(deadline)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				var ne net.Error
				if errors.As(err, &ne) && ne.Timeout() {
					break
				}
				// (an ICMP port unreachable: nothing listens on the gateway's 5351)
				return nil, local, fmt.Errorf("%w: %v", errNoAnswer, err)
			}
			if ok(buf[:n]) {
				return append([]byte(nil), buf[:n]...), local, nil
			}
		}
		if ctx.Err() != nil {
			return nil, local, errNoAnswer
		}
		if wait < time.Second {
			wait *= 2
		}
	}
}

func (p *pmpMapper) mapPort(ctx context.Context, port int, lease time.Duration, _ string) (Mapping, error) {
	if p.pcp {
		return p.pcpMap(ctx, port, lease, nil)
	}
	return p.natpmpMap(ctx, port, lease)
}

// ---------------------------------------------------------------------------------------------------------------
// NAT-PMP

var natpmpResults = map[uint16]string{1: "unsupported version", 2: "not authorized (turned off in the router)",
	3: "network failure", 4: "out of resources", 5: "unsupported opcode"}

func natpmpErr(code uint16) error {
	if t, ok := natpmpResults[code]; ok {
		return fmt.Errorf("NAT-PMP %d: %s", code, t)
	}
	return fmt.Errorf("NAT-PMP %d", code)
}

func (p *pmpMapper) natpmpExternal(ctx context.Context) (netip.Addr, error) {
	resp, _, err := p.exchange(ctx, []byte{0, 0}, func(b []byte) bool { return len(b) >= 12 && b[0] == 0 && b[1] == 128 })
	if err != nil {
		return netip.Addr{}, err
	}
	if code := binary.BigEndian.Uint16(resp[2:]); code != 0 {
		return netip.Addr{}, natpmpErr(code)
	}
	return netip.AddrFrom4([4]byte(resp[8:12])), nil
}

func (p *pmpMapper) natpmpRequest(ctx context.Context, port, external int, lease time.Duration) (extPort int, granted time.Duration, local netip.Addr, err error) {
	req := make([]byte, 12)
	req[1] = 2 // TCP
	binary.BigEndian.PutUint16(req[4:], uint16(port))
	binary.BigEndian.PutUint16(req[6:], uint16(external))
	binary.BigEndian.PutUint32(req[8:], uint32(lease/time.Second))
	resp, local, err := p.exchange(ctx, req, func(b []byte) bool {
		return len(b) >= 16 && b[0] == 0 && b[1] == 130 && int(binary.BigEndian.Uint16(b[8:])) == port
	})
	if err != nil {
		return 0, 0, local, err
	}
	if code := binary.BigEndian.Uint16(resp[2:]); code != 0 {
		return 0, 0, local, natpmpErr(code)
	}
	return int(binary.BigEndian.Uint16(resp[10:])), time.Duration(binary.BigEndian.Uint32(resp[12:])) * time.Second, local, nil
}

func (p *pmpMapper) natpmpMap(ctx context.Context, port int, lease time.Duration) (Mapping, error) {
	ext, err := p.natpmpExternal(ctx)
	if err != nil {
		return Mapping{}, err
	}
	extPort, granted, local, err := p.natpmpRequest(ctx, port, port, lease)
	if err != nil {
		return Mapping{}, err
	}
	mp := Mapping{Method: MethodNATPMP, External: ext, ExternalPort: extPort, InternalIP: local, InternalPort: port,
		Lease: granted, Gateway: p.gw}
	mp.remove = func(ctx context.Context) error {
		_, _, _, err := p.natpmpRequest(ctx, port, 0, 0)
		return err
	}
	mp.renew = func(ctx context.Context) (Mapping, error) {
		ext, err := p.natpmpExternal(ctx)
		if err != nil {
			return Mapping{}, err
		}
		extPort, granted, _, err := p.natpmpRequest(ctx, port, mp.ExternalPort, lease)
		if err != nil {
			return Mapping{}, err
		}
		next := mp
		next.External, next.ExternalPort, next.Lease = ext, extPort, granted
		return next, nil
	}
	return mp, nil
}

// ---------------------------------------------------------------------------------------------------------------
// PCP

var pcpResults = map[byte]string{1: "unsupported version", 2: "not authorized (turned off in the router)",
	3: "malformed request", 4: "unsupported opcode", 5: "unsupported option", 6: "malformed option",
	7: "network failure", 8: "no resources", 9: "unsupported protocol", 10: "user exceeded quota",
	11: "cannot provide the external port", 12: "address mismatch", 13: "excessive remote peers"}

func pcpErr(code byte) error {
	if t, ok := pcpResults[code]; ok {
		return fmt.Errorf("PCP %d: %s", code, t)
	}
	return fmt.Errorf("PCP %d", code)
}

// pcpMapRequest builds a MAP request (RFC 6887 11.1): 24 bytes of header and 36 of MAP.
func pcpMapRequest(client netip.Addr, nonce []byte, port, external int, lease time.Duration) []byte {
	b := make([]byte, 60)
	b[0], b[1] = 2, 1 // version 2, request, opcode MAP
	binary.BigEndian.PutUint32(b[4:], uint32(lease/time.Second))
	c16 := client.As16() // (As16 of an IPv4 address is ::ffff:a.b.c.d, the form PCP wants)
	copy(b[8:24], c16[:])
	copy(b[24:36], nonce)
	b[36] = 6 // TCP
	binary.BigEndian.PutUint16(b[40:], uint16(port))
	binary.BigEndian.PutUint16(b[42:], uint16(external))
	if client.Is4() { // suggested external address: any IPv4 (::ffff:0.0.0.0)
		b[54], b[55] = 0xff, 0xff
	}
	return b
}

func (p *pmpMapper) pcpMap(ctx context.Context, port int, lease time.Duration, nonce []byte) (Mapping, error) {
	if nonce == nil {
		nonce = make([]byte, 12)
		_, _ = rand.Read(nonce)
	}
	client := localAddrTo(p.gw)
	if !client.IsValid() {
		return Mapping{}, errNoAnswer
	}
	send := func(ctx context.Context, external int, lease time.Duration) ([]byte, error) {
		req := pcpMapRequest(client, nonce, port, external, lease)
		resp, _, err := p.exchange(ctx, req, func(b []byte) bool {
			if len(b) >= 4 && b[0] == 0 && b[1] >= 128 { // a NAT-PMP-only gateway answers in its own version
				return true
			}
			return len(b) >= 60 && b[0] == 2 && b[1] == 0x81 && string(b[24:36]) == string(nonce)
		})
		if err != nil {
			return nil, err
		}
		if resp[0] == 0 {
			return nil, fmt.Errorf("%w: the gateway speaks NAT-PMP only", errNoAnswer)
		}
		if resp[3] != 0 {
			return nil, pcpErr(resp[3])
		}
		return resp, nil
	}
	resp, err := send(ctx, port, lease)
	if err != nil {
		return Mapping{}, err
	}
	parse := func(resp []byte) (netip.Addr, int, time.Duration) {
		ext := netip.AddrFrom16([16]byte(resp[44:60])).Unmap()
		return ext, int(binary.BigEndian.Uint16(resp[42:])), time.Duration(binary.BigEndian.Uint32(resp[4:])) * time.Second
	}
	ext, extPort, granted := parse(resp)
	mp := Mapping{Method: MethodPCP, External: ext, ExternalPort: extPort, InternalIP: client, InternalPort: port,
		Lease: granted, Gateway: p.gw}
	mp.remove = func(ctx context.Context) error {
		_, err := send(ctx, 0, 0)
		return err
	}
	mp.renew = func(ctx context.Context) (Mapping, error) {
		resp, err := send(ctx, mp.ExternalPort, lease)
		if err != nil {
			return Mapping{}, err
		}
		next := mp
		next.External, next.ExternalPort, next.Lease = parse(resp)
		return next, nil
	}
	return mp, nil
}
