package server

import (
	"bytes"
	"io"
	"log"
	"net"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/S-BlackDragon/zmp-oot64/server/internal/protocol"
)

// testConfig shortens every timer so the lockstep tests run in well under a second each.
func testConfig() Config {
	logger := log.New(io.Discard, "", 0)
	if os.Getenv("ZMP_TEST_LOG") != "" {
		logger = log.New(os.Stderr, "server: ", log.Lmicroseconds)
	}
	return Config{
		Logger:             logger,
		HelloTimeout:       2 * time.Second,
		WaitAfter:          50 * time.Millisecond,
		WaitRepeat:         100 * time.Millisecond,
		ZeroFillAfter:      500 * time.Millisecond,
		NameTakeoverAfter:  300 * time.Millisecond,
		PingInterval:       time.Hour,
		DelayInterval:      time.Hour,
		PlayerListInterval: time.Hour,
		StatsInterval:      time.Hour,
		HousekeepInterval:  5 * time.Millisecond,
	}
}

func startServerCfg(t *testing.T, cfg Config) (*Server, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg.DataDir = t.TempDir()
	s := New(cfg)
	if err := s.PrepareDataDir(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve(ln) }()
	t.Cleanup(s.Close)
	return s, ln.Addr().String()
}

func startServer(t *testing.T) (*Server, string) {
	return startServerCfg(t, testConfig())
}

func hello(name, room, build string) protocol.Hello {
	return protocol.Hello{T: "HELLO", Proto: 2, BuildHash: build, OotHash: "oot", SohHash: "soh",
		CvarProfileHash: "none", Name: name, Room: room}
}

type msg struct {
	typ  string
	body []byte
}

// testClient is a fake game client: a reader goroutine turns frames into a channel so
// tests can wait with timeouts without corrupting the stream.
type testClient struct {
	conn net.Conn
	ch   chan msg
}

func dial(t *testing.T, addr string, h protocol.Hello) *testClient {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	if err := protocol.WriteFrame(c, h); err != nil {
		t.Fatal(err)
	}
	tc := &testClient{conn: c, ch: make(chan msg, 1024)}
	go func() {
		defer close(tc.ch)
		for {
			body, err := protocol.ReadFrame(c)
			if err != nil {
				return
			}
			typ, err := protocol.Type(body)
			if err != nil {
				return
			}
			tc.ch <- msg{typ, body}
		}
	}()
	return tc
}

func (c *testClient) next(t *testing.T) (string, []byte) {
	t.Helper()
	select {
	case m, ok := <-c.ch:
		if !ok {
			t.Fatal("connection closed")
		}
		return m.typ, m.body
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for a message")
	}
	return "", nil
}

var noise = map[string]bool{"PING": true, "PONG": true, "PLAYER_LIST": true, "WAIT": true}

// expect returns the next message of type typ, skipping noise (PING, PLAYER_LIST, WAIT
// unless asked for). Any other message type in between fails the test.
func (c *testClient) expect(t *testing.T, typ string) []byte {
	t.Helper()
	for {
		got, body := c.next(t)
		if got == typ {
			return body
		}
		if !noise[got] {
			t.Fatalf("expected %s, got %s", typ, got)
		}
	}
}

// expectNone fails if a message of type typ arrives within d.
func (c *testClient) expectNone(t *testing.T, typ string, d time.Duration) {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case m, ok := <-c.ch:
			if !ok {
				return
			}
			if m.typ == typ {
				t.Fatalf("unexpected %s", typ)
			}
		case <-deadline:
			return
		}
	}
}

// expectClosed waits until the server closes the connection.
func (c *testClient) expectClosed(t *testing.T) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case _, ok := <-c.ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("connection not closed")
		}
	}
}

func (c *testClient) send(t *testing.T, v any) {
	t.Helper()
	if err := protocol.WriteFrame(c.conn, v); err != nil {
		t.Fatal(err)
	}
}

func decode[T any](t *testing.T, body []byte) T {
	t.Helper()
	var v T
	if err := protocol.Decode(body, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func pad(b byte) []byte { return []byte{b, 0, 0, 0, 0, 0, 0, 0} }

func (c *testClient) input(t *testing.T, tick int64, b byte) {
	t.Helper()
	c.send(t, protocol.Input{T: "INPUT", Tick: tick, Pad: pad(b)})
}

func (c *testClient) expectTick(t *testing.T, tick int64) protocol.Tick {
	t.Helper()
	m := decode[protocol.Tick](t, c.expect(t, "TICK"))
	if m.Tick != tick {
		t.Fatalf("expected TICK %d, got %d", tick, m.Tick)
	}
	if len(m.Slots) != len(m.Pads) {
		t.Fatalf("slots/pads length mismatch %+v", m)
	}
	return m
}

func padOf(t *testing.T, m protocol.Tick, slot int) []byte {
	t.Helper()
	for i, s := range m.Slots {
		if s == slot {
			return m.Pads[i]
		}
	}
	t.Fatalf("slot %d not in TICK %d (%v)", slot, m.Tick, m.Slots)
	return nil
}

func welcome(t *testing.T, addr, name string) *testClient {
	t.Helper()
	c := dial(t, addr, hello(name, "room", "b"))
	c.expect(t, "WELCOME")
	return c
}

func found(t *testing.T, c *testClient) protocol.GroupFound {
	t.Helper()
	c.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: 85, Entrance: 0, InPlay: true})
	return decode[protocol.GroupFound](t, c.expect(t, "GROUP_FOUND"))
}

