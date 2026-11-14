package zmplogs

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newStore(t *testing.T, cfg Config) (*Store, *httptest.Server) {
	t.Helper()
	if cfg.Dir == "" {
		cfg.Dir = t.TempDir()
	}
	cfg.Logger = log.New(io.Discard, "", 0)
	s := New(cfg)
	srv := httptest.NewServer(s)
	t.Cleanup(func() { srv.Close(); s.Close() })
	return s, srv
}

func gz(t *testing.T, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, _ = w.Write(data)
	_ = w.Close()
	return b.Bytes()
}

type reply struct {
	status int
	size   int64
}

// post sends one upload; chunked makes the body a stream of unknown length (Transfer-Encoding: chunked).
func post(t *testing.T, srv *httptest.Server, token, query string, body []byte, gzipped, chunked bool) reply {
	t.Helper()
	var rd io.Reader = bytes.NewReader(body)
	if chunked {
		pr, pw := io.Pipe()
		go func() {
			for i := 0; i < len(body); i += 7 {
				end := min(i+7, len(body))
				_, _ = pw.Write(body[i:end])
			}
			pw.Close()
		}()
		rd = pr
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+Prefix+"?"+query, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if gzipped {
		req.Header.Set("Content-Encoding", "gzip")
	}
	if chunked {
		req.ContentLength = -1 // (unknown length: Transfer-Encoding chunked)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Size int64 `json:"size"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return reply{resp.StatusCode, out.Size}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func waitFile(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil && strings.Contains(string(b), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	b, _ := os.ReadFile(path)
	t.Fatalf("%s does not say %q: %q", path, want, b)
}

func TestOffWithoutDir(t *testing.T) {
	s := New(Config{Logger: log.New(io.Discard, "", 0)})
	if s.Enabled() || s.Issue("r", "s", 1, "A") != "" {
		t.Fatal("a store without a directory is on")
	}
	srv := httptest.NewServer(s)
	defer srv.Close()
	if r := post(t, srv, "x", "kind=zmp&run=r1&offset=0", []byte("hola"), false, false); r.status != http.StatusNotFound {
		t.Fatalf("status %d, want 404", r.status)
	}
	var nilStore *Store
	if nilStore.Enabled() {
		t.Fatal("a nil store is on")
	}
	nilStore.Event("r", "s", "x") // (no panic)
}

func TestOffWhenNotWritable(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a-file")
	_ = os.WriteFile(file, []byte("x"), 0o644)
	s := New(Config{Dir: filepath.Join(file, "logs"), Logger: log.New(io.Discard, "", 0)})
	if s.Enabled() {
		t.Fatal("a store in a directory that cannot be made is on")
	}
}

func TestUploadGzipChunkedNamesAndAppend(t *testing.T) {
	s, srv := newStore(t, Config{})
	tok := s.Issue("../sala mala/..", "20261113-101500", 7, `Al/ex\..`)
	if tok == "" {
		t.Fatal("no token")
	}
	first := []byte("linea 1\nlinea 2\n")
	r := post(t, srv, tok, "kind=zmp&run=20261113-101400&reason=periodic&offset=0", gz(t, first), true, true)
	if r.status != http.StatusOK || r.size != int64(len(first)) {
		t.Fatalf("first upload: %+v", r)
	}
	dir := filepath.Join(s.cfg.Dir, "sessions", "_sala_mala_", "20261113-101500", "p7-Al_ex_")
	path := filepath.Join(dir, "20261113-101400-zmp.log")
	if got := read(t, path); got != string(first) {
		t.Fatalf("stored %q", got)
	}
	// a retry that overlaps only adds the new part; a plain (not gzip) body works too
	more := []byte("linea 2\nlinea 3\n")
	r = post(t, srv, tok, "kind=zmp&run=20261113-101400&reason=end&offset=8", more, false, false)
	if r.status != http.StatusOK || read(t, path) != "linea 1\nlinea 2\nlinea 3\n" {
		t.Fatalf("append: %+v %q", r, read(t, path))
	}
	// the same bytes again: nothing added
	r = post(t, srv, tok, "kind=zmp&run=20261113-101400&reason=end&offset=8", more, false, false)
	if r.status != http.StatusOK || r.size != 24 {
		t.Fatalf("repeat: %+v", r)
	}
	// a body past the end: 409 with what the store has
	r = post(t, srv, tok, "kind=zmp&run=20261113-101400&reason=end&offset=100", more, false, false)
	if r.status != http.StatusConflict || r.size != 24 {
		t.Fatalf("gap: %+v", r)
	}
	// the run name is sanitized, the kind is a closed list
	r = post(t, srv, tok, "kind=game&run=../../etc&reason=crash&offset=0", []byte("crash\n"), false, false)
	if r.status != http.StatusOK {
		t.Fatalf("game log: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(dir, "_.._etc-game.log")); err != nil {
		t.Fatalf("sanitized run: %v", err)
	}
	if r := post(t, srv, tok, "kind=passwd&run=a&offset=0", []byte("x"), false, false); r.status != http.StatusBadRequest {
		t.Fatalf("bad kind: %+v", r)
	}
	// every path stays under the store
	_ = filepath.WalkDir(filepath.Dir(s.cfg.Dir), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && !strings.HasPrefix(p, s.cfg.Dir) {
			t.Errorf("file outside the store: %s", p)
		}
		return nil
	})
	waitFile(t, filepath.Join(s.cfg.Dir, "sessions", "_sala_mala_", "20261113-101500", "server-events.log"),
		"upload p7-Al_ex_ _.._etc-game reason=crash +6 bytes")
}

func TestTokenRequired(t *testing.T) {
	s, srv := newStore(t, Config{})
	if r := post(t, srv, "", "kind=zmp&run=a&offset=0", []byte("x"), false, false); r.status != http.StatusForbidden {
		t.Fatalf("no token: %+v", r)
	}
	if r := post(t, srv, "0123", "kind=zmp&run=a&offset=0", []byte("x"), false, false); r.status != http.StatusForbidden {
		t.Fatalf("bad token: %+v", r)
	}
	now := time.Now()
	s.cfg.Now = func() time.Time { return now }
	tok := s.Issue("r", "s", 1, "A")
	s.cfg.Now = func() time.Time { return now.Add(DefaultTokenTTL + time.Second) }
	if r := post(t, srv, tok, "kind=zmp&run=a&offset=0", []byte("x"), false, false); r.status != http.StatusForbidden {
		t.Fatalf("expired token: %+v", r)
	}
	// browsers: the preflight and the token in the query
	req, _ := http.NewRequest(http.MethodOptions, srv.URL+Prefix, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("preflight: %d %v", resp.StatusCode, resp.Header)
	}
	s.cfg.Now = time.Now
	tok = s.Issue("r", "s", 2, "B")
	if r := post(t, srv, "", "kind=zmp&run=a&offset=0&token="+tok, []byte("x"), false, false); r.status != http.StatusOK {
		t.Fatalf("token in the query: %+v", r)
	}
}

func TestLimits(t *testing.T) {
	s, srv := newStore(t, Config{MaxUpload: 1000, MaxPerDay: 2500})
	tok := s.Issue("r", "s", 1, "A")
	big := bytes.Repeat([]byte("a"), 1001)
	if r := post(t, srv, tok, "kind=zmp&run=a&offset=0", big, false, false); r.status != http.StatusRequestEntityTooLarge {
		t.Fatalf("too big: %+v", r)
	}
	// the cap is on what the body unpacks to, not on its gzip
	if r := post(t, srv, tok, "kind=zmp&run=a&offset=0", gz(t, bytes.Repeat([]byte("a"), 50000)), true, false); r.status != http.StatusRequestEntityTooLarge {
		t.Fatalf("gzip bomb: %+v", r)
	}
	ok := bytes.Repeat([]byte("b"), 1000)
	for i := 0; i < 2; i++ {
		if r := post(t, srv, tok, "kind=zmp&run=r"+string(rune('0'+i))+"&offset=0", ok, false, false); r.status != http.StatusOK {
			t.Fatalf("upload %d: %+v", i, r)
		}
	}
	if r := post(t, srv, tok, "kind=zmp&run=r9&offset=0", ok, false, false); r.status != http.StatusTooManyRequests {
		t.Fatalf("over the day: %+v", r)
	}
	// another player has its own day
	tok2 := s.Issue("r", "s", 2, "B")
	if r := post(t, srv, tok2, "kind=zmp&run=r9&offset=0", ok, false, false); r.status != http.StatusOK {
		t.Fatalf("other player: %+v", r)
	}
	// a new day starts again
	s.cfg.Now = func() time.Time { return time.Now().Add(24 * time.Hour) }
	tok = s.Issue("r", "s", 3, "A")
	if r := post(t, srv, tok, "kind=zmp&run=r9&offset=0", ok, false, false); r.status != http.StatusOK {
		t.Fatalf("next day: %+v", r)
	}
}

func TestTotalCapRemovesOldestFirst(t *testing.T) {
	s, srv := newStore(t, Config{MaxTotal: 10000, MaxUpload: 4000})
	tok := s.Issue("r", "s", 1, "A")
	chunk := bytes.Repeat([]byte("c"), 3000)
	dir := filepath.Join(s.cfg.Dir, "sessions", "r", "s", "p1-A")
	old := time.Now().Add(-time.Hour)
	for i, run := range []string{"a", "b", "c"} {
		if r := post(t, srv, tok, "kind=zmp&run="+run+"&offset=0", chunk, false, false); r.status != http.StatusOK {
			t.Fatalf("upload %s: %+v", run, r)
		}
		mod := old.Add(time.Duration(i) * time.Minute)
		_ = os.Chtimes(filepath.Join(dir, run+"-zmp.log"), mod, mod)
	}
	// the fourth goes over 10000: the oldest ("a", then "b") go until the store is at 9000 or less
	if r := post(t, srv, tok, "kind=zmp&run=d&offset=0", chunk, false, false); r.status != http.StatusOK {
		t.Fatalf("upload d: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(dir, "a-zmp.log")); !os.IsNotExist(err) {
		t.Fatalf("the oldest file is still there: %v", err)
	}
	for _, run := range []string{"c", "d"} {
		if _, err := os.Stat(filepath.Join(dir, run+"-zmp.log")); err != nil {
			t.Fatalf("%s removed: %v", run, err)
		}
	}
	if got := dirSize(s.cfg.Dir); got > 10000 {
		t.Fatalf("store at %d bytes", got)
	}
}

func TestServerLogAndEvents(t *testing.T) {
	s, _ := newStore(t, Config{})
	logger := log.New(io.MultiWriter(io.Discard, s.ServerLog()), "", 0)
	logger.Printf("joined room id=1")
	s.Event("sala", "20261113-000000", "DESYNC tick=40")
	s.Close()
	day := time.Now().UTC().Format("20060102")
	if got := read(t, filepath.Join(s.cfg.Dir, "server", "zmp-server-"+day+".log")); got != "joined room id=1\n" {
		t.Fatalf("server log %q", got)
	}
	if got := read(t, filepath.Join(s.cfg.Dir, "sessions", "sala", "20261113-000000", "server-events.log")); !strings.HasSuffix(got, " DESYNC tick=40\n") {
		t.Fatalf("events %q", got)
	}
	s.Event("sala", "x", "after close") // (no panic)
}

func TestSanitize(t *testing.T) {
	for in, want := range map[string]string{"": "x", "..": "x", ".": "x", "../..": "_", "Saria": "Saria", "a b/c": "a_b_c",
		"nul": "_nul", "CON.txt": "_CON.txt", "Zelda Ñ": "Zelda___", strings.Repeat("a", 60): strings.Repeat("a", 40)} {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseSize(t *testing.T) {
	for in, want := range map[string]int64{"2147483648": 2 << 30, "2G": 2 << 30, "2GiB": 2 << 30, "512M": 512 << 20, "64k": 64 << 10} {
		if got, err := ParseSize(in); err != nil || got != want {
			t.Errorf("ParseSize(%q) = %d, %v", in, got, err)
		}
	}
	if _, err := ParseSize("mucho"); err == nil {
		t.Error("ParseSize accepted text")
	}
}
