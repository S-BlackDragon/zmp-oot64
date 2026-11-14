// Package invite is the ZMP invite code (the protocol 5 design notes, section 7): a short text a host passes to a
// friend that says where to connect (one to four addresses and a port), which room and, if the host wants, the
// room's password or only a hint of it. Versioned and checksummed so a typo is caught before connecting. The same
// format is decoded by the game clients (C++ and JS) from the document's description and test vectors.
package invite

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"unicode/utf8"
)

// Version of the binary layout.
const Version = 1

// DefaultPort is the port when the code does not carry one.
const DefaultPort = 47100

// Limits of the fields.
const (
	MaxAddresses = 4
	MaxRoom      = 64
	MaxSecret    = 64
	MaxHost      = 253
	MaxURL       = 255
)

// Flags of byte 1.
const (
	flagPort     = 0x01
	flagPassword = 0x02
	flagHint     = 0x04
	flagsKnown   = flagPort | flagPassword | flagHint
)

// Address types.
const (
	addrHost = 1 // a host name (DNS), the code's port applies
	addrURL  = 2 // a ws:// or wss:// URL, used as is
	addrIPv4 = 4
	addrIPv6 = 6
)

// Prefix starts the text of every code; LinkPrefix starts its link.
const (
	Prefix     = "ZMP"
	LinkPrefix = "zmp://join/"
)

// Invite is what a code says.
type Invite struct {
	// Addresses in the order a client tries them: IPv4 or IPv6 literals, host names, or ws:// / wss:// URLs.
	Addresses []string
	// Port of the IP and host name addresses (0: DefaultPort).
	Port int
	Room string
	// Password, if the host put it in the code; Hint, if the host put only a hint (the client asks for the
	// password showing the hint). At most one of them.
	Password string
	Hint     string
}

// Errors of Decode, so a client can say which (all of them mean "check the code").
var (
	ErrEmpty       = errors.New("invite: empty code")
	ErrCharacter   = errors.New("invite: character not in the code's alphabet")
	ErrLength      = errors.New("invite: wrong length")
	ErrChecksum    = errors.New("invite: checksum does not match (typo?)")
	ErrVersion     = errors.New("invite: code from a newer version")
	ErrMalformed   = errors.New("invite: malformed")
	ErrInvalidData = errors.New("invite: invalid field")
)

// crockford is the alphabet: digits and capitals without I, L, O and U.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func checksum(b []byte) []byte {
	h := sha256.New()
	h.Write([]byte("zmp-invite\x00"))
	h.Write(b)
	return h.Sum(nil)[:4]
}

// Encode gives the binary form of inv (version, fields and checksum).
func Encode(inv Invite) ([]byte, error) {
	if len(inv.Addresses) < 1 || len(inv.Addresses) > MaxAddresses {
		return nil, fmt.Errorf("%w: %d addresses (1 to %d)", ErrInvalidData, len(inv.Addresses), MaxAddresses)
	}
	if !utf8.ValidString(inv.Room) || len(inv.Room) < 1 || len(inv.Room) > MaxRoom {
		return nil, fmt.Errorf("%w: room must be 1 to %d bytes of UTF-8", ErrInvalidData, MaxRoom)
	}
	if inv.Password != "" && inv.Hint != "" {
		return nil, fmt.Errorf("%w: a password or a hint, not both", ErrInvalidData)
	}
	secret := inv.Password + inv.Hint
	if !utf8.ValidString(secret) || len(secret) > MaxSecret {
		return nil, fmt.Errorf("%w: password or hint longer than %d bytes", ErrInvalidData, MaxSecret)
	}
	if inv.Port < 0 || inv.Port > 65535 {
		return nil, fmt.Errorf("%w: port %d", ErrInvalidData, inv.Port)
	}
	b := []byte{Version, 0}
	if inv.Port != 0 && inv.Port != DefaultPort {
		b[1] |= flagPort
		b = binary.BigEndian.AppendUint16(b, uint16(inv.Port))
	}
	if inv.Password != "" {
		b[1] |= flagPassword
	} else if inv.Hint != "" {
		b[1] |= flagHint
	}
	b = append(b, byte(len(inv.Addresses)))
	for _, a := range inv.Addresses {
		enc, err := encodeAddress(a)
		if err != nil {
			return nil, err
		}
		b = append(b, enc...)
	}
	b = append(b, byte(len(inv.Room)))
	b = append(b, inv.Room...)
	if secret != "" {
		b = append(b, byte(len(secret)))
		b = append(b, secret...)
	}
	return append(b, checksum(b)...), nil
}

