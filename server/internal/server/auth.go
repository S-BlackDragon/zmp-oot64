package server

// Protocol 5 (the protocol 5 design notes): rejections with arguments, the password challenge, identities, the room
// list, rename, colour and the room key in session, and the limits of a relay.

import (
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/S-BlackDragon/zmp-oot64/server/internal/protocol"
	"github.com/S-BlackDragon/zmp-oot64/server/internal/roomauth"
)

// rejection is a REJECT with its two texts: es for protocols 2 to 4 (what their clients show as it is), en for
// protocol 5 (whose clients show their own text by reason; the detail is for logs), and the arguments of 5.
type rejection struct {
	reason string
	es, en string
	args   map[string]string
}

func rej(reason, es, en string, kv ...string) *rejection {
	r := &rejection{reason: reason, es: es, en: en}
	if len(kv) > 0 {
		r.args = map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			r.args[kv[i]] = kv[i+1]
		}
	}
	return r
}

func (r *rejection) message(proto int) protocol.Reject {
	if proto >= 5 {
		return protocol.Reject{T: "REJECT", Reason: r.reason, Detail: r.en, Args: r.args}
	}
	return protocol.Reject{T: "REJECT", Reason: r.reason, Detail: r.es}
}

// identity of a HELLO: its player_key (protocol 5) or its name (every older client, and 5 without a key).
func identity(h protocol.Hello) string {
	if h.PlayerKey != "" {
		return "k:" + h.PlayerKey
	}
	return "n:" + h.Name
}

// validPlayerKey: 32 lowercase hexadecimal characters.
func validPlayerKey(k string) bool {
	if len(k) != 32 {
		return false
	}
	for i := 0; i < len(k); i++ {
		if !(k[i] >= '0' && k[i] <= '9' || k[i] >= 'a' && k[i] <= 'f') {
			return false
		}
	}
	return true
}

// colorOf: the colour of a HELLO or SET_COLOR, -1 when absent or out of range.
func colorOf(c *int) int {
	if c == nil || *c < 0 || *c > 0xFFFFFF {
		return -1
	}
	return *c
}

// hostOf gives the address part of a transport's RemoteAddr ("1.2.3.4:5678" or "1.2.3.4:5678 (ws)").
func hostOf(remote string) string {
	remote = strings.TrimSuffix(remote, " (ws)")
	if h, _, err := net.SplitHostPort(remote); err == nil {
		return h
	}
	return remote
}

// IsPublicAddress says whether ip is a public internet address: not loopback, private, link-local, CGNAT
// (100.64.0.0/10), unique-local IPv6, multicast or unspecified.
func IsPublicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	if ip.Is4() {
		b := ip.As4()
		if b[0] == 100 && b[1]&0xC0 == 64 { // 100.64.0.0/10, shared address space (CGNAT)
			return false
		}
		if b[0] == 0 || b[0] >= 240 || b[0] == 192 && b[1] == 0 && b[2] == 0 { // this network, reserved, IETF
			return false
		}
	}
	return true
}

// trackConn counts a game connection of ip; false if that address already has the most a relay allows.
func (s *Server) trackConn(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ipConns[ip]++
	return s.cfg.MaxConnsPerIP <= 0 || s.ipConns[ip] <= s.cfg.MaxConnsPerIP
}

func (s *Server) untrackConn(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ipConns[ip]--; s.ipConns[ip] <= 0 {
		delete(s.ipConns, ip)
	}
}

// roomAllowed: a server that hosts only its configured rooms (the host app) refuses the others.
func (s *Server) roomAllowed(room string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.allowedRooms) == 0 {
		return true
	}
	return s.allowedRooms[room]
}

// SetAllowedRooms makes the server host only these rooms (nil or empty: any). Rooms already open stay open.
func (s *Server) SetAllowedRooms(rooms []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowedRooms = nil
	if len(rooms) > 0 {
		s.allowedRooms = map[string]bool{}
		for _, r := range rooms {
			s.allowedRooms[r] = true
		}
	}
}

// SetFixedRoomPassword gives room a password set by the server's owner (the host app's page, its configuration):
// no client can change it, it never expires and it is not written to room_keys.json (it comes from the
// configuration at each start). "" removes the password.
func (s *Server) SetFixedRoomPassword(room, password string) error {
	return s.keys.Set(room, roomauth.RoomKey(room, password), "", true, time.Now())
}

// RoomHasPassword says whether room has a key now.
func (s *Server) RoomHasPassword(room string) bool { return s.keys.Get(room) != nil }

// roomsCreatedByLocked counts the open rooms that ip created.
func (s *Server) roomsCreatedByLocked(ip string) int {
	n := 0
	for _, r := range s.rooms {
		if r.creatorIP == ip {
			n++
		}
	}
	return n
}