// join makes c enter the group and completes the state transfer from leader.
func join(t *testing.T, c, leader *testClient) protocol.GroupJoinPending {
	t.Helper()
	c.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: 85, Entrance: 0})
	p := decode[protocol.GroupJoinPending](t, c.expect(t, "GROUP_JOIN_PENDING"))
	req := decode[protocol.StateBlobRequest](t, leader.expect(t, "STATE_BLOB_REQUEST"))
	if req.Tick != p.SpawnTick || req.ForSlot != p.Slot || req.GroupID != p.GroupID {
		t.Fatalf("request %+v does not match pending %+v", req, p)
	}
	leader.send(t, protocol.StateBlob{T: "STATE_BLOB", GroupID: req.GroupID, Tick: req.Tick, ForSlot: req.ForSlot,
		Data: []byte("state"), Hash: 1})
	blob := decode[protocol.StateBlob](t, c.expect(t, "STATE_BLOB"))
	if blob.Tick != p.SpawnTick || string(blob.Data) != "state" {
		t.Fatalf("bad blob %+v", blob)
	}
	return p
}

// pair returns a (slot 0, leader) and b (slot 1) in one group, with no tick emitted yet.
func pair(t *testing.T, addr string) (*testClient, *testClient) {
	t.Helper()
	a := welcome(t, addr, "A")
	found(t, a)
	b := welcome(t, addr, "B")
	p := join(t, b, a)
	if p.Slot != 1 || p.SpawnTick != 0 || p.Leader != 0 || p.Rejoin {
		t.Fatalf("bad pending %+v", p)
	}
	return a, b
}

