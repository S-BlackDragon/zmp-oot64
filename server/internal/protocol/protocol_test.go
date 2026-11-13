package protocol

import (
	"bytes"
	"testing"

	"github.com/vmihailenco/msgpack/v5"
)

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	in := Hello{T: "HELLO", Proto: Version, BuildHash: "abc", Name: "P1", Room: "r", ClientNonce: 1 << 40}
	if err := WriteFrame(&buf, in); err != nil {
		t.Fatal(err)
	}
	if got := buf.Bytes()[:4]; int(got[0])|int(got[1])<<8|int(got[2])<<16|int(got[3])<<24 != buf.Len()-4 {
		t.Fatalf("length prefix is not little-endian body length: %v", got)
	}
	body, err := ReadFrame(&buf)
	if err != nil {
		t.Fatal(err)
	}
	typ, err := Type(body)
	if err != nil || typ != "HELLO" {
		t.Fatalf("type %q %v", typ, err)
	}
	var out Hello
	if err := Decode(body, &out); err != nil || out != in {
		t.Fatalf("round trip mismatch %+v %v", out, err)
	}
}

func TestFrameTooLarge(t *testing.T) {
	buf := bytes.NewReader([]byte{0xff, 0xff, 0xff, 0x7f})
	if _, err := ReadFrame(buf); err != ErrFrameTooLarge {
		t.Fatalf("expected ErrFrameTooLarge, got %v", err)
	}
}

func TestLockstepWireTypes(t *testing.T) {
	// pad/data must be msgpack bin (0xc4..0xc6) and hashes uint64 (0xcf), which is what the
	// C++ client (nlohmann::json::from_msgpack) expects.
	body, err := msgpack.Marshal(Input{T: "INPUT", Tick: 3, Pad: []byte{1, 2, 3, 4, 5, 6, 7, 8}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, append([]byte{0xc4, 8}, 1, 2, 3, 4, 5, 6, 7, 8)) {
		t.Fatalf("pad not encoded as bin8: %x", body)
	}
	body, err = msgpack.Marshal(Hash{T: "HASH", Tick: 20, Hash: 0xfedcba9876543210})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte{0xcf, 0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10}) {
		t.Fatalf("hash not encoded as uint64: %x", body)
	}
	var h Hash
	if err := Decode(body, &h); err != nil || h.Hash != 0xfedcba9876543210 {
		t.Fatalf("round trip %+v %v", h, err)
	}
	frame, err := Encode(Tick{T: "TICK", Tick: 1, Slots: []int{0}, Pads: [][]byte{{0, 0, 0, 0, 0, 0, 0, 0}}, Events: []Event{{K: "SPAWN", Slot: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	var tk Tick
	if err := Decode(frame[4:], &tk); err != nil || len(tk.Pads) != 1 || tk.Events[0].K != "SPAWN" {
		t.Fatalf("tick round trip %+v %v", tk, err)
	}
}
