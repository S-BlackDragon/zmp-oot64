// Package protocol implements the ZMP wire format (PLAN.md 8.1): every frame is a
// uint32 little-endian body length followed by a MessagePack map with a "t" field.
package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/vmihailenco/msgpack/v5"
)

// Version is incremented on any incompatible change. The server accepts MinVersion..Version; a room keeps the
// version of its first client. Version 2 adds lockstep groups (one per room); version 3 (phase 5) one group per scene,
// the canonical shared game and the world clock (docs/PROTOCOLO.md); version 4 (phase 5b) nobody waits for anybody;
// version 5 (public product, the protocol 5 design notes) room and server passwords by challenge and response, the
// player_key identity, rename and colour in session, the engine and the room list. The simulation of 5 is that of 4.
const Version = 5

// MinVersion is the oldest protocol still served (demos 0.2.x to 0.4.0 speak 2).
const MinVersion = 2

// MaxFrameSize bounds a single message (save state blobs will be well below this).
const MaxFrameSize = 16 << 20

// ErrFrameTooLarge is returned when a peer announces a frame above MaxFrameSize.
var ErrFrameTooLarge = errors.New("frame too large")

// Header is decoded first to dispatch on the message type.
type Header struct {
	T string `msgpack:"t"`
}

// Hello is the first message of a client.
type Hello struct {
	T               string `msgpack:"t"`
	Proto           int    `msgpack:"proto"`
	BuildHash       string `msgpack:"build_hash"`
	OotHash         string `msgpack:"oot_hash"`
	SohHash         string `msgpack:"soh_hash"`
	CvarProfileHash string `msgpack:"cvar_profile_hash"`
	RandoSeed       uint32 `msgpack:"rando_seed"`
	Name            string `msgpack:"name"`
	Room            string `msgpack:"room"`
	ClientNonce     uint64 `msgpack:"client_nonce"`
	// RomName: the ROM version the assets were made from, by name ("PAL GameCube Debug", "NTSC N64 1.2"...).
	// Only for the reject message (phase 5b); the check is on the content hashes. Optional.
	RomName string `msgpack:"rom_name,omitempty"`

	// Protocol 5 (protocol 5 notes 2.1). AuthNonce: 16 random bytes of this connection (required). HasPassword: the
	// player typed a room password. PlayerKey: 32 lowercase hex characters, the player's stable identity (optional:
	// without it the identity is the name). Color: 0xRRGGBB, -1 or absent for the slot's own. Engine: "native" or
	// "wasm" ("" unchecked).
	AuthNonce   []byte `msgpack:"auth_nonce,omitempty"`
	HasPassword bool   `msgpack:"has_password,omitempty"`
	PlayerKey   string `msgpack:"player_key,omitempty"`
	Color       *int   `msgpack:"color,omitempty"`
	Engine      string `msgpack:"engine,omitempty"`
}

// Challenge (S->C, protocol 5): the HELLO needs a proof of a key (server, room) or the key of a new room (create).
type Challenge struct {
	T      string `msgpack:"t"`
	Nonce  []byte `msgpack:"nonce"`
	Server bool   `msgpack:"server"`
	Room   bool   `msgpack:"room"`
	Create bool   `msgpack:"create"`
}

// Auth (C->S, protocol 5) answers a CHALLENGE.
type Auth struct {
	T           string `msgpack:"t"`
	ServerProof []byte `msgpack:"server_proof,omitempty"`
	RoomProof   []byte `msgpack:"room_proof,omitempty"`
	RoomKey     []byte `msgpack:"room_key,omitempty"`
}

// PlayerInfo is one entry of WELCOME.players and PLAYER_LIST.players.
type PlayerInfo struct {
	ID     uint32 `msgpack:"id"`
	Name   string `msgpack:"name"`
	Slot   int    `msgpack:"slot"`   // -1 when not in the group
	RTT    int    `msgpack:"rtt"`    // ms, -1 unknown
	State  string `msgpack:"state"`  // lobby, joining, playing, waiting, disconnected
	Leader bool   `msgpack:"leader"` // source of state blobs and hash reference
	// Protocol 3: scene of the player's group (or the scene it last reported, -1 unknown) and its group id (0: none).
	Scene int    `msgpack:"scene"`
	Group uint32 `msgpack:"group"`
	// Protocol 5: the player's chosen colour (0xRRGGBB, -1 the slot's own); absent for older rooms.
	Color *int `msgpack:"color,omitempty"`
}

