// Package server is the zmp-server: TCP listener, strict handshake, rooms and lockstep groups
// (docs/PROTOCOLO.md): protocol 2 has one group per room; protocol 3 (phase 5) one group per scene, the canonical
// shared game of the room and the world clock. A room keeps the protocol of its first client.
//
// Concurrency model: all room, group and client bookkeeping is guarded by Server.mu.
// Nothing ever writes to a socket while holding it: every client has a buffered send
// queue drained by its own writer goroutine, so the per-client message order is the
// order in which messages were enqueued under the lock (a STATE_BLOB_REQUEST enqueued
// before a TICK always reaches the leader first). A client whose queue overflows is
// dropped.
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/S-BlackDragon/zmp-oot64/server/internal/protocol"
)

// MaxPlayersPerRoom matches ZMP_MAX_PLAYERS (and the number of group slots).
const MaxPlayersPerRoom = 6

// Delay bounds and initial value (docs/PROTOCOLO.md, server rule 11).
const (
	InitialDelay = 2
	MinDelay     = 1
	MaxDelay     = 6
)

// rttSamples is the window used for RTT and jitter.
const rttSamples = 10

// Config holds runtime settings. Zero values take the defaults documented per field.
type Config struct {
	Listen       string
	DataDir      string
	Logger       *log.Logger
	BuildVersion string

	// HelloTimeout bounds the wait for the first message. Default 10 s.
	HelloTimeout time.Duration
	// IdleTimeout drops a client that sends nothing for this long; it is reset by every
	// received frame (clients answer server PINGs every second, so a client stalled in a
	// scene load for several seconds is not dropped). Default 30 s. Also used as the write
	// deadline of a single frame.
	IdleTimeout time.Duration
	// WaitAfter: a blocked tick triggers WAIT after this long. Default 250 ms.
	WaitAfter time.Duration
	// WaitRepeat: WAIT is repeated this often while the tick stays blocked. Default 1 s.
	WaitRepeat time.Duration
	// ZeroFillAfter: a slot blocking the next tick this long is zero-filled. Default 10 s.
	ZeroFillAfter time.Duration
	// SlowFillAfter (protocol 4, phase 5b): a connected player whose input is this late no longer holds its group:
	// its last pad is repeated for up to a second (then zeros) and it catches up by itself. Default 100 ms.
	SlowFillAfter time.Duration
	// PingInterval: server PING to every client. Default 1 s.
	PingInterval time.Duration
	// DelayInterval: recomputation of the input delay D. Default 5 s.
	DelayInterval time.Duration
	// PlayerListInterval: periodic PLAYER_LIST (it is also sent on every change). Default 2 s.
	PlayerListInterval time.Duration
	// StatsInterval: one log line per group with tick, D and players. Default 60 s.
	StatsInterval time.Duration
	// MemInterval: one log line with the memory of the whole process (finding F). Default 60 s.
	MemInterval time.Duration
	// MemCollect (test setting): the garbage collector runs before each of those lines, so "live_kb" is what the
	// server really holds at that moment and not what it has not got round to freeing.
	MemCollect bool
	// TestKeepBlobs (test setting, never in production): the server keeps every state it relays, for ever. The
	// leak the memory check of the resistance tests has to catch.
	TestKeepBlobs bool
	// NameTakeoverAfter: a HELLO with the name of a connected player is name_taken unless
	// that player has been silent this long (or is zero-filled). Default 3 s.
	NameTakeoverAfter time.Duration
	// HousekeepInterval: granularity of the timers above. Default 10 ms.
	HousekeepInterval time.Duration
	// SendQueueSize: messages buffered per client before it is dropped. Default 4096.
	SendQueueSize int
	// SaveEvery: a changed canonical game is written to disk at most this often. Default 1 s.
	SaveEvery time.Duration
	// WebDir: a folder with the browser build of the game (index.html, soh.js, soh.wasm...). When set, a plain HTTP
	// GET on the port serves its files, so players can open http://<server>:<port>/ and play in the browser against
	// this same server. "" (the default) serves only the health line.
	WebDir string
}

