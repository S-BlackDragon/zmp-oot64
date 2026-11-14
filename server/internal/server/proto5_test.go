package server

// Protocol 5 (the protocol 5 design notes): passwords by challenge and response, player_key, rename, colour, engine,
// room list, relay limits, and the older protocols in rooms without a password.

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/netip"
	"strconv"
	"testing"
	"time"

	"github.com/vmihailenco/msgpack/v5"

	"github.com/S-BlackDragon/zmp-oot64/server/internal/invite"
	"github.com/S-BlackDragon/zmp-oot64/server/internal/protocol"
	"github.com/S-BlackDragon/zmp-oot64/server/internal/roomauth"
)

func randBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

func newPlayerKey() string { return hex.EncodeToString(randBytes(16)) }

type p5 struct {
	name, room, password, serverPassword, key, engine string
	color                                             *int
}

func (o p5) hello() protocol.Hello {
	h := hello(o.name, o.room, "b")
	h.Proto = 5
	h.AuthNonce = randBytes(16)
	h.HasPassword = roomauth.Normalize(o.password) != ""
	h.PlayerKey = o.key
	h.Color = o.color
	h.Engine = o.engine
	return h
}

// join5 runs the whole protocol 5 entry as a client would and returns the client and the final message
// (WELCOME or REJECT) with its body.
func join5(t *testing.T, addr string, o p5) (*testClient, string, []byte) {
	t.Helper()
	h := o.hello()
	c := dial(t, addr, h)
	typ, body := c.next(t)
	if typ != "CHALLENGE" {
		return c, typ, body
	}
	ch := decode[protocol.Challenge](t, body)
	if len(ch.Nonce) != 16 {
		t.Fatalf("challenge nonce %x", ch.Nonce)
	}
	a := protocol.Auth{T: "AUTH"}
	if ch.Server && o.serverPassword != "" {
		a.ServerProof = roomauth.Proof(roomauth.ServerKey(o.serverPassword), "server", ch.Nonce, h.AuthNonce, h.Room, h.Name)
	}
	if k := roomauth.RoomKey(h.Room, o.password); k != nil {
		if ch.Room {
			a.RoomProof = roomauth.Proof(k, "room", ch.Nonce, h.AuthNonce, h.Room, h.Name)
		}
		if ch.Create {
			a.RoomKey = k
		}
	}
	c.send(t, a)
	typ, body = c.next(t)
	if typ == "WELCOME" {
		w := decode[protocol.Welcome](t, body)
		if k := roomauth.RoomKey(h.Room, o.password); len(w.RoomConfirm) > 0 &&
			!bytes.Equal(w.RoomConfirm, roomauth.Confirm(k, "room", ch.Nonce, h.AuthNonce)) {
			t.Fatal("WELCOME.room_confirm does not prove the server knows the key")
		}
		if ch.Server && !bytes.Equal(w.ServerConfirm, roomauth.Confirm(roomauth.ServerKey(o.serverPassword), "server", ch.Nonce, h.AuthNonce)) {
			t.Fatal("WELCOME.server_confirm missing or wrong")
		}
	}
	return c, typ, body
}

func mustWelcome(t *testing.T, addr string, o p5) (*testClient, protocol.Welcome) {
	t.Helper()
	c, typ, body := join5(t, addr, o)
	if typ != "WELCOME" {
		t.Fatalf("%s: expected WELCOME, got %s %+v", o.name, typ, decode[protocol.Reject](t, body))
	}
	return c, decode[protocol.Welcome](t, body)
}

func mustReject(t *testing.T, addr string, o p5, reason string) protocol.Reject {
	t.Helper()
	_, typ, body := join5(t, addr, o)
	if typ != "REJECT" {
		t.Fatalf("%s: expected REJECT %s, got %s", o.name, reason, typ)
	}
	r := decode[protocol.Reject](t, body)
	if r.Reason != reason || r.Detail == "" {
		t.Fatalf("%s: expected %s, got %+v", o.name, reason, r)
	}
	return r
}