// Player states reported in PlayerInfo.State.
const (
	StateLobby        = "lobby"
	StateJoining      = "joining"
	StatePlaying      = "playing"
	StateWaiting      = "waiting"
	StateDisconnected = "disconnected"
)

type Welcome struct {
	T        string       `msgpack:"t"`
	PlayerID uint32       `msgpack:"player_id"`
	Room     string       `msgpack:"room"`
	Players  []PlayerInfo `msgpack:"players"`
	Proto    int          `msgpack:"proto"`
	Rejoin   bool         `msgpack:"rejoin"`
	// LeaveShared: this server takes the shared changes sent with LEAVE_GROUP (D-115). A client only sends them, and
	// only then stops counting them as its own, when the server says so.
	LeaveShared bool `msgpack:"leave_shared"`
	// Protocol 5 (protocol 5 notes 2.5): absent for older rooms.
	RoomPassword  bool   `msgpack:"room_password,omitempty"`
	RoomOwner     bool   `msgpack:"room_owner,omitempty"`
	RoomConfirm   []byte `msgpack:"room_confirm,omitempty"`
	ServerConfirm []byte `msgpack:"server_confirm,omitempty"`
	ServerName    string `msgpack:"server_name,omitempty"`
	MaxPlayers    int    `msgpack:"max_players,omitempty"`
}

// Reject closes a handshake. Args (protocol 5 only) carry the values a client puts in its own translated text.
type Reject struct {
	T      string            `msgpack:"t"`
	Reason string            `msgpack:"reason"`
	Detail string            `msgpack:"detail"`
	Args   map[string]string `msgpack:"args,omitempty"`
}

type PlayerList struct {
	T         string       `msgpack:"t"`
	Players   []PlayerInfo `msgpack:"players"`
	Delay     int          `msgpack:"delay"`
	GroupTick int64        `msgpack:"group_tick"` // last emitted tick, -1 if none
	// Protocol 5: whether the room has a password now (absent for older rooms).
	RoomPassword *bool `msgpack:"room_password,omitempty"`
}

// Rename (C->S, protocol 5) asks for a new name; RenameResult answers it (reason bad_name or name_taken).
type Rename struct {
	T    string `msgpack:"t"`
	Name string `msgpack:"name"`
}

type RenameResult struct {
	T      string `msgpack:"t"`
	OK     bool   `msgpack:"ok"`
	Name   string `msgpack:"name"`
	Reason string `msgpack:"reason"`
}

// SetColor (C->S, protocol 5): 0xRRGGBB or -1.
type SetColor struct {
	T     string `msgpack:"t"`
	Color int    `msgpack:"color"`
}

// RoomSetKey (C->S, protocol 5): the room's creator changes its key (empty: no password).
type RoomSetKey struct {
	T       string `msgpack:"t"`
	RoomKey []byte `msgpack:"room_key"`
}

type RoomSetKeyResult struct {
	T      string `msgpack:"t"`
	OK     bool   `msgpack:"ok"`
	Reason string `msgpack:"reason"`
}

// RoomListRequest is a first message instead of HELLO (protocol 5); the server answers ROOM_LIST or REJECT
// rooms_hidden and closes.
type RoomListRequest struct {
	T     string `msgpack:"t"`
	Proto int    `msgpack:"proto"`
}

type RoomListEntry struct {
	Name      string `msgpack:"name"`
	Players   int    `msgpack:"players"`
	Max       int    `msgpack:"max"`
	Proto     int    `msgpack:"proto"`
	Engine    string `msgpack:"engine"`
	BuildHash string `msgpack:"build_hash"`
	Password  bool   `msgpack:"password"`
}

type RoomList struct {
	T          string          `msgpack:"t"`
	ServerName string          `msgpack:"server_name"`
	Rooms      []RoomListEntry `msgpack:"rooms"`
}

type Ping struct {
	T      string `msgpack:"t"`
	Nonce  uint64 `msgpack:"nonce"`
	SentAt uint64 `msgpack:"sent_at"`
}

type Kick struct {
	T      string `msgpack:"t"`
	Reason string `msgpack:"reason"`
}