func encodeAddress(a string) ([]byte, error) {
	a = strings.TrimSpace(a)
	if strings.HasPrefix(a, "[") && strings.HasSuffix(a, "]") {
		a = a[1 : len(a)-1]
	}
	if ip, err := netip.ParseAddr(a); err == nil {
		ip = ip.Unmap()
		if ip.Zone() != "" {
			return nil, fmt.Errorf("%w: address %q has a zone", ErrInvalidData, a)
		}
		if ip.Is4() {
			v := ip.As4()
			return append([]byte{addrIPv4}, v[:]...), nil
		}
		v := ip.As16()
		return append([]byte{addrIPv6}, v[:]...), nil
	}
	lower := strings.ToLower(a)
	if strings.HasPrefix(lower, "ws://") || strings.HasPrefix(lower, "wss://") {
		if len(a) > MaxURL || !printableASCII(a) {
			return nil, fmt.Errorf("%w: URL %q", ErrInvalidData, a)
		}
		return append([]byte{addrURL, byte(len(a))}, a...), nil
	}
	if !validHost(a) {
		return nil, fmt.Errorf("%w: host %q (an IP, a host name or a ws:// or wss:// URL)", ErrInvalidData, a)
	}
	return append([]byte{addrHost, byte(len(a))}, a...), nil
}

func printableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] <= ' ' || s[i] >= 0x7f {
			return false
		}
	}
	return true
}