func TestP5RoomWithoutPasswordNeedsNoChallenge(t *testing.T) {
	_, addr := startServer(t)
	red := 0xC03030
	c, w := mustWelcome(t, addr, p5{name: "A", room: "libre", key: newPlayerKey(), color: &red})
	if w.Proto != 5 || w.RoomPassword || w.MaxPlayers != 6 || len(w.RoomConfirm) != 0 {
		t.Fatalf("welcome %+v", w)
	}
	if len(w.Players) != 1 || w.Players[0].Color == nil || *w.Players[0].Color != red {
		t.Fatalf("players %+v", w.Players)
	}
	_ = c
	// A password typed for a room that has none: in, and WELCOME says the room has no password.
	_, w2 := mustWelcome(t, addr, p5{name: "B", room: "libre", password: "x", key: newPlayerKey()})
	if w2.RoomPassword {
		t.Fatal("the room has no password")
	}
}

func TestP5PasswordRoom(t *testing.T) {
	_, addr := startServer(t)
	a, w := mustWelcome(t, addr, p5{name: "A", room: "bosque", password: "saria", key: newPlayerKey()})
	if !w.RoomPassword || !w.RoomOwner || len(w.RoomConfirm) != 32 {
		t.Fatalf("creator's welcome %+v", w)
	}
	_ = a
	_, wb := mustWelcome(t, addr, p5{name: "B", room: "bosque", password: " saria\n", key: newPlayerKey()})
	if !wb.RoomPassword || wb.RoomOwner {
		t.Fatalf("second player's welcome %+v", wb)
	}
	mustReject(t, addr, p5{name: "C", room: "bosque", password: "mido", key: newPlayerKey()}, "bad_password")
	mustReject(t, addr, p5{name: "D", room: "bosque", key: newPlayerKey()}, "password_required")
	// Protocol 4 cannot enter a room with a password; the detail tells its player to update.
	old := dial(t, addr, hello3("E", "bosque"))
	typ, body := old.next(t)
	r := decode[protocol.Reject](t, body)
	if typ != "REJECT" || r.Reason != "password_required" || r.Args != nil || !bytes.Contains([]byte(r.Detail), []byte("actualiza")) {
		t.Fatalf("protocol 4 in a password room: %s %+v", typ, r)
	}
	// The proof is bound to the connection: replaying B's AUTH on a new connection fails.
	h := p5{name: "F", room: "bosque", password: "saria", key: newPlayerKey()}.hello()
	c1 := dial(t, addr, h)
	ch1 := decode[protocol.Challenge](t, c1.expect(t, "CHALLENGE"))
	proof := roomauth.Proof(roomauth.RoomKey("bosque", "saria"), "room", ch1.Nonce, h.AuthNonce, "bosque", "F")
	c1.conn.Close()
	c2 := dial(t, addr, h)
	ch2 := decode[protocol.Challenge](t, c2.expect(t, "CHALLENGE"))
	if bytes.Equal(ch1.Nonce, ch2.Nonce) {
		t.Fatal("the server nonce must change at each connection")
	}
	c2.send(t, protocol.Auth{T: "AUTH", RoomProof: proof})
	if r := decode[protocol.Reject](t, c2.expect(t, "REJECT")); r.Reason != "bad_password" {
		t.Fatalf("replayed proof: %+v", r)
	}
}

func TestP5PasswordSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	start := func() (*Server, string) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		cfg := testConfig()
		cfg.DataDir = dir
		s := New(cfg)
		if err := s.PrepareDataDir(); err != nil {
			t.Fatal(err)
		}
		go func() { _ = s.Serve(ln) }()
		return s, ln.Addr().String()
	}
	s, addr := start()
	c, _ := mustWelcome(t, addr, p5{name: "A", room: "templo", password: "zelda", key: newPlayerKey()})
	c.conn.Close()
	time.Sleep(50 * time.Millisecond)
	s.Close()
	s2, addr2 := start()
	defer s2.Close()
	// The room is empty and gone from memory, its password is not.
	mustReject(t, addr2, p5{name: "B", room: "templo", key: newPlayerKey()}, "password_required")
	mustReject(t, addr2, p5{name: "B", room: "templo", password: "link", key: newPlayerKey()}, "bad_password")
	mustWelcome(t, addr2, p5{name: "B", room: "templo", password: "zelda", key: newPlayerKey()})
}

