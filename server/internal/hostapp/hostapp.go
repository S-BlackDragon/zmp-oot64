package hostapp

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/S-BlackDragon/zmp-oot64/server/internal/invite"
	"github.com/S-BlackDragon/zmp-oot64/server/internal/portmap"
	"github.com/S-BlackDragon/zmp-oot64/server/internal/roomauth"
	"github.com/S-BlackDragon/zmp-oot64/server/internal/server"
)

//go:embed page.html
var pageHTML []byte

// Options of Run.
type Options struct {
	ConfigPath string // the zmp-server.toml (created if missing)
	Version    string
	Logger     *log.Logger
	NoBrowser  bool   // never open the browser (the desktop app's child, tests)
	StateFile  string // rewritten with the state JSON at each change ("" none)
	// Listen and UIListen replace the file's (tests: 127.0.0.1:0).
	Listen, UIListen string
	// PortMap replaces the router settings (tests: a fake router); nil asks the real one. ForcePortMap asks the
	// router even when the game port is on 127.0.0.1 (tests only: they never listen elsewhere).
	PortMap      *portmap.Config
	ForcePortMap bool
	// Ready is called once both listeners are up, with the page URL (token included).
	Ready func(pageURL string)
	// Stop, when closed, stops the app as the page's Stop button does.
	Stop <-chan struct{}
	// ServerConfig adjusts the game server's configuration before it starts (tests: short timers).
	ServerConfig func(*server.Config)
}

// App is a running host server.
type App struct {
	opts  Options
	log   *log.Logger
	dir   string
	token string

	mu   sync.Mutex
	fc   FileConfig
	port int
	srv  *server.Server
	pm   *portmap.Manager
	ln   net.Listener
	ui   net.Listener
	page string

	stopOnce sync.Once
	stopped  chan struct{}
	lastJSON []byte
}

// PlayerState is one player on the page.
type PlayerState struct {
	Name  string `json:"name"`
	Room  string `json:"room"`
	State string `json:"state"`
	RTT   int    `json:"rtt"`
	Color int    `json:"color"`
}

// InviteState is the code on the page.
type InviteState struct {
	Code      string   `json:"code"`
	Link      string   `json:"link"`
	Addresses []string `json:"addresses"`
}

// ReachState is portmap.State with its texts.
type ReachState struct {
	portmap.State
	MessageES string `json:"message_es"`
	MessageEN string `json:"message_en"`
	AdviceES  string `json:"advice_es"`
	AdviceEN  string `json:"advice_en"`
}

// State is GET /api/state and the state file (protocol 5 notes 9.1).
type State struct {
	Version      string        `json:"version"`
	Listen       string        `json:"listen"`
	Port         int           `json:"port"`
	Room         string        `json:"room"`
	Password     string        `json:"password"`
	PasswordSet  bool          `json:"password_set"`
	InviteSecret string        `json:"invite_secret"`
	Hint         string        `json:"hint"`
	Invite       InviteState   `json:"invite"`
	Players      []PlayerState `json:"players"`
	Reach        ReachState    `json:"reach"`
	Page         string        `json:"page,omitempty"` // (state file only: the page URL with its token)
	Stopping     bool          `json:"stopping"`
}

// Run starts the host app and returns when it stops (page button, Options.Stop, or an error at start).
func Run(opts Options) error {
	a, err := Start(opts)
	if err != nil {
		return err
	}
	a.Wait()
	return nil
}

