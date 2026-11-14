package hostapp

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/S-BlackDragon/zmp-oot64/server/internal/invite"
	"github.com/S-BlackDragon/zmp-oot64/server/internal/portmap"
	"github.com/S-BlackDragon/zmp-oot64/server/internal/protocol"
	"github.com/S-BlackDragon/zmp-oot64/server/internal/roomauth"
	"github.com/S-BlackDragon/zmp-oot64/server/internal/server"
)

func quietLogger() *log.Logger {
	if os.Getenv("ZMP_TEST_LOG") != "" {
		return log.New(os.Stderr, "host: ", log.Lmicroseconds)
	}
	return log.New(io.Discard, "", 0)
}

func startApp(t *testing.T, dir string, extra func(*Options)) *App {
	t.Helper()
	opts := Options{ConfigPath: filepath.Join(dir, FileName), Version: "test", Logger: quietLogger(), NoBrowser: true,
		StateFile: filepath.Join(dir, "state.json"), Listen: "127.0.0.1:0", UIListen: "127.0.0.1:0",
		ServerConfig: func(c *server.Config) { c.HelloTimeout = 2 * time.Second }}
	if extra != nil {
		extra(&opts)
	}
	a, err := Start(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Stop)
	return a
}

func apiCall(t *testing.T, a *App, method, path, token, host string, body any) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	u := strings.SplitN(a.PageURL(), "#", 2)[0]
	req, _ := http.NewRequest(method, strings.TrimSuffix(u, "/")+path, r)
	if token != "" {
		req.Header.Set("X-ZMP-Token", token)
	}
	if host != "" {
		req.Host = host
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func getState(t *testing.T, a *App) State {
	t.Helper()
	code, b := apiCall(t, a, "GET", "/api/state", a.Token(), "", nil)
	if code != 200 {
		t.Fatalf("state: %d %s", code, b)
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatal(err)
	}
	return st
}

// joinByInvite is what a game client does with a code: parse it, try its addresses, enter with the password in it.
func joinByInvite(t *testing.T, code, name, password string) (string, protocol.Reject) {
	t.Helper()
	inv, err := invite.Parse(code)
	if err != nil {
		t.Fatalf("invite %q: %v", code, err)
	}
	if password == "" {
		password = inv.Password
	}
	var conn net.Conn
	for _, a := range inv.Addresses {
		if c, err := net.DialTimeout("tcp", net.JoinHostPort(a, itoa(inv.Port)), time.Second); err == nil {
			conn = c
			break
		}
	}
	if conn == nil {
		t.Fatalf("no address of %v answers", inv.Addresses)
	}
	t.Cleanup(func() { conn.Close() })
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	h := protocol.Hello{T: "HELLO", Proto: 5, BuildHash: "b", OotHash: "o", SohHash: "s", CvarProfileHash: "c",
		Name: name, Room: inv.Room, AuthNonce: nonce, HasPassword: password != ""}
	_ = protocol.WriteFrame(conn, h)
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	read := func() (string, []byte) {
		body, err := protocol.ReadFrame(conn)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		typ, _ := protocol.Type(body)
		return typ, body
	}
	typ, body := read()
	if typ == "CHALLENGE" {
		var ch protocol.Challenge
		_ = protocol.Decode(body, &ch)
		a := protocol.Auth{T: "AUTH"}
		if k := roomauth.RoomKey(inv.Room, password); k != nil && ch.Room {
			a.RoomProof = roomauth.Proof(k, "room", ch.Nonce, nonce, inv.Room, name)
		}
		_ = protocol.WriteFrame(conn, a)
		typ, body = read()
	}
	var r protocol.Reject
	if typ == "REJECT" {
		_ = protocol.Decode(body, &r)
	}
	return typ, r
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestFirstStartAndInvite(t *testing.T) {
	dir := t.TempDir()
	a := startApp(t, dir, nil)
	fc, _, err := LoadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(fc.Room, "zmp-") || len(fc.Password) != 8 || fc.InviteSecret != "password" {
		t.Fatalf("first config %+v", fc)
	}
	st := getState(t, a)
	if st.Room != fc.Room || st.Password != fc.Password || st.Reach.Code != portmap.LocalOnly || st.Reach.MessageES == "" {
		t.Fatalf("state %+v", st)
	}
	inv, err := invite.Parse(st.Invite.Code)
	if err != nil || inv.Room != fc.Room || inv.Password != fc.Password || inv.Addresses[0] != "127.0.0.1" || inv.Port != a.GamePort() {
		t.Fatalf("invite %+v %v", inv, err)
	}
	if !strings.HasPrefix(st.Invite.Link, "zmp://join/") {
		t.Fatalf("link %q", st.Invite.Link)
	}
	// Two friends join with the code; a third with a wrong password does not; another room does not exist here.
	if typ, r := joinByInvite(t, st.Invite.Code, "Saria", ""); typ != "WELCOME" {
		t.Fatalf("first friend: %s %+v", typ, r)
	}
	if typ, r := joinByInvite(t, st.Invite.Code, "Mido", ""); typ != "WELCOME" {
		t.Fatalf("second friend: %s %+v", typ, r)
	}
	if typ, r := joinByInvite(t, st.Invite.Code, "Ruto", "mala"); typ != "REJECT" || r.Reason != "bad_password" {
		t.Fatalf("wrong password: %s %+v", typ, r)
	}
	other, _ := invite.Format(invite.Invite{Addresses: []string{"127.0.0.1"}, Port: a.GamePort(), Room: "otra"})
	if typ, r := joinByInvite(t, other, "Dar", ""); typ != "REJECT" || r.Reason != "room_not_found" {
		t.Fatalf("another room: %s %+v", typ, r)
	}
	deadline := time.Now().Add(3 * time.Second)
	for len(getState(t, a).Players) != 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if ps := getState(t, a).Players; len(ps) != 2 || ps[0].Name != "Mido" || ps[1].Name != "Saria" {
		t.Fatalf("players %+v", ps)
	}
	// The state file is there for a parent program, with the page's address.
	b, err := os.ReadFile(filepath.Join(dir, "state.json"))
	var fileSt State
	if err != nil || json.Unmarshal(b, &fileSt) != nil || fileSt.Invite.Code != st.Invite.Code || !strings.Contains(fileSt.Page, a.Token()) {
		t.Fatalf("state file: %v %s", err, b)
	}
}

func TestPageGuards(t *testing.T) {
	a := startApp(t, t.TempDir(), nil)
	if code, _ := apiCall(t, a, "GET", "/api/state", "", "", nil); code != 403 {
		t.Fatalf("no token: %d", code)
	}
	if code, _ := apiCall(t, a, "GET", "/api/state", "wrong", "", nil); code != 403 {
		t.Fatalf("wrong token: %d", code)
	}
	_, port, _ := net.SplitHostPort(a.ui.Addr().String())
	if code, _ := apiCall(t, a, "GET", "/api/state", a.Token(), "evil.example:"+port, nil); code != 403 {
		t.Fatalf("another Host (DNS rebinding): %d", code)
	}
	if code, b := apiCall(t, a, "GET", "/", "", "", nil); code != 200 || !bytes.Contains(b, []byte("X-ZMP-Token")) {
		t.Fatalf("page: %d", code)
	}
	if code, _ := apiCall(t, a, "GET", "/", "", "localhost:"+port, nil); code != 200 {
		t.Fatalf("localhost: %d", code)
	}
	if h, _, _ := net.SplitHostPort(a.ui.Addr().String()); h != "127.0.0.1" {
		t.Fatalf("the page listens on %s", h)
	}
}

func TestPasswordRoomAndHintChanges(t *testing.T) {
	dir := t.TempDir()
	a := startApp(t, dir, nil)
	old := getState(t, a)
	code, b := apiCall(t, a, "POST", "/api/password", a.Token(), "", map[string]string{"password": "  nueva\n"})
	if code != 200 {
		t.Fatalf("%d %s", code, b)
	}
	st := getState(t, a)
	if st.Password != "nueva" {
		t.Fatalf("password %q", st.Password)
	}
	if typ, r := joinByInvite(t, old.Invite.Code, "A", ""); typ != "REJECT" || r.Reason != "bad_password" {
		t.Fatalf("old code still works: %s %+v", typ, r)
	}
	if typ, _ := joinByInvite(t, st.Invite.Code, "A", ""); typ != "WELCOME" {
		t.Fatalf("new code: %s", typ)
	}
	// A hint instead of the password.
	apiCall(t, a, "POST", "/api/invite", a.Token(), "", map[string]string{"secret": "hint", "hint": "el caballo"})
	inv, _ := invite.Parse(getState(t, a).Invite.Code)
	if inv.Password != "" || inv.Hint != "el caballo" {
		t.Fatalf("hint invite %+v", inv)
	}
	if typ, r := joinByInvite(t, getState(t, a).Invite.Code, "B", ""); typ != "REJECT" || r.Reason != "password_required" {
		t.Fatalf("hint code without typing: %s %+v", typ, r)
	}
	if typ, _ := joinByInvite(t, getState(t, a).Invite.Code, "B", "nueva"); typ != "WELCOME" {
		t.Fatalf("hint code with the password typed: %s", typ)
	}
	// Another room; no password at all.
	apiCall(t, a, "POST", "/api/room", a.Token(), "", map[string]string{"room": "castillo"})
	apiCall(t, a, "POST", "/api/password", a.Token(), "", map[string]string{"password": ""})
	st = getState(t, a)
	if st.Room != "castillo" || st.PasswordSet {
		t.Fatalf("state %+v", st)
	}
	if typ, _ := joinByInvite(t, st.Invite.Code, "C", ""); typ != "WELCOME" {
		t.Fatalf("open room: %s", typ)
	}
	// Generated, and everything kept in the file for the next start.
	apiCall(t, a, "POST", "/api/password", a.Token(), "", map[string]bool{"generate": true})
	st = getState(t, a)
	if len(st.Password) != 8 {
		t.Fatalf("generated %q", st.Password)
	}
	a.Stop()
	fc, _, err := LoadFile(filepath.Join(dir, FileName))
	if err != nil || fc.Room != "castillo" || fc.Password != st.Password || fc.InviteSecret != "hint" || fc.Hint != "el caballo" {
		t.Fatalf("file after changes %+v %v", fc, err)
	}
	a2 := startApp(t, dir, nil)
	if st2 := getState(t, a2); st2.Room != "castillo" || st2.Password != st.Password {
		t.Fatalf("restart %+v", st2)
	}
}

func TestStopButton(t *testing.T) {
	dir := t.TempDir()
	a := startApp(t, dir, nil)
	port := a.GamePort()
	if code, _ := apiCall(t, a, "POST", "/api/stop", a.Token(), "", map[string]string{}); code != 200 {
		t.Fatalf("stop %d", code)
	}
	done := make(chan struct{})
	go func() { a.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the app did not stop")
	}
	if c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", itoa(port)), 300*time.Millisecond); err == nil {
		c.Close()
		t.Fatal("the game port is still open")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "state.json"))
	if !bytes.Contains(b, []byte(`"stopping": true`)) {
		t.Fatalf("state file at the end: %s", b)
	}
}

