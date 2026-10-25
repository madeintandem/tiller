// authority.go owns the console's request policy: which Hosts a bind
// admits, what a bind exposes, and what a bind advertises. AuthorityForBind
// builds the policy once at serve startup from the --host literal; the
// request boundary (boundary.go) enforces it on every request, before any
// project, visit, or asset read.
package serve

import (
	"fmt"
	"net"
	"net/netip"
	"slices"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/store"
)

// ReasonHostInvalid is the exit-2 argument error for an invalid --host
// value; ReasonHostInterfacesUnavailable is the exit-3 environment error
// when a wildcard bind cannot enumerate the machine's interface addresses.
const (
	ReasonHostInvalid               = "host_invalid"
	ReasonHostInterfacesUnavailable = "host_interfaces_unavailable"
)

// Authority is the console's request policy for one bind: the Host
// authorities it admits (names and IPs, matched exactly and not
// interchangeably), whether the bind exposes the console beyond loopback,
// and the bound family's admitted host list for printed destinations. It is
// immutable once built. A wildcard's admit set is a startup snapshot:
// interface addresses that appear later are admitted only after a restart,
// and addresses that disappear stay admitted until then (ui-surface.md
// documents the limit).
type Authority struct {
	bind     netip.Addr
	network  bool
	names    map[string]struct{}
	ips      map[netip.Addr]struct{}
	hostList []string
}

// Bind is the normalized bound literal, so the CLI derives the listener
// family (tcp4 or tcp6) and formats the listen address without re-parsing
// the flag.
func (a *Authority) Bind() netip.Addr { return a.bind }

// Network reports whether the bind exposes the console beyond loopback.
// Network modes carry the extra plain-HTTP protections: the Referer check
// and the framing headers on every response.
func (a *Authority) Network() bool { return a.network }

// Hosts lists IP destinations for the CLI to advertise. Non-wildcard binds
// advertise only their bound literal. Wildcards list the standard loopback
// first, then admitted IPs numerically sorted, all in the listener's family.
// The returned slice is a copy, owned by the caller.
func (a *Authority) Hosts() []string { return slices.Clone(a.hostList) }

// accepts reports whether a strictly parsed request host is admitted: a
// name against the admitted names, an IP against the admitted IPs. The two
// are not interchangeable, so a request to 192.168.1.5:4400 is not
// admitted by a policy that admits localhost on the same port.
func (a *Authority) accepts(name string, ip netip.Addr) bool {
	if name != "" {
		_, ok := a.names[name]
		return ok
	}
	if !ip.IsValid() {
		return false
	}
	_, ok := a.ips[ip]
	return ok
}

// AuthorityForBind validates a --host literal, builds the admit set, and
// returns the console's request policy. Only wildcard binds enumerate the
// machine's interfaces. Parse failures are exit-2 host_invalid argument
// errors and an enumeration failure is exit-3 host_interfaces_unavailable,
// so the CLI refuses before binding or opening a browser.
func AuthorityForBind(host string) (*Authority, error) {
	return authorityForBind(host, net.Interfaces, func(iface net.Interface) ([]net.Addr, error) {
		return iface.Addrs()
	})
}

// authorityForBind injects OS enumeration without mutable package state.
func authorityForBind(host string, listInterfaces func() ([]net.Interface, error), interfaceAddrs func(net.Interface) ([]net.Addr, error)) (*Authority, error) {
	target, err := validateBindHost(host)
	if err != nil {
		return nil, err
	}
	var addrs []netip.Addr
	if target.mode == modeWildcard4 || target.mode == modeWildcard6 {
		if addrs, err = interfaceAddresses(listInterfaces, interfaceAddrs); err != nil {
			return nil, &store.EnvError{
				Reason: ReasonHostInterfacesUnavailable,
				Msg: fmt.Sprintf(
					"a wildcard bind (%s) needs the machine's interface addresses, but they could not be enumerated: %v. Bind a specific locally assigned address instead, or restore interface visibility and retry",
					host, err),
			}
		}
	}
	return authorityForTarget(target, addrs), nil
}