func TestP5TooManyAttempts(t *testing.T) {
	_, addr := startServer(t)
	mustWelcome(t, addr, p5{name: "A", room: "bosque", password: "saria", key: newPlayerKey()})
	for i := 0; i < 5; i++ {
		mustReject(t, addr, p5{name: "B", room: "bosque", password: "no", key: newPlayerKey()}, "bad_password")
	}
	r := mustReject(t, addr, p5{name: "B", room: "bosque", password: "saria", key: newPlayerKey()}, "too_many_attempts")
	if r.Args["retry_after"] == "" {
		t.Fatalf("too_many_attempts without retry_after: %+v", r)
	}
	// Another room of the same address is not blocked.
	mustWelcome(t, addr, p5{name: "B", room: "otra", key: newPlayerKey()})
}

func TestP5ServerPassword(t *testing.T) {
	cfg := testConfig()
	cfg.ServerPassword = "hyrule"
	_, addr := startServerCfg(t, cfg)
	old := dial(t, addr, hello3("V", "r"))
	if r := decode[protocol.Reject](t, old.expect(t, "REJECT")); r.Reason != "server_password_required" {
		t.Fatalf("protocol 4 on a server with a password: %+v", r)
	}
	mustReject(t, addr, p5{name: "A", room: "r"}, "server_password_required")
	mustReject(t, addr, p5{name: "A", room: "r", serverPassword: "gerudo"}, "bad_server_password")
	mustWelcome(t, addr, p5{name: "A", room: "r", serverPassword: "hyrule"})
	// Both keys at once: a room with a password on a server with a password.
	mustWelcome(t, addr, p5{name: "B", room: "s", serverPassword: "hyrule", password: "x", key: newPlayerKey()})
	mustReject(t, addr, p5{name: "C", room: "s", serverPassword: "hyrule", key: newPlayerKey()}, "password_required")
	mustWelcome(t, addr, p5{name: "C", room: "s", serverPassword: "hyrule", password: "x", key: newPlayerKey()})
}

func TestP5RequireRoomPassword(t *testing.T) {
	cfg := testConfig()
	cfg.RequireRoomPassword = true
	_, addr := startServerCfg(t, cfg)
	r := mustReject(t, addr, p5{name: "A", room: "r"}, "password_required")
	if r.Args["create"] != "1" {
		t.Fatalf("args %+v", r.Args)
	}
	old := dial(t, addr, hello3("V", "r2"))
	if r := decode[protocol.Reject](t, old.expect(t, "REJECT")); r.Reason != "password_required" {
		t.Fatalf("protocol 4 creating a room: %+v", r)
	}
	mustWelcome(t, addr, p5{name: "A", room: "r", password: "x"})
}

