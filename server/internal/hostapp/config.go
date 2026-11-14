// Package hostapp is the zmp-server's configuration file and its double-click mode (the protocol 5 design notes
// section 9): the server creates its configuration next to itself, asks the router for the port, and serves on
// 127.0.0.1 a small page with the invite code, the room, its password, the players, the reachability in plain words
// and a Stop button.
package hostapp

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// FileName is the configuration file the server looks for next to itself.
const FileName = "zmp-server.toml"

// FileConfig is zmp-server.toml. Pointers tell "not in the file" from "false/0". The server keys work in any mode;
// the host keys only in the host app (-app, or a double click on Windows).
type FileConfig struct {
	Listen              string `toml:"listen"`
	Data                string `toml:"data"`
	ServerName          string `toml:"server_name"`
	ServerPassword      string `toml:"server_password"`
	ServerPasswordFile  string `toml:"server_password_file"`
	RequireRoomPassword *bool  `toml:"require_room_password"`
	MaxRooms            *int   `toml:"max_rooms"`
	MaxConnsPerIP       *int   `toml:"max_conns_per_ip"`
	MaxRoomsPerIP       *int   `toml:"max_rooms_per_ip"`
	ListRooms           *bool  `toml:"list_rooms"`
	RoomKeyTTL          string `toml:"room_key_ttl"`
	Relay               *bool  `toml:"relay"`

	// Host app.
	Room          string `toml:"room"`
	Password      string `toml:"password"`
	InviteSecret  string `toml:"invite_secret"` // password, hint or none
	Hint          string `toml:"hint"`
	PortMapping   *bool  `toml:"port_mapping"`
	PublicAddress string `toml:"public_address"` // a domain or fixed public IP, first in the invite code
	OpenBrowser   *bool  `toml:"open_browser"`
	UIListen      string `toml:"ui_listen"`
	// "Partidas en esta red" (D-261): announce the room by UDP on the LAN (default true); the UDP port (47100) and,
	// for tests, one address to send it to instead of the multicast group and broadcast ("127.0.0.1").
	LanAnnounce *bool  `toml:"lan_announce"`
	LanPort     int    `toml:"lan_port"`
	LanAddress  string `toml:"lan_address"`
}

// LoadFile reads a configuration file. Unknown keys come back as warnings (never an error: a newer file still
// starts an older server).
func LoadFile(path string) (FileConfig, []string, error) {
	var fc FileConfig
	md, err := toml.DecodeFile(path, &fc)
	if err != nil {
		return fc, nil, fmt.Errorf("%s: %w", path, err)
	}
	var warn []string
	for _, k := range md.Undecoded() {
		warn = append(warn, fmt.Sprintf("%s: unknown key %q ignored", filepath.Base(path), k.String()))
	}
	return fc, warn, nil
}

// Settings are the server's effective options, once flags, environment and file are merged.
type Settings struct {
	Listen              string
	Data                string
	ServerName          string
	ServerPassword      string
	RequireRoomPassword bool
	MaxRooms            int
	MaxConnsPerIP       int
	MaxRoomsPerIP       int
	ListRooms           bool
	RoomKeyTTL          time.Duration
	Relay               bool
}

// Relay defaults (protocol 5 notes section 8), for what no source sets.
const (
	RelayMaxRooms      = 100
	RelayMaxConnsPerIP = 8
	RelayMaxRoomsPerIP = 3
	RelayRoomKeyTTL    = 7 * 24 * time.Hour
)