// Start starts the host app (Run without the wait).
func Start(opts Options) (*App, error) {
	if opts.Logger == nil {
		opts.Logger = log.New(os.Stdout, "", log.LstdFlags)
	}
	a := &App{opts: opts, log: opts.Logger, dir: filepath.Dir(opts.ConfigPath), stopped: make(chan struct{})}
	tok := make([]byte, 16)
	_, _ = rand.Read(tok)
	a.token = hex.EncodeToString(tok)

	fc, created, err := loadOrCreate(opts.ConfigPath, a.log)
	if err != nil {
		return nil, err
	}
	if fc.Room == "" || len(fc.Room) > 64 {
		fc.Room = "zmp-" + RandomWord(4)
		created = true
	}
	switch fc.InviteSecret {
	case "password", "hint", "none":
	default:
		fc.InviteSecret = "password"
	}
	if fc.UIListen == "" {
		fc.UIListen = "127.0.0.1:0"
	}
	if fc.Listen == "" {
		fc.Listen = ":47100"
	}
	if created {
		if err := WriteHostConfig(opts.ConfigPath, fc); err != nil {
			return nil, fmt.Errorf("writing %s: %w", opts.ConfigPath, err)
		}
	}
	a.fc = fc
	st, err := Resolve(map[string]string{}, os.Getenv, fc, a.dir)
	if err != nil {
		return nil, err
	}
	listen := st.Listen
	if opts.Listen != "" {
		listen = opts.Listen
	}
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return nil, fmt.Errorf("the game port %s is not free (another zmp-server running?): %w", listen, err)
	}
	a.ln = ln
	a.port = ln.Addr().(*net.TCPAddr).Port

	cfg := server.Config{Listen: listen, DataDir: st.Data, Logger: a.log, BuildVersion: opts.Version,
		ServerName: st.ServerName, ServerPassword: st.ServerPassword, RequireRoomPassword: st.RequireRoomPassword,
		MaxRooms: st.MaxRooms, MaxConnsPerIP: st.MaxConnsPerIP, MaxRoomsPerIP: st.MaxRoomsPerIP,
		ListRooms: st.ListRooms, RoomKeyTTL: st.RoomKeyTTL}
	if opts.ServerConfig != nil {
		opts.ServerConfig(&cfg)
	}
	a.srv = server.New(cfg)
	if err := a.srv.PrepareDataDir(); err != nil {
		ln.Close()
		return nil, fmt.Errorf("data folder %s: %w", st.Data, err)
	}
	a.srv.SetAllowedRooms([]string{fc.Room})
	if err := a.srv.SetFixedRoomPassword(fc.Room, fc.Password); err != nil {
		ln.Close()
		return nil, err
	}
	go func() { _ = a.srv.Serve(ln) }()

	// The page: 127.0.0.1 only, whatever the file says.
	uiAddr := fc.UIListen
	if opts.UIListen != "" {
		uiAddr = opts.UIListen
	}
	if h, _, err := net.SplitHostPort(uiAddr); err != nil || !isLoopbackHost(h) {
		a.log.Printf("host page: ui_listen %q is not on 127.0.0.1: using 127.0.0.1:0", uiAddr)
		uiAddr = "127.0.0.1:0"
	}
	ui, err := net.Listen("tcp", uiAddr)
	if err != nil {
		ui, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			a.srv.Close()
			return nil, err
		}
	}
	a.ui = ui
	a.page = fmt.Sprintf("http://%s/#t=%s", ui.Addr().String(), a.token)
	go func() { _ = (&http.Server{Handler: a.handler(), ReadHeaderTimeout: 5 * time.Second}).Serve(ui) }()

	// The router.
	mapping := boolOr(fc.PortMapping, true)
	if host, _, _ := net.SplitHostPort(listen); isLoopbackHost(host) && !opts.ForcePortMap {
		a.log.Printf("port mapping: off (listening on this PC only)")
	} else if !mapping {
		a.log.Printf("port mapping: off (port_mapping = false)")
	} else {
		pc := portmap.Config{}
		if opts.PortMap != nil {
			pc = *opts.PortMap
		}
		pc.Port, pc.Logger = a.port, a.log
		a.pm = portmap.Start(pc)
	}

	go a.watch()
	go a.announceLAN() // ("Partidas en esta red", D-261)
	a.log.Printf("host server: room %q, %s, game port %d; page %s", fc.Room, passwordWord(fc.Password), a.port,
		strings.SplitN(a.page, "#", 2)[0])
	a.log.Printf("host server: open %s to see the invite code (this window is the log; close it or press Stop to end)", a.page)
	a.writeState()
	if opts.Ready != nil {
		opts.Ready(a.page)
	}
	if !opts.NoBrowser && boolOr(fc.OpenBrowser, true) {
		if err := openBrowser(a.page); err != nil {
			a.log.Printf("host page: could not open the browser (%v): open the address above by hand", err)
		}
	}
	go func() {
		if opts.Stop != nil {
			select {
			case <-opts.Stop:
				a.Stop()
			case <-a.stopped:
			}
		}
	}()
	return a, nil
}

func passwordWord(p string) string {
	if p == "" {
		return "no password"
	}
	return "with a password"
}

func isLoopbackHost(h string) bool {
	if h == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(h)
	return err == nil && ip.IsLoopback()
}