// both makes a and b send input for ticks [from, to) and drains the resulting TICKs.
func both(t *testing.T, a, b *testClient, from, to int64) {
	t.Helper()
	for tick := from; tick < to; tick++ {
		a.input(t, tick, 1)
		b.input(t, tick, 2)
	}
	for tick := from; tick < to; tick++ {
		a.expectTick(t, tick)
		b.expectTick(t, tick)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func playerState(s *Server, name string) protocol.PlayerInfo {
	for _, r := range s.Rooms() {
		for _, p := range r.Players {
			if p.Name == name {
				return p
			}
		}
	}
	return protocol.PlayerInfo{Slot: -2}
}

// ---------------------------------------------------------------------------------------
// Handshake (phase 0 tests adapted to protocol 2)

func TestWelcomeAndPlayerList(t *testing.T) {
	_, addr := startServer(t)
	a := dial(t, addr, hello("P1", "room", "b1"))
	typ, body := a.next(t)
	if typ != "WELCOME" {
		t.Fatalf("got %s", typ)
	}
	w := decode[protocol.Welcome](t, body)
	if len(w.Players) != 1 || w.Players[0].Name != "P1" || w.Proto != 2 || w.Rejoin {
		t.Fatalf("bad welcome %+v", w)
	}
	b := dial(t, addr, hello("P2", "room", "b1"))
	if typ, _ := b.next(t); typ != "WELCOME" {
		t.Fatalf("P2 got %s", typ)
	}
	for {
		pl := decode[protocol.PlayerList](t, a.expect(t, "PLAYER_LIST"))
		if len(pl.Players) == 2 {
			for _, p := range pl.Players {
				if p.Slot != -1 || p.State != "lobby" || p.Leader {
					t.Fatalf("bad lobby entry %+v", p)
				}
			}
			if pl.Delay != InitialDelay || pl.GroupTick != -1 {
				t.Fatalf("bad list %+v", pl)
			}
			break
		}
	}
}

func expectReject(t *testing.T, c *testClient, reason string) {
	t.Helper()
	typ, body := c.next(t)
	if typ != "REJECT" {
		t.Fatalf("expected REJECT, got %s", typ)
	}
	r := decode[protocol.Reject](t, body)
	if r.Reason != reason || r.Detail == "" {
		t.Fatalf("expected reason %s, got %+v", reason, r)
	}
}

func TestRejectBuildMismatch(t *testing.T) {
	_, addr := startServer(t)
	a := dial(t, addr, hello("P1", "room", "good"))
	if typ, _ := a.next(t); typ != "WELCOME" {
		t.Fatal(typ)
	}
	expectReject(t, dial(t, addr, hello("P2", "room", "bad")), "build_mismatch")
	// Another room has its own reference.
	if typ, _ := dial(t, addr, hello("P2", "other", "bad")).next(t); typ != "WELCOME" {
		t.Fatal(typ)
	}
}

func TestRejectAssetsAndProtocol(t *testing.T) {
	_, addr := startServer(t)
	a := dial(t, addr, hello("P1", "room", "b"))
	a.next(t)
	h := hello("P2", "room", "b")
	h.OotHash = "other"
	expectReject(t, dial(t, addr, h), "oot_mismatch")
	h = hello("P2", "room", "b")
	h.SohHash = "other"
	expectReject(t, dial(t, addr, h), "soh_mismatch")
	h = hello("P2", "room", "b")
	h.CvarProfileHash = "other"
	expectReject(t, dial(t, addr, h), "cvar_profile_mismatch")
	h = hello("P2", "room", "b")
	h.RandoSeed = 5
	expectReject(t, dial(t, addr, h), "rando_seed_mismatch")
	h = hello("P3", "room", "b")
	h.Proto = 1
	expectReject(t, dial(t, addr, h), "protocol_mismatch")
	expectReject(t, dial(t, addr, hello("", "room", "b")), "bad_name")
	expectReject(t, dial(t, addr, hello("P4", "", "b")), "bad_room")
	expectReject(t, dial(t, addr, hello("P1", "room", "b")), "name_taken")
}

func TestRoomFull(t *testing.T) {
	_, addr := startServer(t)
	for i := 0; i < MaxPlayersPerRoom; i++ {
		c := dial(t, addr, hello(string(rune('A'+i)), "room", "b"))
		if typ, _ := c.next(t); typ != "WELCOME" {
			t.Fatal(typ)
		}
	}
	expectReject(t, dial(t, addr, hello("Z", "room", "b")), "room_full")
}

func TestPingPong(t *testing.T) {
	_, addr := startServer(t)
	a := welcome(t, addr, "P1")
	a.send(t, protocol.Ping{T: "PING", Nonce: 7, SentAt: 123})
	p := decode[protocol.Ping](t, a.expect(t, "PONG"))
	if p.Nonce != 7 || p.SentAt != 123 {
		t.Fatalf("bad pong %+v", p)
	}
}

func TestUnknownTypeIgnored(t *testing.T) {
	_, addr := startServer(t)
	a := welcome(t, addr, "P1")
	a.send(t, map[string]any{"t": "SOMETHING_NEW", "x": 1})
	a.send(t, protocol.Ping{T: "PING", Nonce: 8})
	if p := decode[protocol.Ping](t, a.expect(t, "PONG")); p.Nonce != 8 {
		t.Fatalf("bad pong %+v", p)
	}
}

func TestRoomReleasedWhenEmpty(t *testing.T) {
	s, addr := startServer(t)
	a := dial(t, addr, hello("P1", "room", "old"))
	a.next(t)
	a.conn.Close()
	waitFor(t, "room release", func() bool { return len(s.Rooms()) == 0 })
	// A new build can now found the room.
	if typ, _ := dial(t, addr, hello("P1", "room", "new")).next(t); typ != "WELCOME" {
		t.Fatal(typ)
	}
}

func TestBadHello(t *testing.T) {
	_, addr := startServer(t)
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = protocol.WriteFrame(c, protocol.Ping{T: "PING"})
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	body, err := protocol.ReadFrame(c)
	if err != nil {
		t.Fatal(err)
	}
	if r := decode[protocol.Reject](t, body); r.Reason != "bad_hello" {
		t.Fatalf("got %+v", r)
	}
}

// ---------------------------------------------------------------------------------------
// Lockstep

func TestFoundAndJoinWithBlobRelay(t *testing.T) {
	_, addr := startServer(t)
	a := welcome(t, addr, "A")
	gf := found(t, a)
	if gf.Slot != 0 || gf.Tick0 != 0 || gf.Delay != InitialDelay || gf.GroupID == 0 {
		t.Fatalf("bad GROUP_FOUND %+v", gf)
	}
	a.input(t, 0, 1)
	m := a.expectTick(t, 0)
	if !reflect.DeepEqual(m.Slots, []int{0}) || !bytes.Equal(m.Pads[0], pad(1)) || len(m.Events) != 0 || m.Delay != 2 {
		t.Fatalf("bad TICK 0 %+v", m)
	}
	a.input(t, 1, 1)
	a.expectTick(t, 1)

	b := welcome(t, addr, "B")
	b.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: 85})
	p := decode[protocol.GroupJoinPending](t, b.expect(t, "GROUP_JOIN_PENDING"))
	if p.GroupID != gf.GroupID || p.Slot != 1 || p.Leader != 0 || p.SpawnTick != 2 || p.Rejoin {
		t.Fatalf("bad pending %+v", p)
	}
	// The request reaches the leader before TICK 2 can exist.
	req := decode[protocol.StateBlobRequest](t, a.expect(t, "STATE_BLOB_REQUEST"))
	if req.Tick != 2 || req.ForSlot != 1 || req.GroupID != gf.GroupID {
		t.Fatalf("bad request %+v", req)
	}
	a.input(t, 2, 1)
	a.expectNone(t, "TICK", 150*time.Millisecond) // B's input for T is missing
	b.input(t, 2, 2)
	for _, c := range []*testClient{a, b} {
		m := c.expectTick(t, 2)
		if !reflect.DeepEqual(m.Slots, []int{0, 1}) || !bytes.Equal(padOf(t, m, 1), pad(2)) ||
			!reflect.DeepEqual(m.Events, []protocol.Event{{K: "SPAWN", Slot: 1}}) {
			t.Fatalf("bad TICK 2 %+v", m)
		}
	}
	const bigHash = uint64(0xdeadbeefcafef00d)
	data := bytes.Repeat([]byte{0xAB, 0x00, 0xFF}, 100000)
	a.send(t, protocol.StateBlob{T: "STATE_BLOB", GroupID: gf.GroupID, Tick: 2, ForSlot: 1, Data: data, Hash: bigHash})
	blob := decode[protocol.StateBlob](t, b.expect(t, "STATE_BLOB"))
	if blob.Tick != 2 || blob.ForSlot != 1 || blob.Hash != bigHash || !bytes.Equal(blob.Data, data) {
		t.Fatalf("bad relayed blob tick=%d slot=%d hash=%x len=%d", blob.Tick, blob.ForSlot, blob.Hash, len(blob.Data))
	}
	// A second blob for the same request is not expected any more: dropped.
	a.send(t, protocol.StateBlob{T: "STATE_BLOB", GroupID: gf.GroupID, Tick: 2, ForSlot: 1, Data: []byte{1}})
	b.expectNone(t, "STATE_BLOB", 100*time.Millisecond)
}

