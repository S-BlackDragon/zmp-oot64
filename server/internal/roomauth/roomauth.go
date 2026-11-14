// Package roomauth is the password side of ZMP protocol 5 (the protocol 5 design notes, section 3): the key a
// password turns into, the challenge/response proofs, the store of the rooms' keys (they outlive the server's
// restarts with the room's save) and the limit of failed attempts. The password itself never reaches the server:
// clients derive the key and answer the server's challenge with an HMAC of it.
package roomauth

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Iterations of PBKDF2-HMAC-SHA256 in Derive (protocol 5; changing it changes every key).
const Iterations = 10000

// KeySize is the size of a key, a nonce is NonceSize.
const (
	KeySize   = 32
	NonceSize = 16
)

// MaxPassword is the longest password, in bytes of UTF-8 after trimming.
const MaxPassword = 64

// Kinds of key: a room's and the server's.
const (
	KindRoom   = "room"
	KindServer = "server"
)

// Normalize trims the spaces, tabs and line ends a copy-paste adds around a password (nothing else changes).
func Normalize(password string) string { return strings.Trim(password, " \t\r\n") }

// RoomKey derives the key of a room's password: PBKDF2-HMAC-SHA256(password, "zmp-room-v1\0" + room, 10000, 32).
// An empty password (after Normalize) has no key: nil.
func RoomKey(room, password string) []byte {
	return derive("zmp-room-v1\x00"+room, password)
}

// ServerKey derives the key of a server password: PBKDF2-HMAC-SHA256(password, "zmp-server-v1", 10000, 32).
func ServerKey(password string) []byte { return derive("zmp-server-v1", password) }

func derive(salt, password string) []byte {
	password = Normalize(password)
	if password == "" {
		return nil
	}
	k, err := pbkdf2.Key(sha256.New, password, []byte(salt), Iterations, KeySize)
	if err != nil {
		panic("roomauth: pbkdf2: " + err.Error()) // (only for out-of-range parameters, which are constants here)
	}
	return k
}

// Proof is what a client answers a CHALLENGE with for one key: HMAC-SHA256(key, "zmp-auth-v1\0" + kind + "\0" +
// serverNonce + clientNonce + room + "\0" + name).
func Proof(key []byte, kind string, serverNonce, clientNonce []byte, room, name string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("zmp-auth-v1\x00" + kind + "\x00"))
	m.Write(serverNonce)
	m.Write(clientNonce)
	m.Write([]byte(room + "\x00" + name))
	return m.Sum(nil)
}

// Confirm is what the server answers in WELCOME to show it knows the key too: HMAC-SHA256(key, "zmp-auth-v1\0" +
// kind + "-ok\0" + serverNonce + clientNonce). A client may check it (a fake server cannot make it).
func Confirm(key []byte, kind string, serverNonce, clientNonce []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("zmp-auth-v1\x00" + kind + "-ok\x00"))
	m.Write(serverNonce)
	m.Write(clientNonce)
	return m.Sum(nil)
}

// Equal compares two proofs or keys in constant time.
func Equal(a, b []byte) bool { return len(a) == len(b) && subtle.ConstantTimeCompare(a, b) == 1 }

// Nonce makes a random server nonce.
func Nonce() []byte {
	b := make([]byte, NonceSize)
	if _, err := rand.Read(b); err != nil {
		panic("roomauth: no random bytes: " + err.Error())
	}
	return b
}

// OwnerID is how the store remembers who made a room: a hash of the creator's identity (its player_key, or its name
// for clients without one), never the player_key itself.
func OwnerID(identity string) string {
	h := sha256.Sum256([]byte("zmp-owner-v1\x00" + identity))
	return hex.EncodeToString(h[:16])
}

// ---------------------------------------------------------------------------------------------------------------
// Store of the rooms' keys

// Entry is one room with a password.
type Entry struct {
	Key     string    `json:"key"`             // hex of RoomKey
	Owner   string    `json:"owner,omitempty"` // OwnerID of whoever set it ("" = set by the server's own config)
	Fixed   bool      `json:"fixed,omitempty"` // set by the server's configuration: never expires, clients cannot change it
	Created time.Time `json:"created"`
	Used    time.Time `json:"used"`
}

type storeFile struct {
	Version int               `json:"version"`
	Rooms   map[string]*Entry `json:"rooms"`
}

// Store keeps the keys of the rooms that have a password, on disk in <data>/room_keys.json (none in memory only).
// Safe for concurrent use.
type Store struct {
	mu    sync.Mutex
	path  string
	ttl   time.Duration
	rooms map[string]*Entry
}

// FileName is the store's file in the data directory.
const FileName = "room_keys.json"

// OpenStore loads dir/room_keys.json (missing: empty). dir "" keeps the store in memory. ttl > 0 forgets a key not
// used for that long (a relay's abandoned rooms); fixed keys never expire.
func OpenStore(dir string, ttl time.Duration) (*Store, error) {
	s := &Store{ttl: ttl, rooms: map[string]*Entry{}}
	if dir == "" {
		return s, nil
	}
	s.path = filepath.Join(dir, FileName)
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var f storeFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, errors.New(FileName + ": " + err.Error())
	}
	for room, e := range f.Rooms {
		if e != nil && len(e.Key) == 2*KeySize {
			if _, err := hex.DecodeString(e.Key); err == nil {
				e.Fixed = false // (fixed keys come from the configuration at each start, not from the file)
				s.rooms[room] = e
			}
		}
	}
	return s, nil
}