func TestP5PlayerKeyIdentity(t *testing.T) {
	_, addr := startServer(t)
	k1 := newPlayerKey()
	a, _ := mustWelcome(t, addr, p5{name: "Saria", room: "r", key: k1})
	found(t, a)
	other, _ := mustWelcome(t, addr, p5{name: "Otro", room: "r", key: newPlayerKey()})
	if p := join(t, other, a); p.Slot != 1 {
		t.Fatalf("slot %d", p.Slot)
	}
	// Somebody else with the same name, a key of its own: name_taken, even right away.
	mustReject(t, addr, p5{name: "Saria", room: "r", key: newPlayerKey()}, "name_taken")
	mustReject(t, addr, p5{name: "Saria", room: "r"}, "name_taken")
	// The same player comes back at once with a new name: takes the old connection over and finds its slot.
	b, w := mustWelcome(t, addr, p5{name: "Saria2", room: "r", key: k1})
	if !w.Rejoin {
		t.Fatal("the same player_key must rejoin its slot")
	}
	for _, p := range w.Players {
		if p.Name == "Saria" && p.State != protocol.StateDisconnected {
			t.Fatalf("the old connection is still there: %+v", w.Players)
		}
	}
	_ = b
	// Clients without a key keep protocol 4's rule: a live name is taken.
	_, _ = mustWelcome(t, addr, p5{name: "Mido", room: "r"})
	mustReject(t, addr, p5{name: "Mido", room: "r"}, "name_taken")
	// The player_key never travels to the others.
	raw := dial(t, addr, p5{name: "Spy", room: "r"}.hello())
	body := raw.expect(t, "WELCOME")
	if bytes.Contains(body, []byte(k1)) {
		t.Fatal("a player_key was sent to another player")
	}
	if r := mustReject(t, addr, p5{name: "X", room: "r", key: "NOT-HEX"}, "bad_hello"); r.Reason != "bad_hello" {
		t.Fatal()
	}
}

func TestP5RenameAndColor(t *testing.T) {
	_, addr := startServer(t)
	a, _ := mustWelcome(t, addr, p5{name: "A", room: "r", key: newPlayerKey()})
	found(t, a)
	b, _ := mustWelcome(t, addr, p5{name: "B", room: "r", key: newPlayerKey()})
	a.send(t, protocol.Rename{T: "RENAME", Name: "B"})
	if r := decode[protocol.RenameResult](t, a.expect(t, "RENAME_RESULT")); r.OK || r.Reason != "name_taken" || r.Name != "A" {
		t.Fatalf("rename to a taken name: %+v", r)
	}
	a.send(t, protocol.Rename{T: "RENAME", Name: ""})
	if r := decode[protocol.RenameResult](t, a.expect(t, "RENAME_RESULT")); r.OK || r.Reason != "bad_name" {
		t.Fatalf("rename to nothing: %+v", r)
	}
	a.send(t, protocol.Rename{T: "RENAME", Name: "Malon"})
	if r := decode[protocol.RenameResult](t, a.expect(t, "RENAME_RESULT")); !r.OK || r.Name != "Malon" {
		t.Fatalf("rename: %+v", r)
	}
	waitList := func(c *testClient, ok func([]protocol.PlayerInfo) bool) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			typ, body := c.next(t)
			if typ != "PLAYER_LIST" {
				continue
			}
			l := decode[protocol.PlayerList](t, body)
			if l.RoomPassword == nil {
				t.Fatal("protocol 5 PLAYER_LIST without room_password")
			}
			if ok(l.Players) {
				return
			}
		}
		t.Fatal("PLAYER_LIST never showed the change")
	}
	waitList(b, func(ps []protocol.PlayerInfo) bool {
		for _, p := range ps {
			if p.Name == "Malon" && p.Slot == 0 {
				return true
			}
		}
		return false
	})
	a.send(t, protocol.SetColor{T: "SET_COLOR", Color: 0x30C060})
	waitList(b, func(ps []protocol.PlayerInfo) bool {
		for _, p := range ps {
			if p.Name == "Malon" && p.Color != nil && *p.Color == 0x30C060 {
				return true
			}
		}
		return false
	})
	// The freed name can be taken now.
	mustWelcome(t, addr, p5{name: "A", room: "r", key: newPlayerKey()})
}