func loadOrCreate(path string, logger *log.Logger) (FileConfig, bool, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		logger.Printf("host server: first start, writing %s", path)
		return NewHostConfig(), true, nil
	}
	fc, warn, err := LoadFile(path)
	for _, w := range warn {
		logger.Printf("%s", w)
	}
	return fc, false, err
}

// Wait returns when the app has stopped.
func (a *App) Wait() { <-a.stopped }

// PageURL is the page's address with its token.
func (a *App) PageURL() string { return a.page }

// GamePort is the game port it listens on.
func (a *App) GamePort() int { return a.port }

// Token is the page's API token.
func (a *App) Token() string { return a.token }

// Stop closes everything: the router rule is removed, the rooms are saved.
func (a *App) Stop() {
	a.stopOnce.Do(func() {
		a.log.Printf("host server: stopping")
		a.mu.Lock()
		a.writeStateLocked(true)
		a.mu.Unlock()
		if a.pm != nil {
			a.pm.Close()
		}
		a.srv.Close()
		_ = a.ui.Close()
		close(a.stopped)
	})
}

// watch confirms reachability when a player comes from a public address and keeps the state file current.
func (a *App) watch() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	var changed <-chan struct{}
	if a.pm != nil {
		changed = a.pm.Changed()
	}
	for {
		select {
		case <-a.stopped:
			return
		case <-changed:
		case <-t.C:
		}
		if a.pm != nil {
			for _, p := range a.srv.Players() {
				if ip, err := netip.ParseAddr(p.Remote); err == nil && server.IsPublicAddress(ip) {
					a.pm.Confirm()
				}
			}
		}
		a.writeState()
	}
}

// State gives the page's state now.
func (a *App) State() State {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.stateLocked()
}

func (a *App) stateLocked() State {
	fc := a.fc
	st := State{Version: a.opts.Version, Listen: a.ln.Addr().String(), Port: a.port, Room: fc.Room,
		Password: fc.Password, PasswordSet: fc.Password != "", InviteSecret: fc.InviteSecret, Hint: fc.Hint,
		Players: []PlayerState{}}
	for _, p := range a.srv.Players() {
		st.Players = append(st.Players, PlayerState{Name: p.Name, Room: p.Room, State: p.State, RTT: p.RTT, Color: p.Color})
	}
	var rs portmap.State
	switch {
	case a.pm != nil:
		rs = a.pm.State()
	default:
		host, _, _ := net.SplitHostPort(a.ln.Addr().String())
		rs = portmap.State{Code: portmap.Disabled, LAN: portmap.LANAddresses(netip.Addr{})}
		if isLoopbackHost(host) {
			rs = portmap.State{Code: portmap.LocalOnly}
		}
	}
	st.Reach.State = rs
	st.Reach.MessageES, st.Reach.MessageEN, st.Reach.AdviceES, st.Reach.AdviceEN = portmap.Texts(rs, a.port)
	st.Invite = a.inviteLocked(rs)
	return st
}

// inviteLocked builds the code: the public address the host wrote, the router's external address when the port
// is open, then the LAN addresses (the friends at home), at most four. When the router gave another external
// port, the LAN addresses go as ws:// URLs with the real port (the code carries one port).
func (a *App) inviteLocked(rs portmap.State) InviteState {
	fc := a.fc
	port := a.port
	var addrs []string
	if fc.PublicAddress != "" {
		addrs = append(addrs, strings.TrimSpace(fc.PublicAddress))
	}
	open := rs.Code == portmap.Open || rs.Code == portmap.Confirmed && rs.Before != portmap.CGNAT
	if open && rs.External != "" {
		if ip, err := netip.ParseAddr(rs.External); err == nil && server.IsPublicAddress(ip) {
			addrs = append(addrs, rs.External)
		}
	}
	extPort := port
	if open && rs.ExternalPort != 0 {
		extPort = rs.ExternalPort
	}
	lan := rs.LAN
	if rs.Code == portmap.LocalOnly {
		lan = []string{"127.0.0.1"}
	}
	for _, l := range lan {
		if extPort != port {
			l = fmt.Sprintf("ws://%s/", net.JoinHostPort(l, strconv.Itoa(port)))
		}
		addrs = append(addrs, l)
	}
	seen := map[string]bool{}
	var list []string
	for _, x := range addrs {
		if x != "" && !seen[x] && len(list) < invite.MaxAddresses {
			seen[x] = true
			list = append(list, x)
		}
	}
	if len(list) == 0 {
		list = []string{"127.0.0.1"}
	}
	inv := invite.Invite{Addresses: list, Port: extPort, Room: fc.Room}
	switch {
	case fc.Password != "" && fc.InviteSecret == "password":
		inv.Password = fc.Password
	case fc.Password != "" && fc.InviteSecret == "hint" && fc.Hint != "":
		inv.Hint = fc.Hint
	}
	code, err := invite.Format(inv)
	if err != nil {
		a.log.Printf("host page: invite code: %v", err)
		return InviteState{Addresses: list}
	}
	link, _ := invite.Link(inv)
	return InviteState{Code: code, Link: link, Addresses: list}
}

