// Package portmap asks the home router to forward the server's TCP port (UPnP-IGD, PCP or NAT-PMP), learns the
// router's external address from the router itself (no outside service), renews the lease and removes the rule on
// exit, and says honestly what that means for a friend on the internet (the protocol 5 design notes section 9).
package portmap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"sort"
	"sync"
	"time"
)

// Reachability codes (State.Code), stable for clients and the host page.
const (
	Disabled  = "disabled"   // the host turned port opening off
	LocalOnly = "local_only" // the server listens on this PC only
	Checking  = "checking"   // asking the router
	Open      = "open"       // the router forwards the port and its external address is public
	CGNAT     = "cgnat"      // forwarded, but the router's external address is CGNAT or private: another NAT in front
	NoUPnP    = "no_upnp"    // nothing answered (no UPnP, PCP or NAT-PMP, or it is off)
	Refused   = "refused"    // the router answered and did not forward the port
	Confirmed = "confirmed"  // a player arrived from a public address: the server is reachable from the internet
)

// Methods (State.Method).
const (
	MethodUPnP   = "upnp"
	MethodPCP    = "pcp"
	MethodNATPMP = "natpmp"
)

// Mapping is a forwarding rule the router granted.
type Mapping struct {
	Method       string
	External     netip.Addr // the router's external address (invalid: it did not say)
	ExternalPort int
	InternalIP   netip.Addr
	InternalPort int
	Lease        time.Duration // 0: permanent (UPnP routers that only take those)
	Gateway      netip.Addr

	remove func(ctx context.Context) error
	renew  func(ctx context.Context) (Mapping, error)
}

// mapper is one method.
type mapper interface {
	name() string
	mapPort(ctx context.Context, port int, lease time.Duration, desc string) (Mapping, error)
}

// errNoAnswer: the method found no router to talk to (versus a router that said no).
var errNoAnswer = errors.New("no answer")

// Config of a Manager. Zero values take the defaults documented per field.
type Config struct {
	Port        int    // the TCP port to forward (internal and, asked, external)
	Description string // shown in the router's table. Default "zmp-server"
	Lease       time.Duration
	Logger      *log.Logger
	// Timeout of each method. Default 3 s.
	Timeout time.Duration
	// Retry: when nothing worked, try again this often (a router rebooting, UPnP turned on). Default 5 min.
	Retry time.Duration

	// Test hooks: UPnPRootURL skips SSDP and talks to that root description; Gateway and GatewayPort replace the
	// system's default gateway and port 5351; the Disable flags turn methods off.
	UPnPRootURL                            string
	Gateway                                netip.Addr
	GatewayPort                            int
	DisableUPnP, DisablePCP, DisableNATPMP bool
}

func (c *Config) setDefaults() {
	if c.Description == "" {
		c.Description = "zmp-server"
	}
	if c.Lease <= 0 {
		c.Lease = time.Hour
	}
	if c.Timeout <= 0 {
		c.Timeout = 3 * time.Second
	}
	if c.Retry <= 0 {
		c.Retry = 5 * time.Minute
	}
	if c.GatewayPort == 0 {
		c.GatewayPort = 5351
	}
	if c.Logger == nil {
		c.Logger = log.New(nopWriter{}, "", 0)
	}
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

// State is what the host page shows.
type State struct {
	Code         string   `json:"state"`
	Method       string   `json:"method"`
	External     string   `json:"external"`
	ExternalPort int      `json:"external_port"`
	Gateway      string   `json:"gateway"`
	LAN          []string `json:"lan"`
	LeaseS       int      `json:"lease_s"`
	Error        string   `json:"error"`
	// Before is the state before Confirmed (what the router said), "" otherwise.
	Before string `json:"before,omitempty"`
}

// Manager keeps one mapping alive in the background.
type Manager struct {
	cfg     Config
	mu      sync.Mutex
	state   State
	mapping *Mapping
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
	changed chan struct{}
}

// Start begins asking the router in the background. Close removes the rule.
func Start(cfg Config) *Manager {
	cfg.setDefaults()
	m := &Manager{cfg: cfg, state: State{Code: Checking, LAN: LANAddresses(cfg.Gateway)}, stop: make(chan struct{}),
		done: make(chan struct{}), changed: make(chan struct{}, 1)}
	go m.run()
	return m
}

// State gives the current state.
func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.state
	s.LAN = append([]string(nil), s.LAN...)
	return s
}