// fakeNATPMP answers NAT-PMP on 127.0.0.1 with an external address and records the lifetimes asked.
type fakeNATPMP struct {
	conn  *net.UDPConn
	mu    sync.Mutex
	lives []uint32
}

func newFakeNATPMP(t *testing.T, external string) *fakeNATPMP {
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	f := &fakeNATPMP{conn: c}
	ext := netip.MustParseAddr(external).As4()
	go func() {
		buf := make([]byte, 128)
		for {
			n, from, err := c.ReadFromUDP(buf)
			if err != nil {
				return
			}
			b := buf[:n]
			var r []byte
			switch {
			case n == 2 && b[0] == 0 && b[1] == 0:
				r = make([]byte, 12)
				r[1] = 128
				copy(r[8:], ext[:])
			case n == 12 && b[0] == 0 && b[1] == 2:
				r = make([]byte, 16)
				r[1] = 130
				copy(r[8:10], b[4:6])
				life := binary.BigEndian.Uint32(b[8:])
				if life > 0 {
					copy(r[10:12], b[4:6])
				}
				binary.BigEndian.PutUint32(r[12:], life)
				f.mu.Lock()
				f.lives = append(f.lives, life)
				f.mu.Unlock()
			default:
				continue // (PCP: no answer, the client moves on)
			}
			_, _ = c.WriteToUDP(r, from)
		}
	}()
	return f
}