// interfaceAddresses takes one snapshot of addresses on up interfaces.
// Errors refuse the entire snapshot instead of admitting a partial list.
func interfaceAddresses(listInterfaces func() ([]net.Interface, error), interfaceAddrs func(net.Interface) ([]net.Addr, error)) ([]netip.Addr, error) {
	ifaces, err := listInterfaces()
	if err != nil {
		return nil, err
	}
	var addrs []netip.Addr
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addresses, err := interfaceAddrs(iface)
		if err != nil {
			return nil, fmt.Errorf("reading addresses for interface %q: %w", iface.Name, err)
		}
		for _, address := range addresses {
			var ip net.IP
			switch address := address.(type) {
			case *net.IPNet:
				ip = address.IP
			case *net.IPAddr:
				if address.Zone != "" {
					continue
				}
				ip = address.IP
			default:
				continue
			}
			if addr, ok := netip.AddrFromSlice(ip); ok {
				// net.IP often stores ordinary IPv4 as a 16-byte value.
				addrs = append(addrs, addr.Unmap())
			}
		}
	}
	return addrs, nil
}

type bindMode int

const (
	modeLoopback         bindMode = iota // 127.0.0.1: the standard loopback set
	modeExplicitLoopback                 // another loopback literal: the standard set plus the bound literal
	modeWildcard4                        // 0.0.0.0: every IPv4 interface
	modeWildcard6                        // :: every IPv6 interface
	modeSpecific                         // one non-loopback literal: exactly that IP
)

type bindTarget struct {
	addr netip.Addr
	mode bindMode
}

// validateBindHost parses a --host literal. It accepts only bare IPv4 and
// IPv6 literals the console can bind: loopback, wildcard, and usable
// unicast, private and unique-local included. Hostnames are not resolved
// (no DNS), bracketed values and embedded ports carry nothing a literal
// does not, and scoped, link-local, multicast, and broadcast serving are
// out of scope for this flag, so each is refused saying why.
func validateBindHost(host string) (bindTarget, error) {
	refuse := func(why string) error {
		return &store.EnvError{
			Code:   2,
			Reason: ReasonHostInvalid,
			Msg: fmt.Sprintf(
				"invalid --host %q: %s. --host takes a bare IPv4 or IPv6 literal: 127.0.0.1 (the default), 0.0.0.0 (every IPv4 interface), :: (every IPv6 interface), or a specific locally assigned address such as 192.168.1.5 or 2001:db8::1",
				host, why),
		}
	}
	switch {
	case host == "":
		return bindTarget{}, refuse("an empty value is not the default; omit --host or pass 127.0.0.1 explicitly")
	case strings.HasPrefix(host, "["):
		return bindTarget{}, refuse("bracketed values are not supported; pass the bare literal")
	case strings.Contains(host, "%"):
		return bindTarget{}, refuse("zones are not supported; scoped and link-local serving are outside this flag's scope")
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		if _, portErr := netip.ParseAddrPort(host); portErr == nil {
			return bindTarget{}, refuse("an embedded port is not supported; pass the address without a port")
		}
		return bindTarget{}, refuse("not an IP literal; hostnames are not resolved (no DNS)")
	}
	switch {
	case addr.Zone() != "":
		return bindTarget{}, refuse("zones are not supported; scoped and link-local serving are outside this flag's scope")
	case addr.Is4In6():
		return bindTarget{}, refuse("IPv4-mapped IPv6 is not supported; pass the bare IPv4 address")
	case addr.IsUnspecified():
		if addr.Is4() {
			return bindTarget{addr: addr, mode: modeWildcard4}, nil
		}
		return bindTarget{addr: addr, mode: modeWildcard6}, nil
	case addr.IsLoopback():
		if addr == v4Loopback {
			return bindTarget{addr: addr, mode: modeLoopback}, nil
		}
		return bindTarget{addr: addr, mode: modeExplicitLoopback}, nil
	case addr.IsMulticast():
		return bindTarget{}, refuse("multicast addresses cannot be served")
	case addr.IsLinkLocalUnicast():
		return bindTarget{}, refuse("link-local addresses are not supported; scoped and link-local serving are outside this flag's scope")
	}
	if addr.Is4() && addr.As4() == [4]byte{255, 255, 255, 255} {
		return bindTarget{}, refuse("the limited broadcast address cannot be served")
	}
	if !addr.IsGlobalUnicast() {
		return bindTarget{}, refuse("only loopback, wildcard, and global-unicast addresses are supported")
	}
	return bindTarget{addr: addr, mode: modeSpecific}, nil
}