func (c *Config) setDefaults() {
	def := func(d *time.Duration, v time.Duration) {
		if *d <= 0 {
			*d = v
		}
	}
	def(&c.HelloTimeout, 10*time.Second)
	def(&c.IdleTimeout, 30*time.Second)
	def(&c.WaitAfter, 250*time.Millisecond)
	def(&c.WaitRepeat, time.Second)
	def(&c.ZeroFillAfter, 10*time.Second)
	def(&c.SlowFillAfter, 100*time.Millisecond)
	def(&c.PingInterval, time.Second)
	def(&c.DelayInterval, 5*time.Second)
	def(&c.PlayerListInterval, 2*time.Second)
	def(&c.StatsInterval, 60*time.Second)
	def(&c.MemInterval, 60*time.Second)
	def(&c.NameTakeoverAfter, 3*time.Second)
	def(&c.HousekeepInterval, 10*time.Millisecond)
	def(&c.SaveEvery, time.Second)
	if c.SendQueueSize <= 0 {
		c.SendQueueSize = 4096
	}
	if c.Logger == nil {
		c.Logger = log.New(os.Stderr, "", log.LstdFlags|log.Lmicroseconds)
	}
}

// Server owns the rooms. Safe for concurrent use.
type Server struct {
	cfg      Config
	log      *log.Logger
	start    time.Time
	mu       sync.Mutex
	rooms    map[string]*Room
	nextID   uint32
	groupSeq uint32
	ln       net.Listener
	wg       sync.WaitGroup
	closing  bool
	conns    map[net.Conn]struct{}
	stop     chan struct{}
	stopOnce sync.Once

	pendingSaves []*saveJob // canonical games to write (outside the lock)

	keptBlobs [][]byte // (only with Config.TestKeepBlobs)
}

// Room is a set of clients that passed the same handshake, plus its lockstep groups.
type Room struct {
	Name           string
	ref            protocol.Hello // handshake of the first client; later clients must match
	proto          int            // protocol of the room (its first client)
	clients        map[uint32]*client
	groups         map[uint32]*group
	lastPlayerList time.Time

	// Protocol 3: the canonical shared game (nil until the first group is founded) and the world clock.
	canon     *canonical
	clockDayV uint32 // dayTime, 16 bits used
	clockAcc  time.Duration
	clockLast time.Time
}

// sceneGroups: protocol 3 room (one group per scene, canonical game, world clock).
func (r *Room) sceneGroups() bool { return r.proto >= 3 }

// noWait: protocol 4 (phase 5b, demo 0.5.1 on): a group never waits for a player who enters, comes back, reloads its
// state or is slow. Protocol 3 rooms (demo 0.5.0) keep the phase 5 behaviour their clients were written for.
func (r *Room) noWait() bool { return r.proto >= 4 }

func (r *Room) clockDay() uint16 { return uint16(r.clockDayV) }

func (r *Room) clockSet(v uint16) {
	r.clockDayV = uint32(v)
	r.clockAcc = 0
	if r.canon != nil {
		r.canon.dirty = true
	}
}

// clockTickPeriod: the world clock advances once per game tick of real time (20 Hz).
const clockTickPeriod = 50 * time.Millisecond

// advanceClockLocked: every 50 ms of real time the clock moves by the fastest rate among the groups (time runs while
// at least one group is in a scene where it runs, PLAN.md 2.11).
func (r *Room) advanceClockLocked(now time.Time) {
	if r.clockLast.IsZero() {
		r.clockLast = now
		return
	}
	r.clockAcc += now.Sub(r.clockLast)
	r.clockLast = now
	rate := 0
	for _, g := range r.groups {
		if g.hasConnected() && g.rate > rate {
			rate = g.rate
		}
	}
	for r.clockAcc >= clockTickPeriod {
		r.clockAcc -= clockTickPeriod
		if rate > 0 {
			r.clockDayV = (r.clockDayV + uint32(rate)) & 0xFFFF
		}
	}
}

type client struct {
	id       uint32
	name     string
	remote   string
	conn     transport
	room     *Room
	q        chan []byte
	done     chan struct{}
	doneOnce sync.Once
	lastRecv atomic.Int64 // UnixNano of the last received frame

	// Guarded by Server.mu.
	gone         bool               // removed from its room
	slot         *slot              // group slot while this connection owns it
	waitingGroup bool               // JOIN_GROUP answered with GROUP_WAIT, served later
	waitMsg      protocol.JoinGroup // the JOIN_GROUP that waits
	lastScene    int                // scene of its last JOIN_GROUP (-1 none)
	lastShared   []byte             // shared game of its last JOIN_GROUP in play (protocol 3)
	lastLayout   uint32
	nextPing     time.Time
	pingNonce    uint64
	pings        map[uint64]time.Time
	rtts         []float64 // ms, last rttSamples
}

