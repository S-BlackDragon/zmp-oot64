// Command zmp-server is the ZMP room server: it orders the players' inputs per tick (lockstep), seals the events
// shared between groups and keeps each room's canonical save. It never runs the game.
//
// With no arguments on Windows (a double click) or with -app it is the host app: it makes its zmp-server.toml next to
// itself, asks the router for the port, announces its room on the LAN and opens a page on 127.0.0.1 with the invite
// code (docs-zmp guide "Host a game"). Otherwise options come from flags, then ZMP_* variables, then the file given by
// -config or ZMP_CONFIG.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/S-BlackDragon/zmp-oot64/server/internal/hostapp"
	"github.com/S-BlackDragon/zmp-oot64/server/internal/protocol"
	"github.com/S-BlackDragon/zmp-oot64/server/internal/server"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	listen := flag.String("listen", ":47100", "TCP address to listen on (ZMP_LISTEN, listen)")
	dataDir := flag.String("data", "./data", "directory for canonical saves and room keys (ZMP_DATA, data)")
	webDir := flag.String("web", "", "folder with the browser build of the game to serve on the same port (empty: none)")
	showVersion := flag.Bool("version", false, "print version and exit")
	healthcheck := flag.Bool("healthcheck", false, "exit 0 if a server accepts TCP on -listen (for Docker HEALTHCHECK)")
	// Accelerated test runs (the project's test suite, D-079): a hiccup of the test PC lasts many more ticks than
	// in real time, so the local test server waits for a late input instead of filling it. 0: the default (100 ms).
	slowFill := flag.Duration("slow-fill-after", 0, "how late a connected player's input may be before its group goes on without it (0: default)")
	memEvery := flag.Duration("mem-interval", 0, "how often the memory of the process is written to the log (0: every 60 s)")
	memCollect := flag.Bool("mem-collect", false, "test setting: collect garbage before each memory line, so it says what the server holds")
	keepBlobs := flag.Bool("test-keep-blobs", false, "test setting: keep every relayed state for ever (a leak on purpose, for the memory check)")
	// Protocol 5 and hosting. Their values also come from ZMP_* variables and the -config file.
	appMode := flag.Bool("app", false, "host app: config next to the program (or -config), router port, LAN announce, local page with the invite code (the default with no arguments on Windows)")
	configPath := flag.String("config", "", "zmp-server.toml to read (empty: ZMP_CONFIG; in the host app, the one next to the program)")
	noBrowser := flag.Bool("no-browser", false, "host app: do not open the page in the browser")
	stateFile := flag.String("state-file", "", "host app: write the page's state JSON to this file at each change (for a parent program)")
	flag.String("name", "", "server name shown to players (ZMP_SERVER_NAME, server_name)")
	flag.Bool("require-room-password", false, "refuse to make rooms without a password (ZMP_REQUIRE_ROOM_PASSWORD)")
	flag.String("server-password-file", "", "file with a password to use this server at all (ZMP_SERVER_PASSWORD_FILE, or ZMP_SERVER_PASSWORD; never on the command line)")
	flag.Int("max-rooms", 0, "most rooms open at once, 0 no bound (relay: 100)")
	flag.Int("max-conns-per-ip", 0, "most game connections from one address, 0 no bound (relay: 8; keep 0 behind a proxy or tunnel that hides addresses)")
	flag.Int("max-rooms-per-ip", 0, "most open rooms made from one address, 0 no bound (relay: 3)")
	flag.Bool("list-rooms", false, "answer ROOM_LIST_REQUEST with the open rooms (never with a server password)")
	flag.String("room-key-ttl", "", "forget the password of a room unused this long: 30m, 12h, 7d (empty: never; relay: 7d)")
	flag.Bool("relay", false, "public relay: anyone may make rooms, with the relay limits above unless set")
	flag.Parse()

	if *healthcheck {
		addr := *listen
		if strings.HasPrefix(addr, ":") {
			addr = "127.0.0.1" + addr
		}
		c, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			os.Exit(1)
		}
		c.Close()
		return
	}

	if *showVersion {
		fmt.Printf("zmp-server %s protocol %d\n", version, protocol.Version)
		return
	}

	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
	if *appMode || runtime.GOOS == "windows" && len(os.Args) == 1 {
		runHostApp(logger, *configPath, *noBrowser, *stateFile)
		return
	}
	setFlags := map[string]string{}
	flag.Visit(func(f *flag.Flag) { setFlags[f.Name] = f.Value.String() })
	var fileCfg hostapp.FileConfig
	baseDir := ""
	cp := *configPath
	if cp == "" {
		cp = strings.TrimSpace(os.Getenv("ZMP_CONFIG"))
	}
	if cp != "" {
		fc, warn, err := hostapp.LoadFile(cp)
		if err != nil {
			logger.Fatalf("config: %v", err)
		}
		for _, w := range warn {
			logger.Printf("config: %s", w)
		}
		fileCfg, baseDir = fc, filepath.Dir(cp)
		logger.Printf("config: %s", cp)
	}
	st, err := hostapp.Resolve(setFlags, os.Getenv, fileCfg, baseDir)
	if err != nil {
		logger.Fatalf("config: %v", err)
	}
	*listen, *dataDir = st.Listen, st.Data
	srv := server.New(server.Config{Listen: *listen, DataDir: *dataDir, Logger: logger, BuildVersion: version,
		SlowFillAfter: *slowFill, MemInterval: *memEvery, MemCollect: *memCollect, TestKeepBlobs: *keepBlobs,
		WebDir:     *webDir,
		ServerName: st.ServerName, ServerPassword: st.ServerPassword, RequireRoomPassword: st.RequireRoomPassword,
		MaxRooms: st.MaxRooms, MaxConnsPerIP: st.MaxConnsPerIP, MaxRoomsPerIP: st.MaxRoomsPerIP,
		ListRooms: st.ListRooms, RoomKeyTTL: st.RoomKeyTTL})
	if *webDir != "" {
		if fi, err := os.Stat(filepath.Join(*webDir, "index.html")); err != nil || fi.IsDir() {
			logger.Fatalf("-web %s: no index.html there (use the folder of the browser build)", *webDir)
		}
		logger.Printf("serving the browser game from %s on the same port", *webDir)
	}
	if st.Relay {
		logger.Printf("relay: anyone may make rooms; at most %d rooms, %d connections and %d rooms per address, passwords of unused rooms forgotten after %v",
			st.MaxRooms, st.MaxConnsPerIP, st.MaxRoomsPerIP, st.RoomKeyTTL)
	}
	if st.ServerPassword != "" {
		logger.Printf("server password: set (protocol 2 to 4 clients cannot enter)")
	}
	if st.RequireRoomPassword {
		logger.Printf("rooms: only with a password")
	}
	if *memCollect || *keepBlobs {
		logger.Printf("mem-collect %v, test-keep-blobs %v (test settings)", *memCollect, *keepBlobs)
	}
	if *slowFill != 0 {
		logger.Printf("slow-fill-after %v (test setting)", *slowFill)
	}
	if err := srv.PrepareDataDir(); err != nil {
		logger.Fatalf("data dir %s: %v", *dataDir, err)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-sig
		logger.Printf("signal %v, shutting down", s)
		srv.Close()
	}()

	if err := srv.ListenAndServe(); err != nil {
		logger.Fatalf("listen: %v", err)
	}
	logger.Printf("stopped")
}

// runHostApp is the double-click server: its file next to the program (or -config), the router, the LAN announce and
// the local page; closing the console window stops it and removes the router rule.
func runHostApp(logger *log.Logger, configPath string, noBrowser bool, stateFile string) {
	if configPath == "" {
		configPath = os.Getenv("ZMP_CONFIG")
	}
	if configPath == "" {
		exe, err := os.Executable()
		if err != nil {
			logger.Fatalf("host app: %v", err)
		}
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		configPath = filepath.Join(filepath.Dir(exe), hostapp.FileName)
	}
	stop := make(chan struct{})
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM) // (closing the console window arrives as SIGTERM)
	go func() {
		s := <-sig
		logger.Printf("signal %v, shutting down", s)
		close(stop)
	}()
	err := hostapp.Run(hostapp.Options{ConfigPath: configPath, Version: version, Logger: logger, NoBrowser: noBrowser,
		StateFile: stateFile, Stop: stop})
	if err != nil {
		logger.Printf("host app: %v", err)
		fmt.Println("No se pudo arrancar el servidor / The server could not start. Pulsa Intro / Press Enter.")
		_, _ = fmt.Scanln()
		os.Exit(1)
	}
	logger.Printf("stopped")
}