func TestRouterOpensThePortAndTheCodeCarriesThePublicAddress(t *testing.T) {
	f := newFakeNATPMP(t, "203.0.113.50")
	gwPort := f.conn.LocalAddr().(*net.UDPAddr).Port
	dir := t.TempDir()
	a := startApp(t, dir, func(o *Options) {
		o.ForcePortMap = true
		o.PortMap = &portmap.Config{DisableUPnP: true, DisablePCP: true, Gateway: netip.MustParseAddr("127.0.0.1"),
			GatewayPort: gwPort, Timeout: time.Second}
	})
	deadline := time.Now().Add(5 * time.Second)
	var st State
	for time.Now().Before(deadline) {
		if st = getState(t, a); st.Reach.Code == portmap.Open {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if st.Reach.Code != portmap.Open || st.Reach.Method != portmap.MethodNATPMP || !strings.Contains(st.Reach.MessageES, "NAT-PMP") {
		t.Fatalf("reach %+v", st.Reach)
	}
	inv, err := invite.Parse(st.Invite.Code)
	if err != nil || inv.Addresses[0] != "203.0.113.50" || inv.Port != a.GamePort() {
		t.Fatalf("invite %+v %v", inv, err)
	}
	a.Stop()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.lives) < 2 || f.lives[0] == 0 || f.lives[len(f.lives)-1] != 0 {
		t.Fatalf("the rule must be asked and then removed (lifetime 0): %v", f.lives)
	}
}

func TestResolvePriority(t *testing.T) {
	yes, n := true, 5
	fc := FileConfig{Listen: ":1000", Data: "datos", ServerName: "archivo", MaxRooms: &n, ListRooms: &yes, ServerPassword: "delarchivo"}
	env := map[string]string{"ZMP_LISTEN": ":2000", "ZMP_SERVER_NAME": "entorno"}
	st, err := Resolve(map[string]string{"listen": ":3000"}, func(k string) string { return env[k] }, fc, "/base")
	if err != nil {
		t.Fatal(err)
	}
	if st.Listen != ":3000" || st.ServerName != "entorno" || st.MaxRooms != 5 || !st.ListRooms ||
		st.Data != filepath.Join("/base", "datos") || st.ServerPassword != "delarchivo" || st.Relay || st.MaxConnsPerIP != 0 {
		t.Fatalf("%+v", st)
	}
	// The relay's defaults, unless a source sets them; the password from the environment or a file, never a flag.
	pwFile := filepath.Join(t.TempDir(), "pw.txt")
	os.WriteFile(pwFile, []byte("secreta\n"), 0o600)
	env = map[string]string{"ZMP_SERVER_PASSWORD_FILE": pwFile}
	st, err = Resolve(map[string]string{"relay": "true", "max-rooms-per-ip": "1"}, func(k string) string { return env[k] }, FileConfig{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !st.Relay || st.MaxRooms != RelayMaxRooms || st.MaxConnsPerIP != RelayMaxConnsPerIP || st.MaxRoomsPerIP != 1 ||
		st.RoomKeyTTL != RelayRoomKeyTTL || st.ServerPassword != "secreta" || st.Listen != ":47100" || st.Data != "./data" {
		t.Fatalf("relay %+v", st)
	}
	if _, err := Resolve(map[string]string{"max-rooms": "-1"}, func(string) string { return "" }, FileConfig{}, ""); err == nil {
		t.Fatal("a negative bound must be an error")
	}
	if st, _ := Resolve(nil, func(k string) string { return map[string]string{"ZMP_ROOM_KEY_TTL": "3d"}[k] }, FileConfig{}, ""); st.RoomKeyTTL != 72*time.Hour {
		t.Fatalf("ttl %v", st.RoomKeyTTL)
	}
}

func TestConfigFileRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), FileName)
	fc := NewHostConfig()
	fc.Password = `com"illas\y ñ`
	fc.Hint = "línea\nnueva"
	n := 3
	fc.MaxRoomsPerIP = &n
	if err := WriteHostConfig(p, fc); err != nil {
		t.Fatal(err)
	}
	got, warn, err := LoadFile(p)
	if err != nil || len(warn) != 0 {
		t.Fatalf("%v %v", err, warn)
	}
	if got.Password != fc.Password || got.Hint != fc.Hint || got.Room != fc.Room || got.MaxRoomsPerIP == nil || *got.MaxRoomsPerIP != 3 {
		t.Fatalf("round trip %+v", got)
	}
	os.WriteFile(p, []byte("listen = \":1\"\nfuture_key = 1\n"), 0o600)
	if _, warn, err := LoadFile(p); err != nil || len(warn) != 1 {
		t.Fatalf("unknown key: %v %v", warn, err)
	}
}

