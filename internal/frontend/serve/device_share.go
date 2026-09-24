package serve

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

// ErrShareAddress refuses an address the window does not own on a private
// network. Binding a wildcard or a public address is not a sharing decision
// this surface makes.
var ErrShareAddress = errors.New("that address is not a private address on this machine")

// ErrShareClosed is an offer asked of a share that is not listening.
var ErrShareClosed = errors.New("sharing is off")

// ErrShareUnattached is a share opened before the host named what it serves.
var ErrShareUnattached = errors.New("sharing has nothing to serve yet")

// tailnetPrefix is the CGNAT range Tailscale hands out; its traffic is already
// encrypted end to end, which makes it the one routed network worth offering.
var tailnetPrefix = netip.MustParsePrefix("100.64.0.0/10")

// DeviceShare is a window's second door: a listener on a private address that
// paired devices reach the window's hub through, behind a device gate. It is
// shut until the person at the window opens it, and shutting it unpairs every
// device.
type DeviceShare struct {
	registry *DeviceRegistry
	page     fs.FS
	// addresses lists what may be bound; a variable seam so tests need no NIC.
	addresses func() []ShareAddress
	// turn serialises Open and Close, so two opens cannot leave one listener
	// running that nothing will ever stop.
	turn sync.Mutex

	mu      sync.Mutex
	handler http.Handler
	live    *shareListener
}

type shareListener struct {
	origin string
	stop   context.CancelFunc
	done   chan struct{}
}

// ShareAddress is one address a share can listen on.
type ShareAddress struct {
	Interface string      `json:"interface"`
	IP        string      `json:"ip"`
	Kind      AddressKind `json:"kind"`
}

// AddressKind says what reaches an address, read from the interface itself.
type AddressKind string

const (
	// AddressLAN is a broadcast-capable adapter with a hardware address: Wi-Fi
	// or Ethernet, which a phone on the same network can reach.
	AddressLAN AddressKind = "lan"
	// AddressTailnet is a Tailscale address, reachable from anywhere on the
	// tailnet and encrypted on the way.
	AddressTailnet AddressKind = "tailnet"
	// AddressVirtual is anything else: a VPN or proxy tunnel, a VM switch.
	// Listed, because it may be what someone wants, but never the default.
	AddressVirtual AddressKind = "virtual"
)

var addressRank = map[AddressKind]int{AddressLAN: 0, AddressTailnet: 1, AddressVirtual: 2}

// ShareStatus is the whole state a window draws its sharing panel from.
type ShareStatus struct {
	Open         bool           `json:"open"`
	Origin       string         `json:"origin,omitempty"`
	Addresses    []ShareAddress `json:"addresses"`
	Devices      []DeviceView   `json:"devices"`
	OfferExpires *time.Time     `json:"offerExpires,omitempty"`
}

// ShareOffer is a pairing code as a device receives it: a link that carries the
// code in its fragment, and that link drawn as a QR code.
type ShareOffer struct {
	URL     string    `json:"url"`
	QR      string    `json:"qr"`
	Expires time.Time `json:"expires"`
}

// NewDeviceShare returns a closed share serving page to devices.
func NewDeviceShare(page fs.FS) *DeviceShare {
	return &DeviceShare{registry: NewDeviceRegistry(), page: page, addresses: PrivateAddresses}
}

// Attach names the handler devices reach. The hub is built after the share it
// registers routes for, so this closes that loop before anything opens.
func (s *DeviceShare) Attach(h http.Handler) {
	s.mu.Lock()
	s.handler = h
	s.mu.Unlock()
}