// Get gives the key of room (nil: the room has no password).
func (s *Store) Get(room string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.rooms[room]
	if e == nil {
		return nil
	}
	k, _ := hex.DecodeString(e.Key)
	return k
}

// Prune forgets the keys not used for the store's ttl (none if ttl is 0), except fixed ones and the rooms keep says
// are in use (those are marked used now). Returns the rooms forgotten.
func (s *Store) Prune(now time.Time, keep func(room string) bool) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var gone []string
	changed := false
	for room, e := range s.rooms {
		if e.Fixed {
			continue
		}
		if keep != nil && keep(room) {
			if now.Sub(e.Used) >= time.Hour {
				e.Used, changed = now, true
			}
			continue
		}
		if s.ttl > 0 && now.Sub(e.Used) > s.ttl {
			delete(s.rooms, room)
			gone = append(gone, room)
			changed = true
		}
	}
	if changed {
		_ = s.saveLocked()
	}
	sort.Strings(gone)
	return gone
}

// Fixed gives the fixed entries (the server's configuration), to carry them into another store.
func (s *Store) Fixed() map[string][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string][]byte{}
	for r, e := range s.rooms {
		if e.Fixed {
			k, _ := hex.DecodeString(e.Key)
			out[r] = k
		}
	}
	return out
}

// Info gives the entry of room (nil: no password). The caller must not change it.
func (s *Store) Info(room string) *Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.rooms[room]; e != nil {
		c := *e
		return &c
	}
	return nil
}

// Set gives room the key (nil or empty: the room has no password any more) and writes the store.
func (s *Store) Set(room string, key []byte, owner string, fixed bool, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(key) == 0 {
		delete(s.rooms, room)
		return s.saveLocked()
	}
	created := now
	if e := s.rooms[room]; e != nil {
		created = e.Created
	}
	s.rooms[room] = &Entry{Key: hex.EncodeToString(key), Owner: owner, Fixed: fixed, Created: created, Used: now}
	return s.saveLocked()
}

// Touch marks room as used now (a player came in); written at most once an hour per room.
func (s *Store) Touch(room string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.rooms[room]; e != nil && now.Sub(e.Used) >= time.Hour {
		e.Used = now
		_ = s.saveLocked()
	}
}

// Rooms lists the rooms with a password, sorted.
func (s *Store) Rooms() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.rooms))
	for r := range s.rooms {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	f := storeFile{Version: 1, Rooms: map[string]*Entry{}}
	for r, e := range s.rooms {
		if !e.Fixed { // (the configuration's keys are written by nobody: they come back at each start)
			f.Rooms[r] = e
		}
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// ---------------------------------------------------------------------------------------------------------------
// Failed attempts

// Limiter counts failed passwords: MaxFails within Window from one address for one room (or MaxFailsAddr for any
// room) block that address for Block. Safe for concurrent use.
type Limiter struct {
	MaxFails     int
	MaxFailsAddr int
	Window       time.Duration
	Block        time.Duration

	mu    sync.Mutex
	fails map[string][]time.Time
	until map[string]time.Time
}

// NewLimiter: five failures per address and room in a minute, or twenty per address, block for 60 s
// (PRODUCTO_PUBLICO.md 5.2).
func NewLimiter() *Limiter {
	return &Limiter{MaxFails: 5, MaxFailsAddr: 20, Window: time.Minute, Block: time.Minute,
		fails: map[string][]time.Time{}, until: map[string]time.Time{}}
}

// Blocked says whether addr may not try room now, and for how long.
func (l *Limiter) Blocked(addr, room string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range []string{addr + "\x00" + room, addr} {
		if u, ok := l.until[k]; ok {
			if now.Before(u) {
				return true, u.Sub(now)
			}
			delete(l.until, k)
		}
	}
	return false, 0
}

// Fail records a failed attempt; true if this one blocks the address.
func (l *Limiter) Fail(addr, room string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	blocked := false
	for _, kv := range []struct {
		k   string
		max int
	}{{addr + "\x00" + room, l.MaxFails}, {addr, l.MaxFailsAddr}} {
		var keep []time.Time
		for _, t := range l.fails[kv.k] {
			if now.Sub(t) < l.Window {
				keep = append(keep, t)
			}
		}
		keep = append(keep, now)
		if kv.max > 0 && len(keep) >= kv.max {
			l.until[kv.k] = now.Add(l.Block)
			delete(l.fails, kv.k)
			blocked = true
			continue
		}
		l.fails[kv.k] = keep
	}
	if len(l.fails) > 4096 { // (forget old entries so a scan cannot grow the map for ever)
		for k, ts := range l.fails {
			if len(ts) == 0 || now.Sub(ts[len(ts)-1]) >= l.Window {
				delete(l.fails, k)
			}
		}
	}
	return blocked
}
