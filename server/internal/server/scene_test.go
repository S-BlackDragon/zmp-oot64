package server

// Protocol 3 (phase 5): one group per scene, canonical shared game, world clock.

import (
	"bytes"
	"encoding/binary"
	"log"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/S-BlackDragon/zmp-oot64/server/internal/protocol"
)

func hello3(name, room string) protocol.Hello {
	h := hello(name, room, "b")
	h.Proto = 4 // (the name stays from phase 5: scene groups; protocol 4 adds phase 5b's nobody-waits)
	return h
}

func welcome3(t *testing.T, addr, name string) *testClient {
	t.Helper()
	c := dial(t, addr, hello3(name, "room3"))
	w := decode[protocol.Welcome](t, c.expect(t, "WELCOME"))
	if w.Proto != 4 {
		t.Fatalf("welcome proto %d", w.Proto)
	}
	return c
}

// sharedGame is a fake 16-byte shared game: rupees at 0 (s16), flags at 4 (u32).
func sharedGame(rupees int16, flags uint32) []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint16(b, uint16(rupees))
	binary.LittleEndian.PutUint32(b[4:], flags)
	return b
}

func deltaEntry(off uint16, size byte, d int32) []byte {
	b := []byte{0, 0, size, PatchDelta}
	binary.LittleEndian.PutUint16(b, off)
	v := make([]byte, 4)
	binary.LittleEndian.PutUint32(v, uint32(d))
	return append(b, v[:size]...)
}

func bitsEntry(off uint16, size byte, set, clr uint32) []byte {
	b := []byte{0, 0, size, PatchBits}
	binary.LittleEndian.PutUint16(b, off)
	v := make([]byte, 8)
	binary.LittleEndian.PutUint32(v, set)
	binary.LittleEndian.PutUint32(v[4:], clr)
	return append(append(b, v[:size]...), v[4:4+size]...)
}

// settled returns when the server has handled everything this client sent so far (the messages of one connection
// are handled in order: the answer to a PING sent now comes after them). The tests with several connections use it
// where the order between connections matters.
func (c *testClient) settled(t *testing.T) {
	t.Helper()
	c.send(t, protocol.Ping{T: "PING", Nonce: 4242})
	for {
		p := decode[protocol.Ping](t, c.expect(t, "PONG"))
		if p.Nonce == 4242 {
			return
		}
	}
}

func found3(t *testing.T, c *testClient, scene int, shared []byte) protocol.GroupFound {
	t.Helper()
	c.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: scene, Entrance: 1, InPlay: true, Shared: shared,
		SharedLayout: 7})
	f := decode[protocol.GroupFound](t, c.expect(t, "GROUP_FOUND"))
	if f.Scene != scene {
		t.Fatalf("found scene %d, want %d", f.Scene, scene)
	}
	return f
}

// join3 enters the group of `scene` (the leader sends the state).
func join3(t *testing.T, c, leader *testClient, scene int, spawn string) protocol.GroupJoinPending {
	t.Helper()
	c.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: scene, Entrance: 2, Spawn: spawn})
	p := decode[protocol.GroupJoinPending](t, c.expect(t, "GROUP_JOIN_PENDING"))
	req := decode[protocol.StateBlobRequest](t, leader.expect(t, "STATE_BLOB_REQUEST"))
	leader.send(t, protocol.StateBlob{T: "STATE_BLOB", GroupID: req.GroupID, Tick: req.Tick, ForSlot: req.ForSlot,
		Data: []byte("state"), Hash: 1})
	c.expect(t, "STATE_BLOB")
	// Phase 5b: its Link appears at the tick of its first input that is in time (here the group's next tick).
	c.input(t, p.SpawnTick, 2)
	time.Sleep(30 * time.Millisecond)
	return p
}

// A player on its way into a group holds nobody: the group's ticks go on with the inputs of the ones playing, and
// the newcomer's Link appears at the tick of its first input that arrives in time.
func TestJoinDoesNotHoldTheGroup(t *testing.T) {
	_, addr := startServer(t)
	a := welcome3(t, addr, "A")
	found3(t, a, 85, sharedGame(10, 0))
	b := welcome3(t, addr, "B")
	b.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: 85, Entrance: 2, Spawn: "zmp_spawn 9"})
	p := decode[protocol.GroupJoinPending](t, b.expect(t, "GROUP_JOIN_PENDING"))
	req := decode[protocol.StateBlobRequest](t, a.expect(t, "STATE_BLOB_REQUEST"))
	// the group plays on before the state is even sent
	for tick := int64(0); tick < 5; tick++ {
		a.input(t, tick, 1)
		m := a.expectTick(t, tick)
		if len(m.Slots) != 1 || len(m.Events) != 0 {
			t.Fatalf("tick %+v", m)
		}
	}
	a.send(t, protocol.StateBlob{T: "STATE_BLOB", GroupID: req.GroupID, Tick: req.Tick, ForSlot: req.ForSlot,
		Data: []byte("state"), Hash: 1})
	for tick := int64(5); tick < 8; tick++ {
		a.input(t, tick, 1)
		a.expectTick(t, tick)
	}
	// B gets every tick from its state on, and catches up: inputs for ticks already out do nothing
	for tick := p.SpawnTick; tick < 8; tick++ {
		if tick == 5 {
			b.expect(t, "STATE_BLOB")
		}
		b.expectTick(t, tick)
		b.input(t, tick, 2)
	}
	b.input(t, 10, 2)
	time.Sleep(30 * time.Millisecond)
	for tick := int64(8); tick <= 10; tick++ {
		a.input(t, tick, 1)
		m := a.expectTick(t, tick)
		spawn := len(m.Events) == 1 && m.Events[0].K == "SPAWN" && m.Events[0].Slot == 1 && m.Events[0].Cmd == "zmp_spawn 9"
		if spawn != (tick == 10) {
			t.Fatalf("tick %d events %+v", tick, m.Events)
		}
	}
	// from its SPAWN on, B is a player like any other
	a.input(t, 11, 1)
	select {
	case <-time.After(50 * time.Millisecond):
	}
	b.input(t, 11, 2)
	m := a.expectTick(t, 11)
	if len(m.Slots) != 2 {
		t.Fatalf("tick %+v", m)
	}
}