func TestP5RoomSetKey(t *testing.T) {
	_, addr := startServer(t)
	owner, _ := mustWelcome(t, addr, p5{name: "A", room: "r", key: newPlayerKey()})
	other, _ := mustWelcome(t, addr, p5{name: "B", room: "r", key: newPlayerKey()})
	other.send(t, protocol.RoomSetKey{T: "ROOM_SET_KEY", RoomKey: roomauth.RoomKey("r", "mio")})
	if r := decode[protocol.RoomSetKeyResult](t, other.expect(t, "ROOM_SET_KEY_RESULT")); r.OK || r.Reason != "not_owner" {
		t.Fatalf("not the owner: %+v", r)
	}
	owner.send(t, protocol.RoomSetKey{T: "ROOM_SET_KEY", RoomKey: []byte{1, 2}})
	if r := decode[protocol.RoomSetKeyResult](t, owner.expect(t, "ROOM_SET_KEY_RESULT")); r.OK || r.Reason != "bad_key" {
		t.Fatalf("short key: %+v", r)
	}
	owner.send(t, protocol.RoomSetKey{T: "ROOM_SET_KEY", RoomKey: roomauth.RoomKey("r", "nueva")})
	if r := decode[protocol.RoomSetKeyResult](t, owner.expect(t, "ROOM_SET_KEY_RESULT")); !r.OK {
		t.Fatalf("owner: %+v", r)
	}
	mustReject(t, addr, p5{name: "C", room: "r", key: newPlayerKey()}, "password_required")
	mustWelcome(t, addr, p5{name: "C", room: "r", password: "nueva", key: newPlayerKey()})
	// And removes it.
	owner.send(t, protocol.RoomSetKey{T: "ROOM_SET_KEY"})
	if r := decode[protocol.RoomSetKeyResult](t, owner.expect(t, "ROOM_SET_KEY_RESULT")); !r.OK {
		t.Fatalf("remove: %+v", r)
	}
	mustWelcome(t, addr, p5{name: "D", room: "r", key: newPlayerKey()})
}

func TestP5EngineMismatch(t *testing.T) {
	_, addr := startServer(t)
	mustWelcome(t, addr, p5{name: "A", room: "r", engine: "native"})
	r := mustReject(t, addr, p5{name: "B", room: "r", engine: "wasm"}, "engine_mismatch")
	if r.Args["engine"] != "native" || r.Args["build_hash"] != "b" {
		t.Fatalf("args %+v", r.Args)
	}
	mustWelcome(t, addr, p5{name: "C", room: "r"}) // (no engine: not checked)
}

func TestP5RoomList(t *testing.T) {
	list := func(addr string) (string, []byte) {
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		_ = protocol.WriteFrame(c, protocol.RoomListRequest{T: "ROOM_LIST_REQUEST", Proto: 5})
		_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
		body, err := protocol.ReadFrame(c)
		if err != nil {
			t.Fatal(err)
		}
		typ, _ := protocol.Type(body)
		if _, err := protocol.ReadFrame(c); err == nil {
			t.Fatal("the server must close after answering the list")
		}
		return typ, body
	}
	_, addr := startServer(t)
	if typ, body := list(addr); typ != "REJECT" || decode[protocol.Reject](t, body).Reason != "rooms_hidden" {
		t.Fatalf("list off: %s", typ)
	}
	cfg := testConfig()
	cfg.ListRooms, cfg.ServerName = true, "Casa"
	_, addr2 := startServerCfg(t, cfg)
	mustWelcome(t, addr2, p5{name: "A", room: "bosque", password: "x", engine: "wasm"})
	mustWelcome(t, addr2, p5{name: "B", room: "abierta"})
	typ, body := list(addr2)
	l := decode[protocol.RoomList](t, body)
	if typ != "ROOM_LIST" || l.ServerName != "Casa" || len(l.Rooms) != 2 || l.Rooms[0].Name != "abierta" ||
		l.Rooms[0].Password || !l.Rooms[1].Password || l.Rooms[1].Engine != "wasm" || l.Rooms[1].Players != 1 ||
		l.Rooms[1].Max != 6 || l.Rooms[1].Proto != 5 {
		t.Fatalf("list %+v", l)
	}
}

