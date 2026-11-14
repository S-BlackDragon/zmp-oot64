package server

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/S-BlackDragon/zmp-oot64/server/internal/protocol"
)

// maxInputAhead bounds how far in the future an INPUT may be (60 s at 20 Hz); farther
// inputs are ignored so a broken client cannot grow the input table without limit.
const maxInputAhead = 1200

// hashKeep is how many ticks of hashes are kept for comparison.
const hashKeep = 1200

// group is one lockstep simulation (docs/PROTOCOLO.md). Protocol 2: the room's only group. Protocol 3: the group
// of one scene; a room has as many as scenes with players. Guarded by Server.mu.
type group struct {
	id        uint32
	room      *Room
	scene     int // protocol 3: the scene this group plays in (-1 unknown)
	slots     [MaxPlayersPerRoom]*slot
	nextTick  int64 // next tick to emit; nextTick-1 is the last emitted one
	headSince time.Time
	lastWait  time.Time // last WAIT for the current head (zero: none yet)
	delay     int
	leader    int // slot index, -1 when nobody can be leader
	joinSeq   uint64
	inputs    map[int64]map[int][]byte
	events    map[int64][]protocol.Event
	hashes    map[int64]map[int]uint64
	checked   map[int64]map[int]bool
	blobReqs  map[int]*blobReq // outstanding state transfers, by destination slot

	lastShared int64 // last tick whose SHARED patch was taken (every member sends it)
	lastClock  int64 // last tick whose CLOCK report was taken
	rate       int   // world clock speed in this group's scene (dayTime units per tick)
	regrouping bool  // REGROUP sent to its members (another group owns its scene)
	solo       bool  // founded for one player's own cutscene: nobody joins it and it is not regrouped until GROUP_OPEN

	nextDelayCalc time.Time
	nextStats     time.Time
	lastStuckLog  time.Time // last "STUCK" diagnostic line of this group
	resyncs       int
}

// slot is one Link of the simulation; it survives its connection (rejoin by name).
type slot struct {
	index       int
	g           *group
	name        string
	client      *client // nil while disconnected
	lastID      uint32  // id of the last connection, for PLAYER_LIST
	seq         uint64  // entry order, for leader election
	filled      bool    // zero-filled ("disconnected")
	resumeAt    int64   // when filled: real input again from this tick (-1: none)
	activeSince time.Time
	hashFrom    int64 // hashes of earlier ticks are ignored (state not loaded yet)

	// Phase 5b (protocol 4), nobody waits for anybody. pending: the player is loading the group's state and catching
	// up; it has no Link yet and the group does not wait for it. Its Link appears (SPAWN) at the tick of its first
	// input that arrives in time.
	pending  bool
	spawnCmd string
	// (diagnosis, D-094) inputs of a pending player that arrived after the group had played their tick, since its
	// state was relayed, and by how many ticks the last one missed
	lateInputs int
	lateBy     int64
	lastPad    []byte // last real pad, repeated while a slow player is filled
	holdTo     int64  // ticks before this one repeat lastPad instead of zeros (0: zeros)
}

type blobReq struct {
	tick   int64
	sentTo uint32 // client id the STATE_BLOB_REQUEST went to (0: not sent yet)
	join   bool   // join/rejoin (true) or resync (false)
}

func (g *group) slotByName(name string) *slot {
	for _, sl := range g.slots {
		if sl != nil && sl.name == name {
			return sl
		}
	}
	return nil
}

// needsInput says whether slot sl must provide input for tick t (otherwise zeros).
func (sl *slot) needsInput(t int64) bool {
	if sl.pending {
		return false
	}
	if !sl.filled {
		return true
	}
	return sl.resumeAt >= 0 && t >= sl.resumeAt
}

func (g *group) slotState(sl *slot) string {
	switch {
	case sl.client == nil || (sl.filled && sl.resumeAt < 0):
		return protocol.StateDisconnected
	case sl.pending || (g.blobReqs[sl.index] != nil && g.blobReqs[sl.index].join):
		return protocol.StateJoining
	default:
		return protocol.StatePlaying
	}
}

func (g *group) hasConnected() bool {
	for _, sl := range g.slots {
		if sl != nil && sl.client != nil {
			return true
		}
	}
	return false
}

// hasPlaying: somebody connected whose Link is in the simulation (a group of players who are only on their way in
// has nobody to simulate it).
func (g *group) hasPlaying() bool {
	for _, sl := range g.slots {
		if sl != nil && sl.client != nil && !sl.pending {
			return true
		}
	}
	return false
}

// promotePendingLocked: the players who were simulating the group are gone and somebody on its way in already has
// its state: it becomes a normal member now (the group waits for it; there is nobody else to hold up).
func (s *Server) promotePendingLocked(g *group) {
	if g.hasPlaying() {
		return
	}
	for _, sl := range g.slots {
		if sl != nil && sl.client != nil && sl.pending && g.blobReqs[sl.index] == nil {
			sl.pending = false
			// Its client sends input for the ticks it can still be in time for (the newest tick + D), not for this
			// one: its Link stands still until that input arrives, and the ticks in between are emitted then
			// (canEmit). Waiting here for an input that never comes froze the player alone in its group (found by
			// the accelerated soak of the fast suite, D-079).
			sl.filled = true
			sl.resumeAt = -1
			sl.holdTo = 0
			T := g.nextTick
			g.events[T] = append(g.events[T], protocol.Event{K: "SPAWN", Slot: sl.index, Cmd: sl.spawnCmd})
			s.log.Printf("%s slot=%d name=%q was on its way in and is alone: SPAWN at %d", s.groupLabel(g), sl.index,
				sl.name, T)
			return
		}
	}
}