func TestGroupWaitThenServedOnFounding(t *testing.T) {
	_, addr := startServer(t)
	a := welcome(t, addr, "A")
	a.send(t, protocol.JoinGroup{T: "JOIN_GROUP", InPlay: false})
	if w := decode[protocol.GroupWait](t, a.expect(t, "GROUP_WAIT")); w.Reason != "no_group" {
		t.Fatalf("bad wait %+v", w)
	}
	b := welcome(t, addr, "B")
	found(t, b)
	p := decode[protocol.GroupJoinPending](t, a.expect(t, "GROUP_JOIN_PENDING"))
	if p.Slot != 1 || p.SpawnTick != 0 || p.Leader != 0 {
		t.Fatalf("bad pending %+v", p)
	}
	req := decode[protocol.StateBlobRequest](t, b.expect(t, "STATE_BLOB_REQUEST"))
	if req.Tick != 0 || req.ForSlot != 1 {
		t.Fatalf("bad request %+v", req)
	}
	a.input(t, 0, 1)
	b.input(t, 0, 2)
	m := b.expectTick(t, 0)
	if !reflect.DeepEqual(m.Slots, []int{0, 1}) || !reflect.DeepEqual(m.Events, []protocol.Event{{K: "SPAWN", Slot: 1}}) {
		t.Fatalf("bad TICK 0 %+v", m)
	}
}

func TestWaitThenFoundServesWaiters(t *testing.T) {
	_, addr := startServer(t)
	a := welcome(t, addr, "A")
	b := welcome(t, addr, "B")
	c := welcome(t, addr, "C")
	for _, x := range []*testClient{a, b, c} {
		x.send(t, protocol.JoinGroup{T: "JOIN_GROUP", InPlay: false})
		if w := decode[protocol.GroupWait](t, x.expect(t, "GROUP_WAIT")); w.Reason != "no_group" {
			t.Fatalf("bad wait %+v", w)
		}
	}
	// A loads a game and asks again: it founds, and B and C are served at once.
	gf := found(t, a)
	reqs := map[int]protocol.StateBlobRequest{}
	for i := 0; i < 2; i++ {
		r := decode[protocol.StateBlobRequest](t, a.expect(t, "STATE_BLOB_REQUEST"))
		reqs[r.ForSlot] = r
	}
	for i, x := range []*testClient{b, c} {
		p := decode[protocol.GroupJoinPending](t, x.expect(t, "GROUP_JOIN_PENDING"))
		if p.GroupID != gf.GroupID || p.Slot != i+1 || p.SpawnTick != 0 || p.Leader != 0 || p.Rejoin {
			t.Fatalf("bad pending %+v", p)
		}
		if r, ok := reqs[p.Slot]; !ok || r.Tick != 0 {
			t.Fatalf("missing request for slot %d: %+v", p.Slot, reqs)
		}
	}
	// A duplicate JOIN_GROUP from a member is ignored.
	b.send(t, protocol.JoinGroup{T: "JOIN_GROUP", InPlay: true})
	b.expectNone(t, "GROUP_JOIN_PENDING", 100*time.Millisecond)
	a.expectNone(t, "STATE_BLOB_REQUEST", 10*time.Millisecond)
	pl := decode[protocol.PlayerList](t, c.expect(t, "PLAYER_LIST"))
	slots := map[string]int{}
	for _, p := range pl.Players {
		slots[p.Name] = p.Slot
	}
	if !reflect.DeepEqual(slots, map[string]int{"A": 0, "B": 1, "C": 2}) {
		t.Fatalf("bad slots in PLAYER_LIST %+v", pl.Players)
	}
	for _, x := range []*testClient{a, b, c} {
		x.input(t, 0, 1)
	}
	m := a.expectTick(t, 0)
	if !reflect.DeepEqual(m.Events, []protocol.Event{{K: "SPAWN", Slot: 1}, {K: "SPAWN", Slot: 2}}) {
		t.Fatalf("bad events %+v", m.Events)
	}
}