// JoinGroup asks to enter (or found) a lockstep group: the room's only group (protocol 2) or the group of the scene
// (protocol 3).
type JoinGroup struct {
	T        string `msgpack:"t"`
	Scene    int    `msgpack:"scene"`
	Entrance int    `msgpack:"entrance"`
	InPlay   bool   `msgpack:"in_play"`
	// Protocol 3.
	Follow       string `msgpack:"follow,omitempty"`        // join the group of this player (rejoin from scratch)
	Spawn        string `msgpack:"spawn,omitempty"`         // opaque: carried in the SPAWN event (entrance, place, block)
	Shared       []byte `msgpack:"shared,omitempty"`        // the client's shared game (becomes canonical if none)
	SharedLayout uint32 `msgpack:"shared_layout,omitempty"` // layout of Shared; a different layout replaces the canonical
	Overwrite    bool   `msgpack:"overwrite,omitempty"`     // the founder's shared game replaces the room's canonical
	// Solo (phase 5b): found a group of its own even if another one plays in that scene (the player arrives with a
	// scripted cutscene of its own, a blue warp); nobody joins it until the client sends GROUP_OPEN.
	Solo bool `msgpack:"solo,omitempty"`
}

// LeaveGroup removes the sender's Link from the simulation. A player who walks out of its group (protocol 3+, D-115)
// sends with it the shared game's changes of its leaving tick up to that moment: they are the group's, and the members
// who stay send them with that tick's SHARED; the server takes them from the leave only if nobody stays to send it.
type LeaveGroup struct {
	T      string `msgpack:"t"`
	Tick   int64  `msgpack:"tick,omitempty"`
	Shared []byte `msgpack:"shared,omitempty"`
}

type GroupFound struct {
	T       string `msgpack:"t"`
	GroupID uint32 `msgpack:"group_id"`
	Slot    int    `msgpack:"slot"`
	Tick0   int64  `msgpack:"tick0"`
	Delay   int    `msgpack:"delay"`
	// Protocol 3: the room's canonical shared game when the founder must merge it (absent: the founder's own shared
	// game became the canonical one), the world clock and the group's scene.
	Shared  []byte `msgpack:"shared,omitempty"`
	DayTime int32  `msgpack:"day_time"`
	Scene   int    `msgpack:"scene"`
}

type GroupJoinPending struct {
	T         string `msgpack:"t"`
	GroupID   uint32 `msgpack:"group_id"`
	Slot      int    `msgpack:"slot"`
	Leader    int    `msgpack:"leader"`
	SpawnTick int64  `msgpack:"spawn_tick"`
	Rejoin    bool   `msgpack:"rejoin"`
}

type GroupWait struct {
	T      string `msgpack:"t"`
	Reason string `msgpack:"reason"`
}

type StateBlobRequest struct {
	T       string `msgpack:"t"`
	GroupID uint32 `msgpack:"group_id"`
	Tick    int64  `msgpack:"tick"`
	ForSlot int    `msgpack:"for_slot"`
}

// StateBlob travels leader -> server -> destination unchanged.
type StateBlob struct {
	T       string `msgpack:"t"`
	GroupID uint32 `msgpack:"group_id"`
	Tick    int64  `msgpack:"tick"`
	ForSlot int    `msgpack:"for_slot"`
	Data    []byte `msgpack:"data"`
	Hash    uint64 `msgpack:"hash"`
}

// StateBlobHeader decodes the routing fields of a STATE_BLOB without copying data.
type StateBlobHeader struct {
	GroupID uint32 `msgpack:"group_id"`
	Tick    int64  `msgpack:"tick"`
	ForSlot int    `msgpack:"for_slot"`
}

// PadSize is the size of one pad: u32 LE buttons + 4 s8 sticks.
const PadSize = 8

type Input struct {
	T    string `msgpack:"t"`
	Tick int64  `msgpack:"tick"`
	Pad  []byte `msgpack:"pad"`
}

// Event is one entry of TICK.events.
type Event struct {
	K    string `msgpack:"k"`
	Slot int    `msgpack:"slot"`
	// CONSOLE events (test tooling and scripted situations): a console command every client runs at the
	// start of the tick, before the game update (PLAN.md 1.3.3: world changes travel as lockstep events).
	// SPAWN (protocol 3): where and with what the player appears (JoinGroup.Spawn, opaque to the server).
	Cmd string `msgpack:"cmd,omitempty"`
	// SHARED (protocol 3): a patch of the shared game made by another group (see ApplyPatch).
	Data []byte `msgpack:"data,omitempty"`
}