// Resolve merges the sources, the first that has a value wins: flags (only those given on the command line, by
// name), then ZMP_* environment variables, then the file, then the defaults. Passwords never come from a flag (a
// command line shows in the process list and in logs): server_password from ZMP_SERVER_PASSWORD, from the file
// named by -server-password-file / ZMP_SERVER_PASSWORD_FILE / server_password_file, or from the file's
// server_password. baseDir is where relative paths of the file are taken from.
func Resolve(flags map[string]string, env func(string) string, fc FileConfig, baseDir string) (Settings, error) {
	var st Settings
	pick := func(flag, envName, file, def string) string {
		if v, ok := flags[flag]; ok {
			return v
		}
		if v := strings.TrimSpace(env(envName)); v != "" {
			return v
		}
		if file != "" {
			return file
		}
		return def
	}
	b2s := func(b *bool) string {
		if b == nil {
			return ""
		}
		return strconv.FormatBool(*b)
	}
	i2s := func(i *int) string {
		if i == nil {
			return ""
		}
		return strconv.Itoa(*i)
	}
	var err error
	parseBool := func(name, v string) bool {
		if v == "" || err != nil {
			return false
		}
		b, e := strconv.ParseBool(v)
		if e != nil {
			err = fmt.Errorf("%s: %q is not true or false", name, v)
		}
		return b
	}
	parseInt := func(name, v string) int {
		if v == "" || err != nil {
			return 0
		}
		n, e := strconv.Atoi(v)
		if e != nil || n < 0 {
			err = fmt.Errorf("%s: %q is not a number of 0 or more", name, v)
		}
		return n
	}
	st.Listen = pick("listen", "ZMP_LISTEN", fc.Listen, ":47100")
	data := pick("data", "ZMP_DATA", "", "")
	if data == "" && fc.Data != "" {
		data = fc.Data
		if !filepath.IsAbs(data) && baseDir != "" {
			data = filepath.Join(baseDir, data)
		}
	}
	if data == "" {
		data = "./data"
	}
	st.Data = data
	st.ServerName = pick("name", "ZMP_SERVER_NAME", fc.ServerName, "")
	st.Relay = parseBool("relay", pick("relay", "ZMP_RELAY", b2s(fc.Relay), "false"))
	relayInt := func(def, relayDef int) string {
		if st.Relay {
			return strconv.Itoa(relayDef)
		}
		return strconv.Itoa(def)
	}
	st.RequireRoomPassword = parseBool("require-room-password", pick("require-room-password", "ZMP_REQUIRE_ROOM_PASSWORD", b2s(fc.RequireRoomPassword), "false"))
	st.MaxRooms = parseInt("max-rooms", pick("max-rooms", "ZMP_MAX_ROOMS", i2s(fc.MaxRooms), relayInt(0, RelayMaxRooms)))
	st.MaxConnsPerIP = parseInt("max-conns-per-ip", pick("max-conns-per-ip", "ZMP_MAX_CONNS_PER_IP", i2s(fc.MaxConnsPerIP), relayInt(0, RelayMaxConnsPerIP)))
	st.MaxRoomsPerIP = parseInt("max-rooms-per-ip", pick("max-rooms-per-ip", "ZMP_MAX_ROOMS_PER_IP", i2s(fc.MaxRoomsPerIP), relayInt(0, RelayMaxRoomsPerIP)))
	st.ListRooms = parseBool("list-rooms", pick("list-rooms", "ZMP_LIST_ROOMS", b2s(fc.ListRooms), "false"))
	ttlDef := "0"
	if st.Relay {
		ttlDef = RelayRoomKeyTTL.String()
	}
	if ttl := pick("room-key-ttl", "ZMP_ROOM_KEY_TTL", fc.RoomKeyTTL, ttlDef); err == nil {
		d, e := parseDuration(ttl)
		if e != nil {
			err = fmt.Errorf("room-key-ttl: %v", e)
		}
		st.RoomKeyTTL = d
	}
	if err != nil {
		return st, err
	}
	// The server password.
	if v := env("ZMP_SERVER_PASSWORD"); strings.TrimSpace(v) != "" {
		st.ServerPassword = v
	} else if pf := pick("server-password-file", "ZMP_SERVER_PASSWORD_FILE", fc.ServerPasswordFile, ""); pf != "" {
		if _, fromFlag := flags["server-password-file"]; !fromFlag && env("ZMP_SERVER_PASSWORD_FILE") == "" &&
			!filepath.IsAbs(pf) && baseDir != "" {
			pf = filepath.Join(baseDir, pf)
		}
		b, e := os.ReadFile(pf)
		if e != nil {
			return st, fmt.Errorf("server password file: %w", e)
		}
		st.ServerPassword = strings.TrimSpace(string(b))
		if st.ServerPassword == "" {
			return st, errors.New("server password file is empty")
		}
	} else {
		st.ServerPassword = fc.ServerPassword
	}
	return st, nil
}

// parseDuration takes Go durations plus days ("7d").
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil || n < 0 {
			return 0, fmt.Errorf("%q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("%q (use 30m, 12h or 7d)", s)
	}
	return d, nil
}

// alphabet of generated names and passwords: lowercase letters and digits a person cannot confuse (no 0/o, 1/l/i).
const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"

// RandomWord gives n characters of alphabet.
func RandomWord(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		x, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			panic("hostapp: no random numbers: " + err.Error())
		}
		b.WriteByte(alphabet[x.Int64()])
	}
	return b.String()
}

// NewHostConfig is the file a first start writes: a random room, a random password of 8 characters in the code,
// the port opened by the router, the page opened in the browser.
func NewHostConfig() FileConfig {
	yes := true
	return FileConfig{Listen: ":47100", Data: "data", Room: "zmp-" + RandomWord(4), Password: RandomWord(8),
		InviteSecret: "password", PortMapping: &yes, OpenBrowser: &yes, UIListen: "127.0.0.1:0", LanAnnounce: &yes}
}