func TestBundleOrderAndDuplicates(t *testing.T) {
	_, addr := startServer(t)
	a, b := pair(t, addr)
	// B runs ahead; A sends its inputs in reverse order. Nothing is emitted until tick 0 is
	// complete, then 0..3 come out in order.
	for tick := int64(0); tick < 4; tick++ {
		b.input(t, tick, byte(20+tick))
	}
	for tick := int64(3); tick > 0; tick-- {
		a.input(t, tick, byte(10+tick))
	}
	a.expectNone(t, "TICK", 150*time.Millisecond)
	a.input(t, 0, 10)
	for tick := int64(0); tick < 4; tick++ {
		for _, c := range []*testClient{a, b} {
			m := c.expectTick(t, tick)
			if !reflect.DeepEqual(m.Slots, []int{0, 1}) || !bytes.Equal(m.Pads[0], pad(byte(10+tick))) ||
				!bytes.Equal(m.Pads[1], pad(byte(20+tick))) {
				t.Fatalf("bad TICK %+v", m)
			}
			if tick == 0 && !reflect.DeepEqual(m.Events, []protocol.Event{{K: "SPAWN", Slot: 1}}) {
				t.Fatalf("expected SPAWN in TICK 0: %+v", m.Events)
			}
			if tick > 0 && len(m.Events) != 0 {
				t.Fatalf("unexpected events %+v", m.Events)
			}
		}
	}
	// Duplicates: the first input wins; inputs for emitted ticks are ignored.
	a.input(t, 4, 9)
	a.input(t, 4, 7)
	a.input(t, 2, 7)
	b.input(t, 4, 5)
	b.input(t, 4, 6)
	m := a.expectTick(t, 4)
	if !bytes.Equal(m.Pads[0], pad(9)) || !bytes.Equal(m.Pads[1], pad(5)) {
		t.Fatalf("duplicate handling: %+v", m)
	}
	b.expectTick(t, 4)
	a.expectNone(t, "TICK", 100*time.Millisecond)
	// A client that raised D sends several ticks ahead at once; still only complete ticks go out.
	for tick := int64(5); tick < 9; tick++ {
		a.input(t, tick, 1)
	}
	b.input(t, 5, 2)
	a.expectTick(t, 5)
	a.expectNone(t, "TICK", 100*time.Millisecond)
	// Malformed pads are ignored.
	b.send(t, protocol.Input{T: "INPUT", Tick: 6, Pad: []byte{1, 2, 3}})
	a.expectNone(t, "TICK", 100*time.Millisecond)
	b.input(t, 6, 2)
	a.expectTick(t, 6)
}

func TestWaitThenZeroFillThenRecovery(t *testing.T) {
	s, addr := startServer(t)
	cfg := testConfig()
	a, b := pair(t, addr)
	both(t, a, b, 0, 2)
	start := time.Now()
	a.input(t, 2, 1)
	w := decode[protocol.Wait](t, a.expect(t, "WAIT"))
	if w.Tick != 2 || !reflect.DeepEqual(w.Missing, []string{"B"}) || w.Ms < cfg.WaitAfter.Milliseconds() {
		t.Fatalf("bad WAIT %+v", w)
	}
	if el := time.Since(start); el < cfg.WaitAfter-10*time.Millisecond {
		t.Fatalf("WAIT too early (%v)", el)
	}
	w2 := decode[protocol.Wait](t, a.expect(t, "WAIT")) // repeated while blocked
	if w2.Ms <= w.Ms {
		t.Fatalf("WAIT not repeated with a growing ms: %+v then %+v", w, w2)
	}
	m := a.expectTick(t, 2)
	if el := time.Since(start); el < cfg.ZeroFillAfter-20*time.Millisecond {
		t.Fatalf("zero-fill too early (%v)", el)
	}
	if !bytes.Equal(padOf(t, m, 1), make([]byte, 8)) || !bytes.Equal(padOf(t, m, 0), pad(1)) {
		t.Fatalf("bad zero-filled TICK %+v", m)
	}
	waitFor(t, "B disconnected", func() bool { return playerState(s, "B").State == "disconnected" })
	// The group goes on at A's pace.
	a.input(t, 3, 1)
	a.expectTick(t, 3)
	a.input(t, 4, 1)
	a.expectTick(t, 4)

	// B (slow, still connected) catches up: its drained TICKs were emitted without it.
	for tick := int64(2); tick < 5; tick++ {
		b.expectTick(t, tick)
	}
	// Input for an unemitted tick takes B out of zero-fill: tick 5 needs both again.
	b.input(t, 5, 2)
	waitFor(t, "B playing", func() bool { return playerState(s, "B").State == "playing" })
	a.input(t, 5, 1)
	m = a.expectTick(t, 5)
	if !bytes.Equal(padOf(t, m, 1), pad(2)) {
		t.Fatalf("B not recovered: %+v", m)
	}
	a.input(t, 6, 1)
	a.expectNone(t, "TICK", 100*time.Millisecond)
	b.input(t, 6, 2)
	a.expectTick(t, 6)
}

func TestZeroFillRecoveryAhead(t *testing.T) {
	s, addr := startServer(t)
	a, b := pair(t, addr)
	both(t, a, b, 0, 1)
	a.input(t, 1, 1)
	a.expectTick(t, 1) // after zero-fill of B
	waitFor(t, "B disconnected", func() bool { return playerState(s, "B").State == "disconnected" })
	// B's first new input is for tick 4 while the next tick is 2: ticks 2 and 3 stay zeros.
	b.input(t, 4, 7)
	a.input(t, 2, 1)
	a.input(t, 3, 1)
	for tick := int64(2); tick < 4; tick++ {
		if m := a.expectTick(t, tick); !bytes.Equal(padOf(t, m, 1), make([]byte, 8)) {
			t.Fatalf("expected zeros for B at %d: %+v", tick, m)
		}
	}
	a.input(t, 4, 1)
	if m := a.expectTick(t, 4); !bytes.Equal(padOf(t, m, 1), pad(7)) {
		t.Fatalf("expected B's pad at 4: %+v", m)
	}
	a.input(t, 5, 1)
	a.expectNone(t, "TICK", 100*time.Millisecond)
	b.input(t, 5, 2)
	a.expectTick(t, 5)
}