// The inputs of a player on its way in that arrive after the group played their tick do nothing, and the server's
// log says how many there were when its Link appears (D-094: over a connection with latency every one of them was
// late for 9 seconds and nothing in any log said so).
func TestLateInputsOfAPlayerEnteringAreCounted(t *testing.T) {
	cfg := testConfig()
	var logged lockedBuffer
	cfg.Logger = log.New(&logged, "", 0)
	_, addr := startServerCfg(t, cfg)
	a := welcome3(t, addr, "A")
	found3(t, a, 85, sharedGame(10, 0))
	b := welcome3(t, addr, "B")
	b.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: 85, Entrance: 2, Spawn: "zmp_spawn 9"})
	decode[protocol.GroupJoinPending](t, b.expect(t, "GROUP_JOIN_PENDING"))
	req := decode[protocol.StateBlobRequest](t, a.expect(t, "STATE_BLOB_REQUEST"))
	a.send(t, protocol.StateBlob{T: "STATE_BLOB", GroupID: req.GroupID, Tick: req.Tick, ForSlot: req.ForSlot,
		Data: []byte("state"), Hash: 1})
	b.expect(t, "STATE_BLOB")
	// each input of B is for the tick the group has just played: late by one, three times
	for tick := int64(0); tick < 3; tick++ {
		a.input(t, tick, 1)
		if m := a.expectTick(t, tick); len(m.Slots) != 1 || len(m.Events) != 0 {
			t.Fatalf("tick %+v", m)
		}
		b.input(t, tick, 2)
	}
	b.input(t, 3, 2)
	time.Sleep(30 * time.Millisecond)
	a.input(t, 3, 1)
	if m := a.expectTick(t, 3); len(m.Events) != 1 || m.Events[0].K != "SPAWN" {
		t.Fatalf("tick %+v", m)
	}
	want := "caught up: SPAWN at 3 (head 3; 3 of its inputs arrived late, the last one by 1 ticks)"
	if !strings.Contains(logged.String(), want) {
		t.Fatalf("the log does not say %q: %s", want, logged.String())
	}
}

func TestApplyPatch(t *testing.T) {
	data := sharedGame(50, 0x10)
	patch := append(deltaEntry(0, 2, 5), bitsEntry(4, 4, 0x3, 0x10)...)
	if err := ApplyPatch(data, patch); err != nil {
		t.Fatal(err)
	}
	if got := int16(binary.LittleEndian.Uint16(data)); got != 55 {
		t.Fatalf("rupees %d", got)
	}
	if got := binary.LittleEndian.Uint32(data[4:]); got != 0x3 {
		t.Fatalf("flags %x", got)
	}
	// Negative delta, two groups earning and spending: both count.
	if err := ApplyPatch(data, deltaEntry(0, 2, -60)); err != nil {
		t.Fatal(err)
	}
	if got := int16(binary.LittleEndian.Uint16(data)); got != -5 {
		t.Fatalf("rupees %d", got)
	}
	for _, bad := range [][]byte{{1}, {0, 0, 3, 0}, {15, 0, 2, 1, 0, 0}, {0, 0, 1, 9, 0}, {0, 0, 4, 0, 1}} {
		if ApplyPatch(sharedGame(0, 0), bad) == nil {
			t.Fatalf("patch %v accepted", bad)
		}
	}
}

