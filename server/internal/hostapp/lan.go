package hostapp

import (
	"encoding/json"
	"net"
	"os"
	"strconv"
	"time"
)

// The LAN announce of "Partidas en esta red" (the LAN list format): every 2 s one JSON datagram
//
//	{"zmp":1, "server_name":"PC-Saria", "port":47100, "proto":5,
//	 "rooms":[{"name":"bosque", "players":2, "max":6, "engine":"native", "build_hash":"...", "password":true}]}
//
// to the multicast group 239.255.47.100 (TTL 1: never beyond this network) and to 255.255.255.255, UDP port 47100.
// The game's list takes the sender's IP and the announced port. Only sending: no port is opened, so no firewall
// question. engine and build_hash are the room's once somebody created it, empty before.

// LanGroup and LanPort are the announce's multicast group and UDP port.
const (
	LanGroup = "239.255.47.100"
	LanPort  = 47100
	// LanEvery is the time between two announces; the list forgets what it has not heard for 6 s.
	LanEvery = 2 * time.Second
	// LanMaxPlayers is the room size the list shows.
	LanMaxPlayers = 6
)

// LanRoom is one room of the announce.
type LanRoom struct {
	Name      string `json:"name"`
	Players   int    `json:"players"`
	Max       int    `json:"max"`
	Engine    string `json:"engine"`
	BuildHash string `json:"build_hash"`
	Password  bool   `json:"password"`
}

// LanAnnounce is the datagram.
type LanAnnounce struct {
	ZMP        int       `json:"zmp"`
	ServerName string    `json:"server_name"`
	Port       int       `json:"port"`
	Proto      int       `json:"proto"`
	Rooms      []LanRoom `json:"rooms"`
}

// lanTargets says where the announce goes, or why it does not go anywhere ("" targets and the reason).
func (a *App) lanTargets() ([]*net.UDPAddr, string) {
	a.mu.Lock()
	fc := a.fc
	a.mu.Unlock()
	if !boolOr(fc.LanAnnounce, true) {
		return nil, "off (lan_announce = false)"
	}
	port := fc.LanPort
	if port <= 0 || port > 65535 {
		port = LanPort
	}
	if fc.LanAddress != "" {
		ip := net.ParseIP(fc.LanAddress)
		if ip == nil || ip.To4() == nil {
			return nil, "off (lan_address " + strconv.Quote(fc.LanAddress) + " is not an IPv4 address)"
		}
		return []*net.UDPAddr{{IP: ip, Port: port}}, ""
	}
	if host, _, _ := net.SplitHostPort(a.ln.Addr().String()); isLoopbackHost(host) {
		return nil, "off (listening on this PC only)"
	}
	return []*net.UDPAddr{{IP: net.ParseIP(LanGroup), Port: port}, {IP: net.IPv4bcast, Port: port}}, ""
}

// lanPayload is the datagram now.
func (a *App) lanPayload() []byte {
	a.mu.Lock()
	fc := a.fc
	a.mu.Unlock()
	name := fc.ServerName
	if name == "" {
		name, _ = os.Hostname()
	}
	if len(name) > 64 {
		name = name[:64]
	}
	room := LanRoom{Name: fc.Room, Max: LanMaxPlayers, Password: fc.Password != ""}
	for _, p := range a.srv.Players() {
		if p.Room == fc.Room {
			room.Players++
		}
	}
	for _, r := range a.srv.Rooms() {
		if r.Name == fc.Room {
			room.BuildHash, room.Engine = r.BuildHash, r.Engine
			if room.Engine == "" && r.BuildHash != "" {
				room.Engine = "native" // (protocol 2-4 clients say nothing: they are all the desktop game)
			}
		}
	}
	b, _ := json.Marshal(LanAnnounce{ZMP: 1, ServerName: name, Port: a.port, Proto: 5, Rooms: []LanRoom{room}})
	return b
}

// announceLAN sends the announce until the app stops.
func (a *App) announceLAN() {
	to, why := a.lanTargets()
	if len(to) == 0 {
		a.log.Printf("lan announce: %s", why)
		return
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		a.log.Printf("lan announce: no UDP socket (%v): the game's LAN list will not show this room", err)
		return
	}
	defer conn.Close()
	names := ""
	for i, t := range to {
		if i > 0 {
			names += " and "
		}
		names += t.String()
	}
	a.log.Printf("lan announce: room %q every %v to %s", a.State().Room, LanEvery, names)
	t := time.NewTicker(LanEvery)
	defer t.Stop()
	failed := map[string]bool{}
	for {
		b := a.lanPayload()
		for _, dst := range to {
			if _, err := conn.WriteToUDP(b, dst); err != nil {
				if !failed[dst.String()] { // (once: a PC without a network says it every 2 s otherwise)
					failed[dst.String()] = true
					a.log.Printf("lan announce: sending to %s: %v", dst, err)
				}
			} else if failed[dst.String()] {
				delete(failed, dst.String())
				a.log.Printf("lan announce: sending to %s works again", dst)
			}
		}
		select {
		case <-a.stopped:
			return
		case <-t.C:
		}
	}
}