// ClientEvent is EVENT (C->S): the server stamps it into the next unemitted tick of the sender's group.
type ClientEvent struct {
	T       string `msgpack:"t"`
	Kind    string `msgpack:"kind"`
	Payload string `msgpack:"payload"`
}

type Tick struct {
	T      string   `msgpack:"t"`
	Tick   int64    `msgpack:"tick"`
	Slots  []int    `msgpack:"slots"`
	Pads   [][]byte `msgpack:"pads"`
	Events []Event  `msgpack:"events"`
	Delay  int      `msgpack:"delay"`
	// Protocol 3: the world clock (gSaveContext.dayTime) the group adopts at the start of the tick; -1 none.
	DayTime int32 `msgpack:"day_time"`
}

// GroupScene (C->S, protocol 3): the whole group changed scene inside the simulation (a cutscene, a warp song, the
// game over): the group now belongs to that scene.
type GroupScene struct {
	T     string `msgpack:"t"`
	Scene int    `msgpack:"scene"`
}

// Regroup (S->C, protocol 3): another group already plays in the scene this client's group moved to; the client
// leaves its group and joins that one.
type Regroup struct {
	T     string `msgpack:"t"`
	Scene int    `msgpack:"scene"`
}

// Shared (C->S, protocol 3): changes of the shared game made by the sender's group in its tick `tick` (every member
// sends the same patch; the first one counts).
type Shared struct {
	T    string `msgpack:"t"`
	Tick int64  `msgpack:"tick"`
	Data []byte `msgpack:"data"`
}

// Clock (C->S, protocol 3): how fast time runs in the sender's scene (dayTime units per tick) and, when the group
// set the time (Sun's Song, a cutscene), the new time.
type Clock struct {
	T    string `msgpack:"t"`
	Tick int64  `msgpack:"tick"` // group tick (every member sends it; the first one per tick counts)
	Rate int    `msgpack:"rate"`
	Set  int32  `msgpack:"set"` // -1: no change
}

type Hash struct {
	T    string `msgpack:"t"`
	Tick int64  `msgpack:"tick"`
	Hash uint64 `msgpack:"hash"`
}

type Resync struct {
	T       string `msgpack:"t"`
	GroupID uint32 `msgpack:"group_id"`
	BadTick int64  `msgpack:"bad_tick"`
	Tick    int64  `msgpack:"tick"`
	Leader  int    `msgpack:"leader"`
	Targets []int  `msgpack:"targets"`
}

type Wait struct {
	T       string   `msgpack:"t"`
	Tick    int64    `msgpack:"tick"`
	Missing []string `msgpack:"missing"`
	Ms      int64    `msgpack:"ms"`
}

// Encode returns v as a complete frame (length prefix + body).
func Encode(v any) ([]byte, error) {
	body, err := msgpack.Marshal(v)
	if err != nil {
		return nil, err
	}
	return FrameBody(body)
}

// FrameBody prefixes an already encoded body with its length.
func FrameBody(body []byte) ([]byte, error) {
	if len(body) > MaxFrameSize {
		return nil, ErrFrameTooLarge
	}
	buf := make([]byte, 4+len(body))
	binary.LittleEndian.PutUint32(buf, uint32(len(body)))
	copy(buf[4:], body)
	return buf, nil
}

// WriteFrame encodes v as MessagePack and writes one frame.
func WriteFrame(w io.Writer, v any) error {
	buf, err := Encode(v)
	if err != nil {
		return err
	}
	_, err = w.Write(buf)
	return err
}

// ReadFrame reads one frame body.
func ReadFrame(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(hdr[:])
	if n > MaxFrameSize {
		return nil, ErrFrameTooLarge
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

// Type returns the "t" field of a frame body.
func Type(body []byte) (string, error) {
	var h Header
	if err := msgpack.Unmarshal(body, &h); err != nil {
		return "", fmt.Errorf("bad message: %w", err)
	}
	return h.T, nil
}

// Decode unmarshals a frame body into v.
func Decode(body []byte, v any) error {
	return msgpack.Unmarshal(body, v)
}