func TestSceneGroupsSplitAndPlayerList(t *testing.T) {
	_, addr := startServer(t)
	a := welcome3(t, addr, "A")
	found3(t, a, 85, sharedGame(10, 0))
	b := welcome3(t, addr, "B")
	p := join3(t, b, a, 85, "zmp_spawn 1")
	if p.Slot != 1 {
		t.Fatalf("B slot %d", p.Slot)
	}
	c := welcome3(t, addr, "C")
	fc := found3(t, c, 0, sharedGame(10, 0))
	if fc.Slot != 0 || !bytes.Equal(fc.Shared, sharedGame(10, 0)) {
		t.Fatalf("C founding %+v", fc)
	}
	// A's group gets the SPAWN of B with its spawn info; C's group is separate: its ticks never wait for A or B.
	a.input(t, 0, 1)
	b.input(t, 0, 2)
	m := a.expectTick(t, 0)
	if len(m.Events) != 1 || m.Events[0].K != "SPAWN" || m.Events[0].Slot != 1 || m.Events[0].Cmd != "zmp_spawn 1" {
		t.Fatalf("events %+v", m.Events)
	}
	if len(m.Slots) != 2 || m.DayTime < 0 {
		t.Fatalf("tick %+v", m)
	}
	c.input(t, 0, 3)
	mc := c.expectTick(t, 0)
	if len(mc.Slots) != 1 || mc.Slots[0] != 0 {
		t.Fatalf("C tick %+v", mc)
	}
	for {
		pl := decode[protocol.PlayerList](t, c.expect(t, "PLAYER_LIST"))
		scenes := map[string]int{}
		groups := map[string]uint32{}
		for _, pi := range pl.Players {
			scenes[pi.Name] = pi.Scene
			groups[pi.Name] = pi.Group
		}
		if scenes["A"] == 85 && scenes["B"] == 85 && scenes["C"] == 0 && groups["A"] == groups["B"] &&
			groups["A"] != groups["C"] && groups["C"] != 0 {
			break
		}
	}
}

func TestLeaveThenWaitThenFoundGetsCanonical(t *testing.T) {
	_, addr := startServer(t)
	a := welcome3(t, addr, "A")
	found3(t, a, 85, sharedGame(10, 1))
	b := welcome3(t, addr, "B")
	join3(t, b, a, 85, "")
	both(t, a, b, 0, 3)
	// B walks out of the scene: it leaves, then asks for the destination while it still loads it.
	b.send(t, protocol.LeaveGroup{T: "LEAVE_GROUP"})
	b.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: 0, Entrance: 0, InPlay: false})
	w := decode[protocol.GroupWait](t, b.expect(t, "GROUP_WAIT"))
	if w.Reason != "no_group" {
		t.Fatalf("wait %+v", w)
	}
	a.input(t, 3, 1)
	m := a.expectTick(t, 3)
	if len(m.Events) != 1 || m.Events[0].K != "DESPAWN" || m.Events[0].Slot != 1 || len(m.Slots) != 1 {
		t.Fatalf("tick %+v", m)
	}
	// A's group changes the shared game (rupees +5, a flag): the canonical follows.
	a.send(t, protocol.Shared{T: "SHARED", Tick: 3, Data: append(deltaEntry(0, 2, 5), bitsEntry(4, 4, 2, 0)...)})
	// (A's next tick comes back after the server took the patch: the messages of one connection are handled in
	// order. Without this, B's request, on another connection, could be handled first: the test failed once with
	// the PC busy.)
	a.input(t, 4, 1)
	a.expectTick(t, 4)
	// B arrived: it founds the destination's group and gets the canonical game to merge.
	f := found3(t, b, 0, sharedGame(10, 1))
	if !bytes.Equal(f.Shared, sharedGame(15, 3)) {
		t.Fatalf("canonical %v", f.Shared)
	}
}

// D-115: a player walks out of its group in the very tick the group changed the shared game (77 rupees). The members
// who stay send that tick's patch; the one the leaver brings with LEAVE_GROUP is dropped, or the rupees count twice
// (finding AL: 150 - 73 - 73 = 4 rupees for everybody).
func TestLeaveSharedDroppedWhenOthersStay(t *testing.T) {
	_, addr := startServer(t)
	a := dial(t, addr, hello3("A", "room3"))
	if w := decode[protocol.Welcome](t, a.expect(t, "WELCOME")); !w.LeaveShared {
		t.Fatalf("the server does not say it takes the shared changes of a leave: %+v", w)
	}
	found3(t, a, 85, sharedGame(150, 0))
	b := welcome3(t, addr, "B")
	join3(t, b, a, 85, "")
	both(t, a, b, 0, 3)
	// (the group's tick goes on after B's exit: A's patch of tick 3 also has a flag set later in that tick)
	b.send(t, protocol.LeaveGroup{T: "LEAVE_GROUP", Tick: 3, Shared: deltaEntry(0, 2, -73)})
	b.settled(t)
	a.send(t, protocol.Shared{T: "SHARED", Tick: 3, Data: append(deltaEntry(0, 2, -73), bitsEntry(4, 4, 2, 0)...)})
	a.settled(t)
	d := welcome3(t, addr, "D")
	if f := found3(t, d, 0, sharedGame(0, 0)); !bytes.Equal(f.Shared, sharedGame(77, 2)) {
		t.Fatalf("canonical %v, want 77 rupees and the flag", f.Shared)
	}
}