// enqueue queues one encoded frame. Never blocks; a full queue drops the client.
func (s *Server) enqueue(c *client, frame []byte) {
	select {
	case c.q <- frame:
	default:
		if !c.gone {
			s.log.Printf("id=%d name=%q: send queue overflow (%d), dropping client", c.id, c.name, cap(c.q))
		}
		_ = c.conn.Close()
	}
}

func (s *Server) sendLocked(c *client, v any) {
	frame, err := protocol.Encode(v)
	if err != nil {
		s.log.Printf("encode %T: %v", v, err)
		return
	}
	s.enqueue(c, frame)
}

func (c *client) stopWriter() { c.doneOnce.Do(func() { close(c.done) }) }

func (s *Server) writer(c *client) {
	defer s.wg.Done()
	write := func(f []byte) bool {
		_ = c.conn.SetWriteDeadline(time.Now().Add(s.cfg.IdleTimeout))
		if err := c.conn.WriteFrame(f); err != nil {
			_ = c.conn.Close()
			return false
		}
		return true
	}
	for {
		select {
		case f := <-c.q:
			if !write(f) {
				return
			}
		case <-c.done:
			for { // flush what is already queued, then stop
				select {
				case f := <-c.q:
					if !write(f) {
						return
					}
				default:
					return
				}
			}
		}
	}
}

// New creates a server. Call Serve or ListenAndServe.
func New(cfg Config) *Server {
	cfg.setDefaults()
	return &Server{cfg: cfg, log: cfg.Logger, start: time.Now(), rooms: map[string]*Room{},
		conns: map[net.Conn]struct{}{}, stop: make(chan struct{})}
}

// PrepareDataDir makes sure the data directory (canonical saves, later phases) is writable.
func (s *Server) PrepareDataDir() error {
	if s.cfg.DataDir == "" {
		return nil
	}
	saves := filepath.Join(s.cfg.DataDir, "saves")
	if err := os.MkdirAll(saves, 0o755); err != nil {
		return err
	}
	probe := filepath.Join(s.cfg.DataDir, ".write_test")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return fmt.Errorf("data dir not writable: %w", err)
	}
	return os.Remove(probe)
}

// ListenAndServe listens on cfg.Listen and serves until Close.
func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.cfg.Listen)
	if err != nil {
		return err
	}
	return s.Serve(ln)
}

// Addr returns the listening address (after Serve started).
func (s *Server) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return nil
	}
	return s.ln.Addr()
}

// Serve accepts connections on ln.
func (s *Server) Serve(ln net.Listener) error {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		_ = ln.Close()
		return nil
	}
	s.ln = ln
	s.wg.Add(1)
	s.mu.Unlock()
	go s.housekeep()
	// The HTTP side of the same port (WebSocket clients and the health line, D-239): fed by sniffAndServe.
	httpLn := newChanListener(ln.Addr())
	httpSrv := &http.Server{Handler: s.httpHandler(), ReadHeaderTimeout: s.cfg.HelloTimeout, IdleTimeout: 5 * time.Second,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context { return context.WithValue(ctx, connKey{}, c) }}
	go func() { _ = httpSrv.Serve(httpLn) }()
	defer func() { _ = httpLn.Close() }()
	s.log.Printf("zmp-server %s protocol %d listening on %s (tcp and websocket)", s.cfg.BuildVersion, protocol.Version, ln.Addr())
	for {
		conn, err := ln.Accept()
		if err != nil {
			s.mu.Lock()
			closing := s.closing
			s.mu.Unlock()
			if closing {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return err
		}
		if tc, ok := conn.(*net.TCPConn); ok {
			_ = tc.SetNoDelay(true)
		}
		s.mu.Lock()
		if s.closing {
			s.mu.Unlock()
			_ = conn.Close()
			return nil
		}
		s.conns[conn] = struct{}{}
		s.wg.Add(1)
		s.mu.Unlock()
		go func() {
			defer s.wg.Done()
			s.sniffAndServe(conn, httpLn)
			s.mu.Lock()
			delete(s.conns, conn)
			s.mu.Unlock()
		}()
	}
}

