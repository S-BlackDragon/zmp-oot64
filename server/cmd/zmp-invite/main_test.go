package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestEncodeDecode(t *testing.T) {
	var out, errOut bytes.Buffer
	if rc := run([]string{"encode", "-addr", "203.0.113.7", "-addr", "192.168.1.20", "-room", "bosque", "-password", "saria"}, &out, &errOut); rc != 0 {
		t.Fatalf("encode rc=%d %s", rc, errOut.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if lines[0] != "ZMP-0410-416B-01RG-E160-N00H-81K2-DXSQ-2XB5-0NSP-2WK9-C4AY-X329" {
		t.Fatalf("text %q", lines[0])
	}
	out.Reset()
	if rc := run([]string{"decode", lines[1]}, &out, &errOut); rc != 0 {
		t.Fatalf("decode rc=%d %s", rc, errOut.String())
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["room"] != "bosque" || got["password"] != "saria" || got["port"].(float64) != 47100 {
		t.Fatalf("decoded %v", got)
	}
	if rc := run([]string{"decode", "ZMP-0000"}, &out, &errOut); rc != 1 {
		t.Fatalf("bad code rc=%d", rc)
	}
	if rc := run([]string{"encode", "-room", "x"}, &out, &errOut); rc != 2 {
		t.Fatalf("no address rc=%d", rc)
	}
}