// D-115: the leaver was the last one in its group: nobody else sends its leaving tick, so the changes it brings with
// LEAVE_GROUP are that tick's patch (the canonical and the other groups get them once).
func TestLeaveSharedTakenWhenLastLeaves(t *testing.T) {
	_, addr := startServer(t)
	a := welcome3(t, addr, "A")
	found3(t, a, 85, sharedGame(10, 0))
	c := welcome3(t, addr, "C")
	found3(t, c, 0, sharedGame(10, 0))
	a.input(t, 0, 1)
	a.expectTick(t, 0)
	patch := append(deltaEntry(0, 2, 20), bitsEntry(4, 4, 2, 0)...)
	a.send(t, protocol.LeaveGroup{T: "LEAVE_GROUP", Tick: 1, Shared: patch})
	a.settled(t)
	c.input(t, 0, 1)
	if m := c.expectTick(t, 0); len(m.Events) != 1 || m.Events[0].K != "SHARED" || !bytes.Equal(m.Events[0].Data, patch) {
		t.Fatalf("C events %+v", m.Events)
	}
	d := welcome3(t, addr, "D")
	if f := found3(t, d, 5, sharedGame(0, 0)); !bytes.Equal(f.Shared, sharedGame(30, 2)) {
		t.Fatalf("canonical %v", f.Shared)
	}
}

func TestSharedPatchForwardedOnce(t *testing.T) {
	_, addr := startServer(t)
	a := welcome3(t, addr, "A")
	found3(t, a, 85, sharedGame(10, 0))
	b := welcome3(t, addr, "B")
	join3(t, b, a, 85, "")
	c := welcome3(t, addr, "C")
	found3(t, c, 0, sharedGame(10, 0))
	patch := deltaEntry(0, 2, 20)
	// every member of A's group sends the same patch for its tick 7; only the first counts
	a.send(t, protocol.Shared{T: "SHARED", Tick: 7, Data: patch})
	b.send(t, protocol.Shared{T: "SHARED", Tick: 7, Data: patch})
	a.settled(t) // (three connections: C's input must not be handled before the patches; it was, once, with the PC busy)
	b.settled(t)
	c.input(t, 0, 1)
	m := c.expectTick(t, 0)
	if len(m.Events) != 1 || m.Events[0].K != "SHARED" || !bytes.Equal(m.Events[0].Data, patch) {
		t.Fatalf("C events %+v", m.Events)
	}
	c.input(t, 1, 1)
	if m := c.expectTick(t, 1); len(m.Events) != 0 {
		t.Fatalf("duplicate forwarded: %+v", m.Events)
	}
	// the sender's group does not get its own patch back
	both(t, a, b, 0, 1)
	d := welcome3(t, addr, "D")
	fd := found3(t, d, 5, sharedGame(0, 0))
	if !bytes.Equal(fd.Shared, sharedGame(30, 0)) {
		t.Fatalf("canonical %v", fd.Shared)
	}
}

func TestWorldClock(t *testing.T) {
	_, addr := startServer(t)
	a := welcome3(t, addr, "A")
	f := found3(t, a, 81, sharedGame(0, 0))
	if f.DayTime != 0x8000 {
		t.Fatalf("initial day time %x", f.DayTime)
	}
	a.input(t, 0, 1)
	d0 := a.expectTick(t, 0).DayTime
	a.send(t, protocol.Clock{T: "CLOCK", Tick: 0, Rate: 10, Set: -1})
	time.Sleep(500 * time.Millisecond) // ~10 clock steps
	a.input(t, 1, 1)
	d1 := a.expectTick(t, 1).DayTime
	if d1-d0 < 60 || d1-d0 > 140 {
		t.Fatalf("clock moved %d in 0.5 s at rate 10 (%x -> %x)", d1-d0, d0, d1)
	}
	a.send(t, protocol.Clock{T: "CLOCK", Tick: 1, Rate: 0, Set: 0xC000})
	time.Sleep(100 * time.Millisecond)
	a.input(t, 2, 1)
	if d := a.expectTick(t, 2).DayTime; d != 0xC000 {
		t.Fatalf("day time %x after set", d)
	}
	// a second group in a scene where time stands still: the room's clock, not its own
	b := welcome3(t, addr, "B")
	fb := found3(t, b, 0, sharedGame(0, 0))
	if fb.DayTime != 0xC000 {
		t.Fatalf("B founded at %x", fb.DayTime)
	}
}

func TestCanonicalSurvivesRestart(t *testing.T) {
	cfg := testConfig()
	cfg.SaveEvery = 10 * time.Millisecond
	dir := t.TempDir()
	start := func() (*Server, string) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		c := cfg
		c.DataDir = dir
		s := New(c)
		if err := s.PrepareDataDir(); err != nil {
			t.Fatal(err)
		}
		go func() { _ = s.Serve(ln) }()
		return s, ln.Addr().String()
	}
	s1, addr1 := start()
	a := welcome3(t, addr1, "A")
	found3(t, a, 85, sharedGame(10, 0))
	a.send(t, protocol.Shared{T: "SHARED", Tick: 1, Data: append(deltaEntry(0, 2, 90), bitsEntry(4, 1, 0x80, 0)...)})
	a.send(t, protocol.Clock{T: "CLOCK", Tick: 2, Rate: 0, Set: 0x4000})
	time.Sleep(200 * time.Millisecond)
	s1.Close() // the process dies (writes what it has)
	s2, addr2 := start()
	t.Cleanup(s2.Close)
	b := welcome3(t, addr2, "A")
	f := found3(t, b, 85, sharedGame(10, 0))
	if !bytes.Equal(f.Shared, sharedGame(100, 0x80)) || f.DayTime != 0x4000 {
		t.Fatalf("after restart: shared %v day %x", f.Shared, f.DayTime)
	}
}