func TestP5RelayLimits(t *testing.T) {
	cfg := testConfig()
	cfg.MaxConnsPerIP, cfg.MaxRoomsPerIP, cfg.MaxRooms = 3, 2, 3
	_, addr := startServerCfg(t, cfg)
	mustWelcome(t, addr, p5{name: "A", room: "r1"})
	mustWelcome(t, addr, p5{name: "B", room: "r2"})
	r := mustReject(t, addr, p5{name: "C", room: "r3"}, "rooms_limit")
	if r.Args["per_ip"] != "1" {
		t.Fatalf("args %+v", r.Args)
	}
	mustWelcome(t, addr, p5{name: "C", room: "r1"})
	mustReject(t, addr, p5{name: "D", room: "r1"}, "too_many_connections")
}

func TestP5HostRoomsOnly(t *testing.T) {
	s, addr := startServer(t)
	s.SetAllowedRooms([]string{"casa"})
	if err := s.SetFixedRoomPassword("casa", "puerta"); err != nil {
		t.Fatal(err)
	}
	mustReject(t, addr, p5{name: "A", room: "otra"}, "room_not_found")
	mustReject(t, addr, p5{name: "A", room: "casa"}, "password_required")
	a, w := mustWelcome(t, addr, p5{name: "A", room: "casa", password: "puerta", key: newPlayerKey()})
	if w.RoomOwner {
		t.Fatal("nobody owns a room the server's owner configured")
	}
	a.send(t, protocol.RoomSetKey{T: "ROOM_SET_KEY"})
	if r := decode[protocol.RoomSetKeyResult](t, a.expect(t, "ROOM_SET_KEY_RESULT")); r.OK || r.Reason != "fixed" {
		t.Fatalf("fixed room: %+v", r)
	}
	// The host changes it from its page.
	if err := s.SetFixedRoomPassword("casa", "nueva"); err != nil {
		t.Fatal(err)
	}
	mustReject(t, addr, p5{name: "B", room: "casa", password: "puerta"}, "bad_password")
	mustWelcome(t, addr, p5{name: "B", room: "casa", password: "nueva"})
}

// TestOldProtocolsGetNoNewFields: a protocol 2 or 4 client of a server that speaks 5 sees the messages it always saw.
func TestIsPublicAddress(t *testing.T) {
	for a, want := range map[string]bool{"203.0.113.7": true, "2001:4860::1": true, "127.0.0.1": false, "::1": false,
		"192.168.1.5": false, "10.2.3.4": false, "100.64.1.1": false, "169.254.0.1": false, "fd12::1": false, "0.0.0.0": false} {
		if got := IsPublicAddress(netip.MustParseAddr(a)); got != want {
			t.Errorf("IsPublicAddress(%s) = %v", a, got)
		}
	}
}

func TestOldProtocolsGetNoNewFields(t *testing.T) {
	_, addr := startServer(t)
	for _, proto := range []int{2, 3, 4} {
		h := hello("P", "old"+itoa(proto), "b")
		h.Proto = proto
		c := dial(t, addr, h)
		body := c.expect(t, "WELCOME")
		var m map[string]any
		if err := msgpack.Unmarshal(body, &m); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"room_password", "room_owner", "room_confirm", "server_confirm", "server_name", "max_players"} {
			if _, ok := m[k]; ok {
				t.Fatalf("protocol %d WELCOME carries %s", proto, k)
			}
		}
		for _, p := range m["players"].([]any) {
			if _, ok := p.(map[string]any)["color"]; ok {
				t.Fatalf("protocol %d player carries color", proto)
			}
		}
		if proto >= 3 {
			c.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: 85, Entrance: 1, InPlay: true, Shared: sharedGame(0, 0), SharedLayout: 7})
		} else {
			c.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: 85, InPlay: true})
		}
		c.expect(t, "GROUP_FOUND")
		// a bad build still gets its Spanish detail and no args
		bad := dial(t, addr, func() protocol.Hello { h := hello("Q", "old"+itoa(proto), "zz"); h.Proto = proto; return h }())
		r := decode[protocol.Reject](t, bad.expect(t, "REJECT"))
		if r.Reason != "build_mismatch" || r.Args != nil || !bytes.Contains([]byte(r.Detail), []byte("version del juego")) {
			t.Fatalf("protocol %d reject %+v", proto, r)
		}
	}
}