// Changed is signalled (without blocking) at every change of the state.
func (m *Manager) Changed() <-chan struct{} { return m.changed }

func (m *Manager) setState(s State) {
	m.mu.Lock()
	if m.state.Code == Confirmed && s.Code != Disabled {
		s.Before, s.Code = s.Code, Confirmed
	}
	m.state = s
	m.mu.Unlock()
	select {
	case m.changed <- struct{}{}:
	default:
	}
}

// Confirm records that a player came in from a public address: whatever the router said, the server is reachable.
func (m *Manager) Confirm() {
	m.mu.Lock()
	if m.state.Code == Confirmed || m.state.Code == Disabled {
		m.mu.Unlock()
		return
	}
	m.state.Before, m.state.Code = m.state.Code, Confirmed
	m.mu.Unlock()
	m.cfg.Logger.Printf("port mapping: a player came in from the internet: reachable (router said %s)", m.State().Before)
	select {
	case m.changed <- struct{}{}:
	default:
	}
}

// Close stops renewing and removes the rule from the router (3 s at most).
func (m *Manager) Close() {
	m.once.Do(func() { close(m.stop) })
	<-m.done
}

func (m *Manager) mappers() []mapper {
	var out []mapper
	if !m.cfg.DisableUPnP {
		out = append(out, &upnpMapper{rootURL: m.cfg.UPnPRootURL})
	}
	gws := []netip.Addr{m.cfg.Gateway}
	if !m.cfg.Gateway.IsValid() {
		gws = DefaultGateways()
	}
	for _, gw := range gws {
		if !m.cfg.DisablePCP {
			out = append(out, &pmpMapper{gw: gw, port: m.cfg.GatewayPort, pcp: true})
		}
		if !m.cfg.DisableNATPMP {
			out = append(out, &pmpMapper{gw: gw, port: m.cfg.GatewayPort})
		}
	}
	return out
}

// tryAll runs the methods in order (UPnP, PCP, NAT-PMP) and keeps the first rule granted.
func (m *Manager) tryAll() {
	var errs []string
	refused := false
	for _, mp := range m.mappers() {
		ctx, cancel := context.WithTimeout(context.Background(), m.cfg.Timeout)
		mapping, err := mp.mapPort(ctx, m.cfg.Port, m.cfg.Lease, m.cfg.Description)
		cancel()
		if err == nil {
			m.mu.Lock()
			m.mapping = &mapping
			m.mu.Unlock()
			m.setState(m.stateOf(mapping))
			m.cfg.Logger.Printf("port mapping: %s forwards TCP %d -> %s:%d, external address %s, lease %v: %s",
				mapping.Method, mapping.ExternalPort, mapping.InternalIP, mapping.InternalPort, addrText(mapping.External),
				mapping.Lease, m.State().Code)
			return
		}
		if !errors.Is(err, errNoAnswer) {
			refused = true
		}
		errs = append(errs, mp.name()+": "+err.Error())
	}
	st := State{Code: NoUPnP, LAN: LANAddresses(m.cfg.Gateway)}
	if refused {
		st.Code = Refused
	}
	if gws := DefaultGateways(); len(gws) > 0 {
		st.Gateway = gws[0].String()
	}
	if len(errs) > 0 {
		st.Error = joinErrs(errs)
	}
	m.setState(st)
	m.cfg.Logger.Printf("port mapping: %s (%s); next try in %v", st.Code, st.Error, m.cfg.Retry)
}

func joinErrs(errs []string) string {
	out := ""
	for i, e := range errs {
		if i > 0 {
			out += "; "
		}
		out += e
	}
	return out
}

func addrText(a netip.Addr) string {
	if !a.IsValid() {
		return "unknown"
	}
	return a.String()
}