func TestOverwriteAndLayoutReplaceCanonical(t *testing.T) {
	_, addr := startServer(t)
	a := welcome3(t, addr, "A")
	found3(t, a, 85, sharedGame(10, 0))
	b := welcome3(t, addr, "B")
	b.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: 1, InPlay: true, Shared: sharedGame(77, 0), SharedLayout: 7,
		Overwrite: true})
	if f := decode[protocol.GroupFound](t, b.expect(t, "GROUP_FOUND")); len(f.Shared) != 0 {
		t.Fatalf("overwrite still sent the old canonical %v", f.Shared)
	}
	c := welcome3(t, addr, "C")
	c.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: 2, InPlay: true, Shared: sharedGame(1, 0), SharedLayout: 8})
	if f := decode[protocol.GroupFound](t, c.expect(t, "GROUP_FOUND")); len(f.Shared) != 0 {
		t.Fatalf("another layout got the old canonical %v", f.Shared)
	}
	d := welcome3(t, addr, "D")
	d.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: 3, InPlay: true, Shared: sharedGame(0, 0), SharedLayout: 8})
	if f := decode[protocol.GroupFound](t, d.expect(t, "GROUP_FOUND")); !bytes.Equal(f.Shared, sharedGame(1, 0)) {
		t.Fatalf("canonical %v", f.Shared)
	}
}

func TestGroupSceneRegroup(t *testing.T) {
	_, addr := startServer(t)
	a := welcome3(t, addr, "A")
	found3(t, a, 85, sharedGame(0, 0))
	c := welcome3(t, addr, "C")
	found3(t, c, 0, sharedGame(0, 0))
	// C's whole group moves to scene 85 inside its simulation: A's group already plays there
	c.send(t, protocol.GroupScene{T: "GROUP_SCENE", Scene: 85})
	r := decode[protocol.Regroup](t, c.expect(t, "REGROUP"))
	if r.Scene != 85 {
		t.Fatalf("regroup %+v", r)
	}
	c.send(t, protocol.LeaveGroup{T: "LEAVE_GROUP"})
	p := join3(t, c, a, 85, "")
	if p.Slot != 1 {
		t.Fatalf("slot %d", p.Slot)
	}
	// a group alone in its new scene just changes scene
	d := welcome3(t, addr, "D")
	found3(t, d, 3, sharedGame(0, 0))
	d.send(t, protocol.GroupScene{T: "GROUP_SCENE", Scene: 4})
	d.expectNone(t, "REGROUP", 200*time.Millisecond)
	e := welcome3(t, addr, "E")
	e.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: 4})
	decode[protocol.GroupJoinPending](t, e.expect(t, "GROUP_JOIN_PENDING"))
}

func TestFollowAndNoGameJoinsLargestGroup(t *testing.T) {
	_, addr := startServer(t)
	a := welcome3(t, addr, "A")
	found3(t, a, 85, sharedGame(0, 0))
	b := welcome3(t, addr, "B")
	join3(t, b, a, 85, "")
	c := welcome3(t, addr, "C")
	found3(t, c, 0, sharedGame(0, 0))
	// a client without a game and no preference: the group with most players
	d := welcome3(t, addr, "D")
	d.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: -1})
	if p := decode[protocol.GroupJoinPending](t, d.expect(t, "GROUP_JOIN_PENDING")); p.Slot != 2 {
		t.Fatalf("D pending %+v", p)
	}
	// one that follows C, even with its own game loaded somewhere else
	e := welcome3(t, addr, "E")
	e.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: 40, InPlay: true, Follow: "C"})
	p := decode[protocol.GroupJoinPending](t, e.expect(t, "GROUP_JOIN_PENDING"))
	req := decode[protocol.StateBlobRequest](t, c.expect(t, "STATE_BLOB_REQUEST"))
	if p.Slot != 1 || req.ForSlot != 1 {
		t.Fatalf("E pending %+v request %+v", p, req)
	}
}

func TestWaiterForSceneServedWhenFounded(t *testing.T) {
	_, addr := startServer(t)
	a := welcome3(t, addr, "A")
	found3(t, a, 85, sharedGame(0, 0))
	b := welcome3(t, addr, "B")
	b.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: 0})
	b.expect(t, "GROUP_WAIT")
	c := welcome3(t, addr, "C")
	found3(t, c, 0, sharedGame(0, 0))
	if p := decode[protocol.GroupJoinPending](t, b.expect(t, "GROUP_JOIN_PENDING")); p.Slot != 1 {
		t.Fatalf("B pending %+v", p)
	}
	c.expect(t, "STATE_BLOB_REQUEST")
}