func TestRejoinSameSlotWithoutSpawn(t *testing.T) {
	s, addr := startServer(t)
	a, b := pair(t, addr)
	both(t, a, b, 0, 3)
	b.conn.Close()
	waitFor(t, "B gone", func() bool {
		p := playerState(s, "B")
		return p.State == "disconnected" && p.Slot == 1
	})
	a.input(t, 3, 1)
	a.expectNone(t, "TICK", 100*time.Millisecond) // B still blocks (not yet zero-filled)

	b2 := dial(t, addr, hello("B", "room", "b"))
	if w := decode[protocol.Welcome](t, b2.expect(t, "WELCOME")); !w.Rejoin {
		t.Fatalf("expected rejoin in WELCOME: %+v", w)
	}
	b2.send(t, protocol.JoinGroup{T: "JOIN_GROUP"})
	p := decode[protocol.GroupJoinPending](t, b2.expect(t, "GROUP_JOIN_PENDING"))
	if p.Slot != 1 || !p.Rejoin || p.SpawnTick != 3 || p.Leader != 0 {
		t.Fatalf("bad rejoin pending %+v", p)
	}
	req := decode[protocol.StateBlobRequest](t, a.expect(t, "STATE_BLOB_REQUEST"))
	if req.Tick != 3 || req.ForSlot != 1 {
		t.Fatalf("bad request %+v", req)
	}
	b2.input(t, 3, 2)
	m := a.expectTick(t, 3)
	if len(m.Events) != 0 || !bytes.Equal(padOf(t, m, 1), pad(2)) {
		t.Fatalf("rejoin must not SPAWN: %+v", m)
	}
	b2.expectTick(t, 3)
}

func TestRejoinAfterZeroFill(t *testing.T) {
	s, addr := startServer(t)
	a, b := pair(t, addr)
	both(t, a, b, 0, 1)
	b.conn.Close()
	a.input(t, 1, 1)
	a.expectTick(t, 1) // zero-filled after the timeout
	waitFor(t, "B gone", func() bool { return playerState(s, "B").State == "disconnected" })
	a.input(t, 2, 1)
	a.expectTick(t, 2)
	b2 := welcome(t, addr, "B")
	b2.send(t, protocol.JoinGroup{T: "JOIN_GROUP"})
	p := decode[protocol.GroupJoinPending](t, b2.expect(t, "GROUP_JOIN_PENDING"))
	if p.SpawnTick != 3 || !p.Rejoin {
		t.Fatalf("bad pending %+v", p)
	}
	a.expect(t, "STATE_BLOB_REQUEST")
	a.input(t, 3, 1)
	a.expectNone(t, "TICK", 100*time.Millisecond) // out of zero-fill from T
	b2.input(t, 3, 2)
	a.expectTick(t, 3)
}

func TestLeaveGroupDespawn(t *testing.T) {
	s, addr := startServer(t)
	a, b := pair(t, addr)
	both(t, a, b, 0, 2)
	b.input(t, 2, 2) // pending input of a leaving slot is discarded
	b.send(t, protocol.LeaveGroup{T: "LEAVE_GROUP"})
	waitFor(t, "B in lobby", func() bool { return playerState(s, "B").State == "lobby" })
	a.input(t, 2, 1)
	m := a.expectTick(t, 2)
	if !reflect.DeepEqual(m.Slots, []int{0}) || !reflect.DeepEqual(m.Events, []protocol.Event{{K: "DESPAWN", Slot: 1}}) {
		t.Fatalf("bad TICK after leave %+v", m)
	}
	// B joins again: a fresh slot with SPAWN (the old one was freed).
	b.send(t, protocol.JoinGroup{T: "JOIN_GROUP"})
	p := decode[protocol.GroupJoinPending](t, b.expect(t, "GROUP_JOIN_PENDING"))
	if p.Rejoin || p.SpawnTick != 3 {
		t.Fatalf("bad pending %+v", p)
	}
	a.expect(t, "STATE_BLOB_REQUEST")
	a.input(t, 3, 1)
	b.input(t, 3, 2)
	if m := a.expectTick(t, 3); !reflect.DeepEqual(m.Events, []protocol.Event{{K: "SPAWN", Slot: p.Slot}}) {
		t.Fatalf("expected SPAWN: %+v", m)
	}
}