// Open listens on ip, one of the addresses Status lists, on a port the system
// picks. A share already open is closed first, so its devices go with it.
func (s *DeviceShare) Open(ip string) (ShareStatus, error) {
	if !slices.ContainsFunc(s.addresses(), func(a ShareAddress) bool { return a.IP == ip }) {
		return s.Status(), ErrShareAddress
	}
	s.turn.Lock()
	defer s.turn.Unlock()
	s.closeLocked()
	s.mu.Lock()
	handler := s.handler
	s.mu.Unlock()
	if handler == nil {
		return s.Status(), ErrShareUnattached
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if err != nil {
		return s.Status(), err
	}
	origin := "http://" + ln.Addr().String()
	ctx, stop := context.WithCancel(context.Background())
	live := &shareListener{origin: origin, stop: stop, done: make(chan struct{})}
	gate := NewDeviceGate(handler, DeviceGateOptions{Registry: s.registry, Origin: origin, Page: s.page, Machine: machineName()})
	s.mu.Lock()
	s.live = live
	s.mu.Unlock()
	go func() {
		defer close(live.done)
		if err := runGracefulListener(ctx, ln, gate); err != nil {
			slog.Warn("serve: device share stopped", "origin", origin, "err", err)
		}
	}()
	return s.Status(), nil
}

// Close stops listening and unpairs every device.
func (s *DeviceShare) Close() {
	s.turn.Lock()
	defer s.turn.Unlock()
	s.closeLocked()
}

func (s *DeviceShare) closeLocked() {
	s.mu.Lock()
	live := s.live
	s.live = nil
	s.mu.Unlock()
	s.registry.RevokeAll()
	if live != nil {
		live.stop()
		<-live.done
	}
}

// Offer mints a pairing code for the open share.
func (s *DeviceShare) Offer() (ShareOffer, error) {
	s.mu.Lock()
	live := s.live
	s.mu.Unlock()
	if live == nil {
		return ShareOffer{}, ErrShareClosed
	}
	code, expires := s.registry.Offer()
	link := live.origin + "/#pair=" + code
	svg, err := QRSVG(link)
	if err != nil {
		s.registry.Withdraw()
		return ShareOffer{}, err
	}
	return ShareOffer{URL: link, QR: svg, Expires: expires}, nil
}

// Revoke unpairs one device.
func (s *DeviceShare) Revoke(id string) bool { return s.registry.Revoke(id) }

// Status reports what is open, what could be, and who is paired.
func (s *DeviceShare) Status() ShareStatus {
	s.mu.Lock()
	live := s.live
	s.mu.Unlock()
	st := ShareStatus{Addresses: s.addresses(), Devices: s.registry.Devices()}
	if live != nil {
		st.Open, st.Origin = true, live.origin
	}
	if exp, ok := s.registry.OfferExpires(); ok {
		st.OfferExpires = &exp
	}
	return st
}

// machineName is what a device is told it is driving: this computer's name,
// or a plain word where the system will not say.
func machineName() string {
	if name, err := os.Hostname(); err == nil && strings.TrimSpace(name) != "" {
		return name
	}
	return "this computer"
}

// OffInternet reports whether plain HTTP to ip stays off the public internet:
// loopback, a private range, or a tailnet, whose traffic is already encrypted.
// Only there may a credential ride an unencrypted link.
func OffInternet(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsLoopback() || ip.IsPrivate() || tailnetPrefix.Contains(ip)
}

// PrivateAddresses is every private IPv4 address on an up interface, the ones
// a phone is likeliest to reach first: the first entry is the default.
func PrivateAddresses() []ShareAddress {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	out := []ShareAddress{}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			pfx, err := netip.ParsePrefix(a.String())
			if err != nil {
				continue
			}
			ip := pfx.Addr().Unmap()
			if !ip.Is4() {
				continue
			}
			kind := AddressVirtual
			switch {
			case tailnetPrefix.Contains(ip):
				kind = AddressTailnet
			case !ip.IsPrivate():
				continue
			case len(iface.HardwareAddr) > 0 && iface.Flags&net.FlagBroadcast != 0:
				kind = AddressLAN
			}
			out = append(out, ShareAddress{Interface: iface.Name, IP: ip.String(), Kind: kind})
		}
	}
	slices.SortStableFunc(out, func(a, b ShareAddress) int { return addressRank[a.Kind] - addressRank[b.Kind] })
	return out
}
