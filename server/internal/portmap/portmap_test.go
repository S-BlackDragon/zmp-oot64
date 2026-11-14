package portmap

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeIGD is a router with UPnP-IGD on 127.0.0.1: a root description and a WANIPConnection:1 control URL.
type fakeIGD struct {
	srv          *httptest.Server
	mu           sync.Mutex
	mappings     map[string]string // "TCP:47100" -> internal client
	leases       []string
	deleted      []string
	external     string
	onlyPerm     bool // error 725 for any lease but 0
	conflict     bool // error 718
	addCalls     int
	externalCall int
}

const rootDesc = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
<specVersion><major>1</major><minor>0</minor></specVersion>
<device>
<deviceType>urn:schemas-upnp-org:device:InternetGatewayDevice:1</deviceType>
<friendlyName>Fake router</friendlyName><UDN>uuid:fake-igd</UDN>
<deviceList><device>
<deviceType>urn:schemas-upnp-org:device:WANDevice:1</deviceType><UDN>uuid:fake-wan</UDN>
<deviceList><device>
<deviceType>urn:schemas-upnp-org:device:WANConnectionDevice:1</deviceType><UDN>uuid:fake-wanconn</UDN>
<serviceList><service>
<serviceType>urn:schemas-upnp-org:service:WANIPConnection:1</serviceType>
<serviceId>urn:upnp-org:serviceId:WANIPConn1</serviceId>
<controlURL>/ctl</controlURL><eventSubURL>/evt</eventSubURL><SCPDURL>/scpd.xml</SCPDURL>
</service></serviceList>
</device></deviceList>
</device></deviceList>
</device>
</root>`

var tagRe = regexp.MustCompile(`<(New\w+)>([^<]*)</New\w+>`)

func newFakeIGD(t *testing.T) *fakeIGD {
	f := &fakeIGD{mappings: map[string]string{}, external: "203.0.113.7"}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/root.xml" {
			w.Header().Set("Content-Type", "text/xml")
			io.WriteString(w, rootDesc)
			return
		}
		if r.URL.Path != "/ctl" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		args := map[string]string{}
		for _, m := range tagRe.FindAllStringSubmatch(string(body), -1) {
			args[m[1]] = m[2]
		}
		action := r.Header.Get("SOAPAction")
		action = strings.Trim(action[strings.Index(action, "#")+1:], `"`)
		f.mu.Lock()
		defer f.mu.Unlock()
		fault := func(code int) {
			w.WriteHeader(500)
			fmt.Fprintf(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><s:Fault><faultcode>s:Client</faultcode><faultstring>UPnPError</faultstring><detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0"><errorCode>%d</errorCode><errorDescription>x</errorDescription></UPnPError></detail></s:Fault></s:Body></s:Envelope>`, code)
		}
		ok := func(inner string) {
			w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
			fmt.Fprintf(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:%sResponse xmlns:u="urn:schemas-upnp-org:service:WANIPConnection:1">%s</u:%sResponse></s:Body></s:Envelope>`, action, inner, action)
		}
		switch action {
		case "AddPortMapping":
			f.addCalls++
			if f.conflict {
				fault(718)
				return
			}
			if f.onlyPerm && args["NewLeaseDuration"] != "0" {
				fault(725)
				return
			}
			f.mappings[args["NewProtocol"]+":"+args["NewExternalPort"]] = args["NewInternalClient"] + ":" + args["NewInternalPort"]
			f.leases = append(f.leases, args["NewLeaseDuration"])
			ok("")
		case "DeletePortMapping":
			k := args["NewProtocol"] + ":" + args["NewExternalPort"]
			delete(f.mappings, k)
			f.deleted = append(f.deleted, k)
			ok("")
		case "GetExternalIPAddress":
			f.externalCall++
			ok("<NewExternalIPAddress>" + f.external + "</NewExternalIPAddress>")
		default:
			fault(401)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeIGD) snapshot() (map[string]string, []string, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m := map[string]string{}
	for k, v := range f.mappings {
		m[k] = v
	}
	return m, append([]string(nil), f.leases...), append([]string(nil), f.deleted...)
}

func waitState(t *testing.T, m *Manager, code string) State {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s := m.State(); s.Code == code {
			return s
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("state %+v, want %s", m.State(), code)
	return State{}
}

func TestUPnPOpenRenewRemove(t *testing.T) {
	f := newFakeIGD(t)
	m := Start(Config{Port: 47100, Lease: 2 * time.Second, UPnPRootURL: f.srv.URL + "/root.xml",
		DisablePCP: true, DisableNATPMP: true, Timeout: time.Second})
	st := waitState(t, m, Open)
	if st.Method != MethodUPnP || st.External != "203.0.113.7" || st.ExternalPort != 47100 || st.LeaseS != 2 {
		t.Fatalf("state %+v", st)
	}
	maps, _, _ := f.snapshot()
	if maps["TCP:47100"] != "127.0.0.1:47100" {
		t.Fatalf("router table %v", maps)
	}
	// Renewal at half the lease (and a new external address is picked up).
	f.mu.Lock()
	f.external = "198.51.100.9"
	f.mu.Unlock()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if _, leases, _ := f.snapshot(); len(leases) >= 2 && m.State().External == "198.51.100.9" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, leases, _ := f.snapshot(); len(leases) < 2 || m.State().External != "198.51.100.9" {
		t.Fatalf("no renewal: leases %v state %+v", leases, m.State())
	}
	m.Close()
	maps, _, deleted := f.snapshot()
	if len(maps) != 0 || len(deleted) != 1 || deleted[0] != "TCP:47100" {
		t.Fatalf("rule not removed on exit: table %v deleted %v", maps, deleted)
	}
}

func TestUPnPOnlyPermanentLeases(t *testing.T) {
	f := newFakeIGD(t)
	f.onlyPerm = true
	m := Start(Config{Port: 47123, UPnPRootURL: f.srv.URL + "/root.xml", DisablePCP: true, DisableNATPMP: true})
	st := waitState(t, m, Open)
	if st.LeaseS != 0 {
		t.Fatalf("lease %d, want 0 (permanent)", st.LeaseS)
	}
	m.Close()
	if maps, _, _ := f.snapshot(); len(maps) != 0 {
		t.Fatalf("a permanent rule must be removed on exit too: %v", maps)
	}
}

func TestUPnPCGNATAndConflict(t *testing.T) {
	for _, ext := range []string{"100.72.3.4", "192.168.0.2", "10.1.2.3"} {
		f := newFakeIGD(t)
		f.external = ext
		m := Start(Config{Port: 47100, UPnPRootURL: f.srv.URL + "/root.xml", DisablePCP: true, DisableNATPMP: true})
		st := waitState(t, m, CGNAT)
		if st.External != ext {
			t.Fatalf("%+v", st)
		}
		es, en, aes, aen := Texts(st, 47100)
		if !strings.Contains(es, "CGNAT") || !strings.Contains(en, "CGNAT") || !strings.Contains(aes, "relé") || aen == "" {
			t.Fatalf("texts %q %q %q", es, en, aes)
		}
		m.Close()
	}
	f := newFakeIGD(t)
	f.conflict = true
	m := Start(Config{Port: 47100, UPnPRootURL: f.srv.URL + "/root.xml", DisablePCP: true, DisableNATPMP: true, Retry: time.Hour})
	st := waitState(t, m, Refused)
	if !strings.Contains(st.Error, "718") {
		t.Fatalf("%+v", st)
	}
	m.Close()
}

func TestNothingAnswers(t *testing.T) {
	// A UPnP URL with nobody behind it and a gateway port nobody listens on.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := "http://" + ln.Addr().String() + "/root.xml"
	ln.Close()
	u, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	port := u.LocalAddr().(*net.UDPAddr).Port
	u.Close()
	m := Start(Config{Port: 47100, UPnPRootURL: dead, Gateway: netip.MustParseAddr("127.0.0.1"), GatewayPort: port,
		Timeout: 700 * time.Millisecond, Retry: time.Hour})
	st := waitState(t, m, NoUPnP)
	es, _, _, _ := Texts(st, 47100)
	if !strings.Contains(es, "Router sin UPnP") {
		t.Fatalf("text %q", es)
	}
	m.Confirm()
	if s := m.State(); s.Code != Confirmed || s.Before != NoUPnP {
		t.Fatalf("after a player from the internet: %+v", s)
	}
	m.Close()
}

// fakePMP is a gateway on 127.0.0.1 that speaks NAT-PMP, and PCP if pcp is set.
type fakePMP struct {
	conn     *net.UDPConn
	pcp      bool
	external [4]byte
	mu       sync.Mutex
	maps     map[int]uint32 // internal port -> lifetime
	requests int
}

func newFakePMP(t *testing.T, pcp bool, external string) *fakePMP {
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	f := &fakePMP{conn: c, pcp: pcp, external: netip.MustParseAddr(external).As4(), maps: map[int]uint32{}}
	t.Cleanup(func() { c.Close() })
	go func() {
		buf := make([]byte, 1100)
		for {
			n, from, err := c.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if resp := f.answer(buf[:n]); resp != nil {
				_, _ = c.WriteToUDP(resp, from)
			}
		}
	}()
	return f
}

func (f *fakePMP) port() int { return f.conn.LocalAddr().(*net.UDPAddr).Port }

func (f *fakePMP) answer(b []byte) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	switch {
	case len(b) >= 2 && b[0] == 2 && f.pcp && b[1] == 1 && len(b) >= 60: // PCP MAP
		r := make([]byte, 60)
		r[0], r[1] = 2, 0x81
		life := binary.BigEndian.Uint32(b[4:])
		binary.BigEndian.PutUint32(r[4:], life)
		copy(r[24:36], b[24:36])
		r[36] = b[36]
		port := int(binary.BigEndian.Uint16(b[40:]))
		copy(r[40:42], b[40:42])
		ext := binary.BigEndian.Uint16(b[42:])
		if life == 0 {
			delete(f.maps, port)
			ext = 0
		} else {
			f.maps[port] = life
			if ext == 0 {
				ext = uint16(port)
			}
		}
		binary.BigEndian.PutUint16(r[42:], ext)
		r[54], r[55] = 0xff, 0xff
		copy(r[56:60], f.external[:])
		return r
	case len(b) >= 2 && b[0] == 2: // PCP to a NAT-PMP-only gateway: NAT-PMP's "unsupported version"
		return []byte{0, b[1] | 0x80, 0, 1, 0, 0, 0, 0}
	case len(b) == 2 && b[0] == 0 && b[1] == 0: // NAT-PMP external address
		r := make([]byte, 12)
		r[1] = 128
		copy(r[8:], f.external[:])
		return r
	case len(b) == 12 && b[0] == 0 && b[1] == 2: // NAT-PMP map TCP
		r := make([]byte, 16)
		r[1] = 130
		port := int(binary.BigEndian.Uint16(b[4:]))
		life := binary.BigEndian.Uint32(b[8:])
		copy(r[8:10], b[4:6])
		if life == 0 {
			delete(f.maps, port)
		} else {
			f.maps[port] = life
			binary.BigEndian.PutUint16(r[10:], uint16(port))
		}
		binary.BigEndian.PutUint32(r[12:], life)
		return r
	}
	return nil
}

func (f *fakePMP) table() map[int]uint32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	m := map[int]uint32{}
	for k, v := range f.maps {
		m[k] = v
	}
	return m
}

func TestNATPMPAndPCP(t *testing.T) {
	for _, tc := range []struct {
		pcp    bool
		method string
	}{{false, MethodNATPMP}, {true, MethodPCP}} {
		f := newFakePMP(t, tc.pcp, "203.0.113.50")
		m := Start(Config{Port: 47100, Lease: 2 * time.Second, DisableUPnP: true,
			Gateway: netip.MustParseAddr("127.0.0.1"), GatewayPort: f.port(), Timeout: time.Second})
		st := waitState(t, m, Open)
		if st.Method != tc.method || st.External != "203.0.113.50" || st.ExternalPort != 47100 || st.LeaseS != 2 {
			t.Fatalf("%s: %+v", tc.method, st)
		}
		if f.table()[47100] != 2 {
			t.Fatalf("%s: gateway table %v", tc.method, f.table())
		}
		before := f.requests
		time.Sleep(1500 * time.Millisecond) // (renewal at half the lease)
		f.mu.Lock()
		renewed := f.requests > before
		f.mu.Unlock()
		if !renewed {
			t.Fatalf("%s: no renewal", tc.method)
		}
		m.Close()
		if len(f.table()) != 0 {
			t.Fatalf("%s: rule not removed: %v", tc.method, f.table())
		}
	}
	// CGNAT seen by NAT-PMP.
	f := newFakePMP(t, false, "100.64.0.9")
	m := Start(Config{Port: 47100, DisableUPnP: true, Gateway: netip.MustParseAddr("127.0.0.1"), GatewayPort: f.port()})
	if st := waitState(t, m, CGNAT); st.External != "100.64.0.9" {
		t.Fatalf("%+v", st)
	}
	m.Close()
}

func TestClassify(t *testing.T) {
	for addr, want := range map[string]string{"203.0.113.7": Open, "8.8.8.8": Open, "2001:4860::1": Open,
		"100.64.0.1": CGNAT, "100.127.255.254": CGNAT, "100.128.0.1": Open, "10.0.0.1": CGNAT, "172.16.5.4": CGNAT,
		"192.168.1.1": CGNAT, "169.254.1.1": CGNAT, "fd00::1": CGNAT} {
		if got := Classify(netip.MustParseAddr(addr)); got != want {
			t.Errorf("Classify(%s) = %s, want %s", addr, got, want)
		}
	}
}

func TestParseProcRoute(t *testing.T) {
	text := "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n" +
		"eth0\t0001A8C0\t00000000\t0001\t0\t0\t100\t00FFFFFF\t0\t0\t0\n"
	got := parseProcRoute(bufio.NewScanner(strings.NewReader(text)))
	if len(got) != 1 || got[0].String() != "192.168.1.1" {
		t.Fatalf("%v", got)
	}
}