// readAnnounce waits for one LAN announce on conn (or fails after the timeout).
func readAnnounce(t *testing.T, conn *net.UDPConn, timeout time.Duration) (LanAnnounce, *net.UDPAddr, bool) {
	t.Helper()
	buf := make([]byte, 8192)
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	n, from, err := conn.ReadFromUDP(buf)
	if err != nil {
		return LanAnnounce{}, nil, false
	}
	var ann LanAnnounce
	if err := json.Unmarshal(buf[:n], &ann); err != nil {
		t.Fatalf("announce is not JSON: %v %q", err, buf[:n])
	}
	return ann, from, true
}

// The double-click server announces its room for the game's "Partidas en esta red" list (D-261 format): the room,
// its players, whether it has a password, the game port; engine and build hash once somebody created the room.
func TestLanAnnounce(t *testing.T) {
	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	dir := t.TempDir()
	fc := NewHostConfig()
	fc.ServerName, fc.LanAddress, fc.LanPort = "Casa-Saria", "127.0.0.1", udp.LocalAddr().(*net.UDPAddr).Port
	if err := WriteHostConfig(filepath.Join(dir, FileName), fc); err != nil {
		t.Fatal(err)
	}
	a := startApp(t, dir, nil)
	ann, from, ok := readAnnounce(t, udp, 3*time.Second)
	if !ok {
		t.Fatal("no announce in 3 s")
	}
	if ann.ZMP != 1 || ann.Proto != 5 || ann.ServerName != "Casa-Saria" || ann.Port != a.GamePort() || len(ann.Rooms) != 1 {
		t.Fatalf("announce %+v", ann)
	}
	r := ann.Rooms[0]
	if r.Name != fc.Room || r.Players != 0 || r.Max != 6 || !r.Password || r.BuildHash != "" || r.Engine != "" {
		t.Fatalf("room %+v", r)
	}
	if !from.IP.IsLoopback() {
		t.Fatalf("sent from %v", from)
	}
	// A friend enters: the next announces count it and carry the room's build.
	st := getState(t, a)
	if typ, rj := joinByInvite(t, st.Invite.Code, "Saria", ""); typ != "WELCOME" {
		t.Fatalf("friend: %s %+v", typ, rj)
	}
	deadline := time.Now().Add(6 * time.Second)
	for {
		ann, _, ok = readAnnounce(t, udp, 3*time.Second)
		if ok && ann.Rooms[0].Players == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the announce never counted the player: %+v", ann)
		}
	}
	if r := ann.Rooms[0]; r.BuildHash != "b" || r.Engine != "native" {
		t.Fatalf("room after the join %+v", r)
	}
	// The password removed on the page: the list says the room is open.
	if code, b := apiCall(t, a, "POST", "/api/password", a.Token(), "", map[string]string{"password": ""}); code != 200 {
		t.Fatalf("password: %d %s", code, b)
	}
	deadline = time.Now().Add(6 * time.Second)
	for {
		ann, _, ok = readAnnounce(t, udp, 3*time.Second)
		if ok && !ann.Rooms[0].Password {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the announce still says password: %+v", ann)
		}
	}
	// Stopped: no more announces.
	a.Stop()
	for {
		if _, _, ok := readAnnounce(t, udp, 2*LanEvery+500*time.Millisecond); !ok {
			break
		}
	}
	if _, _, ok := readAnnounce(t, udp, 2*LanEvery+500*time.Millisecond); ok {
		t.Fatal("an announce after Stop")
	}
}