func (m *Manager) stateOf(mp Mapping) State {
	st := State{Code: Classify(mp.External), Method: mp.Method, ExternalPort: mp.ExternalPort,
		LAN: LANAddresses(mp.Gateway), LeaseS: int(mp.Lease / time.Second)}
	if mp.External.IsValid() {
		st.External = mp.External.String()
	}
	if mp.Gateway.IsValid() {
		st.Gateway = mp.Gateway.String()
	}
	if mp.InternalIP.IsValid() && mp.InternalIP.Is4() {
		st.LAN = uniq(append([]string{mp.InternalIP.String()}, st.LAN...))
	}
	return st
}

// Classify gives the reachability of a forwarded port from the router's external address: Open if it is public;
// CGNAT if it is shared address space (100.64.0.0/10) or private (another router or the operator's NAT in front);
// with no address known, Open (the rule exists; only a friend can tell).
func Classify(external netip.Addr) string {
	if !external.IsValid() || external.IsUnspecified() {
		return Open
	}
	if isPublic(external) {
		return Open
	}
	return CGNAT
}

func isPublic(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	if ip.Is4() {
		b := ip.As4()
		if b[0] == 100 && b[1]&0xC0 == 64 || b[0] == 0 || b[0] >= 240 {
			return false
		}
	}
	return true
}

func (m *Manager) run() {
	defer close(m.done)
	m.tryAll()
	for {
		m.mu.Lock()
		mp := m.mapping
		m.mu.Unlock()
		wait := m.cfg.Retry
		if mp != nil {
			if mp.Lease <= 0 {
				wait = 24 * time.Hour // (permanent: nothing to renew; the rule goes at Close)
			} else {
				wait = mp.Lease / 2
			}
		}
		select {
		case <-m.stop:
			m.remove()
			return
		case <-time.After(wait):
		}
		if mp == nil {
			m.tryAll()
			continue
		}
		if mp.Lease <= 0 {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), m.cfg.Timeout)
		renewed, err := mp.renew(ctx)
		cancel()
		if err != nil {
			m.cfg.Logger.Printf("port mapping: renewing the %s rule failed (%v): asking again", mp.Method, err)
			m.mu.Lock()
			m.mapping = nil
			m.mu.Unlock()
			m.tryAll()
			continue
		}
		m.mu.Lock()
		m.mapping = &renewed
		m.mu.Unlock()
		m.setState(m.stateOf(renewed))
	}
}

func (m *Manager) remove() {
	m.mu.Lock()
	mp := m.mapping
	m.mapping = nil
	m.mu.Unlock()
	if mp == nil || mp.remove == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := mp.remove(ctx); err != nil {
		m.cfg.Logger.Printf("port mapping: removing the %s rule failed: %v (it expires by itself in %v)", mp.Method, err, mp.Lease)
		return
	}
	m.cfg.Logger.Printf("port mapping: %s rule for TCP %d removed", mp.Method, mp.ExternalPort)
}

// LANAddresses lists this machine's private IPv4 addresses on interfaces that are up: first the one that reaches
// gw (when given and found), then the others, at most three.
func LANAddresses(gw netip.Addr) []string {
	var out []string
	if gw.IsValid() {
		if ip := localAddrTo(gw); ip.IsValid() && ip.IsPrivate() {
			out = append(out, ip.String())
		}
	}
	ifaces, _ := net.Interfaces()
	var rest []string
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			pfx, err := netip.ParsePrefix(a.String())
			if err != nil {
				continue
			}
			ip := pfx.Addr()
			if ip.Is4() && ip.IsPrivate() {
				rest = append(rest, ip.String())
			}
		}
	}
	sort.Strings(rest)
	out = uniq(append(out, rest...))
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

