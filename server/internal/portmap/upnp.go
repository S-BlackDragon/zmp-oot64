package portmap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"sync"
	"time"

	"github.com/huin/goupnp"
	"github.com/huin/goupnp/dcps/internetgateway2"
	"github.com/huin/goupnp/soap"
)

// igdClient is what the three WAN connection services of an Internet Gateway Device have in common.
type igdClient interface {
	AddPortMappingCtx(ctx context.Context, remoteHost string, externalPort uint16, protocol string,
		internalPort uint16, internalClient string, enabled bool, description string, lease uint32) error
	DeletePortMappingCtx(ctx context.Context, remoteHost string, externalPort uint16, protocol string) error
	GetExternalIPAddressCtx(ctx context.Context) (string, error)
	GetServiceClient() *goupnp.ServiceClient
}

// upnpMapper: UPnP-IGD (v1 and v2; WANIPConnection 2 and 1, WANPPPConnection 1), with the maintained goupnp.
type upnpMapper struct {
	rootURL string // test hook: no SSDP, this root description
}

func (u *upnpMapper) name() string { return MethodUPnP }

// clients finds the gateways' WAN connection services: by SSDP (three searches at once), or from rootURL.
func (u *upnpMapper) clients(ctx context.Context) ([]igdClient, error) {
	var out []igdClient
	if u.rootURL != "" {
		loc, err := url.Parse(u.rootURL)
		if err != nil {
			return nil, err
		}
		root, err := goupnp.DeviceByURLCtx(ctx, loc)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errNoAnswer, err)
		}
		if cs, err := internetgateway2.NewWANIPConnection2ClientsFromRootDevice(root, loc); err == nil {
			for _, c := range cs {
				out = append(out, c)
			}
		}
		if cs, err := internetgateway2.NewWANIPConnection1ClientsFromRootDevice(root, loc); err == nil {
			for _, c := range cs {
				out = append(out, c)
			}
		}
		if cs, err := internetgateway2.NewWANPPPConnection1ClientsFromRootDevice(root, loc); err == nil {
			for _, c := range cs {
				out = append(out, c)
			}
		}
		return out, nil
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	add := func(cs []igdClient) {
		mu.Lock()
		out = append(out, cs...)
		mu.Unlock()
	}
	wg.Add(3)
	go func() {
		defer wg.Done()
		cs, _, _ := internetgateway2.NewWANIPConnection2ClientsCtx(ctx)
		var l []igdClient
		for _, c := range cs {
			l = append(l, c)
		}
		add(l)
	}()
	go func() {
		defer wg.Done()
		cs, _, _ := internetgateway2.NewWANIPConnection1ClientsCtx(ctx)
		var l []igdClient
		for _, c := range cs {
			l = append(l, c)
		}
		add(l)
	}()
	go func() {
		defer wg.Done()
		cs, _, _ := internetgateway2.NewWANPPPConnection1ClientsCtx(ctx)
		var l []igdClient
		for _, c := range cs {
			l = append(l, c)
		}
		add(l)
	}()
	wg.Wait()
	return out, nil
}

// upnpErrorCode gives the UPnP error code of a SOAP fault (0 if it is not one).
func upnpErrorCode(err error) int {
	var f *soap.SOAPFaultError
	if errors.As(err, &f) {
		return f.Detail.UPnPError.Errorcode
	}
	return 0
}

func (u *upnpMapper) mapPort(ctx context.Context, port int, lease time.Duration, desc string) (Mapping, error) {
	clients, err := u.clients(ctx)
	if err != nil {
		return Mapping{}, err
	}
	if len(clients) == 0 {
		return Mapping{}, errNoAnswer
	}
	var lastErr error
	for _, c := range clients {
		sc := c.GetServiceClient()
		local := netip.Addr{}
		if ip := sc.LocalAddr(); ip != nil {
			local, _ = netip.AddrFromSlice(ip)
			local = local.Unmap()
		}
		gw := netip.Addr{}
		if sc.Location != nil {
			if h, err := netip.ParseAddr(sc.Location.Hostname()); err == nil {
				gw = h.Unmap()
			}
		}
		if !local.IsValid() && gw.IsValid() {
			local = localAddrTo(gw)
		}
		if !local.IsValid() || !local.Is4() {
			lastErr = errors.New("no local IPv4 address towards the router")
			continue
		}
		secs := uint32(lease / time.Second)
		add := func(ctx context.Context, secs uint32) error {
			return c.AddPortMappingCtx(ctx, "", uint16(port), "TCP", uint16(port), local.String(), true, desc, secs)
		}
		err := add(ctx, secs)
		if code := upnpErrorCode(err); code == 725 { // OnlyPermanentLeasesSupported
			secs = 0
			err = add(ctx, 0)
		}
		if err != nil {
			lastErr = upnpError(err)
			continue
		}
		ext := netip.Addr{}
		if s, err := c.GetExternalIPAddressCtx(ctx); err == nil {
			if a, err := netip.ParseAddr(s); err == nil {
				ext = a.Unmap()
			}
		}
		mp := Mapping{Method: MethodUPnP, External: ext, ExternalPort: port, InternalIP: local, InternalPort: port,
			Lease: time.Duration(secs) * time.Second, Gateway: gw}
		client := c
		mp.remove = func(ctx context.Context) error {
			return client.DeletePortMappingCtx(ctx, "", uint16(port), "TCP")
		}
		mp.renew = func(ctx context.Context) (Mapping, error) {
			if err := add(ctx, secs); err != nil {
				return Mapping{}, upnpError(err)
			}
			if s, err := client.GetExternalIPAddressCtx(ctx); err == nil {
				if a, err := netip.ParseAddr(s); err == nil {
					next := mp
					next.External = a.Unmap()
					return next, nil
				}
			}
			return mp, nil
		}
		return mp, nil
	}
	return Mapping{}, lastErr
}

// upnpError turns a router's answer into a short text (the codes of the IGD specification).
func upnpError(err error) error {
	switch upnpErrorCode(err) {
	case 0:
		var ne net.Error
		if errors.As(err, &ne) {
			return fmt.Errorf("%w: %v", errNoAnswer, err)
		}
		return err
	case 718:
		return errors.New("UPnP 718: the port is already forwarded to another device")
	case 606:
		return errors.New("UPnP 606: the router does not let programs open ports")
	case 724:
		return errors.New("UPnP 724: the router wants the same external and internal port")
	default:
		return fmt.Errorf("UPnP %d: %v", upnpErrorCode(err), err)
	}
}