func TestHashMismatchResync(t *testing.T) {
	_, addr := startServer(t)
	a, b := pair(t, addr)
	both(t, a, b, 0, 22)
	// Equal hashes: nothing.
	a.send(t, protocol.Hash{T: "HASH", Tick: 0, Hash: 42})
	b.send(t, protocol.Hash{T: "HASH", Tick: 0, Hash: 42})
	b.expectNone(t, "RESYNC", 100*time.Millisecond)
	// Different hash (B before the leader, 64-bit values).
	b.send(t, protocol.Hash{T: "HASH", Tick: 20, Hash: 0xffffffffffff0002})
	a.send(t, protocol.Hash{T: "HASH", Tick: 20, Hash: 0xffffffffffff0001})
	for _, c := range []*testClient{a, b} {
		r := decode[protocol.Resync](t, c.expect(t, "RESYNC"))
		if r.BadTick != 20 || r.Tick != 22 || r.Leader != 0 || !reflect.DeepEqual(r.Targets, []int{1}) {
			t.Fatalf("bad RESYNC %+v", r)
		}
	}
	req := decode[protocol.StateBlobRequest](t, a.expect(t, "STATE_BLOB_REQUEST"))
	if req.Tick != 22 || req.ForSlot != 1 {
		t.Fatalf("bad request %+v", req)
	}
	// No second resync for B while one is in progress.
	a.send(t, protocol.Hash{T: "HASH", Tick: 21, Hash: 1})
	b.send(t, protocol.Hash{T: "HASH", Tick: 21, Hash: 2})
	b.expectNone(t, "RESYNC", 100*time.Millisecond)
	a.send(t, protocol.StateBlob{T: "STATE_BLOB", GroupID: req.GroupID, Tick: 22, ForSlot: 1, Data: []byte("s")})
	b.expect(t, "STATE_BLOB")
	// After the transfer B is compared again (hashes from T on).
	both(t, a, b, 22, 41)
	a.send(t, protocol.Hash{T: "HASH", Tick: 40, Hash: 1})
	b.send(t, protocol.Hash{T: "HASH", Tick: 40, Hash: 3})
	if r := decode[protocol.Resync](t, b.expect(t, "RESYNC")); r.BadTick != 40 || r.Tick != 41 {
		t.Fatalf("bad RESYNC %+v", r)
	}
}

func TestLeaderChangeOnDisconnect(t *testing.T) {
	s, addr := startServer(t)
	a, b := pair(t, addr)
	both(t, a, b, 0, 2)
	c := welcome(t, addr, "C")
	c.send(t, protocol.JoinGroup{T: "JOIN_GROUP"})
	p := decode[protocol.GroupJoinPending](t, c.expect(t, "GROUP_JOIN_PENDING"))
	if p.Slot != 2 || p.Leader != 0 || p.SpawnTick != 2 {
		t.Fatalf("bad pending %+v", p)
	}
	a.expect(t, "STATE_BLOB_REQUEST")
	// The leader dies before sending the blob: B (next oldest) gets the same request
	// (tick 2 not emitted yet: A's slot still blocks it).
	a.conn.Close()
	req := decode[protocol.StateBlobRequest](t, b.expect(t, "STATE_BLOB_REQUEST"))
	if req.Tick != 2 || req.ForSlot != 2 {
		t.Fatalf("bad reissued request %+v", req)
	}
	waitFor(t, "B leader", func() bool { return playerState(s, "B").Leader })
	if playerState(s, "C").State != "joining" || playerState(s, "A").State != "disconnected" {
		t.Fatalf("states: A=%+v C=%+v", playerState(s, "A"), playerState(s, "C"))
	}
	b.send(t, protocol.StateBlob{T: "STATE_BLOB", GroupID: req.GroupID, Tick: 2, ForSlot: 2, Data: []byte("x")})
	c.expect(t, "STATE_BLOB")
	waitFor(t, "C playing", func() bool { return playerState(s, "C").State == "playing" })
	// Hashes are now compared against B.
	b.input(t, 2, 2)
	c.input(t, 2, 3)
	b.expectTick(t, 2) // A zero-filled after the timeout
	c.expectTick(t, 2)
	b.send(t, protocol.Hash{T: "HASH", Tick: 2, Hash: 5})
	c.send(t, protocol.Hash{T: "HASH", Tick: 2, Hash: 6})
	if r := decode[protocol.Resync](t, c.expect(t, "RESYNC")); r.Leader != 1 || !reflect.DeepEqual(r.Targets, []int{2}) {
		t.Fatalf("bad RESYNC %+v", r)
	}
}

func TestLeaderChangeAfterRequestedTickEmitted(t *testing.T) {
	_, addr := startServer(t)
	a, b := pair(t, addr)
	c := welcome(t, addr, "C")
	c.send(t, protocol.JoinGroup{T: "JOIN_GROUP"})
	p := decode[protocol.GroupJoinPending](t, c.expect(t, "GROUP_JOIN_PENDING"))
	a.expect(t, "STATE_BLOB_REQUEST")
	for tick := p.SpawnTick; tick < p.SpawnTick+2; tick++ {
		a.input(t, tick, 1)
		b.input(t, tick, 2)
		c.input(t, tick, 3)
		b.expectTick(t, tick)
		c.expectTick(t, tick)
	}
	// A leaves the group (clean) before answering: the tick it was asked for is gone.
	a.send(t, protocol.LeaveGroup{T: "LEAVE_GROUP"})
	r := decode[protocol.Resync](t, c.expect(t, "RESYNC"))
	if r.BadTick != p.SpawnTick || r.Tick != p.SpawnTick+2 || r.Leader != 1 || !reflect.DeepEqual(r.Targets, []int{2}) {
		t.Fatalf("bad RESYNC %+v", r)
	}
	b.expect(t, "RESYNC") // the whole group gets it, before B's request
	req := decode[protocol.StateBlobRequest](t, b.expect(t, "STATE_BLOB_REQUEST"))
	if req.Tick != p.SpawnTick+2 || req.ForSlot != 2 {
		t.Fatalf("bad request %+v", req)
	}
}