// challengeFor is the pre-lock half of the password check (protocol 5 notes 2.2): what the CHALLENGE asks for,
// or a rejection. need is false when no CHALLENGE is needed.
func (s *Server) challengeFor(h protocol.Hello) (ch protocol.Challenge, need bool, r *rejection) {
	s.mu.Lock()
	_, exists := s.rooms[h.Room]
	s.mu.Unlock()
	roomKey := s.keys.Get(h.Room)
	if h.Proto < 5 {
		if s.serverKey != nil {
			return ch, false, rej("server_password_required",
				"Este servidor tiene contrasena: actualiza el juego para entrar.", "")
		}
		if roomKey != nil {
			return ch, false, rej("password_required",
				"Esta sala tiene contrasena: actualiza el juego para entrar.", "")
		}
		return ch, false, nil
	}
	ch = protocol.Challenge{T: "CHALLENGE", Server: s.serverKey != nil, Room: roomKey != nil,
		Create: roomKey == nil && !exists && h.HasPassword}
	if ch.Room && !h.HasPassword && !ch.Server {
		return ch, false, rej("password_required", "", "This room has a password.")
	}
	if !ch.Server && !ch.Room && !ch.Create {
		return ch, false, nil
	}
	ch.Nonce = roomauth.Nonce()
	return ch, true, nil
}

// checkServerProof checks the server half of an AUTH (outside the lock).
func (s *Server) checkServerProof(h protocol.Hello, ch protocol.Challenge, a *protocol.Auth) (confirm []byte, r *rejection) {
	if !ch.Server {
		return nil, nil
	}
	if len(a.ServerProof) == 0 {
		return nil, rej("server_password_required", "", "This server has a password.")
	}
	want := roomauth.Proof(s.serverKey, roomauth.KindServer, ch.Nonce, h.AuthNonce, h.Room, h.Name)
	if !roomauth.Equal(a.ServerProof, want) {
		return nil, rej("bad_server_password", "", "Wrong server password.")
	}
	return roomauth.Confirm(s.serverKey, roomauth.KindServer, ch.Nonce, h.AuthNonce), nil
}

// roomAuthLocked is the room half, under the lock with the room as it is now (another client may have made it
// since the CHALLENGE). It gives the key to set if this client creates the room, and the confirmation.
func (s *Server) roomAuthLocked(h protocol.Hello, ch protocol.Challenge, a *protocol.Auth, exists bool) (newKey, confirm []byte, r *rejection) {
	cur := s.keys.Get(h.Room)
	if cur != nil {
		switch {
		case h.Proto < 5:
			return nil, nil, rej("password_required", "Esta sala tiene contrasena: actualiza el juego para entrar.", "")
		case a == nil || len(a.RoomProof) == 0 && len(a.RoomKey) == 0:
			return nil, nil, rej("password_required", "", "This room has a password.")
		case len(a.RoomProof) > 0:
			want := roomauth.Proof(cur, roomauth.KindRoom, ch.Nonce, h.AuthNonce, h.Room, h.Name)
			if !roomauth.Equal(a.RoomProof, want) {
				return nil, nil, rej("bad_password", "", "Wrong room password.")
			}
		default: // room_key for a room another client made with a key since the CHALLENGE
			if !roomauth.Equal(a.RoomKey, cur) {
				return nil, nil, rej("bad_password", "", "Wrong room password.")
			}
		}
		return nil, roomauth.Confirm(cur, roomauth.KindRoom, ch.Nonce, h.AuthNonce), nil
	}
	if exists {
		return nil, nil, nil // a room without a password: in, whatever the client typed
	}
	if a != nil && len(a.RoomKey) > 0 {
		if len(a.RoomKey) != roomauth.KeySize {
			return nil, nil, rej("bad_hello", "", "room_key must be 32 bytes.")
		}
		newKey = a.RoomKey
	}
	if newKey == nil && s.cfg.RequireRoomPassword {
		return nil, nil, rej("password_required", "Este servidor solo crea salas con contrasena: actualiza el juego.",
			"This server only makes rooms with a password.", "create", "1")
	}
	if newKey != nil {
		confirm = roomauth.Confirm(newKey, roomauth.KindRoom, ch.Nonce, h.AuthNonce)
	}
	return newKey, confirm, nil
}

// serveRoomList answers a ROOM_LIST_REQUEST (a first message instead of HELLO) and closes.
func (s *Server) serveRoomList(conn transport, body []byte) {
	var req protocol.RoomListRequest
	_ = protocol.Decode(body, &req)
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if !s.cfg.ListRooms || s.serverKey != nil {
		_ = writeNow(conn, rej("rooms_hidden", "", "This server does not list its rooms.").message(5))
		return
	}
	s.mu.Lock()
	list := protocol.RoomList{T: "ROOM_LIST", ServerName: s.cfg.ServerName, Rooms: []protocol.RoomListEntry{}}
	for _, r := range s.rooms {
		list.Rooms = append(list.Rooms, protocol.RoomListEntry{Name: r.Name, Players: len(r.clients),
			Max: MaxPlayersPerRoom, Proto: r.proto, Engine: r.ref.Engine, BuildHash: r.ref.BuildHash})
	}
	s.mu.Unlock()
	for i := range list.Rooms {
		list.Rooms[i].Password = s.keys.Get(list.Rooms[i].Name) != nil
	}
	sort.Slice(list.Rooms, func(i, j int) bool { return list.Rooms[i].Name < list.Rooms[j].Name })
	_ = writeNow(conn, list)
}