// tomlString quotes s as a TOML basic string.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func boolOr(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}

// WriteHostConfig writes the host app's file (atomically), with comments in both languages. The server keys that
// were set are kept.
func WriteHostConfig(path string, fc FileConfig) error {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	w("# zmp-server: servidor anfitrión de ZMP / ZMP host server.")
	w("# Lo reescribe la página local del servidor; puedes editarlo con el servidor parado.")
	w("# The server's local page rewrites it; edit it with the server stopped.")
	w("")
	w("# Dirección y puerto TCP donde escucha (\":47100\" = todas las interfaces; \"127.0.0.1:47100\" = solo este PC).")
	w("# Listening address and TCP port (\":47100\" = every interface; \"127.0.0.1:47100\" = this PC only).")
	w("listen = %s", tomlString(fc.Listen))
	w("# Carpeta de datos (partidas de las salas y sus claves), relativa a este archivo. / Data folder.")
	w("data = %s", tomlString(fc.Data))
	w("server_name = %s", tomlString(fc.ServerName))
	w("")
	w("# Sala y contraseña (vacía = sin contraseña). / Room and password (empty = none).")
	w("room = %s", tomlString(fc.Room))
	w("password = %s", tomlString(fc.Password))
	w("# Qué lleva el código de invitación: \"password\", \"hint\" (solo la pista) o \"none\".")
	w("# What the invite code carries: \"password\", \"hint\" (only the hint) or \"none\".")
	w("invite_secret = %s", tomlString(fc.InviteSecret))
	w("hint = %s", tomlString(fc.Hint))
	w("")
	w("# Pedir al router que abra el puerto (UPnP, PCP, NAT-PMP). / Ask the router to open the port.")
	w("port_mapping = %v", boolOr(fc.PortMapping, true))
	w("# Dominio o IP pública fija que va primero en el código (opcional). / Domain or fixed public IP (optional).")
	w("public_address = %s", tomlString(fc.PublicAddress))
	w("# Abrir la página en el navegador al arrancar, y dónde se sirve (siempre en 127.0.0.1).")
	w("# Open the page in the browser at start, and where it is served (always on 127.0.0.1).")
	w("open_browser = %v", boolOr(fc.OpenBrowser, true))
	w("ui_listen = %s", tomlString(fc.UIListen))
	w("# Anunciar la sala en esta red para la lista \"Partidas en esta red\" del juego (UDP 47100, solo la LAN).")
	w("# Announce the room on this network for the game's \"Games on this network\" list (UDP 47100, LAN only).")
	w("lan_announce = %v", boolOr(fc.LanAnnounce, true))
	extra := []string{}
	if fc.LanPort != 0 {
		extra = append(extra, fmt.Sprintf("lan_port = %d", fc.LanPort))
	}
	if fc.LanAddress != "" {
		extra = append(extra, "lan_address = "+tomlString(fc.LanAddress))
	}
	if fc.ServerPasswordFile != "" {
		extra = append(extra, "server_password_file = "+tomlString(fc.ServerPasswordFile))
	}
	if fc.ServerPassword != "" {
		extra = append(extra, "server_password = "+tomlString(fc.ServerPassword))
	}
	if fc.RequireRoomPassword != nil {
		extra = append(extra, fmt.Sprintf("require_room_password = %v", *fc.RequireRoomPassword))
	}
	if fc.MaxRooms != nil {
		extra = append(extra, fmt.Sprintf("max_rooms = %d", *fc.MaxRooms))
	}
	if fc.MaxConnsPerIP != nil {
		extra = append(extra, fmt.Sprintf("max_conns_per_ip = %d", *fc.MaxConnsPerIP))
	}
	if fc.MaxRoomsPerIP != nil {
		extra = append(extra, fmt.Sprintf("max_rooms_per_ip = %d", *fc.MaxRoomsPerIP))
	}
	if fc.ListRooms != nil {
		extra = append(extra, fmt.Sprintf("list_rooms = %v", *fc.ListRooms))
	}
	if fc.RoomKeyTTL != "" {
		extra = append(extra, "room_key_ttl = "+tomlString(fc.RoomKeyTTL))
	}
	if fc.Relay != nil {
		extra = append(extra, fmt.Sprintf("relay = %v", *fc.Relay))
	}
	if len(extra) > 0 {
		w("")
		w("# Opciones del servidor (protocol 5 notes). / Server options.")
		for _, e := range extra {
			w("%s", e)
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