// Close stops the listener, writes the canonical games and disconnects everyone.
func (s *Server) Close() {
	s.mu.Lock()
	var jobs []*saveJob
	for _, r := range s.rooms {
		if job := s.canonicalJobLocked(r, time.Now()); job != nil {
			jobs = append(jobs, job)
		}
	}
	jobs = append(jobs, s.pendingSaves...)
	s.pendingSaves = nil
	s.mu.Unlock()
	s.writeJobs(jobs)
	s.mu.Lock()
	s.closing = true
	if s.ln != nil {
		_ = s.ln.Close()
	}
	for c := range s.conns {
		_ = c.Close()
	}
	s.mu.Unlock()
	s.stopOnce.Do(func() { close(s.stop) })
	s.wg.Wait()
}

// RoomSnapshot is used by tests and logs.
type RoomSnapshot struct {
	Name    string
	Players []protocol.PlayerInfo
}

// Rooms returns a sorted snapshot of all rooms.
func (s *Server) Rooms() []RoomSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []RoomSnapshot
	for _, r := range s.rooms {
		out = append(out, RoomSnapshot{Name: r.Name, Players: r.playersLocked()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (c *client) rttStats() (mean, jitter float64, ok bool) {
	if len(c.rtts) == 0 {
		return 0, 0, false
	}
	for _, v := range c.rtts {
		mean += v
	}
	mean /= float64(len(c.rtts))
	for _, v := range c.rtts {
		jitter += math.Abs(v - mean)
	}
	jitter /= float64(len(c.rtts))
	return mean, jitter, true
}

// playersLocked lists the room's connections and the group slots whose connection is gone. Slot, leader and state
// are relative to each player's own group; scene and group tell the groups apart (protocol 3).
func (r *Room) playersLocked() []protocol.PlayerInfo {
	out := make([]protocol.PlayerInfo, 0, len(r.clients)+2)
	for _, c := range r.clients {
		p := protocol.PlayerInfo{ID: c.id, Name: c.name, Slot: -1, RTT: -1, State: protocol.StateLobby,
			Scene: c.lastScene}
		if mean, _, ok := c.rttStats(); ok {
			p.RTT = int(math.Round(mean))
		}
		if c.waitingGroup {
			p.State = protocol.StateWaiting
		}
		if c.slot != nil {
			g := c.slot.g
			p.Slot = c.slot.index
			p.State = g.slotState(c.slot)
			p.Leader = g.leader == c.slot.index
			p.Group = g.id
			if r.sceneGroups() {
				p.Scene = g.scene
			}
		}
		out = append(out, p)
	}
	for _, g := range r.sortedGroups() {
		for _, sl := range g.slots {
			if sl != nil && sl.client == nil {
				out = append(out, protocol.PlayerInfo{ID: sl.lastID, Name: sl.name, Slot: sl.index, RTT: -1,
					State: protocol.StateDisconnected, Scene: g.scene, Group: g.id})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// playerListLocked sends PLAYER_LIST to every client of the room with the input delay and last tick of its own group.
func (s *Server) playerListLocked(r *Room) {
	players := r.playersLocked()
	var common []byte
	for _, c := range r.sortedClients() {
		if c.slot == nil {
			if common == nil {
				f, err := protocol.Encode(protocol.PlayerList{T: "PLAYER_LIST", Players: players, Delay: InitialDelay,
					GroupTick: -1})
				if err != nil {
					return
				}
				common = f
			}
			s.enqueue(c, common)
			continue
		}
		g := c.slot.g
		f, err := protocol.Encode(protocol.PlayerList{T: "PLAYER_LIST", Players: players, Delay: g.delay,
			GroupTick: g.nextTick - 1})
		if err != nil {
			return
		}
		s.enqueue(c, f)
	}
	r.lastPlayerList = time.Now()
}

// newRoomLocked creates a room for the first client's handshake; a protocol 3 room gets its canonical game from disk.
func (s *Server) newRoomLocked(hello protocol.Hello) *Room {
	room := &Room{Name: hello.Room, ref: hello, proto: hello.Proto, clients: map[uint32]*client{},
		groups: map[uint32]*group{}}
	if room.sceneGroups() {
		if c := s.loadCanonical(room.Name); c != nil {
			room.canon = c
			room.clockDayV = uint32(c.DayTime)
		} else {
			room.clockDayV = 0x8000 // noon
		}
	}
	s.rooms[hello.Room] = room
	return room
}

func (r *Room) sortedClients() []*client {
	out := make([]*client, 0, len(r.clients))
	for _, c := range r.clients {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

// checkHandshake compares a newcomer with the room reference. Returns reason and detail.
func checkHandshake(ref, h protocol.Hello) (string, string) {
	if h.BuildHash != ref.BuildHash {
		return "build_mismatch", fmt.Sprintf("Tu version del juego (%s) no coincide con la de la sala (%s).", short(h.BuildHash), short(ref.BuildHash))
	}
	if h.OotHash != ref.OotHash {
		// Phase 5b: the versions by name. The check itself is on the content: two versions whose simulation
		// data and resource list are the same have the same hash and play together.
		if h.RomName != "" && ref.RomName != "" && h.RomName != ref.RomName {
			return "oot_mismatch", fmt.Sprintf("Tu ROM es %s y la sala usa %s. Todos los jugadores de una sala tienen que generar los assets desde la misma version de ROM.", h.RomName, ref.RomName)
		}
		if h.RomName != "" && h.RomName == ref.RomName {
			return "oot_mismatch", fmt.Sprintf("Tu ROM y la de la sala son %s, pero el contenido de tu oot.o2r (%s) no coincide con el de la sala (%s). Vuelve a generar los assets desde la ROM original, sin mods.", h.RomName, short(h.OotHash), short(ref.OotHash))
		}
		return "oot_mismatch", fmt.Sprintf("Tu oot.o2r (%s) no coincide con el de la sala (%s). Genera los assets desde la misma ROM.", short(h.OotHash), short(ref.OotHash))
	}
	if h.SohHash != ref.SohHash {
		return "soh_mismatch", fmt.Sprintf("Tu soh.o2r (%s) no coincide con el de la sala (%s).", short(h.SohHash), short(ref.SohHash))
	}
	if h.CvarProfileHash != ref.CvarProfileHash {
		return "cvar_profile_mismatch", "Tu perfil de ajustes de juego no coincide con el de la sala."
	}
	if h.RandoSeed != ref.RandoSeed {
		return "rando_seed_mismatch", fmt.Sprintf("Tu semilla de randomizer (%d) no coincide con la de la sala (%d).", h.RandoSeed, ref.RandoSeed)
	}
	return "", ""
}

func short(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// writeNow encodes v and writes it on the connection at once (before a client has its writer: the REJECTs).
func writeNow(conn transport, v any) error {
	frame, err := protocol.Encode(v)
	if err != nil {
		return err
	}
	return conn.WriteFrame(frame)
}

func (s *Server) handle(conn transport) {
	remote := conn.RemoteAddr()
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(s.cfg.HelloTimeout))
	body, err := conn.ReadFrame()
	if err != nil {
		// A bare connect+close (Docker healthcheck, port probe) is not worth a log line.
		if !errors.Is(err, io.EOF) {
			s.log.Printf("%s: no HELLO: %v", remote, err)
		}
		return
	}
	var hello protocol.Hello
	if t, err := protocol.Type(body); err != nil || t != "HELLO" || protocol.Decode(body, &hello) != nil {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_ = writeNow(conn, protocol.Reject{T: "REJECT", Reason: "bad_hello", Detail: "El primer mensaje debe ser HELLO."})
		s.log.Printf("%s: rejected (bad_hello)", remote)
		return
	}
	reject := func(reason, detail string) {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_ = writeNow(conn, protocol.Reject{T: "REJECT", Reason: reason, Detail: detail})
		s.log.Printf("%s: rejected name=%q room=%q reason=%s", remote, hello.Name, hello.Room, reason)
	}
	if hello.Proto < protocol.MinVersion || hello.Proto > protocol.Version {
		reject("protocol_mismatch", fmt.Sprintf("Version de protocolo %d; el servidor usa de %d a %d. Actualiza el juego.",
			hello.Proto, protocol.MinVersion, protocol.Version))
		return
	}
	if hello.Name == "" || len(hello.Name) > 32 {
		reject("bad_name", "El nombre debe tener entre 1 y 32 caracteres.")
		return
	}
	if hello.Room == "" || len(hello.Room) > 64 {
		reject("bad_room", "El nombre de la sala debe tener entre 1 y 64 caracteres.")
		return
	}

	now := time.Now()
	c := &client{name: hello.Name, remote: remote, conn: conn, q: make(chan []byte, s.cfg.SendQueueSize),
		done: make(chan struct{}), pings: map[uint64]time.Time{}, nextPing: now}
	c.lastRecv.Store(now.UnixNano())

	s.mu.Lock()
	room, ok := s.rooms[hello.Room]
	if ok && room.proto != hello.Proto {
		s.mu.Unlock()
		reject("protocol_mismatch", fmt.Sprintf("La sala usa la version de protocolo %d y tu juego la %d: usad todos la misma version del juego.",
			room.proto, hello.Proto))
		return
	}
	if ok {
		if reason, detail := checkHandshake(room.ref, hello); reason != "" {
			s.mu.Unlock()
			reject(reason, detail)
			return
		}
		for _, other := range room.sortedClients() {
			if other.name != hello.Name {
				continue
			}
			idle := now.Sub(time.Unix(0, other.lastRecv.Load()))
			marked := other.slot != nil && other.slot.filled && other.slot.resumeAt < 0
			if idle < s.cfg.NameTakeoverAfter && !marked {
				s.mu.Unlock()
				reject("name_taken", fmt.Sprintf("Ya hay un jugador llamado %q en la sala.", hello.Name))
				return
			}
			s.log.Printf("%s: name=%q room=%q takes over id=%d (silent %v, zero-filled %v)", remote, hello.Name,
				room.Name, other.id, idle.Round(time.Millisecond), marked)
			s.removeClientLocked(other, "replaced by a new connection")
			_ = other.conn.Close()
		}
		if _, still := s.rooms[hello.Room]; !still { // the takeover emptied the room
			room = s.newRoomLocked(hello)
		}
		if len(room.clients) >= MaxPlayersPerRoom {
			s.mu.Unlock()
			reject("room_full", fmt.Sprintf("La sala esta llena (%d jugadores).", MaxPlayersPerRoom))
			return
		}
	} else {
		room = s.newRoomLocked(hello)
	}
	s.nextID++
	c.id = s.nextID
	c.room = room
	room.clients[c.id] = c
	c.lastScene = -1
	rejoin := room.slotByName(c.name) != nil
	s.wg.Add(1)
	go s.writer(c)
	s.sendLocked(c, protocol.Welcome{T: "WELCOME", PlayerID: c.id, Room: room.Name, Players: room.playersLocked(),
		Proto: room.proto, Rejoin: rejoin, LeaveShared: true})
	s.playerListLocked(room)
	s.mu.Unlock()
	s.log.Printf("%s: joined room id=%d name=%q room=%q build=%s rejoin=%v", remote, c.id, c.name, room.Name,
		short(hello.BuildHash), rejoin)

	for {
		_ = conn.SetReadDeadline(time.Now().Add(s.cfg.IdleTimeout))
		body, err := conn.ReadFrame()
		if err != nil {
			s.mu.Lock()
			gone := c.gone
			s.mu.Unlock()
			if !errors.Is(err, io.EOF) && !gone {
				s.log.Printf("%s: id=%d read: %v", remote, c.id, err)
			}
			break
		}
		c.lastRecv.Store(time.Now().UnixNano())
		if len(body) >= 1<<20 {
			// (finding F: a frame this big is memory the process asks for at once; none is expected)
			s.log.Printf("%s: id=%d BIG FRAME of %d bytes", remote, c.id, len(body))
		}
		t, err := protocol.Type(body)
		if err != nil {
			s.log.Printf("%s: id=%d %v", remote, c.id, err)
			break
		}
		s.dispatch(c, t, body)
	}
	s.mu.Lock()
	s.removeClientLocked(c, "connection closed")
	s.mu.Unlock()
	c.stopWriter()
}

func (s *Server) dispatch(c *client, t string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.gone {
		return
	}
	switch t {
	case "PING":
		var p protocol.Ping
		if protocol.Decode(body, &p) == nil {
			s.sendLocked(c, protocol.Ping{T: "PONG", Nonce: p.Nonce, SentAt: p.SentAt})
		}
	case "PONG":
		var p protocol.Ping
		if protocol.Decode(body, &p) == nil {
			if sent, ok := c.pings[p.Nonce]; ok {
				delete(c.pings, p.Nonce)
				rtt := float64(time.Since(sent).Microseconds()) / 1000
				c.rtts = append(c.rtts, rtt)
				if len(c.rtts) > rttSamples {
					c.rtts = c.rtts[len(c.rtts)-rttSamples:]
				}
			}
		}
	case "JOIN_GROUP":
		var m protocol.JoinGroup
		if protocol.Decode(body, &m) == nil {
			s.joinGroupLocked(c, m)
		}
	case "LEAVE_GROUP":
		var m protocol.LeaveGroup
		if protocol.Decode(body, &m) == nil && len(m.Shared) > 0 {
			s.leaveSharedLocked(c, m)
		}
		s.leaveGroupLocked(c)
	case "INPUT":
		var m protocol.Input
		if protocol.Decode(body, &m) == nil {
			s.inputLocked(c, m)
		}
	case "HASH":
		var m protocol.Hash
		if protocol.Decode(body, &m) == nil {
			s.hashLocked(c, m)
		}
	case "STATE_BLOB":
		var m protocol.StateBlobHeader
		if protocol.Decode(body, &m) == nil {
			s.stateBlobLocked(c, m, body)
		}
	case "EVENT":
		var m protocol.ClientEvent
		if protocol.Decode(body, &m) == nil && m.Kind == "CONSOLE" && len(m.Payload) > 0 && len(m.Payload) < 256 {
			if c.slot != nil && c.slot.client == c {
				g := c.slot.g
				T := g.nextTick
				g.events[T] = append(g.events[T], protocol.Event{K: "CONSOLE", Slot: c.slot.index, Cmd: m.Payload})
				s.log.Printf("%s: CONSOLE event from slot %d at tick %d: %s", s.groupLabel(g), c.slot.index, T, m.Payload)
			}
		}
	case "GROUP_SCENE":
		var m protocol.GroupScene
		if protocol.Decode(body, &m) == nil {
			s.groupSceneLocked(c, m)
		}
	case "GROUP_OPEN":
		s.groupOpenLocked(c)
	case "SHARED":
		var m protocol.Shared
		if protocol.Decode(body, &m) == nil {
			s.sharedLocked(c, m)
		}
	case "CLOCK":
		var m protocol.Clock
		if protocol.Decode(body, &m) == nil {
			s.clockLocked(c, m)
		}
	default:
		// Unknown types are ignored for forward compatibility within the same version.
	}
}

// removeClientLocked takes a connection out of its room (idempotent).
func (s *Server) removeClientLocked(c *client, why string) {
	if c.gone {
		return
	}
	c.gone = true
	room := c.room
	delete(room.clients, c.id)
	s.log.Printf("left room id=%d name=%q room=%q (%s)", c.id, c.name, room.Name, why)
	if c.slot != nil && c.slot.client == c {
		s.detachSlotLocked(c.slot)
	}
	c.slot = nil
	c.waitingGroup = false
	if len(room.clients) == 0 {
		for _, g := range room.sortedGroups() {
			s.destroyGroupLocked(g, "room empty")
		}
		if job := s.canonicalJobLocked(room, time.Now()); job != nil {
			s.pendingSaves = append(s.pendingSaves, job)
		}
		delete(s.rooms, room.Name)
		return
	}
	s.playerListLocked(room)
}

func (s *Server) housekeep() {
	defer s.wg.Done()
	t := time.NewTicker(s.cfg.HousekeepInterval)
	defer t.Stop()
	nextMem := time.Now().Add(s.cfg.MemInterval)
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			now := time.Now()
			if !now.Before(nextMem) {
				nextMem = now.Add(s.cfg.MemInterval)
				s.logMemory()
			}
			s.mu.Lock()
			for _, r := range s.rooms {
				s.housekeepRoomLocked(r, now)
			}
			jobs := s.pendingSaves
			s.pendingSaves = nil
			s.mu.Unlock()
			s.writeJobs(jobs)
		}
	}
}

// logMemory writes one line with the memory of the whole process (finding F: the process was once seen with 48 MB
// in RAM while the last group line said Go had 16 MB from the system; that line is only written by groups that live
// a minute, so nothing said what the memory was when it was measured). What Go has (heap in use, heap it keeps
// idle, heap given back, everything it got from the system, stacks, collections so far) next to what the operating
// system counts (resident: what tasklist and docker stats show; private), and how much there is to hold. live_kb
// is the heap right after a collection (-1 unless Config.MemCollect): what the server holds.
func (s *Server) logMemory() {
	live := int64(-1)
	var m runtime.MemStats
	if s.cfg.MemCollect {
		runtime.GC()
		runtime.ReadMemStats(&m)
		live = int64(m.HeapAlloc / 1024)
	} else {
		runtime.ReadMemStats(&m)
	}
	resident, private := processMemoryKB()
	s.mu.Lock()
	rooms, clients, groups, queued := len(s.rooms), 0, 0, 0
	for _, r := range s.rooms {
		clients += len(r.clients)
		groups += len(r.groups)
		for _, c := range r.clients {
			queued += len(c.q)
		}
	}
	s.mu.Unlock()
	s.log.Printf("mem live_kb=%d resident_kb=%d private_kb=%d go_sys_kb=%d heap_alloc_kb=%d heap_inuse_kb=%d heap_idle_kb=%d "+
		"heap_released_kb=%d stack_kb=%d gc=%d goroutines=%d rooms=%d groups=%d clients=%d queued_frames=%d",
		live, resident, private, m.Sys/1024, m.HeapAlloc/1024, m.HeapInuse/1024, m.HeapIdle/1024, m.HeapReleased/1024,
		m.StackSys/1024, m.NumGC, runtime.NumGoroutine(), rooms, groups, clients, queued)
}

func (s *Server) housekeepRoomLocked(r *Room, now time.Time) {
	for _, c := range r.sortedClients() {
		if now.Before(c.nextPing) {
			continue
		}
		c.nextPing = now.Add(s.cfg.PingInterval)
		c.pingNonce++
		c.pings[c.pingNonce] = now
		for n, at := range c.pings { // forget unanswered pings
			if now.Sub(at) > 30*time.Second || c.pingNonce-n >= 2*rttSamples {
				delete(c.pings, n)
			}
		}
		s.sendLocked(c, protocol.Ping{T: "PING", Nonce: c.pingNonce, SentAt: uint64(now.Sub(s.start).Milliseconds())})
	}
	for _, g := range r.sortedGroups() {
		if r.groups[g.id] == g {
			s.housekeepGroupLocked(g, now)
		}
	}
	if r.sceneGroups() {
		r.advanceClockLocked(now)
		if r.canon != nil && !r.canon.dirty && now.Sub(r.canon.savedAt) >= 60*time.Second && len(r.groups) > 0 {
			r.canon.dirty = true // the clock moved: keep it on disk every minute
		}
		if r.canon != nil && r.canon.dirty && now.Sub(r.canon.savedAt) >= s.cfg.SaveEvery {
			if job := s.canonicalJobLocked(r, now); job != nil {
				s.pendingSaves = append(s.pendingSaves, job)
			}
		}
	}
	if now.Sub(r.lastPlayerList) >= s.cfg.PlayerListInterval {
		s.playerListLocked(r)
	}
}

// ComputeDelay is the input delay formula of docs/PROTOCOLO.md (rule 11):
// D = clamp(ceil((RTT_max/2 + jitter_max) / 50) + 1, 1, 6), with times in ms.
func ComputeDelay(rttMaxMs, jitterMaxMs float64) int {
	// Input for tick t+D must make the round trip (client -> server -> every client) within D ticks of
	// 50 ms, with twice the jitter and a 20 ms margin for the client's frame granularity (D-026; PLAN.md
	// 2.13's "+1" never allowed D=1 on a LAN).
	d := int(math.Ceil((rttMaxMs + 2*jitterMaxMs + 20) / 50))
	if d < MinDelay {
		d = MinDelay
	}
	if d > MaxDelay {
		d = MaxDelay
	}
	return d
}
