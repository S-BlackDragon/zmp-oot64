package server

// Canonical shared game of a room (protocol 3, PLAN.md 2.11, docs/DECISIONES.md D-061).
//
// The server does not know the game's save layout. Clients describe the shared part of the save context as a compact
// byte buffer (the "shared game": items, upgrades, quest items, keys, rupees, heart containers, the permanent flags of
// every scene, event flags...) whose layout is identified by a number. Groups report their changes as patches; the
// server applies them to the canonical buffer, forwards them to the other groups and writes the buffer to disk.

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Patch entry operations.
const (
	// PatchBits sets and clears bits: v = (v | set) &^ clear. Two groups changing different bits of the same field
	// both keep their change.
	PatchBits = 0
	// PatchDelta adds a signed amount (rupees, keys, heart containers): two groups earning and spending at the same
	// time both count.
	PatchDelta = 1
)

var errBadPatch = errors.New("malformed patch")

// ApplyPatch applies a patch to data in place. Entry: u16 LE offset, u8 size (1, 2 or 4), u8 op, then for PatchBits
// the set mask and the clear mask (size bytes each, LE), for PatchDelta the signed amount (size bytes, LE).
func ApplyPatch(data, patch []byte) error {
	for len(patch) > 0 {
		if len(patch) < 4 {
			return errBadPatch
		}
		off := int(binary.LittleEndian.Uint16(patch))
		size := int(patch[2])
		op := patch[3]
		patch = patch[4:]
		if size != 1 && size != 2 && size != 4 {
			return errBadPatch
		}
		if off+size > len(data) {
			return fmt.Errorf("patch offset %d+%d out of %d bytes", off, size, len(data))
		}
		get := func(b []byte) uint32 {
			switch size {
			case 1:
				return uint32(b[0])
			case 2:
				return uint32(binary.LittleEndian.Uint16(b))
			}
			return binary.LittleEndian.Uint32(b)
		}
		put := func(b []byte, v uint32) {
			switch size {
			case 1:
				b[0] = byte(v)
			case 2:
				binary.LittleEndian.PutUint16(b, uint16(v))
			default:
				binary.LittleEndian.PutUint32(b, v)
			}
		}
		switch op {
		case PatchBits:
			if len(patch) < 2*size {
				return errBadPatch
			}
			set, clr := get(patch), get(patch[size:])
			patch = patch[2*size:]
			v := get(data[off:])
			put(data[off:], (v|set)&^clr)
		case PatchDelta:
			if len(patch) < size {
				return errBadPatch
			}
			d := get(patch)
			patch = patch[size:]
			put(data[off:], get(data[off:])+d) // two's complement: adding the raw value adds the signed amount
		default:
			return errBadPatch
		}
	}
	return nil
}

// canonical is the shared game of a room plus the world clock, as stored on disk.
type canonical struct {
	Layout  uint32
	Data    []byte
	DayTime uint16
	dirty   bool
	savedAt time.Time
}

type canonicalFile struct {
	Room    string `json:"room"`
	Layout  uint32 `json:"layout"`
	DayTime uint16 `json:"day_time"`
	Updated string `json:"updated"`
	Shared  string `json:"shared"` // base64
}

// savePath is data/saves/<room, only safe characters>-<hash of the exact name>.json.
func (s *Server) savePath(room string) string {
	var b strings.Builder
	for _, r := range room {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
		if b.Len() >= 40 {
			break
		}
	}
	sum := sha1.Sum([]byte(room))
	return filepath.Join(s.cfg.DataDir, "saves", b.String()+"-"+hex.EncodeToString(sum[:4])+".json")
}

func (s *Server) loadCanonical(room string) *canonical {
	if s.cfg.DataDir == "" {
		return nil
	}
	raw, err := os.ReadFile(s.savePath(room))
	if err != nil {
		return nil
	}
	var f canonicalFile
	if err := json.Unmarshal(raw, &f); err != nil || f.Room != room {
		s.log.Printf("room=%q: canonical save unreadable (%v), ignored", room, err)
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(f.Shared)
	if err != nil || len(data) == 0 {
		return nil
	}
	s.log.Printf("room=%q: canonical game loaded (%d bytes, layout %08x, day time %04x, saved %s)", room, len(data),
		f.Layout, f.DayTime, f.Updated)
	return &canonical{Layout: f.Layout, Data: data, DayTime: f.DayTime, savedAt: time.Now()}
}

// saveJob is a snapshot written outside the server lock.
type saveJob struct {
	path string
	body []byte
}

func (s *Server) canonicalJobLocked(r *Room, now time.Time) *saveJob {
	c := r.canon
	if c == nil || s.cfg.DataDir == "" {
		return nil
	}
	c.DayTime = r.clockDay()
	f := canonicalFile{Room: r.Name, Layout: c.Layout, DayTime: c.DayTime, Updated: now.UTC().Format(time.RFC3339),
		Shared: base64.StdEncoding.EncodeToString(c.Data)}
	body, err := json.MarshalIndent(f, "", " ")
	if err != nil {
		return nil
	}
	c.dirty = false
	c.savedAt = now
	return &saveJob{path: s.savePath(r.Name), body: body}
}

// writeJobs writes snapshots atomically (temporary file + rename). Called without the server lock.
func (s *Server) writeJobs(jobs []*saveJob) {
	for _, j := range jobs {
		if j == nil {
			continue
		}
		tmp := j.path + ".tmp"
		if err := os.WriteFile(tmp, j.body, 0o644); err != nil {
			s.log.Printf("canonical save %s: %v", j.path, err)
			continue
		}
		if err := os.Rename(tmp, j.path); err != nil {
			s.log.Printf("canonical save %s: %v", j.path, err)
		}
	}
}