// validHost: letters, digits, '-' and '.', 1 to 253 bytes, labels of 1 to 63 that do not start or end with '-'.
func validHost(h string) bool {
	if len(h) < 1 || len(h) > MaxHost {
		return false
	}
	for _, label := range strings.Split(h, ".") {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

// Decode reads the binary form.
func Decode(b []byte) (Invite, error) {
	var inv Invite
	if len(b) < 1 {
		return inv, ErrEmpty
	}
	if b[0] != Version {
		if b[0] > Version {
			return inv, ErrVersion
		}
		return inv, ErrMalformed
	}
	if len(b) < 2+1+3+2+4 { // (version, flags, count, the shortest address, a one-byte room, checksum)
		return inv, ErrLength
	}
	body, sum := b[:len(b)-4], b[len(b)-4:]
	if !equal(checksum(body), sum) {
		return inv, ErrChecksum
	}
	flags := body[1]
	if flags&^flagsKnown != 0 || flags&flagPassword != 0 && flags&flagHint != 0 {
		return inv, ErrVersion
	}
	p := body[2:]
	need := func(n int) bool { return len(p) >= n }
	inv.Port = DefaultPort
	if flags&flagPort != 0 {
		if !need(2) {
			return inv, ErrMalformed
		}
		inv.Port = int(binary.BigEndian.Uint16(p))
		p = p[2:]
		if inv.Port == 0 {
			return inv, ErrInvalidData
		}
	}
	if !need(1) {
		return inv, ErrMalformed
	}
	n := int(p[0])
	p = p[1:]
	if n < 1 || n > MaxAddresses {
		return inv, ErrInvalidData
	}
	for i := 0; i < n; i++ {
		if !need(1) {
			return inv, ErrMalformed
		}
		typ := p[0]
		p = p[1:]
		switch typ {
		case addrIPv4:
			if !need(4) {
				return inv, ErrMalformed
			}
			inv.Addresses = append(inv.Addresses, netip.AddrFrom4([4]byte(p[:4])).String())
			p = p[4:]
		case addrIPv6:
			if !need(16) {
				return inv, ErrMalformed
			}
			inv.Addresses = append(inv.Addresses, netip.AddrFrom16([16]byte(p[:16])).String())
			p = p[16:]
		case addrHost, addrURL:
			if !need(1) || !need(1+int(p[0])) {
				return inv, ErrMalformed
			}
			s := string(p[1 : 1+int(p[0])])
			p = p[1+int(p[0]):]
			if typ == addrHost && !validHost(s) {
				return inv, ErrInvalidData
			}
			l := strings.ToLower(s)
			if typ == addrURL && (!printableASCII(s) || !(strings.HasPrefix(l, "ws://") || strings.HasPrefix(l, "wss://"))) {
				return inv, ErrInvalidData
			}
			inv.Addresses = append(inv.Addresses, s)
		default:
			return inv, ErrVersion // (an address type of a newer version)
		}
	}
	str := func(max int) (string, error) {
		if !need(1) || !need(1+int(p[0])) {
			return "", ErrMalformed
		}
		s := string(p[1 : 1+int(p[0])])
		p = p[1+int(p[0]):]
		if len(s) < 1 || len(s) > max || !utf8.ValidString(s) {
			return "", ErrInvalidData
		}
		return s, nil
	}
	var err error
	if inv.Room, err = str(MaxRoom); err != nil {
		return inv, err
	}
	if flags&(flagPassword|flagHint) != 0 {
		s, err := str(MaxSecret)
		if err != nil {
			return inv, err
		}
		if flags&flagPassword != 0 {
			inv.Password = s
		} else {
			inv.Hint = s
		}
	}
	if len(p) != 0 {
		return inv, ErrMalformed
	}
	return inv, nil
}

func equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Base32 encodes b in Crockford's alphabet, 5 bits per character, most significant first, the last character
// padded with zero bits.
func Base32(b []byte) string {
	var sb strings.Builder
	var acc uint32
	bits := 0
	for _, x := range b {
		acc = acc<<8 | uint32(x)
		bits += 8
		for bits >= 5 {
			bits -= 5
			sb.WriteByte(crockford[(acc>>uint(bits))&31])
		}
	}
	if bits > 0 {
		sb.WriteByte(crockford[(acc<<uint(5-bits))&31])
	}
	return sb.String()
}

// unBase32 is the inverse of Base32 on characters already normalized to the alphabet.
func unBase32(s string) ([]byte, error) {
	if (len(s)*5)%8 >= 5 {
		return nil, ErrLength // (a character too many: no length of bytes gives this count)
	}
	out := make([]byte, 0, len(s)*5/8)
	var acc uint32
	bits := 0
	for i := 0; i < len(s); i++ {
		v := strings.IndexByte(crockford, s[i])
		if v < 0 {
			return nil, ErrCharacter
		}
		acc = acc<<5 | uint32(v)
		bits += 5
		if bits >= 8 {
			bits -= 8
			out = append(out, byte(acc>>uint(bits)))
		}
	}
	if bits > 0 && acc&(1<<uint(bits)-1) != 0 {
		return nil, ErrLength // (padding bits must be zero)
	}
	return out, nil
}

// Format gives the text of a code: "ZMP-" and groups of four characters separated by dashes.
func Format(inv Invite) (string, error) {
	b, err := Encode(inv)
	if err != nil {
		return "", err
	}
	s := Base32(b)
	var sb strings.Builder
	sb.WriteString(Prefix)
	for i := 0; i < len(s); i += 4 {
		sb.WriteByte('-')
		sb.WriteString(s[i:min(i+4, len(s))])
	}
	return sb.String(), nil
}

// Link gives the zmp:// link of a code (the same characters, without dashes).
func Link(inv Invite) (string, error) {
	b, err := Encode(inv)
	if err != nil {
		return "", err
	}
	return LinkPrefix + Base32(b), nil
}

// Normalize turns what a person typed or pasted into the code's characters: the link or "ZMP" prefix, dashes,
// spaces and line ends go away, letters become capitals, O reads as 0 and I and L as 1.
func Normalize(text string) string {
	t := strings.TrimSpace(text)
	low := strings.ToLower(t)
	for _, p := range []string{LinkPrefix, "zmp://", "zmp:"} {
		if strings.HasPrefix(low, p) {
			t = t[len(p):]
			break
		}
	}
	var sb strings.Builder
	for _, r := range t {
		switch {
		case r == '-' || r == ' ' || r == '\t' || r == '\r' || r == '\n' || r == '/' || r == '.':
			continue
		case r >= 'a' && r <= 'z':
			r -= 'a' - 'A'
		}
		switch r {
		case 'O':
			r = '0'
		case 'I', 'L':
			r = '1'
		}
		sb.WriteRune(r)
	}
	s := sb.String()
	// The text prefix: a body of version 1 to 31 starts with '0' (its first five bits are those of the version
	// byte), so "ZMP" at the start is always the prefix.
	if strings.HasPrefix(s, "ZMP") {
		s = s[3:]
	}
	return s
}

// Parse reads a code as text or as a link.
func Parse(text string) (Invite, error) {
	s := Normalize(text)
	if s == "" {
		return Invite{}, ErrEmpty
	}
	b, err := unBase32(s)
	if err != nil {
		return Invite{}, err
	}
	return Decode(b)
}
