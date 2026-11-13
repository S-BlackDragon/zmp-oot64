package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/S-BlackDragon/zmp-oot64/server/internal/protocol"
)

// wsClient speaks the protocol over WebSocket: one binary message per body (D-239).
type wsClient struct {
	c *websocket.Conn
}

func dialWS(t *testing.T, addr string, h protocol.Hello) *wsClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws://"+addr+"/", nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	w := &wsClient{c: c}
	w.send(t, h)
	return w
}

func (w *wsClient) send(t *testing.T, v any) {
	t.Helper()
	frame, err := protocol.Encode(v)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.c.Write(ctx, websocket.MessageBinary, frame[4:]); err != nil {
		t.Fatalf("websocket write: %v", err)
	}
}

func (w *wsClient) next(t *testing.T) (string, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	typ, body, err := w.c.Read(ctx)
	if err != nil {
		t.Fatalf("websocket read: %v", err)
	}
	if typ != websocket.MessageBinary {
		t.Fatalf("websocket message type %v, want binary", typ)
	}
	typeName, err := protocol.Type(body)
	if err != nil {
		t.Fatal(err)
	}
	return typeName, body
}

func (w *wsClient) expect(t *testing.T, typ string) []byte {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got, body := w.next(t)
		if got == typ {
			return body
		}
	}
	t.Fatalf("no %s over websocket", typ)
	return nil
}

func (w *wsClient) close() { _ = w.c.Close(websocket.StatusNormalClosure, "") }

// A browser client (WebSocket) and a native client (TCP) share the port and the room.
func TestWebSocketAndTCPShareRoom(t *testing.T) {
	_, addr := startServer(t)
	tcp := dial(t, addr, hello("Nativo", "mixta", "b1"))
	tcp.expect(t, "WELCOME")

	ws := dialWS(t, addr, hello("Navegador", "mixta", "b1"))
	defer ws.close()
	welcome := decode[protocol.Welcome](t, ws.expect(t, "WELCOME"))
	if welcome.Room != "mixta" || welcome.PlayerID == 0 {
		t.Fatalf("welcome %+v", welcome)
	}
	names := map[string]bool{}
	for _, p := range welcome.Players {
		names[p.Name] = true
	}
	if !names["Nativo"] || !names["Navegador"] {
		t.Fatalf("players in WELCOME: %v", names)
	}

	// The native side learns about the browser through PLAYER_LIST.
	deadline := time.Now().Add(5 * time.Second)
	for {
		list := decode[protocol.PlayerList](t, tcp.expect(t, "PLAYER_LIST"))
		if len(list.Players) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("tcp client never saw two players: %+v", list.Players)
		}
	}

	// PING/PONG over WebSocket, and the handshake rules apply the same way.
	ws.send(t, protocol.Ping{T: "PING", Nonce: 7, SentAt: 123})
	pong := decode[protocol.Ping](t, ws.expect(t, "PONG"))
	if pong.Nonce != 7 || pong.SentAt != 123 {
		t.Fatalf("pong %+v", pong)
	}
	bad := dialWS(t, addr, hello("Intruso", "mixta", "otro-build"))
	defer bad.close()
	reject := decode[protocol.Reject](t, bad.expect(t, "REJECT"))
	if reject.Reason != "build_mismatch" {
		t.Fatalf("reject %+v", reject)
	}
}

// A plain GET on the port answers the health line; any other path is 404.
func TestHTTPHealthLine(t *testing.T) {
	_, addr := startServer(t)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(string(body), "zmp-server ") {
		t.Fatalf("healthz: %d %q", resp.StatusCode, body)
	}
	resp, err = client.Get("http://" + addr + "/otra")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("unknown path: %d", resp.StatusCode)
	}
}

// With WebDir the port also serves the browser build's files; /healthz still answers the health line, a WebSocket
// still joins a room, and nothing outside the folder is reachable.
func TestHTTPServesWebDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>zmp</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "soh.wasm"), []byte("\x00asm\x01\x00\x00\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	cfg.WebDir = dir
	_, addr := startServerCfg(t, cfg)
	client := &http.Client{Timeout: 5 * time.Second}
	get := func(path string) (int, string, string) {
		resp, err := client.Get("http://" + addr + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp.StatusCode, resp.Header.Get("Content-Type"), string(body)
	}
	if code, _, body := get("/"); code != 200 || !strings.Contains(body, "<title>zmp</title>") {
		t.Fatalf("index: %d %q", code, body)
	}
	if code, typ, _ := get("/soh.wasm"); code != 200 || typ != "application/wasm" {
		t.Fatalf("wasm: %d %q", code, typ)
	}
	if code, _, body := get("/healthz"); code != 200 || !strings.HasPrefix(body, "zmp-server ") {
		t.Fatalf("healthz: %d %q", code, body)
	}
	if code, _, _ := get("/../go.mod"); code != 404 {
		t.Fatalf("outside the folder: %d", code)
	}
	if code, _, _ := get("/missing.js"); code != 404 {
		t.Fatalf("missing file: %d", code)
	}
	ws := dialWS(t, addr, hello("web", "sala-web", "b1"))
	defer ws.close()
	ws.expect(t, "WELCOME")
}

// Bytes that are neither a frame nor HTTP close the connection without a crash, and the port keeps serving.
func TestGarbageFirstBytesClosed(t *testing.T) {
	_, addr := startServer(t)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("XXXXXXXXXXXXXXXX")); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 16)
	if _, err := conn.Read(buf); err == nil {
		t.Fatal("expected the connection to be closed")
	}
	tcp := dial(t, addr, hello("Despues", "sigue", "b1"))
	tcp.expect(t, "WELCOME")
}

// A WebSocket that goes away is removed from its room like a closed socket.
func TestWebSocketCloseLeavesRoom(t *testing.T) {
	s, addr := startServer(t)
	ws := dialWS(t, addr, hello("Fugaz", "sala-ws", "b1"))
	ws.expect(t, "WELCOME")
	ws.close()
	waitFor(t, "room released", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		_, ok := s.rooms["sala-ws"]
		return !ok
	})
}
