package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/S-BlackDragon/zmp-oot64/server/internal/protocol"
)

// transport is one client connection seen as a sequence of protocol frames (docs/PLAN_WEB.md 7.2, D-239). Two kinds
// share the listening port: raw TCP, what the native game speaks (uint32 little-endian length + MessagePack body),
// and WebSocket, what a browser can open (one binary message per MessagePack body, no length prefix). Which one a
// connection is gets decided by its first bytes (sniffAndServe); from then on the room logic does not care.
type transport interface {
	// ReadFrame returns the next message body.
	ReadFrame() ([]byte, error)
	// WriteFrame writes one frame as protocol.Encode makes it (length + body); the WebSocket side drops the length.
	WriteFrame(frame []byte) error
	Close() error
	RemoteAddr() string
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}

// tcpTransport: the original protocol over a TCP connection (the bytes already peeked at are in r).
type tcpTransport struct {
	conn net.Conn
	r    *bufio.Reader
}

func (t *tcpTransport) ReadFrame() ([]byte, error)         { return protocol.ReadFrame(t.r) }
func (t *tcpTransport) WriteFrame(frame []byte) error      { _, err := t.conn.Write(frame); return err }
func (t *tcpTransport) Close() error                       { return t.conn.Close() }
func (t *tcpTransport) RemoteAddr() string                 { return t.conn.RemoteAddr().String() }
func (t *tcpTransport) SetReadDeadline(d time.Time) error  { return t.conn.SetReadDeadline(d) }
func (t *tcpTransport) SetWriteDeadline(d time.Time) error { return t.conn.SetWriteDeadline(d) }

// wsTransport: one WebSocket binary message per protocol body. Deadlines go to the underlying TCP connection, as
// for tcpTransport: a read that times out closes the WebSocket for good, which is what the idle timeout means.
type wsTransport struct {
	ws     *websocket.Conn
	raw    net.Conn
	remote string
}

func (t *wsTransport) ReadFrame() ([]byte, error) {
	for {
		typ, data, err := t.ws.Read(context.Background())
		if err != nil {
			return nil, wsReadError(err)
		}
		if typ != websocket.MessageBinary {
			continue // (text messages are not part of the protocol; ignored like an unknown frame)
		}
		if len(data) > protocol.MaxFrameSize {
			return nil, protocol.ErrFrameTooLarge
		}
		return data, nil
	}
}

func (t *wsTransport) WriteFrame(frame []byte) error {
	if len(frame) < 4 {
		return errors.New("short frame")
	}
	return t.ws.Write(context.Background(), websocket.MessageBinary, frame[4:])
}

func (t *wsTransport) Close() error {
	_ = t.ws.CloseNow()
	return t.raw.Close()
}

func (t *wsTransport) RemoteAddr() string                 { return t.remote }
func (t *wsTransport) SetReadDeadline(d time.Time) error  { return t.raw.SetReadDeadline(d) }
func (t *wsTransport) SetWriteDeadline(d time.Time) error { return t.raw.SetWriteDeadline(d) }

// wsReadError turns the ways a peer goes away into io.EOF, which handle does not log (same as a closed TCP socket).
func wsReadError(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return io.EOF
	}
	if websocket.CloseStatus(err) != -1 {
		return io.EOF
	}
	return err
}

// peekedConn is the connection handed to the HTTP server after sniffing: reads go through the buffered reader that
// holds the peeked bytes, and Close signals the goroutine of Serve that this connection is over.
type peekedConn struct {
	net.Conn
	r    *bufio.Reader
	done chan struct{}
	once sync.Once
}

func (c *peekedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

func (c *peekedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { close(c.done) })
	return err
}

// chanListener feeds the HTTP server with the connections that turned out to speak HTTP.
type chanListener struct {
	ch   chan net.Conn
	stop chan struct{}
	addr net.Addr
	once sync.Once
}

func newChanListener(addr net.Addr) *chanListener {
	return &chanListener{ch: make(chan net.Conn), stop: make(chan struct{}), addr: addr}
}

func (l *chanListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.ch:
		return c, nil
	case <-l.stop:
		return nil, net.ErrClosed
	}
}

func (l *chanListener) Close() error {
	l.once.Do(func() { close(l.stop) })
	return nil
}

func (l *chanListener) Addr() net.Addr { return l.addr }

// hand gives a connection to the HTTP server; false if the server is stopping.
func (l *chanListener) hand(c net.Conn) bool {
	select {
	case l.ch <- c:
		return true
	case <-l.stop:
		return false
	}
}

type connKey struct{}

// httpHandler serves the HTTP side of the port: a WebSocket upgrade becomes a client like any other; a plain GET
// answers a short health line (what a tunnel, a load balancer or a monitoring tool can poll) or, with Config.WebDir,
// the files of the browser build of the game.
func (s *Server) httpHandler() http.Handler {
	var files http.Handler
	if s.cfg.WebDir != "" {
		files = http.FileServer(http.Dir(s.cfg.WebDir))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			if r.URL.Path == "/healthz" || (files == nil && r.URL.Path == "/") {
				w.Header().Set("Connection", "close")
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Header().Set("Cache-Control", "no-store")
				fmt.Fprintf(w, "zmp-server %s protocol %d\n", s.cfg.BuildVersion, protocol.Version)
				return
			}
			if files == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
				w.Header().Set("Connection", "close")
				http.NotFound(w, r)
				return
			}
			// (every player of a room must run the very same soh.wasm: the browser asks again each time instead of
			// keeping an old copy after the host updates the folder)
			w.Header().Set("Cache-Control", "no-cache")
			files.ServeHTTP(w, r)
			return
		}
		raw, _ := r.Context().Value(connKey{}).(net.Conn)
		if raw == nil {
			http.Error(w, "no connection", http.StatusInternalServerError)
			return
		}
		// (the origin is not checked: the game's own HELLO handshake decides who enters a room)
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true,
			CompressionMode: websocket.CompressionDisabled})
		if err != nil {
			s.log.Printf("%s: websocket upgrade: %v", raw.RemoteAddr(), err)
			return
		}
		ws.SetReadLimit(protocol.MaxFrameSize)
		s.handle(&wsTransport{ws: ws, raw: raw, remote: raw.RemoteAddr().String() + " (ws)"})
	})
}

// sniffAndServe reads the first bytes of a new connection and serves it as raw TCP or as HTTP/WebSocket. It returns
// when the connection is over, whichever kind it was.
func (s *Server) sniffAndServe(conn net.Conn, httpLn *chanListener) {
	r := bufio.NewReader(conn)
	_ = conn.SetReadDeadline(time.Now().Add(s.cfg.HelloTimeout))
	head, err := r.Peek(4)
	if err != nil {
		// A bare connect+close (Docker healthcheck, port probe) is not worth a log line.
		if !errors.Is(err, io.EOF) {
			s.log.Printf("%s: no HELLO: %v", conn.RemoteAddr(), err)
		}
		_ = conn.Close()
		return
	}
	if string(head) == "GET " {
		pc := &peekedConn{Conn: conn, r: r, done: make(chan struct{})}
		if !httpLn.hand(pc) {
			_ = conn.Close()
			return
		}
		<-pc.done
		return
	}
	s.handle(&tcpTransport{conn: conn, r: r})
}