func TestNameTakenVsTakeover(t *testing.T) {
	s, addr := startServer(t)
	a, b := pair(t, addr)
	_ = a
	// B just talked: its name is taken.
	b.input(t, 0, 2)
	expectReject(t, dial(t, addr, hello("B", "room", "b")), "name_taken")
	// B goes silent (no PONGs, no inputs) past NameTakeoverAfter: a new connection replaces it.
	time.Sleep(testConfig().NameTakeoverAfter + 50*time.Millisecond)
	b2 := dial(t, addr, hello("B", "room", "b"))
	w := decode[protocol.Welcome](t, b2.expect(t, "WELCOME"))
	if !w.Rejoin {
		t.Fatalf("expected rejoin %+v", w)
	}
	b.expectClosed(t)
	b2.send(t, protocol.JoinGroup{T: "JOIN_GROUP"})
	if p := decode[protocol.GroupJoinPending](t, b2.expect(t, "GROUP_JOIN_PENDING")); p.Slot != 1 || !p.Rejoin {
		t.Fatalf("bad pending %+v", p)
	}
	n := 0
	for _, r := range s.Rooms() {
		n += len(r.Players)
	}
	if n != 2 {
		t.Fatalf("expected 2 players after takeover, got %+v", s.Rooms())
	}
}

func TestTakeoverOfZeroFilledPlayer(t *testing.T) {
	s, addr := startServer(t)
	a, b := pair(t, addr)
	a.input(t, 0, 1)
	a.expectTick(t, 0) // B zero-filled
	waitFor(t, "B disconnected", func() bool { return playerState(s, "B").State == "disconnected" })
	b.send(t, protocol.Ping{T: "PING"}) // recent traffic does not protect a zero-filled slot
	b2 := dial(t, addr, hello("B", "room", "b"))
	b2.expect(t, "WELCOME")
	b.expectClosed(t)
}

func TestServerPingMeasuresRTT(t *testing.T) {
	cfg := testConfig()
	cfg.PingInterval = 20 * time.Millisecond
	cfg.PlayerListInterval = 50 * time.Millisecond
	cfg.DelayInterval = 100 * time.Millisecond
	_, addr := startServerCfg(t, cfg)
	a := welcome(t, addr, "A")
	a.send(t, protocol.JoinGroup{T: "JOIN_GROUP", InPlay: true})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		typ, body := a.next(t)
		switch typ {
		case "PING":
			p := decode[protocol.Ping](t, body)
			a.send(t, protocol.Ping{T: "PONG", Nonce: p.Nonce, SentAt: p.SentAt})
		case "PLAYER_LIST":
			pl := decode[protocol.PlayerList](t, body)
			if len(pl.Players) == 1 && pl.Players[0].RTT >= 0 && pl.Players[0].Leader && pl.Players[0].Slot == 0 {
				if pl.Delay < MinDelay || pl.Delay > MaxDelay {
					t.Fatalf("bad delay %d", pl.Delay)
				}
				return
			}
		}
	}
	t.Fatal("RTT never measured")
}

func TestComputeDelay(t *testing.T) {
	cases := []struct {
		rtt, jit float64
		want     int
	}{
		{0, 0, 1}, {1, 0, 1}, {20, 3, 1}, {60, 0, 2}, {60, 10, 2}, {100, 1, 3}, {180, 20, 5}, {400, 50, 6}, {5000, 0, 6},
	}
	for _, c := range cases {
		if got := ComputeDelay(c.rtt, c.jit); got != c.want {
			t.Errorf("ComputeDelay(%v, %v) = %d, want %d", c.rtt, c.jit, got, c.want)
		}
	}
}

func TestSendQueueOverflowDropsClient(t *testing.T) {
	cfg := testConfig()
	cfg.SendQueueSize = 4
	_, addr := startServerCfg(t, cfg)
	// A raw client that never reads: the kernel buffers fill, then the queue overflows.
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = protocol.WriteFrame(c, hello("A", "room", "b"))
	_ = protocol.WriteFrame(c, protocol.JoinGroup{T: "JOIN_GROUP", InPlay: true})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for n := uint64(0); n < 2000000; n++ { // every PING queues a PONG nobody reads
			if protocol.WriteFrame(c, protocol.Ping{T: "PING", Nonce: n}) != nil {
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("client was never dropped")
	}
}

// Phase 5b, point 19: the reject names both ROM versions; the same content under two names is accepted.
func TestRejectNamesBothRomVersions(t *testing.T) {
	_, addr := startServer(t)
	h := hello("P1", "room", "b")
	h.RomName = "PAL GameCube Debug"
	a := dial(t, addr, h)
	a.next(t)
	h = hello("P2", "room", "b")
	h.OotHash = "other"
	h.RomName = "NTSC N64 1.2"
	c := dial(t, addr, h)
	typ, body := c.next(t)
	if typ != "REJECT" {
		t.Fatalf("expected REJECT, got %s", typ)
	}
	r := decode[protocol.Reject](t, body)
	if r.Reason != "oot_mismatch" || !bytes.Contains([]byte(r.Detail), []byte("NTSC N64 1.2")) ||
		!bytes.Contains([]byte(r.Detail), []byte("PAL GameCube Debug")) {
		t.Fatalf("the reject does not name both versions: %+v", r)
	}
	h = hello("P3", "room", "b")
	h.RomName = "PAL GameCube" // same content hash, another name: equivalent versions play together
	if typ, _ := dial(t, addr, h).next(t); typ != "WELCOME" {
		t.Fatalf("same content under another version name was not accepted: %s", typ)
	}
}