func (a *App) writeState() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.writeStateLocked(false)
}

func (a *App) writeStateLocked(stopping bool) {
	if a.opts.StateFile == "" {
		return
	}
	st := a.stateLocked()
	st.Stopping, st.Page = stopping, a.page
	b, _ := json.MarshalIndent(st, "", "  ")
	if string(b) == string(a.lastJSON) {
		return
	}
	a.lastJSON = b
	tmp := a.opts.StateFile + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err == nil {
		_ = os.Rename(tmp, a.opts.StateFile)
	}
}

// ---------------------------------------------------------------------------------------------------------------
// The page and its API

func (a *App) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; frame-ancestors 'none'")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(pageHTML)
	})
	api := func(method, path string, fn func(w http.ResponseWriter, r *http.Request)) {
		mux.HandleFunc(method+" "+path, func(w http.ResponseWriter, r *http.Request) {
			if !roomauth.Equal([]byte(r.Header.Get("X-ZMP-Token")), []byte(a.token)) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			fn(w, r)
		})
	}
	reply := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	api("GET", "/api/state", func(w http.ResponseWriter, r *http.Request) { reply(w, a.State()) })
	api("POST", "/api/password", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Password string `json:"password"`
			Generate bool   `json:"generate"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		pw := roomauth.Normalize(req.Password)
		if req.Generate {
			pw = RandomWord(8)
		}
		if len(pw) > roomauth.MaxPassword {
			http.Error(w, "password longer than 64 bytes", http.StatusBadRequest)
			return
		}
		if err := a.update(func(fc *FileConfig) error {
			if err := a.srv.SetFixedRoomPassword(fc.Room, pw); err != nil {
				return err
			}
			fc.Password = pw
			return nil
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.log.Printf("host page: room password %s", map[bool]string{true: "removed", false: "changed"}[pw == ""])
		reply(w, a.State())
	})
	api("POST", "/api/room", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Room string `json:"room"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		room := strings.TrimSpace(req.Room)
		if room == "" || len(room) > 64 {
			http.Error(w, "the room must be 1 to 64 bytes", http.StatusBadRequest)
			return
		}
		if err := a.update(func(fc *FileConfig) error {
			old := fc.Room
			if room == old {
				return nil
			}
			if err := a.srv.SetFixedRoomPassword(room, fc.Password); err != nil {
				return err
			}
			a.srv.SetAllowedRooms([]string{room})
			_ = a.srv.SetFixedRoomPassword(old, "") // (its players stay until they leave; nobody else gets in)
			fc.Room = room
			return nil
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.log.Printf("host page: room is now %q", room)
		reply(w, a.State())
	})
	api("POST", "/api/invite", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Secret string `json:"secret"`
			Hint   string `json:"hint"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil ||
			req.Secret != "password" && req.Secret != "hint" && req.Secret != "none" || len(req.Hint) > invite.MaxSecret {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := a.update(func(fc *FileConfig) error {
			fc.InviteSecret, fc.Hint = req.Secret, strings.TrimSpace(req.Hint)
			return nil
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		reply(w, a.State())
	})
	api("POST", "/api/stop", func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]bool{"stopping": true})
		go a.Stop()
	})
	// Only requests addressed to this page by name: a web page cannot reach it through DNS rebinding.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		_, port, _ := net.SplitHostPort(a.ui.Addr().String())
		if host != "127.0.0.1:"+port && host != "localhost:"+port && host != "[::1]:"+port {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// update changes the configuration, applies it and writes the file.
func (a *App) update(fn func(fc *FileConfig) error) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	fc := a.fc
	if err := fn(&fc); err != nil {
		return err
	}
	a.fc = fc
	if err := WriteHostConfig(a.opts.ConfigPath, fc); err != nil {
		a.log.Printf("host page: %s not saved: %v", a.opts.ConfigPath, err)
	}
	a.writeStateLocked(false)
	return nil
}
