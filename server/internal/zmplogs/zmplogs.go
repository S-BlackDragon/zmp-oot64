// Package zmplogs is the log store of zmp-server (D-247): each game client uploads its
// own logs (the game's log and the ZMP lines) to the server it plays on, over HTTP on the same port, so a failure on
// any PC can be read on the server afterwards. The server writes its own lines and each room's events there too.
//
// It is off unless a directory is configured. A client only uploads when its WELCOME says log_collect, and every
// upload carries the token of that WELCOME: the server names the files from what it knows about that connection
// (room, session, player id and name), never from what the client sends, and every name is sanitized.
//
// Layout under Dir:
//
//	sessions/<room>/<session>/server-events.log          the server's events of that room session
//	sessions/<room>/<session>/p<id>-<name>/<run>-<kind>.log  a client's log of one run of the game (appended)
//	server/zmp-server-<yyyymmdd>.log                     every line the server writes to its own log
//
// Limits: a body is at most MaxUpload bytes (after gzip), a player name at most MaxPerDay bytes per UTC day, all the
// players together MaxDay, and the whole store MaxTotal: past it the oldest files go first.
package zmplogs

import (
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Prefix is the path of the upload endpoint on the server's port.
const Prefix = "/zmp/logs"

// Defaults of the limits.
const (
	DefaultMaxUpload = 8 << 20
	DefaultMaxPerDay = 128 << 20
	DefaultMaxDay    = 1 << 30
	DefaultMaxTotal  = 2 << 30
	DefaultTokenTTL  = 72 * time.Hour
	DefaultMaxTokens = 4096
	maxNamePart      = 40
)

// Config of the store. An empty Dir keeps it off.
type Config struct {
	Dir       string
	MaxUpload int64
	MaxPerDay int64
	MaxDay    int64
	MaxTotal  int64
	TokenTTL  time.Duration
	MaxTokens int
	Logger    *log.Logger
	Now       func() time.Time
}

type ticket struct {
	room, session, player, name string
	issued                      time.Time
}

// Store is safe for concurrent use. A nil or disabled Store answers every call as "off".
type Store struct {
	cfg     Config
	enabled bool

	upMu     sync.Mutex // one upload at a time between reading a log's size and appending to it
	mu       sync.Mutex
	tickets  map[string]*ticket
	order    []string
	total    int64
	day      string
	dayBytes map[string]int64
	dayAll   int64

	lines     chan line // server lines and room events, written by one goroutine
	done      chan struct{}
	closeOnce sync.Once
	dropped   int64
}

type line struct {
	path string // relative to Dir
	text string
}

// New opens the store. A directory that cannot be written turns it off with one log line (the server goes on).
func New(cfg Config) *Store {
	if cfg.MaxUpload <= 0 {
		cfg.MaxUpload = DefaultMaxUpload
	}
	if cfg.MaxPerDay <= 0 {
		cfg.MaxPerDay = DefaultMaxPerDay
	}
	if cfg.MaxDay <= 0 {
		cfg.MaxDay = DefaultMaxDay
	}
	if cfg.MaxTotal <= 0 {
		cfg.MaxTotal = DefaultMaxTotal
	}
	if cfg.TokenTTL <= 0 {
		cfg.TokenTTL = DefaultTokenTTL
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = DefaultMaxTokens
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	s := &Store{cfg: cfg, tickets: map[string]*ticket{}, dayBytes: map[string]int64{}}
	if cfg.Dir == "" {
		return s
	}
	if err := probeDir(cfg.Dir); err != nil {
		s.logf("zmp logs: OFF, the store %s cannot be written: %v", cfg.Dir, err)
		return s
	}
	s.enabled = true
	s.total = dirSize(cfg.Dir)
	s.lines = make(chan line, 4096)
	s.done = make(chan struct{})
	go s.lineWriter()
	s.logf("zmp logs: ON, store %s (%d MiB used of %d MiB; at most %d MiB per upload, %d MiB per player and day)",
		cfg.Dir, s.total>>20, cfg.MaxTotal>>20, cfg.MaxUpload>>20, cfg.MaxPerDay>>20)
	return s
}

func probeDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".probe-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

func dirSize(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, e := d.Info(); e == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}

// Enabled says whether the store collects logs (what WELCOME announces).
func (s *Store) Enabled() bool { return s != nil && s.enabled }

// Close stops the writer of the server lines (after writing what is queued).
func (s *Store) Close() {
	if !s.Enabled() {
		return
	}
	s.closeOnce.Do(func() {
		close(s.lines)
		<-s.done
	})
}

func (s *Store) logf(format string, args ...any) {
	if s.cfg.Logger != nil {
		s.cfg.Logger.Printf(format, args...)
	}
}

// Sanitize turns any text into a file name part: [A-Za-z0-9._-] only (anything else is '_'), at most 40 bytes, never
// "", "." or "..", never starting or ending with '.' (no hidden files, no path parts).
func Sanitize(text string) string {
	var b strings.Builder
	for i := 0; i < len(text) && b.Len() < maxNamePart; i++ {
		c := text[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.' {
			b.WriteByte(c)
		} else {
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), ".") // (Windows drops a final dot: "a." and "a" would be the same file)
	if out == "" {
		return "x"
	}
	switch base := strings.ToUpper(strings.SplitN(out, ".", 2)[0]); base {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		out = "_" + out // (device names on Windows, where the store may be tried out)
	}
	return out
}

// SessionID is the name of a room session: the UTC time its room was made.
func SessionID(created time.Time) string { return created.UTC().Format("20060102-150405") }

// Issue gives a connection the token its uploads carry; "" when the store is off.
func (s *Store) Issue(room, session string, playerID uint32, name string) string {
	if !s.Enabled() {
		return ""
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return ""
	}
	token := hex.EncodeToString(raw[:])
	now := s.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tickets[token] = &ticket{room: Sanitize(room), session: Sanitize(session),
		player: fmt.Sprintf("p%d-%s", playerID, Sanitize(name)), name: Sanitize(name), issued: now}
	s.order = append(s.order, token)
	// (forget the expired ones and the oldest past MaxTokens; order is the issue order)
	for len(s.order) > 0 {
		t := s.tickets[s.order[0]]
		if t != nil && len(s.order) <= s.cfg.MaxTokens && now.Sub(t.issued) < s.cfg.TokenTTL {
			break
		}
		delete(s.tickets, s.order[0])
		s.order = s.order[1:]
	}
	return token
}

func (s *Store) ticketOf(token string) *ticket {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tickets[token]
	if t == nil || s.cfg.Now().Sub(t.issued) >= s.cfg.TokenTTL {
		return nil
	}
	return t
}

// Event writes one line in the events of a room session (never blocks: a full queue drops the line and counts it).
func (s *Store) Event(room, session, text string) {
	if !s.Enabled() {
		return
	}
	s.queue(filepath.Join("sessions", Sanitize(room), Sanitize(session), "server-events.log"), text)
}

// ServerLog is a writer for the server's own log (io.MultiWriter with stdout): one file per UTC day. It never fails
// and never blocks.
func (s *Store) ServerLog() io.Writer { return serverLog{s} }

type serverLog struct{ s *Store }

func (w serverLog) Write(p []byte) (int, error) {
	if w.s.Enabled() {
		day := w.s.cfg.Now().UTC().Format("20060102")
		w.s.queue(filepath.Join("server", "zmp-server-"+day+".log"), strings.TrimRight(string(p), "\n"))
	}
	return len(p), nil
}

func (s *Store) queue(path, text string) {
	defer func() { _ = recover() }() // (a send after Close)
	select {
	case s.lines <- line{path: path, text: text}:
	default:
		s.mu.Lock()
		s.dropped++
		s.mu.Unlock()
	}
}

func (s *Store) lineWriter() {
	defer close(s.done)
	for l := range s.lines {
		text := l.text
		if !strings.HasPrefix(l.path, "server"+string(filepath.Separator)) {
			text = s.cfg.Now().UTC().Format("2006-01-02 15:04:05.000") + " " + text
		}
		_, _ = s.appendFile(l.path, []byte(text+"\n"), "")
	}
}

// appendFile appends data to Dir/rel and keeps the store under its cap. Returns the size of the file after it.
func (s *Store) appendFile(rel string, data []byte, keep string) (int64, error) {
	full := filepath.Join(s.cfg.Dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return 0, err
	}
	f, err := os.OpenFile(full, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	n, err := f.Write(data)
	info, statErr := f.Stat()
	f.Close()
	s.mu.Lock()
	s.total += int64(n)
	over := s.total > s.cfg.MaxTotal
	s.mu.Unlock()
	if over {
		s.prune(full, keep)
	}
	if err != nil {
		return 0, err
	}
	if statErr != nil {
		return 0, statErr
	}
	return info.Size(), nil
}

// prune deletes the oldest files until the store is at 90 % of its cap (never the file just written).
func (s *Store) prune(just, keep string) {
	total, removed, freed := PruneOldest(s.cfg.Dir, s.cfg.MaxTotal, just, keep)
	s.mu.Lock()
	s.total = total
	s.mu.Unlock()
	if removed > 0 {
		s.logf("zmp logs: store over %d MiB: removed the %d oldest files (%d MiB)", s.cfg.MaxTotal>>20, removed, freed>>20)
	}
}

// PruneOldest keeps a log folder under maxTotal bytes: when it is over, it deletes its oldest files (by modification
// time) until it is at 90 % of maxTotal, never the files in keep, and removes the folders it leaves empty. It returns
// the folder's size after it, how many files it removed and how many bytes. Shared by the players' log store and the
// project's other log uploads (D-249).
func PruneOldest(dir string, maxTotal int64, keep ...string) (total int64, removed int, freed int64) {
	type entry struct {
		path string
		size int64
		mod  time.Time
	}
	kept := map[string]bool{}
	for _, k := range keep {
		if k != "" {
			kept[filepath.Clean(k)] = true
		}
	}
	var files []entry
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return nil
		}
		total += info.Size()
		if !kept[filepath.Clean(p)] {
			files = append(files, entry{p, info.Size(), info.ModTime()})
		}
		return nil
	})
	if maxTotal <= 0 || total <= maxTotal {
		return total, 0, 0
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	target := maxTotal / 10 * 9
	for _, f := range files {
		if total <= target {
			break
		}
		if os.Remove(f.path) == nil {
			total -= f.size
			freed += f.size
			removed++
			removeEmptyParents(filepath.Dir(f.path), dir)
		}
	}
	return total, removed, freed
}

func removeEmptyParents(dir, root string) {
	root = filepath.Clean(root)
	for dir = filepath.Clean(dir); dir != root && strings.HasPrefix(dir, root); dir = filepath.Dir(dir) {
		if os.Remove(dir) != nil { // (not empty)
			return
		}
	}
}

// Kinds of client log and reasons of an upload (anything else is refused or written as "other").
var (
	kinds   = map[string]bool{"game": true, "zmp": true, "dump": true}
	reasons = map[string]bool{"end": true, "desync": true, "crash": true, "periodic": true, "previous": true}
)

func cors(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	h.Set("Access-Control-Max-Age", "86400")
}

func answer(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Connection", "close")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ServeHTTP is POST /zmp/logs?kind=game|zmp|dump&run=<run>&reason=<why>&offset=<n>[&gz=1], token in
// "Authorization: Bearer <token>" (or &token=). The body is the bytes of that log from offset on (gzip with
// "Content-Encoding: gzip" or gz=1; chunked or not). The store appends what it does not have yet: a body that starts
// before its end only adds the new part (a retry), one that starts past it is refused with 409 and the size the store
// has, so the client sends from there. Answer: {"size": <bytes of that log in the store>}.
func (s *Store) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cors(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST, OPTIONS")
		answer(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
		return
	}
	if !s.Enabled() {
		if s != nil {
			s.logf("zmp logs: upload from %s refused: this server does not collect logs", r.RemoteAddr)
		}
		answer(w, http.StatusNotFound, map[string]string{"error": "this server does not collect logs"})
		return
	}
	q := r.URL.Query()
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token == "" {
		token = q.Get("token")
	}
	t := s.ticketOf(token)
	if t == nil {
		s.logf("zmp logs: upload from %s refused: unknown or expired token", r.RemoteAddr)
		answer(w, http.StatusForbidden, map[string]string{"error": "unknown or expired token"})
		return
	}
	kind, run := q.Get("kind"), q.Get("run")
	if !kinds[kind] || run == "" || len(run) > maxNamePart {
		answer(w, http.StatusBadRequest, map[string]string{"error": "bad kind or run"})
		return
	}
	reason := q.Get("reason")
	if !reasons[reason] {
		reason = "other"
	}
	offset := int64(-1)
	if v := q.Get("offset"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			answer(w, http.StatusBadRequest, map[string]string{"error": "bad offset"})
			return
		}
		offset = n
	}
	body := io.Reader(http.MaxBytesReader(w, r.Body, s.cfg.MaxUpload+1))
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") || q.Get("gz") == "1" {
		zr, err := gzip.NewReader(body)
		if err != nil {
			answer(w, http.StatusBadRequest, map[string]string{"error": "bad gzip"})
			return
		}
		defer zr.Close()
		body = zr
	}
	data, err := io.ReadAll(io.LimitReader(body, s.cfg.MaxUpload+1))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) || int64(len(data)) > s.cfg.MaxUpload {
		s.logf("zmp logs: upload of %s/%s refused: more than %d bytes", t.player, kind, s.cfg.MaxUpload)
		answer(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "upload too big"})
		return
	}
	if err != nil {
		answer(w, http.StatusBadRequest, map[string]string{"error": "cannot read the body"})
		return
	}
	rel := filepath.Join("sessions", t.room, t.session, t.player, Sanitize(run)+"-"+kind+".log")
	full := filepath.Join(s.cfg.Dir, rel)

	s.upMu.Lock()
	defer s.upMu.Unlock()
	s.mu.Lock()
	day := s.cfg.Now().UTC().Format("20060102")
	if day != s.day {
		s.day, s.dayBytes, s.dayAll = day, map[string]int64{}, 0
	}
	cur := int64(0)
	if info, err := os.Stat(full); err == nil {
		cur = info.Size()
	}
	if offset > cur {
		s.mu.Unlock()
		answer(w, http.StatusConflict, map[string]any{"size": cur, "error": "the store has less of this log"})
		return
	}
	if offset >= 0 {
		skip := cur - offset
		if skip >= int64(len(data)) {
			data = nil
		} else {
			data = data[skip:]
		}
	}
	n := int64(len(data))
	if s.dayBytes[t.name]+n > s.cfg.MaxPerDay || s.dayAll+n > s.cfg.MaxDay {
		s.mu.Unlock()
		s.logf("zmp logs: upload of %s/%s refused: the daily limit is reached", t.player, kind)
		answer(w, http.StatusTooManyRequests, map[string]any{"size": cur, "error": "daily limit reached"})
		return
	}
	s.dayBytes[t.name] += n
	s.dayAll += n
	s.mu.Unlock()

	size := cur
	if n > 0 {
		var err error
		if size, err = s.appendFile(rel, data, ""); err != nil {
			s.logf("zmp logs: cannot store %s: %v", rel, err)
			answer(w, http.StatusInternalServerError, map[string]string{"error": "cannot store the log"})
			return
		}
	}
	s.Event(t.room, t.session, fmt.Sprintf("upload %s %s-%s reason=%s +%d bytes (now %d)", t.player, Sanitize(run),
		kind, reason, n, size))
	answer(w, http.StatusOK, map[string]any{"size": size})
}

// ParseSize reads "2147483648", "512M", "2G", "2GiB" (binary units).
func ParseSize(text string) (int64, error) {
	t := strings.TrimSpace(strings.ToUpper(text))
	t = strings.TrimSuffix(strings.TrimSuffix(t, "IB"), "B")
	mult := int64(1)
	switch {
	case strings.HasSuffix(t, "K"):
		mult, t = 1<<10, strings.TrimSuffix(t, "K")
	case strings.HasSuffix(t, "M"):
		mult, t = 1<<20, strings.TrimSuffix(t, "M")
	case strings.HasSuffix(t, "G"):
		mult, t = 1<<30, strings.TrimSuffix(t, "G")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("bad size %q", text)
	}
	return n * mult, nil
}