func TestProtocolMixRejected(t *testing.T) {
	_, addr := startServer(t)
	welcome3(t, addr, "A")
	old := dial(t, addr, hello("B", "room3", "b"))
	expectReject(t, old, "protocol_mismatch")
	// an old client in its own room still works
	o := dial(t, addr, hello("C", "old", "b"))
	if w := decode[protocol.Welcome](t, o.expect(t, "WELCOME")); w.Proto != 2 {
		t.Fatalf("old room proto %d", w.Proto)
	}
	bad := dial(t, addr, protocol.Hello{T: "HELLO", Proto: 5, BuildHash: "b", Name: "D", Room: "x"})
	expectReject(t, bad, "protocol_mismatch")
}

func TestSoloGroupForOwnCutscene(t *testing.T) {
	_, addr := startServer(t)
	a := welcome3(t, addr, "A")
	found3(t, a, 85, sharedGame(0, 0))
	// B arrives in scene 85 with a cutscene of its own (blue warp): a group of its own, although A plays there
	b := welcome3(t, addr, "B")
	b.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: 85, InPlay: true, Solo: true, Shared: sharedGame(0, 0),
		SharedLayout: 7})
	f := decode[protocol.GroupFound](t, b.expect(t, "GROUP_FOUND"))
	if f.Slot != 0 {
		t.Fatalf("B found %+v", f)
	}
	// nobody joins the solo group: C enters scene 85 and goes to A's
	c := welcome3(t, addr, "C")
	c.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: 85})
	decode[protocol.GroupJoinPending](t, c.expect(t, "GROUP_JOIN_PENDING"))
	a.expect(t, "STATE_BLOB_REQUEST")
	b.expectNone(t, "STATE_BLOB_REQUEST", 100*time.Millisecond)
	// its cutscene moves through other scenes without being regrouped
	b.send(t, protocol.GroupScene{T: "GROUP_SCENE", Scene: 81})
	b.send(t, protocol.GroupScene{T: "GROUP_SCENE", Scene: 85})
	b.expectNone(t, "REGROUP", 150*time.Millisecond)
	// the cutscene ends: B joins the others of its scene
	b.send(t, protocol.Header{T: "GROUP_OPEN"})
	r := decode[protocol.Regroup](t, b.expect(t, "REGROUP"))
	if r.Scene != 85 {
		t.Fatalf("regroup %+v", r)
	}
}

// Demo 0.5.0 speaks protocol 3 and was written for the phase 5 join: its Link appears at the join tick and the group
// waits for the newcomer's input from that tick. A protocol 3 room keeps exactly that (protocol 4 does not wait).
func TestProtocol3RoomKeepsThePhase5Join(t *testing.T) {
	_, addr := startServer(t)
	hello := func(name string) protocol.Hello {
		h := hello3(name, "room-p3")
		h.Proto = 3
		return h
	}
	a := dial(t, addr, hello("A"))
	if w := decode[protocol.Welcome](t, a.expect(t, "WELCOME")); w.Proto != 3 {
		t.Fatalf("welcome proto %d", w.Proto)
	}
	found3(t, a, 85, sharedGame(10, 0))
	b := dial(t, addr, hello("B"))
	b.expect(t, "WELCOME")
	b.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: 85, Entrance: 2, Spawn: "zmp_spawn 9"})
	p := decode[protocol.GroupJoinPending](t, b.expect(t, "GROUP_JOIN_PENDING"))
	req := decode[protocol.StateBlobRequest](t, a.expect(t, "STATE_BLOB_REQUEST"))
	a.send(t, protocol.StateBlob{T: "STATE_BLOB", GroupID: req.GroupID, Tick: req.Tick, ForSlot: req.ForSlot,
		Data: []byte("state"), Hash: 1})
	b.expect(t, "STATE_BLOB")
	// the join tick needs both inputs and carries the SPAWN
	a.input(t, p.SpawnTick, 1)
	b.input(t, p.SpawnTick, 2)
	m := a.expectTick(t, p.SpawnTick)
	if len(m.Slots) != 2 || len(m.Events) != 1 || m.Events[0].K != "SPAWN" || m.Events[0].Cmd != "zmp_spawn 9" {
		t.Fatalf("join tick %+v", m)
	}
}

// Found by the accelerated soak of the fast suite: B is on its way into A's group (it has the state, its Link has not
// appeared yet) when A leaves. B inherits the group; its client sends input from the newest tick + D on, never for
// the tick the server is at. The ticks must flow all the same (before, B waited for ever alone in its group).
func TestJoinerInheritsTheGroupAndIsNotStuck(t *testing.T) {
	_, addr := startServer(t)
	a := welcome3(t, addr, "A")
	found3(t, a, 45, sharedGame(10, 0))
	for tick := int64(0); tick < 3; tick++ {
		a.input(t, tick, 1)
		a.expectTick(t, tick)
	}
	b := welcome3(t, addr, "B")
	b.send(t, protocol.JoinGroup{T: "JOIN_GROUP", Scene: 45, Entrance: 2, Spawn: "zmp_spawn 9"})
	p := decode[protocol.GroupJoinPending](t, b.expect(t, "GROUP_JOIN_PENDING"))
	req := decode[protocol.StateBlobRequest](t, a.expect(t, "STATE_BLOB_REQUEST"))
	a.send(t, protocol.StateBlob{T: "STATE_BLOB", GroupID: req.GroupID, Tick: req.Tick, ForSlot: req.ForSlot,
		Data: []byte("state"), Hash: 1})
	b.expect(t, "STATE_BLOB")
	a.send(t, protocol.LeaveGroup{T: "LEAVE_GROUP"})
	time.Sleep(30 * time.Millisecond)
	// B's inputs: from one tick after the head (D = 2: newest tick + D - 1), a handful of them at once
	head := p.SpawnTick
	for tick := head + 1; tick <= head+5; tick++ {
		b.input(t, tick, 2)
	}
	spawned := false
	for tick := head; tick <= head+5; tick++ {
		m := b.expectTick(t, tick)
		for _, e := range m.Events {
			if e.K == "SPAWN" && e.Slot == 1 {
				spawned = true
			}
		}
	}
	if !spawned {
		t.Fatalf("B's Link never appeared")
	}
}