// renameLocked serves RENAME (protocol 5).
func (s *Server) renameLocked(c *client, m protocol.Rename) {
	room := c.room
	res := protocol.RenameResult{T: "RENAME_RESULT", Name: c.name}
	switch {
	case m.Name == "" || len(m.Name) > 32:
		res.Reason = "bad_name"
	case m.Name == c.name:
		res.OK = true
	default:
		for _, o := range room.clients {
			if o != c && o.name == m.Name {
				res.Reason = "name_taken"
			}
		}
	}
	if res.Reason != "" {
		s.sendLocked(c, res)
		return
	}
	if m.Name != c.name {
		s.log.Printf("room=%q id=%d renamed %q -> %q", room.Name, c.id, c.name, m.Name)
		if c.slot != nil && c.slot.client == c {
			c.slot.name = m.Name
		}
		if c.playerKey == "" { // (without a key the name is the identity: it moves with the name)
			old := c.ident
			c.ident = "n:" + m.Name
			if sl := room.slotByIdent(old); sl != nil {
				sl.ident = c.ident
			}
		}
		c.name = m.Name
	}
	res.OK, res.Name = true, c.name
	s.sendLocked(c, res)
	s.playerListLocked(room)
}

// setColorLocked serves SET_COLOR (protocol 5).
func (s *Server) setColorLocked(c *client, m protocol.SetColor) {
	if m.Color < -1 || m.Color > 0xFFFFFF {
		return
	}
	c.color = m.Color
	if c.slot != nil && c.slot.client == c {
		c.slot.color = m.Color
	}
	s.playerListLocked(c.room)
}

// isOwnerLocked: c made its room with its player_key (and the key, if any, is not the server's own).
func (s *Server) isOwnerLocked(c *client) bool {
	if c.playerKey == "" {
		return false
	}
	id := roomauth.OwnerID(c.ident)
	if e := s.keys.Info(c.room.Name); e != nil {
		return !e.Fixed && e.Owner == id
	}
	return c.room.owner == id
}

// roomSetKeyLocked serves ROOM_SET_KEY (protocol 5).
func (s *Server) roomSetKeyLocked(c *client, m protocol.RoomSetKey) {
	res := protocol.RoomSetKeyResult{T: "ROOM_SET_KEY_RESULT"}
	e := s.keys.Info(c.room.Name)
	switch {
	case e != nil && e.Fixed:
		res.Reason = "fixed"
	case !s.isOwnerLocked(c):
		res.Reason = "not_owner"
	case len(m.RoomKey) != 0 && len(m.RoomKey) != roomauth.KeySize:
		res.Reason = "bad_key"
	case len(m.RoomKey) == 0 && s.cfg.RequireRoomPassword:
		res.Reason = "password_required"
	}
	if res.Reason == "" {
		if err := s.keys.Set(c.room.Name, m.RoomKey, roomauth.OwnerID(c.ident), false, time.Now()); err != nil {
			s.log.Printf("room=%q: room key not saved: %v", c.room.Name, err)
		}
		res.OK = true
		s.log.Printf("room=%q id=%d name=%q set the room's password (%v)", c.room.Name, c.id, c.name, len(m.RoomKey) > 0)
	}
	s.sendLocked(c, res)
	if res.OK {
		s.playerListLocked(c.room)
	}
}

// PlayerSnapshot is one connected player for the host app's page.
type PlayerSnapshot struct {
	Room   string `json:"room"`
	Name   string `json:"name"`
	State  string `json:"state"`
	RTT    int    `json:"rtt"`
	Color  int    `json:"color"`
	Remote string `json:"-"` // (the address the connection came from: only for the reachability check)
}

// Players lists the connected players of every room (sorted by room and name).
func (s *Server) Players() []PlayerSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []PlayerSnapshot
	for _, r := range s.rooms {
		infos := map[uint32]protocol.PlayerInfo{}
		for _, p := range r.playersLocked() {
			infos[p.ID] = p
		}
		for _, c := range r.clients {
			p := infos[c.id]
			out = append(out, PlayerSnapshot{Room: r.Name, Name: c.name, State: p.State, RTT: p.RTT, Color: c.color,
				Remote: hostOf(c.remote)})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Room != out[j].Room {
			return out[i].Room < out[j].Room
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func itoa(n int) string { return strconv.Itoa(n) }