// localAddrTo: the local address the system would use to reach ip (no packet is sent).
func localAddrTo(ip netip.Addr) netip.Addr {
	c, err := net.DialUDP("udp", nil, net.UDPAddrFromAddrPort(netip.AddrPortFrom(ip, 9)))
	if err != nil {
		return netip.Addr{}
	}
	defer c.Close()
	if a, ok := c.LocalAddr().(*net.UDPAddr); ok {
		if ap := a.AddrPort(); ap.IsValid() {
			return ap.Addr().Unmap()
		}
	}
	return netip.Addr{}
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// Texts of a state for the host page (es, en): what it means and what to do.
func Texts(st State, port int) (msgES, msgEN, adviceES, adviceEN string) {
	method := map[string]string{MethodUPnP: "UPnP", MethodPCP: "PCP", MethodNATPMP: "NAT-PMP"}[st.Method]
	help := func() (string, string) {
		return fmt.Sprintf("Opciones: usa un relé (un zmp-server en internet), una VPN como Tailscale o ZeroTier, o abre a mano el puerto %d TCP en tu router hacia este PC (%s).", port, first(st.LAN)),
			fmt.Sprintf("Options: use a relay (a zmp-server on the internet), a VPN such as Tailscale or ZeroTier, or forward TCP port %d by hand on your router to this PC (%s).", port, first(st.LAN))
	}
	switch st.Code {
	case Disabled:
		return "No se pide al router que abra el puerto.", "The router is not asked to open the port.",
			"Desde tu red local entran con el código. Desde internet, abre el puerto a mano o usa un relé o una VPN.",
			"From your local network friends join with the code. From the internet, forward the port by hand or use a relay or a VPN."
	case LocalOnly:
		return "Solo este PC puede entrar (escucha en 127.0.0.1).", "Only this PC can join (listening on 127.0.0.1).",
			"Para jugar con otros, escucha en todas las interfaces (listen = \":" + fmt.Sprint(port) + "\").",
			"To play with others, listen on every interface (listen = \":" + fmt.Sprint(port) + "\")."
	case Checking:
		return "Preguntando al router…", "Asking the router…", "", ""
	case Open:
		return fmt.Sprintf("Abierto por %s: tu dirección pública es %s, puerto %d.", method, orUnknown(st.External, "desconocida"), st.ExternalPort),
			fmt.Sprintf("Opened by %s: your public address is %s, port %d.", method, orUnknown(st.External, "unknown"), st.ExternalPort),
			"Pásale el código a tu amigo. Si no consigue entrar, puede ser el cortafuegos de Windows: permite zmp-server en redes privadas y públicas.",
			"Give your friend the code. If they cannot get in, it may be Windows Firewall: allow zmp-server on private and public networks."
	case CGNAT:
		es, en := help()
		return fmt.Sprintf("Posible CGNAT: el router abrió el puerto por %s, pero su dirección exterior (%s) no es pública. Tu operadora u otro router comparten la salida a internet y desde fuera probablemente no se llega.", method, st.External),
			fmt.Sprintf("Possible CGNAT: the router opened the port by %s, but its external address (%s) is not public. Your provider or another router shares the way out and the internet probably cannot reach you.", method, st.External),
			"En tu red local sí entran con el código. " + es, "On your local network friends can join with the code. " + en
	case NoUPnP:
		es, en := help()
		return "Router sin UPnP: no respondió a UPnP, PCP ni NAT-PMP (puede estar apagado en su configuración).",
			"Router without UPnP: it did not answer UPnP, PCP or NAT-PMP (it may be turned off in its settings).",
			"En tu red local sí entran con el código. Puedes encender UPnP en el router, o: " + es,
			"On your local network friends can join with the code. You can turn UPnP on in the router, or: " + en
	case Refused:
		es, en := help()
		return "El router respondió pero no abrió el puerto (" + st.Error + ").", "The router answered but did not open the port (" + st.Error + ").",
			"Puede que otro equipo use ya ese puerto. " + es, "Another device may already use that port. " + en
	case Confirmed:
		return "Se llega desde internet: ya ha entrado un jugador desde fuera de tu red.", "Reachable from the internet: a player already joined from outside your network.",
			"", ""
	}
	return st.Code, st.Code, "", ""
}

func first(l []string) string {
	if len(l) == 0 {
		return "?"
	}
	return l[0]
}

func orUnknown(s, unknown string) string {
	if s == "" {
		return unknown
	}
	return s
}