// A player alone in its group whose input for the head tick is late while its inputs for later ticks are already
// here (it was filled as "slow"): the ticks flow again from the first of those inputs, nobody has to send anything.
func TestSoloPlayerFilledWithLaterInputsStored(t *testing.T) {
	_, addr := startServer(t)
	a := welcome3(t, addr, "A")
	found3(t, a, 85, sharedGame(10, 0))
	a.input(t, 0, 1)
	a.expectTick(t, 0)
	for tick := int64(2); tick <= 5; tick++ { // (nothing for tick 1)
		a.input(t, tick, 1)
	}
	for tick := int64(1); tick <= 5; tick++ {
		a.expectTick(t, tick)
	}
}

// lockedBuffer is a log sink the server's goroutines and the test can share.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// A resynchronization can never leave a group stopped (found by the fast suite, D-079: the group's head tick was
// waiting for a player, that player was found different and stopped being waited for, and nobody told the head: the
// others had already sent every input they may send ahead, so no message ever came to move it). Twice in a row for
// the same player, as it was seen. The "slow player" timers are set out of reach: only the resync may move the head.
func TestResyncNeverLeavesTheGroupStopped(t *testing.T) {
	cfg := testConfig()
	cfg.ZeroFillAfter = time.Hour
	cfg.SlowFillAfter = time.Hour
	var logged lockedBuffer
	cfg.Logger = log.New(&logged, "", 0)
	_, addr := startServerCfg(t, cfg)
	// (the periodic safety net that emits a tick nobody tried to emit must not be what moves the group)
	defer func() {
		if strings.Contains(logged.String(), "UNSTUCK") {
			t.Fatalf("the group was moved by the safety net: %s", logged.String())
		}
	}()
	a := welcome3(t, addr, "A")
	found3(t, a, 85, sharedGame(10, 0))
	b := welcome3(t, addr, "B")
	p := join3(t, b, a, 85, "zmp_spawn 9")
	head := p.SpawnTick
	a.input(t, head, 1)
	a.expectTick(t, head)
	b.expectTick(t, head)
	head++
	for round := 0; round < 2; round++ {
		// A sends every input it may send ahead; B's input for the head tick is late: the head waits for B
		for tick := head; tick < head+5; tick++ {
			a.input(t, tick, 1)
		}
		a.expectNone(t, "TICK", 80*time.Millisecond)
		// B is found different from the leader
		a.send(t, protocol.Hash{T: "HASH", Tick: head - 1, Hash: 1})
		b.send(t, protocol.Hash{T: "HASH", Tick: head - 1, Hash: 2})
		r := decode[protocol.Resync](t, a.expect(t, "RESYNC"))
		if r.Tick != head || len(r.Targets) != 1 || r.Targets[0] != 1 {
			t.Fatalf("round %d: bad RESYNC %+v (head %d)", round, r, head)
		}
		req := decode[protocol.StateBlobRequest](t, a.expect(t, "STATE_BLOB_REQUEST"))
		// the group goes on by itself, with nobody sending anything else
		for tick := head; tick < head+5; tick++ {
			m := a.expectTick(t, tick)
			if !bytes.Equal(padOf(t, m, 1), zeroPad) {
				t.Fatalf("round %d: B's pad in tick %d is not the fill", round, tick)
			}
		}
		// B loads the leader's state and plays again: its first input after loading is in time
		a.send(t, protocol.StateBlob{T: "STATE_BLOB", GroupID: req.GroupID, Tick: req.Tick, ForSlot: 1,
			Data: []byte("state"), Hash: 1})
		b.expect(t, "RESYNC")
		for tick := head; tick < head+5; tick++ {
			b.expectTick(t, tick)
		}
		b.expect(t, "STATE_BLOB")
		head += 5
		b.input(t, head, 2)
		time.Sleep(30 * time.Millisecond) // (two connections: B's input has to be in before A's completes the tick)
		a.input(t, head, 1)
		m := a.expectTick(t, head)
		if bytes.Equal(padOf(t, m, 1), zeroPad) {
			t.Fatalf("round %d: B's input after the resync was not used in tick %d", round, head)
		}
		b.expectTick(t, head)
		head++
	}
}

