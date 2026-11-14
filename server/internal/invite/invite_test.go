package invite

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type fields struct {
	Addresses []string `json:"addresses"`
	Port      int      `json:"port"`
	Room      string   `json:"room"`
	Password  string   `json:"password"`
	Hint      string   `json:"hint"`
}

type vectorFile struct {
	Valid []struct {
		Fields fields `json:"fields"`
		Hex    string `json:"hex"`
		Text   string `json:"text"`
		Link   string `json:"link"`
	} `json:"valid"`
	SameAsFirst []string `json:"same_as_first"`
	Invalid     []struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	} `json:"invalid"`
}

func load(t *testing.T) vectorFile {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v vectorFile
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func toInvite(f fields) Invite {
	return Invite{Addresses: f.Addresses, Port: f.Port, Room: f.Room, Password: f.Password, Hint: f.Hint}
}

var errorsByName = map[string]error{"empty": ErrEmpty, "checksum": ErrChecksum, "length": ErrLength,
	"character": ErrCharacter, "version": ErrVersion, "malformed": ErrMalformed, "invalid": ErrInvalidData}

func TestVectors(t *testing.T) {
	v := load(t)
	for _, c := range v.Valid {
		inv := toInvite(c.Fields)
		b, err := Encode(inv)
		if err != nil {
			t.Fatal(err)
		}
		if hex.EncodeToString(b) != c.Hex {
			t.Errorf("Encode(%+v) = %x, want %s", inv, b, c.Hex)
		}
		if s, _ := Format(inv); s != c.Text {
			t.Errorf("Format = %s, want %s", s, c.Text)
		}
		if l, _ := Link(inv); l != c.Link {
			t.Errorf("Link = %s, want %s", l, c.Link)
		}
		for _, text := range []string{c.Text, c.Link} {
			got, err := Parse(text)
			if err != nil {
				t.Fatalf("Parse(%s): %v", text, err)
			}
			if !reflect.DeepEqual(got, inv) {
				t.Errorf("Parse(%s) = %+v, want %+v", text, got, inv)
			}
		}
	}
	first := toInvite(v.Valid[0].Fields)
	for _, text := range v.SameAsFirst {
		got, err := Parse(text)
		if err != nil || !reflect.DeepEqual(got, first) {
			t.Errorf("Parse(%q) = %+v, %v; want the first vector", text, got, err)
		}
	}
	for _, c := range v.Invalid {
		_, err := Parse(c.Text)
		if !errors.Is(err, errorsByName[c.Error]) {
			t.Errorf("Parse(%q) = %v, want %s", c.Text, err, c.Error)
		}
	}
}

// TestEveryTypoIsCaught: changing any one character of a code never gives another valid code.
func TestEveryTypoIsCaught(t *testing.T) {
	inv := Invite{Addresses: []string{"203.0.113.7", "192.168.1.20"}, Room: "bosque", Password: "saria"}
	b, _ := Encode(inv)
	s := Base32(b)
	for i := 0; i < len(s); i++ {
		for _, r := range crockford {
			if byte(r) == s[i] {
				continue
			}
			typo := s[:i] + string(r) + s[i+1:]
			if got, err := Parse(typo); err == nil {
				t.Fatalf("typo at %d (%s) gave a valid code: %+v", i, typo, got)
			}
		}
	}
}

func TestRoundTripAndLimits(t *testing.T) {
	cases := []Invite{
		{Addresses: []string{"127.0.0.1"}, Port: 47100, Room: "a"},
		{Addresses: []string{"[::1]"}, Port: 1, Room: strings.Repeat("ñ", 32), Password: strings.Repeat("x", 64)},
		{Addresses: []string{"10.0.0.1", "10.0.0.2", "a.b", "ws://127.0.0.1:47100/"}, Port: 65535, Room: "sala"},
	}
	for _, inv := range cases {
		s, err := Format(inv)
		if err != nil {
			t.Fatalf("%+v: %v", inv, err)
		}
		got, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%s): %v", s, err)
		}
		want := inv
		want.Addresses = append([]string(nil), inv.Addresses...)
		if want.Addresses[0] == "[::1]" {
			want.Addresses[0] = "::1"
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("round trip %+v -> %+v", want, got)
		}
	}
	bad := []Invite{
		{Room: "x"},
		{Addresses: []string{"1.2.3.4", "1.2.3.4", "1.2.3.4", "1.2.3.4", "1.2.3.4"}, Room: "x"},
		{Addresses: []string{"1.2.3.4"}},
		{Addresses: []string{"1.2.3.4"}, Room: strings.Repeat("x", 65)},
		{Addresses: []string{"1.2.3.4"}, Room: "x", Password: "a", Hint: "b"},
		{Addresses: []string{"1.2.3.4"}, Room: "x", Password: strings.Repeat("x", 65)},
		{Addresses: []string{"bad host!"}, Room: "x"},
		{Addresses: []string{"-a.com"}, Room: "x"},
		{Addresses: []string{"http://x"}, Room: "x"},
		{Addresses: []string{"fe80::1%eth0"}, Room: "x"},
		{Addresses: []string{"1.2.3.4"}, Room: "x", Port: 70000},
		{Addresses: []string{"1.2.3.4"}, Room: "\xff"},
	}
	for _, inv := range bad {
		if _, err := Encode(inv); !errors.Is(err, ErrInvalidData) {
			t.Errorf("Encode(%+v) = %v, want ErrInvalidData", inv, err)
		}
	}
}

// TestDecodeNeverPanics: every truncation and every single-byte change of a valid code fails cleanly.
func TestDecodeNeverPanics(t *testing.T) {
	b, _ := Encode(Invite{Addresses: []string{"2001:db8::7", "juego.example.org"}, Room: "bosque", Hint: "h"})
	for n := 0; n < len(b); n++ {
		if _, err := Decode(b[:n]); err == nil {
			t.Fatalf("a truncated code (%d bytes) decoded", n)
		}
	}
	for i := range b {
		for x := 0; x < 256; x++ {
			c := append([]byte(nil), b...)
			c[i] = byte(x)
			if c[i] == b[i] {
				continue
			}
			_, _ = Decode(c)
		}
	}
	// A body that passes the checksum but lies about its lengths.
	body := []byte{Version, 0, 1, addrHost, 200, 'a'}
	if _, err := Decode(append(body, checksum(body)...)); err == nil {
		t.Fatal("a lying length decoded")
	}
}