// authorityForTarget builds a policy over an injected interface snapshot;
// only wildcard modes consult it. Keeping construction pure over the list
// is what lets the policy's tests be deterministic, no developer LAN, no
// interface order, no DNS. The enumeration wrapper supplies the OS snapshot.
func authorityForTarget(target bindTarget, addrs []netip.Addr) *Authority {
	a := &Authority{
		bind:  target.addr,
		names: make(map[string]struct{}),
		ips:   make(map[netip.Addr]struct{}),
	}
	allow := func(ip netip.Addr) { a.ips[ip] = struct{}{} }
	switch target.mode {
	case modeLoopback, modeExplicitLoopback:
		a.names["localhost"] = struct{}{}
		allow(v4Loopback)
		allow(netip.IPv6Loopback())
		if target.mode == modeExplicitLoopback {
			allow(target.addr)
		}
	case modeWildcard4, modeWildcard6:
		a.network = true
		a.names["localhost"] = struct{}{}
		wantV4 := target.mode == modeWildcard4
		if wantV4 {
			allow(v4Loopback)
		} else {
			allow(netip.IPv6Loopback())
		}
		for _, addr := range addrs {
			if usableWildcardAddr(addr, wantV4) {
				allow(addr)
			}
		}
	case modeSpecific:
		a.network = true
		allow(target.addr)
	}
	a.hostList = hostList(target, a.ips)
	return a
}

// hostList is the bound family's admit set for printed destinations. A
// non-wildcard bind advertises exactly its bound literal, the browser URL
// to open; a wildcard advertises the standard loopback first, then its
// admitted interface addresses sorted numerically, so the printed URLs and
// the Host admission cannot disagree.
func hostList(target bindTarget, ips map[netip.Addr]struct{}) []string {
	if target.mode != modeWildcard4 && target.mode != modeWildcard6 {
		return []string{target.addr.String()}
	}
	var std netip.Addr
	wantV4 := target.mode == modeWildcard4
	if wantV4 {
		std = v4Loopback
	} else {
		std = netip.IPv6Loopback()
	}
	hosts := []string{std.String()}
	var rest []netip.Addr
	for ip := range ips {
		if ip == std {
			continue
		}
		if (wantV4 && ip.Is4()) || (!wantV4 && ip.Is6() && !ip.Is4In6()) {
			rest = append(rest, ip)
		}
	}
	sort.Slice(rest, func(i, j int) bool {
		return rest[i].Compare(rest[j]) < 0
	})
	for _, ip := range rest {
		hosts = append(hosts, ip.String())
	}
	return hosts
}

// usableWildcardAddr admits only unzoned loopback or global-unicast
// literals in the listener's family, never wildcard destinations.
func usableWildcardAddr(addr netip.Addr, wantV4 bool) bool {
	return addr.IsValid() && addr.Zone() == "" && addr.Is4() == wantV4 &&
		!addr.Is4In6() && (addr.IsLoopback() || addr.IsGlobalUnicast())
}

// v4Loopback is 127.0.0.1, the standard IPv4 loopback.
var v4Loopback = netip.AddrFrom4([4]byte{127, 0, 0, 1})

// DefaultAuthority is the default bind's policy (127.0.0.1): the standard
// loopback set, the console's posture when --host is omitted or explicit
// 127.0.0.1. Tests and embeds that stand in for the CLI pass it explicitly;
// production always goes through AuthorityForBind.
func DefaultAuthority() *Authority {
	return authorityForTarget(bindTarget{addr: v4Loopback, mode: modeLoopback}, nil)
}