// What a player sent before it was found different does not count after it loads the leader's state: its inputs
// stored for later ticks came from a game that is no longer its game, and they hid its new ones (the first input for
// a tick wins), so it stayed filled until it sent one beyond them.
func TestInputsOfBeforeAResyncAreDropped(t *testing.T) {
	cfg := testConfig()
	cfg.ZeroFillAfter = time.Hour
	cfg.SlowFillAfter = time.Hour
	_, addr := startServerCfg(t, cfg)
	a := welcome3(t, addr, "A")
	found3(t, a, 85, sharedGame(10, 0))
	b := welcome3(t, addr, "B")
	p := join3(t, b, a, 85, "zmp_spawn 9")
	head := p.SpawnTick
	a.input(t, head, 1)
	a.expectTick(t, head)
	b.expectTick(t, head)
	head++
	for tick := head; tick < head+4; tick++ { // B's inputs of before, stored for ticks still to come
		b.input(t, tick, 7)
	}
	time.Sleep(30 * time.Millisecond)
	a.send(t, protocol.Hash{T: "HASH", Tick: head - 1, Hash: 1})
	b.send(t, protocol.Hash{T: "HASH", Tick: head - 1, Hash: 2})
	a.expect(t, "RESYNC")
	req := decode[protocol.StateBlobRequest](t, a.expect(t, "STATE_BLOB_REQUEST"))
	a.send(t, protocol.StateBlob{T: "STATE_BLOB", GroupID: req.GroupID, Tick: req.Tick, ForSlot: 1,
		Data: []byte("state"), Hash: 1})
	b.expect(t, "RESYNC")
	b.expect(t, "STATE_BLOB")
	b.input(t, head+1, 2) // its first input after loading
	time.Sleep(30 * time.Millisecond)
	for tick := head; tick < head+2; tick++ {
		a.input(t, tick, 1)
		m := a.expectTick(t, tick)
		want := zeroPad
		if tick == head+1 {
			want = pad(2)
		}
		if !bytes.Equal(padOf(t, m, 1), want) {
			t.Fatalf("tick %d: B's pad %v, want %v", tick, padOf(t, m, 1), want)
		}
	}
}

// The leader answers a request for the state of tick T with the state of a later tick (it had already played T when
// the request reached its game): the player who waits for it gets that state. It was dropped and the player stayed
// frozen for ever (found by the fast suite with a mutant that makes players differ, D-082).
func TestLateStateOfTheLeaderIsRelayed(t *testing.T) {
	cfg := testConfig()
	cfg.ZeroFillAfter = time.Hour
	cfg.SlowFillAfter = time.Hour
	_, addr := startServerCfg(t, cfg)
	a := welcome3(t, addr, "A")
	found3(t, a, 85, sharedGame(10, 0))
	b := welcome3(t, addr, "B")
	p := join3(t, b, a, 85, "zmp_spawn 9")
	head := p.SpawnTick
	a.input(t, head, 1)
	a.expectTick(t, head)
	b.expectTick(t, head)
	head++
	a.send(t, protocol.Hash{T: "HASH", Tick: head - 1, Hash: 1})
	b.send(t, protocol.Hash{T: "HASH", Tick: head - 1, Hash: 2})
	a.expect(t, "RESYNC")
	b.expect(t, "RESYNC")
	req := decode[protocol.StateBlobRequest](t, a.expect(t, "STATE_BLOB_REQUEST"))
	if req.Tick != head {
		t.Fatalf("request for tick %d, head %d", req.Tick, head)
	}
	// the group plays two more ticks before the leader answers
	for tick := head; tick < head+2; tick++ {
		a.input(t, tick, 1)
		a.expectTick(t, tick)
		b.expectTick(t, tick)
	}
	// never the state of a tick the group has not reached, nor of an earlier one
	for _, bad := range []int64{head + 3, head - 1} {
		a.send(t, protocol.StateBlob{T: "STATE_BLOB", GroupID: req.GroupID, Tick: bad, ForSlot: 1, Data: []byte("x"),
			Hash: 1})
	}
	b.expectNone(t, "STATE_BLOB", 80*time.Millisecond)
	a.send(t, protocol.StateBlob{T: "STATE_BLOB", GroupID: req.GroupID, Tick: head + 2, ForSlot: 1,
		Data: []byte("state"), Hash: 1})
	got := decode[protocol.StateBlobHeader](t, b.expect(t, "STATE_BLOB"))
	if got.Tick != head+2 {
		t.Fatalf("relayed state of tick %d, want %d", got.Tick, head+2)
	}
	// B's hashes of before that tick are not compared; from it on they are
	b.send(t, protocol.Hash{T: "HASH", Tick: head + 1, Hash: 99})
	a.send(t, protocol.Hash{T: "HASH", Tick: head + 1, Hash: 1})
	a.expectNone(t, "RESYNC", 80*time.Millisecond)
	b.input(t, head+2, 2)
	time.Sleep(30 * time.Millisecond)
	a.input(t, head+2, 1)
	m := a.expectTick(t, head+2)
	if !bytes.Equal(padOf(t, m, 1), pad(2)) {
		t.Fatalf("B's input after loading was not used: %+v", m)
	}
}