// Nothing is announced when the file says no, or when the server listens on this PC only.
func TestLanAnnounceOff(t *testing.T) {
	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	port := udp.LocalAddr().(*net.UDPAddr).Port
	no := false
	for _, c := range []struct {
		name string
		fc   func(*FileConfig)
	}{
		{"lan_announce = false", func(fc *FileConfig) { fc.LanAnnounce, fc.LanAddress, fc.LanPort = &no, "127.0.0.1", port }},
		{"listening on 127.0.0.1", func(fc *FileConfig) { fc.LanPort = port }},
	} {
		dir := t.TempDir()
		fc := NewHostConfig()
		c.fc(&fc)
		if err := WriteHostConfig(filepath.Join(dir, FileName), fc); err != nil {
			t.Fatal(err)
		}
		a := startApp(t, dir, nil)
		if ann, _, ok := readAnnounce(t, udp, LanEvery+time.Second); ok {
			t.Fatalf("%s: announced %+v", c.name, ann)
		}
		a.Stop()
	}
	// The keys survive a round trip through the file.
	dir := t.TempDir()
	fc := NewHostConfig()
	fc.LanAnnounce, fc.LanAddress, fc.LanPort = &no, "127.0.0.1", 47311
	path := filepath.Join(dir, FileName)
	if err := WriteHostConfig(path, fc); err != nil {
		t.Fatal(err)
	}
	back, warn, err := LoadFile(path)
	if err != nil || len(warn) != 0 || back.LanAnnounce == nil || *back.LanAnnounce || back.LanAddress != "127.0.0.1" || back.LanPort != 47311 {
		t.Fatalf("round trip %+v %v %v", back, warn, err)
	}
}