func (g *group) connectedCount() int {
	n := 0
	for _, sl := range g.slots {
		if sl != nil && sl.client != nil {
			n++
		}
	}
	return n
}

func (g *group) members() []*client {
	var out []*client
	for _, sl := range g.slots {
		if sl != nil && sl.client != nil {
			out = append(out, sl.client)
		}
	}
	return out
}

func (s *Server) broadcastGroupLocked(g *group, v any) {
	frame, err := protocol.Encode(v)
	if err != nil {
		s.log.Printf("encode %T: %v", v, err)
		return
	}
	for _, c := range g.members() {
		s.enqueue(c, frame)
	}
}

func (s *Server) groupLabel(g *group) string {
	if g.room.sceneGroups() {
		return fmt.Sprintf("room=%q group=%d scene=%d", g.room.Name, g.id, g.scene)
	}
	return fmt.Sprintf("room=%q group=%d", g.room.Name, g.id)
}

// sortedGroups returns the room's groups by id (deterministic iteration).
func (r *Room) sortedGroups() []*group {
	out := make([]*group, 0, len(r.groups))
	for _, g := range r.groups {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

// slotByName finds the slot a name owns in any group of the room.
func (r *Room) slotByName(name string) *slot {
	for _, g := range r.sortedGroups() {
		if sl := g.slotByName(name); sl != nil {
			return sl
		}
	}
	return nil
}

// targetGroup is the group a JOIN_GROUP enters (nil: found one or wait). Protocol 2: the room's only group.
// Protocol 3: the group of the player it follows, else the group of its scene, else (a client without a game that
// names no scene) the group with most players.
func (r *Room) targetGroup(m protocol.JoinGroup) *group {
	groups := r.sortedGroups()
	if !r.sceneGroups() {
		if len(groups) > 0 {
			return groups[0]
		}
		return nil
	}
	if m.Follow != "" {
		for _, g := range groups {
			if sl := g.slotByName(m.Follow); sl != nil && sl.client != nil {
				return g
			}
		}
	}
	var best *group
	for _, g := range groups {
		if g.regrouping || g.solo || !g.hasConnected() {
			continue
		}
		if m.Scene >= 0 && g.scene != m.Scene {
			continue
		}
		if m.Scene < 0 && m.InPlay {
			continue
		}
		if best == nil || g.connectedCount() > best.connectedCount() {
			best = g
		}
	}
	return best
}

// ---------------------------------------------------------------------------------------
// Founding, joining, leaving

func (s *Server) joinGroupLocked(c *client, m protocol.JoinGroup) {
	room := c.room
	if c.slot != nil && c.slot.client == c {
		return // already a member: duplicate JOIN_GROUP is ignored
	}
	c.waitingGroup = false
	c.lastScene = m.Scene
	if m.InPlay && len(m.Shared) > 0 {
		c.lastShared = m.Shared
		c.lastLayout = m.SharedLayout
	}

	sl := room.slotByName(c.name)
	var g *group
	if sl != nil {
		g = sl.g // rejoin: the group that kept this player's Link
	} else if !(m.Solo && m.InPlay && room.sceneGroups()) {
		g = room.targetGroup(m)
	}
	if g == nil {
		if !m.InPlay {
			c.waitingGroup = true
			c.waitMsg = m
			s.sendLocked(c, protocol.GroupWait{T: "GROUP_WAIT", Reason: "no_group"})
			s.log.Printf("room=%q id=%d name=%q waits for a group (no_group, scene %d, follow %q)", room.Name, c.id,
				c.name, m.Scene, m.Follow)
			s.playerListLocked(room)
			return
		}
		s.foundGroupLocked(c, m)
		return
	}

	rejoin := sl != nil
	idx := -1
	if rejoin {
		idx = sl.index
	} else {
		for i, other := range g.slots {
			if other == nil {
				idx = i
				break
			}
		}
		if idx < 0 {
			c.waitingGroup = true
			c.waitMsg = m
			s.sendLocked(c, protocol.GroupWait{T: "GROUP_WAIT", Reason: "group_full"})
			s.log.Printf("%s id=%d name=%q waits (group_full)", s.groupLabel(g), c.id, c.name)
			s.playerListLocked(room)
			return
		}
	}

	// Somebody else must be able to send the state. If nobody can (every other member is
	// gone or itself loading), the group is dead: found a fresh one (or wait for one).
	source := false
	for i, other := range g.slots {
		if i != idx && other != nil && other.client != nil && g.blobReqs[i] == nil {
			source = true
			break
		}
	}
	if !source {
		s.destroyGroupLocked(g, "no member can provide the state for "+c.name)
		s.joinGroupLocked(c, m)
		return
	}

	T := g.nextTick
	now := time.Now()
	if rejoin {
		if old := sl.client; old != nil && old != c {
			old.slot = nil
		}
	} else {
		sl = &slot{index: idx, g: g, name: c.name, seq: g.joinSeq}
		g.joinSeq++
		g.slots[idx] = sl
		spawn := m.Spawn
		if spawn == "" && m.Follow != "" {
			// a player without a game of its own who follows somebody appears next to that player
			if f := g.slotByName(m.Follow); f != nil {
				spawn = fmt.Sprintf("zmp_spawn_near %d", f.index)
			}
		}
		if room.noWait() {
			// Phase 5b: the group goes on while this player loads its state and catches up (see inputLocked)
			sl.pending = true
			sl.spawnCmd = spawn
		} else {
			g.events[T] = append(g.events[T], protocol.Event{K: "SPAWN", Slot: idx, Cmd: spawn})
		}
	}
	sl.client = c
	sl.lastID = c.id
	c.slot = sl
	sl.filled = false
	sl.resumeAt = -1
	if room.noWait() && !sl.pending {
		// a player who comes back to its Link: it stands still until this player's input arrives; nobody waits
		sl.filled = true
		sl.holdTo = 0
	}
	sl.activeSince = now
	sl.hashFrom = T
	for t, in := range g.inputs { // inputs of a previous connection do not count
		if t >= T {
			delete(in, idx)
		}
	}
	g.blobReqs[idx] = &blobReq{tick: T, join: true}
	s.electLeaderLocked(g)
	s.sendLocked(c, protocol.GroupJoinPending{T: "GROUP_JOIN_PENDING", GroupID: g.id, Slot: idx, Leader: g.leader,
		SpawnTick: T, Rejoin: rejoin})
	s.log.Printf("%s id=%d name=%q enters slot=%d spawn_tick=%d rejoin=%v leader=%d from scene=%d entrance=%d",
		s.groupLabel(g), c.id, c.name, idx, T, rejoin, g.leader, m.Scene, m.Entrance)
	if req := g.blobReqs[idx]; req != nil && req.sentTo == 0 {
		s.sendBlobRequestLocked(g, idx)
	}
	s.playerListLocked(room)
}

func (s *Server) foundGroupLocked(c *client, m protocol.JoinGroup) {
	room := c.room
	s.groupSeq++
	now := time.Now()
	g := &group{id: s.groupSeq, room: room, scene: m.Scene, headSince: now, delay: InitialDelay, leader: 0, joinSeq: 1,
		inputs: map[int64]map[int][]byte{}, events: map[int64][]protocol.Event{},
		hashes: map[int64]map[int]uint64{}, checked: map[int64]map[int]bool{}, blobReqs: map[int]*blobReq{},
		lastShared: -1, lastClock: -1, nextDelayCalc: now.Add(s.cfg.DelayInterval),
		nextStats: now.Add(s.cfg.StatsInterval)}
	g.solo = m.Solo && room.sceneGroups()
	sl := &slot{index: 0, g: g, name: c.name, client: c, lastID: c.id, seq: 0, resumeAt: -1, activeSince: now}
	g.slots[0] = sl
	c.slot = sl
	room.groups[g.id] = g
	msg := protocol.GroupFound{T: "GROUP_FOUND", GroupID: g.id, Slot: 0, Tick0: 0, Delay: g.delay, DayTime: -1,
		Scene: g.scene}
	canon := "-"
	if room.sceneGroups() {
		msg.DayTime = int32(room.clockDay())
		switch {
		case len(m.Shared) == 0:
			// a founder without a shared game (should not happen): keep whatever the room has
			if room.canon != nil {
				msg.Shared = room.canon.Data
				canon = "sent"
			}
		case room.canon == nil || m.Overwrite || room.canon.Layout != m.SharedLayout || len(room.canon.Data) != len(m.Shared):
			why := "new"
			if room.canon != nil {
				why = fmt.Sprintf("replaced (overwrite=%v, layout %08x -> %08x)", m.Overwrite, room.canon.Layout,
					m.SharedLayout)
			}
			room.canon = &canonical{Layout: m.SharedLayout, Data: append([]byte(nil), m.Shared...), dirty: true}
			canon = "from the founder, " + why
		default:
			msg.Shared = room.canon.Data
			canon = "sent to the founder"
		}
	}
	s.sendLocked(c, msg)
	s.log.Printf("%s founded by id=%d name=%q entrance=%d delay=%d canonical: %s", s.groupLabel(g), c.id, c.name,
		m.Entrance, g.delay, canon)
	s.serveWaitingLocked(room)
	s.playerListLocked(room)
}

// serveWaitingLocked admits the clients that got GROUP_WAIT and now have a group to enter, oldest connection first.
func (s *Server) serveWaitingLocked(room *Room) {
	for _, w := range room.sortedClients() {
		if !w.waitingGroup || w.gone {
			continue
		}
		m := w.waitMsg
		m.InPlay = false // served only into an existing group (it founds by itself when it can)
		if room.slotByName(w.name) != nil || room.targetGroup(m) != nil {
			s.joinGroupLocked(w, m)
		}
	}
}

func (s *Server) leaveGroupLocked(c *client) {
	room := c.room
	if c.waitingGroup {
		c.waitingGroup = false
		s.playerListLocked(room)
	}
	sl := c.slot
	if sl == nil || sl.client != c {
		return
	}
	g := sl.g
	T := g.nextTick
	idx := sl.index
	scheduled := false
	for t, evs := range g.events { // its Link was about to appear: it never does
		for i := 0; i < len(evs); i++ {
			if t >= T && evs[i].K == "SPAWN" && evs[i].Slot == idx {
				evs = append(evs[:i], evs[i+1:]...)
				g.events[t] = evs
				scheduled = true
				i--
			}
		}
	}
	if !sl.pending && !scheduled {
		g.events[T] = append(g.events[T], protocol.Event{K: "DESPAWN", Slot: idx})
	}
	g.slots[idx] = nil
	c.slot = nil
	delete(g.blobReqs, idx)
	for t, m := range g.inputs {
		if t >= T {
			delete(m, idx)
		}
	}
	s.log.Printf("%s id=%d name=%q leaves slot=%d (DESPAWN at %d)", s.groupLabel(g), c.id, c.name, idx, T)
	if !g.hasConnected() {
		s.destroyGroupLocked(g, "no connected members")
	} else {
		s.promotePendingLocked(g)
		s.electLeaderLocked(g)
		s.tryEmitLocked(g)
		s.serveWaitingLocked(room)
	}
	s.playerListLocked(room)
}

// detachSlotLocked is called when the connection owning a slot goes away. The slot stays
// (its Link keeps existing, zero-filled after ZeroFillAfter) until the name rejoins.
func (s *Server) detachSlotLocked(sl *slot) {
	g := sl.g
	c := sl.client
	sl.client = nil
	if c != nil {
		c.slot = nil
	}
	if req := g.blobReqs[sl.index]; req != nil {
		delete(g.blobReqs, sl.index)
	}
	s.log.Printf("%s slot=%d name=%q lost its connection", s.groupLabel(g), sl.index, sl.name)
	if sl.pending {
		g.slots[sl.index] = nil // it had no Link yet
	} else if g.room.noWait() {
		// Phase 5b: nobody waits for a lost connection; its Link stands still until the player comes back
		sl.filled = true
		sl.resumeAt = -1
		sl.holdTo = 0
	}
	if !g.hasConnected() {
		s.destroyGroupLocked(g, "no connected members")
		return
	}
	s.promotePendingLocked(g)
	s.electLeaderLocked(g)
	s.tryEmitLocked(g)
}

func (s *Server) destroyGroupLocked(g *group, why string) {
	room := g.room
	if room.groups[g.id] != g {
		return
	}
	s.log.Printf("%s destroyed at tick %d (%s)", s.groupLabel(g), g.nextTick-1, why)
	for _, sl := range g.slots {
		if sl != nil && sl.client != nil {
			sl.client.slot = nil
		}
	}
	delete(room.groups, g.id)
}

// groupSceneLocked: the sender's whole group moved to another scene inside its simulation (protocol 3).
func (s *Server) groupSceneLocked(c *client, m protocol.GroupScene) {
	room := c.room
	if !room.sceneGroups() || c.slot == nil || c.slot.client != c {
		return
	}
	g := c.slot.g
	if g.scene == m.Scene || g.regrouping {
		return
	}
	old := g.scene
	g.scene = m.Scene
	s.log.Printf("%s the whole group moved from scene %d", s.groupLabel(g), old)
	s.regroupIfSharedLocked(g)
	s.playerListLocked(room)
}

// regroupIfSharedLocked: another group already plays in g's scene: g's players join it, one at a time. A solo group
// (a player in its own cutscene) waits until it opens.
func (s *Server) regroupIfSharedLocked(g *group) {
	if g.solo || g.regrouping {
		return
	}
	for _, h := range g.room.sortedGroups() {
		if h != g && h.scene == g.scene && h.hasConnected() && !h.regrouping && !h.solo {
			g.regrouping = true
			s.log.Printf("%s: group %d already plays in scene %d, REGROUP", s.groupLabel(g), h.id, g.scene)
			s.broadcastGroupLocked(g, protocol.Regroup{T: "REGROUP", Scene: g.scene})
			return
		}
	}
}

// groupOpenLocked: the player of a solo group finished its cutscene (phase 5b).
func (s *Server) groupOpenLocked(c *client) {
	if c.slot == nil || c.slot.client != c || !c.slot.g.solo {
		return
	}
	g := c.slot.g
	g.solo = false
	s.log.Printf("%s open (its player's cutscene ended)", s.groupLabel(g))
	s.regroupIfSharedLocked(g)
	if !g.regrouping {
		s.serveWaitingLocked(g.room)
	}
	s.playerListLocked(g.room)
}

// sharedLocked: a patch of the shared game from the sender's group (protocol 3).
func (s *Server) sharedLocked(c *client, m protocol.Shared) {
	room := c.room
	if !room.sceneGroups() || c.slot == nil || c.slot.client != c || len(m.Data) == 0 {
		return
	}
	g := c.slot.g
	if m.Tick <= g.lastShared {
		return // another member already sent this tick's patch
	}
	g.lastShared = m.Tick
	if room.canon != nil {
		if err := ApplyPatch(room.canon.Data, m.Data); err != nil {
			s.log.Printf("%s: bad SHARED patch at tick %d (%d bytes): %v", s.groupLabel(g), m.Tick, len(m.Data), err)
			return
		}
		room.canon.dirty = true
	}
	n := 0
	for _, h := range room.sortedGroups() {
		if h == g {
			continue
		}
		h.events[h.nextTick] = append(h.events[h.nextTick], protocol.Event{K: "SHARED", Slot: -1, Data: m.Data})
		n++
	}
	s.log.Printf("%s SHARED tick=%d (%d bytes) -> %d other groups", s.groupLabel(g), m.Tick, len(m.Data), n)
}

// leaveSharedLocked: the shared changes of a player's leaving tick (D-115). They are its group's: if anybody else
// stays connected in the group, it sends them with that tick's SHARED and these are dropped (taking both would count
// a counter twice). If the leaver was the last one, nobody will send that tick: these are its SHARED.
func (s *Server) leaveSharedLocked(c *client, m protocol.LeaveGroup) {
	if c.slot == nil || c.slot.client != c {
		return
	}
	g := c.slot.g
	for _, sl := range g.slots {
		if sl != nil && sl.client != nil && sl.client != c {
			s.log.Printf("%s id=%d name=%q leaves with the shared changes of tick %d: the members who stay send them",
				s.groupLabel(g), c.id, c.name, m.Tick)
			return
		}
	}
	s.log.Printf("%s id=%d name=%q leaves last: the shared changes of its tick %d are the group's patch",
		s.groupLabel(g), c.id, c.name, m.Tick)
	s.sharedLocked(c, protocol.Shared{T: "SHARED", Tick: m.Tick, Data: m.Shared})
}

// clockLocked: time speed of the sender's scene, or a new time set by its group (protocol 3).
func (s *Server) clockLocked(c *client, m protocol.Clock) {
	room := c.room
	if !room.sceneGroups() || c.slot == nil || c.slot.client != c {
		return
	}
	g := c.slot.g
	if m.Tick <= g.lastClock {
		return // another member already reported this tick
	}
	g.lastClock = m.Tick
	if m.Rate >= 0 && m.Rate <= 1000 && m.Rate != g.rate {
		s.log.Printf("%s clock rate %d -> %d", s.groupLabel(g), g.rate, m.Rate)
		g.rate = m.Rate
	}
	if m.Set >= 0 && m.Set <= 0xFFFF && uint16(m.Set) != room.clockDay() {
		s.log.Printf("%s set the clock to %04x (was %04x)", s.groupLabel(g), m.Set, room.clockDay())
		room.clockSet(uint16(m.Set))
	}
}

// ---------------------------------------------------------------------------------------
// Leader and state transfer

// electLeaderLocked picks the oldest connected member that is not zero-filled and not
// waiting for a state blob (fallback: oldest connected member not waiting for a blob).
func (s *Server) electLeaderLocked(g *group) {
	best := -1
	for pass := 0; pass < 2 && best < 0; pass++ {
		for i, sl := range g.slots {
			// (a connected player whose input was late for a moment, holdTo > 0, still has the state: it stays leader)
			if sl == nil || sl.client == nil || g.blobReqs[i] != nil {
				continue
			}
			away := sl.filled && !(g.room.noWait() && sl.holdTo > 0)
			if pass == 0 && (away || sl.pending) {
				continue
			}
			if best < 0 || sl.seq < g.slots[best].seq {
				best = i
			}
		}
	}
	if best == g.leader {
		return
	}
	old := g.leader
	g.leader = best
	name := ""
	if best >= 0 {
		name = g.slots[best].name
	}
	s.log.Printf("%s leader %d -> %d (%s)", s.groupLabel(g), old, best, name)
	s.reissueBlobRequestsLocked(g)
	s.playerListLocked(g.room)
}

// reissueBlobRequestsLocked re-sends outstanding requests to the current leader. If the
// requested tick was already emitted the new leader may have simulated it: the transfer
// moves to the next tick with a RESYNC so the destination waits for the new tick.
func (s *Server) reissueBlobRequestsLocked(g *group) {
	if g.leader < 0 {
		return
	}
	leaderID := g.slots[g.leader].client.id
	idxs := make([]int, 0, len(g.blobReqs))
	for i := range g.blobReqs {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	for _, i := range idxs {
		req := g.blobReqs[i]
		if req.sentTo == leaderID {
			continue
		}
		if req.tick < g.nextTick {
			// The destination was told to load the state at req.tick, which is already out.
			old := req.tick
			req.tick = g.nextTick
			g.slots[i].hashFrom = req.tick
			s.log.Printf("%s leader changed before the blob for slot=%d: RESYNC bad_tick=%d tick=%d",
				s.groupLabel(g), i, old, req.tick)
			s.broadcastGroupLocked(g, protocol.Resync{T: "RESYNC", GroupID: g.id, BadTick: old, Tick: req.tick,
				Leader: g.leader, Targets: []int{i}})
		}
		s.sendBlobRequestLocked(g, i)
	}
}

func (s *Server) sendBlobRequestLocked(g *group, forSlot int) {
	req := g.blobReqs[forSlot]
	if req == nil || g.leader < 0 {
		return
	}
	lc := g.slots[g.leader].client
	req.sentTo = lc.id
	s.sendLocked(lc, protocol.StateBlobRequest{T: "STATE_BLOB_REQUEST", GroupID: g.id, Tick: req.tick, ForSlot: forSlot})
	s.log.Printf("%s STATE_BLOB_REQUEST tick=%d for_slot=%d -> leader slot=%d id=%d", s.groupLabel(g), req.tick,
		forSlot, g.leader, lc.id)
}

func (s *Server) stateBlobLocked(c *client, h protocol.StateBlobHeader, body []byte) {
	if c.slot == nil || c.slot.client != c {
		return
	}
	g := c.slot.g
	req := g.blobReqs[h.ForSlot]
	// The state of a later tick than the one asked for is as good (D-082): where nobody waits for anybody the group
	// goes on while the leader answers, and a leader that had already played the tick sends the state it has. It
	// was dropped, and whoever waited for it stayed frozen for good. Never of an earlier tick, nor of a tick the
	// group has not reached, nor from somebody the request was not sent to.
	if h.GroupID != g.id || req == nil || h.ForSlot == c.slot.index || h.Tick < req.tick || h.Tick > g.nextTick ||
		(h.Tick != req.tick && (!g.room.noWait() || c.id != req.sentTo)) {
		s.log.Printf("%s dropped STATE_BLOB from slot=%d tick=%d for_slot=%d (%d bytes): not expected",
			s.groupLabel(g), c.slot.index, h.Tick, h.ForSlot, len(body))
		return
	}
	delete(g.blobReqs, h.ForSlot)
	dst := g.slots[h.ForSlot]
	if dst == nil || dst.client == nil {
		return
	}
	if h.Tick != req.tick {
		s.log.Printf("%s STATE_BLOB for slot=%d is of tick %d (asked for %d): the leader had already played on",
			s.groupLabel(g), h.ForSlot, h.Tick, req.tick)
		dst.hashFrom = h.Tick
	}
	frame, err := protocol.FrameBody(body)
	if err != nil {
		return
	}
	s.enqueue(dst.client, frame)
	if s.cfg.TestKeepBlobs {
		s.keptBlobs = append(s.keptBlobs, append([]byte(nil), body...)) // (test setting: a leak on purpose)
	}
	s.log.Printf("%s relayed STATE_BLOB tick=%d slot %d -> %d (%d bytes)", s.groupLabel(g), h.Tick,
		c.slot.index, h.ForSlot, len(body))
	s.electLeaderLocked(g)
	s.playerListLocked(g.room)
}

// ---------------------------------------------------------------------------------------
// Inputs and ticks

func (s *Server) inputLocked(c *client, m protocol.Input) {
	sl := c.slot
	if sl == nil || sl.client != c || len(m.Pad) != protocol.PadSize {
		return
	}
	g := sl.g
	if m.Tick < g.nextTick || m.Tick >= g.nextTick+maxInputAhead {
		if sl.pending && m.Tick < g.nextTick && g.blobReqs[sl.index] == nil {
			sl.lateInputs++
			sl.lateBy = g.nextTick - m.Tick
		}
		return
	}
	if g.room.noWait() && g.blobReqs[sl.index] != nil {
		return // (phase 5b) it has not got the state yet: inputs of before the transfer do not count
	}
	if sl.pending {
		// Phase 5b: the player caught up with the group (its input is for a tick still to come): its Link appears at
		// that tick. Until then its slot gives no input and holds nobody.
		sl.pending = false
		sl.filled = true
		sl.holdTo = 0
		sl.resumeAt = -1
		g.events[m.Tick] = append(g.events[m.Tick], protocol.Event{K: "SPAWN", Slot: sl.index, Cmd: sl.spawnCmd})
		s.log.Printf("%s slot=%d name=%q caught up: SPAWN at %d (head %d; %d of its inputs arrived late, the last "+
			"one by %d ticks)", s.groupLabel(g), sl.index, sl.name, m.Tick, g.nextTick, sl.lateInputs, sl.lateBy)
		sl.lateInputs, sl.lateBy = 0, 0
		defer s.playerListLocked(g.room)
	}
	in := g.inputs[m.Tick]
	if in == nil {
		in = map[int][]byte{}
		g.inputs[m.Tick] = in
	}
	if _, dup := in[sl.index]; dup {
		return // the first input wins
	}
	in[sl.index] = m.Pad
	sl.lastPad = m.Pad
	if sl.filled && (sl.resumeAt < 0 || m.Tick < sl.resumeAt) {
		// A slow client catching up: real input again from this tick.
		if m.Tick == g.nextTick {
			sl.filled = false
			sl.resumeAt = -1
		} else {
			sl.resumeAt = m.Tick
		}
		s.log.Printf("%s slot=%d name=%q leaves zero-fill at tick %d", s.groupLabel(g), sl.index, sl.name, m.Tick)
		if !sl.filled {
			s.electLeaderLocked(g)
		}
		s.playerListLocked(g.room)
	}
	s.tryEmitLocked(g)
}

// canEmit: every slot has input for t or is zero-filled for t, and at least one real
// input exists (a group made only of zero-filled slots waits instead of spinning). A filled slot whose real input
// is already here for a later tick counts: the ticks up to that one are emitted with its fill (otherwise a player
// alone in its group, filled for a moment, waited for ever for ticks that needed an input it had already sent).
func (g *group) canEmit(t int64) bool {
	hasReal := false
	for i, sl := range g.slots {
		if sl == nil {
			continue
		}
		if !sl.needsInput(t) {
			if !sl.pending && sl.filled && sl.client != nil && sl.resumeAt > t {
				hasReal = true
			}
			continue
		}
		if _, ok := g.inputs[t][i]; !ok {
			return false
		}
		hasReal = true
	}
	return hasReal
}

func (g *group) missing(t int64) []string {
	var out []string
	for i, sl := range g.slots {
		if sl == nil || !sl.needsInput(t) {
			continue
		}
		if _, ok := g.inputs[t][i]; !ok {
			out = append(out, sl.name)
		}
	}
	return out
}

var zeroPad = make([]byte, protocol.PadSize)

func (s *Server) tryEmitLocked(g *group) {
	if g.room.groups[g.id] != g {
		return
	}
	recovered := false
	day := int32(-1)
	if g.room.sceneGroups() {
		day = int32(g.room.clockDay())
	}
	for g.canEmit(g.nextTick) {
		t := g.nextTick
		msg := protocol.Tick{T: "TICK", Tick: t, Slots: []int{}, Pads: [][]byte{}, Events: g.events[t], Delay: g.delay,
			DayTime: day}
		if msg.Events == nil {
			msg.Events = []protocol.Event{}
		}
		for i, sl := range g.slots {
			if sl == nil {
				continue
			}
			if sl.pending {
				continue // no Link yet
			}
			pad := zeroPad
			if sl.needsInput(t) {
				pad = g.inputs[t][i]
			} else if t < sl.holdTo && sl.lastPad != nil {
				pad = sl.lastPad // a slow player keeps doing what it was doing for a moment
			}
			msg.Slots = append(msg.Slots, i)
			msg.Pads = append(msg.Pads, pad)
		}
		s.broadcastGroupLocked(g, msg)
		delete(g.inputs, t)
		delete(g.events, t)
		g.nextTick++
		g.headSince = time.Now()
		g.lastWait = time.Time{}
		for _, sl := range g.slots {
			if sl != nil && sl.filled && sl.resumeAt >= 0 && g.nextTick >= sl.resumeAt {
				sl.filled = false
				sl.resumeAt = -1
				recovered = true
			}
		}
	}
	if recovered {
		s.electLeaderLocked(g)
		s.playerListLocked(g.room)
	}
}

// ---------------------------------------------------------------------------------------
// Hashes

func (s *Server) hashLocked(c *client, m protocol.Hash) {
	sl := c.slot
	if sl == nil || sl.client != c {
		return
	}
	g := sl.g
	if m.Tick < sl.hashFrom || m.Tick < g.nextTick-hashKeep {
		return
	}
	hs := g.hashes[m.Tick]
	if hs == nil {
		hs = map[int]uint64{}
		g.hashes[m.Tick] = hs
	}
	if _, dup := hs[sl.index]; dup {
		return
	}
	hs[sl.index] = m.Hash
	for t := range g.hashes {
		if t < g.nextTick-hashKeep {
			delete(g.hashes, t)
			delete(g.checked, t)
		}
	}
	s.checkHashesLocked(g, m.Tick)
}

func (s *Server) checkHashesLocked(g *group, t int64) {
	if g.leader < 0 {
		return
	}
	hs := g.hashes[t]
	lh, ok := hs[g.leader]
	if !ok {
		return
	}
	ck := g.checked[t]
	if ck == nil {
		ck = map[int]bool{}
		g.checked[t] = ck
	}
	var targets []int
	for i, h := range hs {
		if i == g.leader || ck[i] {
			continue
		}
		ck[i] = true
		if h == lh || g.slots[i] == nil || g.blobReqs[i] != nil {
			continue // equal, slot gone, or a transfer to it is already in progress
		}
		targets = append(targets, i)
	}
	if len(targets) == 0 {
		return
	}
	sort.Ints(targets)
	T := g.nextTick
	for _, i := range targets {
		s.log.Printf("%s DESYNC tick=%d slot=%d hash=%016x leader slot=%d hash=%016x: RESYNC at %d",
			s.groupLabel(g), t, i, hs[i], g.leader, lh, T)
		s.logs.Event(g.room.Name, g.room.session, fmt.Sprintf("DESYNC group=%d scene=%d tick=%d slot=%d name=%q "+
			"hash=%016x leader slot=%d hash=%016x: RESYNC at %d", g.id, g.scene, t, i, g.slots[i].name, hs[i], g.leader, lh, T))
		g.blobReqs[i] = &blobReq{tick: T}
		g.slots[i].hashFrom = T
		g.resyncs++
		if g.room.noWait() {
			// Phase 5b: only the player that differs stops; the group goes on and it catches up after loading
			g.slots[i].filled = true
			g.slots[i].resumeAt = -1
			g.slots[i].holdTo = 0
			// What it sent for the ticks to come was decided in a game that is no longer its game: it does not count
			// (and, stored, it hid its inputs of after the load: the first input for a tick wins).
			for tt, in := range g.inputs {
				if tt >= T {
					delete(in, i)
				}
			}
		}
	}
	s.broadcastGroupLocked(g, protocol.Resync{T: "RESYNC", GroupID: g.id, BadTick: t, Tick: T, Leader: g.leader,
		Targets: targets})
	for _, i := range targets {
		s.sendBlobRequestLocked(g, i)
	}
	if g.room.noWait() {
		// The head tick may have been waiting for exactly these players, and the others may already have sent every
		// input they send ahead: nobody else will come to move it (the group stopped for ever; D-082).
		s.tryEmitLocked(g)
	}
}

// ---------------------------------------------------------------------------------------
// Timers

func (s *Server) housekeepGroupLocked(g *group, now time.Time) {
	room := g.room
	if !g.canEmit(g.nextTick) {
		// Zero-fill slots that have been blocking the head for too long.
		changed := false
		for i, sl := range g.slots {
			if sl == nil || !sl.needsInput(g.nextTick) {
				continue
			}
			if _, ok := g.inputs[g.nextTick][i]; ok {
				continue
			}
			since := g.headSince
			if sl.activeSince.After(since) {
				since = sl.activeSince
			}
			after := s.cfg.ZeroFillAfter
			if room.noWait() && s.cfg.SlowFillAfter < after {
				after = s.cfg.SlowFillAfter // phase 5b: a slow player catches up by itself, the group does not wait
			}
			if now.Sub(since) >= after {
				sl.filled = true
				sl.resumeAt = -1
				sl.holdTo = 0
				if room.noWait() && sl.client != nil {
					sl.holdTo = g.nextTick + 20
					// (its inputs for later ticks may already be here: real input again from the first of them)
					for t, in := range g.inputs {
						if _, ok := in[i]; ok && t > g.nextTick && (sl.resumeAt < 0 || t < sl.resumeAt) {
							sl.resumeAt = t
						}
					}
				}
				changed = true
				s.log.Printf("%s slot=%d name=%q zero-filled from tick %d (blocked %v, connected %v)",
					s.groupLabel(g), i, sl.name, g.nextTick, now.Sub(since).Round(time.Millisecond), sl.client != nil)
			}
		}
		if changed {
			s.electLeaderLocked(g)
			s.tryEmitLocked(g)
			s.playerListLocked(room)
		}
	} else if g.hasConnected() {
		// Safety net: a head tick that can be emitted is emitted. Whatever changes who a tick waits for has to try
		// it itself; if this line ever shows in a log, something does not (D-082).
		head := g.nextTick
		s.tryEmitLocked(g)
		if room.groups[g.id] == g && g.nextTick > head {
			s.log.Printf("UNSTUCK %s: tick %d could be emitted and nothing had tried (now at %d)", s.groupLabel(g), head,
				g.nextTick)
		}
	}
	if room.groups[g.id] != g {
		return
	}
	blocked := now.Sub(g.headSince)
	if missing := g.missing(g.nextTick); len(missing) > 0 && blocked >= s.cfg.WaitAfter &&
		(g.lastWait.IsZero() || now.Sub(g.lastWait) >= s.cfg.WaitRepeat) {
		g.lastWait = now
		s.broadcastGroupLocked(g, protocol.Wait{T: "WAIT", Tick: g.nextTick, Missing: missing, Ms: blocked.Milliseconds()})
	}
	if !now.Before(g.nextDelayCalc) {
		g.nextDelayCalc = now.Add(s.cfg.DelayInterval)
		var rttMax, jitMax float64
		n := 0
		for _, c := range g.members() {
			if mean, jit, ok := c.rttStats(); ok {
				n++
				rttMax = max(rttMax, mean)
				jitMax = max(jitMax, jit)
			}
		}
		if n > 0 {
			if d := ComputeDelay(rttMax, jitMax); d != g.delay {
				s.log.Printf("%s delay %d -> %d (rtt_max %.1f ms, jitter_max %.1f ms)", s.groupLabel(g), g.delay, d,
					rttMax, jitMax)
				g.delay = d
				s.playerListLocked(room)
			}
		}
	}
	if blocked >= 2*time.Second && now.Sub(g.lastStuckLog) >= 5*time.Second {
		g.lastStuckLog = now
		// A group that has not emitted a tick for two seconds: who the head tick is waiting for and why (the
		// state of every slot), once per stats interval. Found necessary in the fast suite (D-079): "stuck" has to
		// be readable from the log.
		var detail []string
		for i, sl := range g.slots {
			if sl == nil {
				continue
			}
			_, has := g.inputs[g.nextTick][i]
			ahead := 0
			for t, in := range g.inputs {
				if _, ok := in[i]; ok && t > g.nextTick {
					ahead++
				}
			}
			detail = append(detail, fmt.Sprintf("%d:%s{conn=%v pending=%v filled=%v resumeAt=%d holdTo=%d blobReq=%v "+
				"needs=%v has=%v ahead=%d}", i, sl.name, sl.client != nil, sl.pending, sl.filled, sl.resumeAt, sl.holdTo,
				g.blobReqs[i] != nil, sl.needsInput(g.nextTick), has, ahead))
		}
		s.log.Printf("STUCK %s head=%d for %v: %s", s.groupLabel(g), g.nextTick, blocked.Round(time.Millisecond),
			strings.Join(detail, " "))
	}
	if !now.Before(g.nextStats) {
		g.nextStats = now.Add(s.cfg.StatsInterval)
		var names []string
		for _, sl := range g.slots {
			if sl != nil {
				names = append(names, fmt.Sprintf("%d:%s(%s)", sl.index, sl.name, g.slotState(sl)))
			}
		}
		// (the memory of the whole server process goes in the same line: heap in use and what the system gave it)
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		s.log.Printf("stats %s tick=%d delay=%d leader=%d resyncs=%d rate=%d players=[%s] heap_kb=%d sys_kb=%d",
			s.groupLabel(g), g.nextTick-1, g.delay, g.leader, g.resyncs, g.rate, strings.Join(names, " "),
			mem.HeapAlloc/1024, mem.Sys/1024)
	}
}
