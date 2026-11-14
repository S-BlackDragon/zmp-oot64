// Command zmp-server is the ZMP room server: it orders the players' inputs per tick (lockstep), seals the events
// shared between groups and keeps each room's canonical save. It never runs the game.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/S-BlackDragon/zmp-oot64/server/internal/protocol"
	"github.com/S-BlackDragon/zmp-oot64/server/internal/server"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	listen := flag.String("listen", ":47100", "TCP address to listen on")
	dataDir := flag.String("data", "./data", "directory for canonical saves")
	webDir := flag.String("web", "", "folder with the browser build of the game to serve on the same port (empty: none)")
	showVersion := flag.Bool("version", false, "print version and exit")
	healthcheck := flag.Bool("healthcheck", false, "exit 0 if a server accepts TCP on -listen (for Docker HEALTHCHECK)")
	// Accelerated test runs (the project's test suite, D-079): a hiccup of the test PC lasts many more ticks than
	// in real time, so the local test server waits for a late input instead of filling it. 0: the default (100 ms).
	slowFill := flag.Duration("slow-fill-after", 0, "how late a connected player's input may be before its group goes on without it (0: default)")
	memEvery := flag.Duration("mem-interval", 0, "how often the memory of the process is written to the log (0: every 60 s)")
	memCollect := flag.Bool("mem-collect", false, "test setting: collect garbage before each memory line, so it says what the server holds")
	keepBlobs := flag.Bool("test-keep-blobs", false, "test setting: keep every relayed state for ever (a leak on purpose, for the memory check)")
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
	srv := server.New(server.Config{Listen: *listen, DataDir: *dataDir, Logger: logger, BuildVersion: version,
		SlowFillAfter: *slowFill, MemInterval: *memEvery, MemCollect: *memCollect, TestKeepBlobs: *keepBlobs,
		WebDir: *webDir})
	if *webDir != "" {
		if st, err := os.Stat(filepath.Join(*webDir, "index.html")); err != nil || st.IsDir() {
			logger.Fatalf("-web %s: no index.html there (use the folder of the browser build)", *webDir)
		}
		logger.Printf("serving the browser game from %s on the same port", *webDir)
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
