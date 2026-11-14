package server

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/S-BlackDragon/zmp-oot64/server/internal/protocol"
	"github.com/S-BlackDragon/zmp-oot64/server/internal/zmplogs"
)

func uploadTo(t *testing.T, addr, token, query string, body []byte) int {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, _ = w.Write(body)
	_ = w.Close()
	req, _ := http.NewRequest(http.MethodPost, "http://"+addr+zmplogs.Prefix+"?"+query, &b)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// D-247: a server with a log store announces it in WELCOME with a token, and takes that client's upload on the game's
// own port (the first bytes say HTTP); the files are named from what the server knows about the connection.
func TestLogStoreAnnouncedAndUpload(t *testing.T) {
	cfg := testConfig()
	cfg.LogDir = filepath.Join(t.TempDir(), "logs")
	s, addr := startServerCfg(t, cfg)
	c := dial(t, addr, hello("Saria/../x", "sala-1", "b"))
	w := decode[protocol.Welcome](t, c.expect(t, "WELCOME"))
	if !w.LogCollect || len(w.LogToken) != 32 {
		t.Fatalf("WELCOME log_collect=%v token=%q", w.LogCollect, w.LogToken)
	}
	if st := uploadTo(t, addr, w.LogToken, "kind=zmp&run=20261113-120000&reason=periodic&offset=0", []byte("zmp: init\n")); st != http.StatusOK {
		t.Fatalf("upload status %d", st)
	}
	if st := uploadTo(t, addr, "otro", "kind=zmp&run=r&offset=0", []byte("x")); st != http.StatusForbidden {
		t.Fatalf("upload with a token nobody was given: %d", st)
	}
	s.mu.Lock()
	session := s.rooms["sala-1"].session
	s.mu.Unlock()
	dir := filepath.Join(cfg.LogDir, "sessions", "sala-1", session)
	got, err := os.ReadFile(filepath.Join(dir, "p1-Saria_.._x", "20261113-120000-zmp.log"))
	if err != nil || string(got) != "zmp: init\n" {
		t.Fatalf("stored %q, %v", got, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		ev, _ := os.ReadFile(filepath.Join(dir, "server-events.log"))
		sl, _ := os.ReadFile(filepath.Join(cfg.LogDir, "server", "zmp-server-"+time.Now().UTC().Format("20060102")+".log"))
		if strings.Contains(string(ev), `joined id=1 name="Saria/../x"`) && strings.Contains(string(ev), "upload p1-Saria_.._x") &&
			strings.Contains(string(sl), `joined room id=1`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("events %q; server log %q", ev, sl)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Without a store nothing is announced and nothing is taken (public servers that do not collect never get logs).
func TestNoLogStoreNothingAnnounced(t *testing.T) {
	_, addr := startServer(t)
	c := dial(t, addr, hello("A", "sala-2", "b"))
	w := decode[protocol.Welcome](t, c.expect(t, "WELCOME"))
	if w.LogCollect || w.LogToken != "" {
		t.Fatalf("WELCOME of a server without a store: log_collect=%v token=%q", w.LogCollect, w.LogToken)
	}
	if st := uploadTo(t, addr, "x", "kind=zmp&run=r&offset=0", []byte("x")); st != http.StatusNotFound {
		t.Fatalf("upload to a server without a store: %d", st)
	}
}
