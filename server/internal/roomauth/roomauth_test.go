package roomauth

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type vectors struct {
	RoomKeys []struct {
		Room, Password, Key string
	} `json:"room_keys"`
	ServerKeys []struct {
		Password, Key string
	} `json:"server_keys"`
	Proofs []struct {
		Kind        string `json:"kind"`
		Key         string `json:"key"`
		ServerNonce string `json:"server_nonce"`
		ClientNonce string `json:"client_nonce"`
		Room        string `json:"room"`
		Name        string `json:"name"`
		Proof       string `json:"proof"`
		Confirm     string `json:"confirm"`
	} `json:"proofs"`
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestVectors: the values the clients (C++ and JS) check their own code against.
func TestVectors(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	for _, c := range v.RoomKeys {
		if got := hex.EncodeToString(RoomKey(c.Room, c.Password)); got != c.Key {
			t.Errorf("RoomKey(%q, %q) = %s, want %s", c.Room, c.Password, got, c.Key)
		}
	}
	for _, c := range v.ServerKeys {
		if got := hex.EncodeToString(ServerKey(c.Password)); got != c.Key {
			t.Errorf("ServerKey(%q) = %s, want %s", c.Password, got, c.Key)
		}
	}
	for _, c := range v.Proofs {
		k, sn, cn := unhex(t, c.Key), unhex(t, c.ServerNonce), unhex(t, c.ClientNonce)
		if got := hex.EncodeToString(Proof(k, c.Kind, sn, cn, c.Room, c.Name)); got != c.Proof {
			t.Errorf("Proof %s = %s, want %s", c.Kind, got, c.Proof)
		}
		if c.Confirm != "" {
			if got := hex.EncodeToString(Confirm(k, c.Kind, sn, cn)); got != c.Confirm {
				t.Errorf("Confirm %s = %s, want %s", c.Kind, got, c.Confirm)
			}
		}
	}
	if len(v.RoomKeys) == 0 || len(v.Proofs) == 0 {
		t.Fatal("no vectors")
	}
}

func TestEmptyPasswordHasNoKey(t *testing.T) {
	if RoomKey("bosque", "  \t") != nil || ServerKey("") != nil {
		t.Fatal("an empty password must give no key")
	}
	if Equal(RoomKey("a", "x"), RoomKey("b", "x")) {
		t.Fatal("the room is the salt: two rooms with one password must have different keys")
	}
}

func TestStorePersistsAndExpires(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	s, err := OpenStore(dir, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	k := RoomKey("bosque", "saria")
	if err := s.Set("bosque", k, OwnerID("k:01"), false, now); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("fija", RoomKey("fija", "x"), "", true, now); err != nil {
		t.Fatal(err)
	}
	s2, err := OpenStore(dir, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !Equal(s2.Get("bosque"), k) {
		t.Fatal("the key did not survive a restart")
	}
	if s2.Get("fija") != nil {
		t.Fatal("a fixed key (from the configuration) must not be written to the file")
	}
	if s2.Info("bosque").Owner != OwnerID("k:01") {
		t.Fatal("owner lost")
	}
	s2.Set("activa", RoomKey("activa", "y"), "", false, now)
	s2.Set("fija", RoomKey("fija", "x"), "", true, now)
	later := now.Add(8 * 24 * time.Hour)
	if gone := s2.Prune(later, func(r string) bool { return r == "activa" }); len(gone) != 1 || gone[0] != "bosque" {
		t.Fatalf("Prune forgot %v, want [bosque] (activa is in use, fija is fixed)", gone)
	}
	if s2.Get("bosque") != nil || s2.Get("activa") == nil || s2.Get("fija") == nil {
		t.Fatal("wrong keys after Prune")
	}
	if s2.Info("activa").Used != later {
		t.Fatal("a room in use must be marked used")
	}
	if s3, _ := OpenStore(dir, 0); s3.Get("bosque") != nil || s3.Get("activa") == nil {
		t.Fatal("the expired key must be gone from the file too")
	}
	// Remove.
	s.Set("bosque", k, "", false, now)
	s.Set("bosque", nil, "", false, now)
	if s.Get("bosque") != nil {
		t.Fatal("Set(nil) must remove the password")
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter()
	now := time.Now()
	for i := 0; i < 4; i++ {
		if l.Fail("1.2.3.4", "bosque", now) {
			t.Fatalf("blocked after %d failures", i+1)
		}
	}
	if b, _ := l.Blocked("1.2.3.4", "bosque", now); b {
		t.Fatal("blocked too early")
	}
	if !l.Fail("1.2.3.4", "bosque", now) {
		t.Fatal("the fifth failure must block")
	}
	if b, d := l.Blocked("1.2.3.4", "bosque", now.Add(time.Second)); !b || d <= 0 {
		t.Fatal("not blocked")
	}
	if b, _ := l.Blocked("1.2.3.4", "otra", now); b {
		t.Fatal("another room of the same address must not be blocked (only 5 failures)")
	}
	if b, _ := l.Blocked("5.6.7.8", "bosque", now); b {
		t.Fatal("another address must not be blocked")
	}
	if b, _ := l.Blocked("1.2.3.4", "bosque", now.Add(61*time.Second)); b {
		t.Fatal("the block must end after 60 s")
	}
	// Twenty failures over many rooms block the address.
	l2 := NewLimiter()
	blocked := false
	for i := 0; i < 20; i++ {
		blocked = l2.Fail("9.9.9.9", string(rune('a'+i)), now)
	}
	if !blocked {
		t.Fatal("20 failures from one address must block it")
	}
	if b, _ := l2.Blocked("9.9.9.9", "zz", now); !b {
		t.Fatal("the address block must cover every room")
	}
}