// TestP5OverWebSocket: the browser goes through the same challenge.
func TestP5OverWebSocket(t *testing.T) {
	_, addr := startServer(t)
	mustWelcome(t, addr, p5{name: "A", room: "web", password: "deku"})
	h := p5{name: "W", room: "web", password: "deku", engine: "wasm"}.hello()
	h.Engine = ""
	w := dialWS(t, addr, h)
	defer w.close()
	ch := decode[protocol.Challenge](t, w.expect(t, "CHALLENGE"))
	if !ch.Room || ch.Create || ch.Server {
		t.Fatalf("challenge %+v", ch)
	}
	w.send(t, protocol.Auth{T: "AUTH", RoomProof: roomauth.Proof(roomauth.RoomKey("web", "deku"), "room", ch.Nonce, h.AuthNonce, "web", "W")})
	if wl := decode[protocol.Welcome](t, w.expect(t, "WELCOME")); !wl.RoomPassword || wl.Proto != 5 {
		t.Fatalf("welcome %+v", wl)
	}
}

func TestP5AuthTimeoutAndBadAnswer(t *testing.T) {
	cfg := testConfig()
	cfg.HelloTimeout = 300 * time.Millisecond
	_, addr := startServerCfg(t, cfg)
	mustWelcome(t, addr, p5{name: "A", room: "r", password: "x"})
	c := dial(t, addr, p5{name: "B", room: "r", password: "x"}.hello())
	c.expect(t, "CHALLENGE")
	if r := decode[protocol.Reject](t, c.expect(t, "REJECT")); r.Reason != "auth_timeout" {
		t.Fatalf("%+v", r)
	}
	d := dial(t, addr, p5{name: "B", room: "r", password: "x"}.hello())
	d.expect(t, "CHALLENGE")
	d.send(t, protocol.Ping{T: "PING"})
	if r := decode[protocol.Reject](t, d.expect(t, "REJECT")); r.Reason != "bad_hello" {
		t.Fatalf("%+v", r)
	}
	h := p5{name: "C", room: "r"}.hello()
	h.AuthNonce = nil
	e := dial(t, addr, h)
	if r := decode[protocol.Reject](t, e.expect(t, "REJECT")); r.Reason != "bad_hello" {
		t.Fatalf("%+v", r)
	}
}

// TestP5RelayTwoClientsByInvite: a host makes its room on a relay with a password, its code carries the relay's
// address, and a friend gets in with the code alone.
func TestP5RelayTwoClientsByInvite(t *testing.T) {
	cfg := testConfig()
	cfg.MaxRooms, cfg.MaxConnsPerIP, cfg.MaxRoomsPerIP, cfg.RoomKeyTTL = 100, 8, 3, 7*24*time.Hour
	_, addr := startServerCfg(t, cfg)
	host, port, _ := net.SplitHostPort(addr)
	mustWelcome(t, addr, p5{name: "Anfitrion", room: "relevo-k3f9", password: "tingle42", key: newPlayerKey()})
	n, _ := strconv.Atoi(port)
	code, err := invite.Format(invite.Invite{Addresses: []string{host}, Port: n, Room: "relevo-k3f9", Password: "tingle42"})
	if err != nil {
		t.Fatal(err)
	}
	inv, err := invite.Parse(code)
	if err != nil {
		t.Fatal(err)
	}
	friendAddr := net.JoinHostPort(inv.Addresses[0], strconv.Itoa(inv.Port))
	_, w := mustWelcome(t, friendAddr, p5{name: "Amigo", room: inv.Room, password: inv.Password, key: newPlayerKey()})
	if !w.RoomPassword || len(w.Players) != 2 {
		t.Fatalf("friend's welcome %+v", w)
	}
	mustReject(t, friendAddr, p5{name: "Extrano", room: inv.Room, key: newPlayerKey()}, "password_required")
}
