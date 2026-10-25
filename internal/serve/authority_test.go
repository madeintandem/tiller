package serve

import (
	"errors"
	"net"
	"net/netip"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func mustAddr(t *testing.T, s string) netip.Addr {
	t.Helper()
	addr, err := netip.ParseAddr(s)
	require.NoError(t, err, s)
	return addr
}

func snapshotAddrs(t *testing.T, ss ...string) []netip.Addr {
	t.Helper()
	addrs := make([]netip.Addr, 0, len(ss))
	for _, s := range ss {
		addrs = append(addrs, mustAddr(t, s))
	}
	return addrs
}

// admitsIP reports whether the policy admits a request whose strictly parsed
// host is the IP literal, and admitsName the one whose host is a name.
func admitsIP(t *testing.T, a *Authority, ip string) bool {
	t.Helper()
	return a.accepts("", mustAddr(t, ip))
}

func admitsName(t *testing.T, a *Authority, name string) bool {
	t.Helper()
	return a.accepts(name, netip.Addr{})
}

func TestAuthorityForBindAcceptsLiterals(t *testing.T) {
	for _, tc := range []struct {
		host    string
		bind    string
		network bool
	}{
		{host: "127.0.0.1", bind: "127.0.0.1"},
		{host: "127.0.0.2", bind: "127.0.0.2"},
		{host: "::1", bind: "::1"},
		{host: "0.0.0.0", bind: "0.0.0.0", network: true},
		{host: "::", bind: "::", network: true},
		{host: "192.168.1.5", bind: "192.168.1.5", network: true},
		{host: "10.0.0.1", bind: "10.0.0.1", network: true},
		{host: "8.8.8.8", bind: "8.8.8.8", network: true},
		{host: "fd12:3456::1", bind: "fd12:3456::1", network: true},
		{host: "2001:0DB8:0:0::1", bind: "2001:db8::1", network: true},
	} {
		t.Run(tc.host, func(t *testing.T) {
			a, err := authorityForBind(tc.host,
				func() ([]net.Interface, error) { return nil, nil },
				func(net.Interface) ([]net.Addr, error) {
					t.Fatal("an empty interface list must not enumerate addresses")
					return nil, nil
				})
			require.NoError(t, err)
			require.Equal(t, mustAddr(t, tc.bind), a.Bind())
			require.Equal(t, tc.network, a.Network())
		})
	}
}

// TestValidateBindHostRefusesUnsupportedValues pins D1's refusals: every
// unsupported spelling is an exit-2 host_invalid argument error whose message
// says what the flag does take, before anything could bind or open a browser.
func TestValidateBindHostRefusesUnsupportedValues(t *testing.T) {
	for _, host := range []string{
		"",          // an explicitly empty value is not the default
		"localhost", // hostnames are not resolved (no DNS)
		"example.com",
		"[::1]", // bracketed values
		"[192.168.1.5]",
		"192.168.1.5:4400", // embedded ports
		"fe80::1%eth0",     // zones
		"2001:db8::1%eth0",
		"::ffff:192.168.1.5", // IPv4-mapped IPv6
		"224.0.0.1",          // multicast
		"ff02::1",
		"169.254.1.1", // link-local
		"fe80::1",
		"255.255.255.255", // limited broadcast
		"not-an-ip",
		"192.168.001.5", // strict parsing: a leading-zero octet is not an IP
	} {
		a, err := AuthorityForBind(host)
		require.Error(t, err, host)
		require.Nil(t, a, host)
		var env *store.EnvError
		require.True(t, errors.As(err, &env), host)
		require.Equal(t, 2, env.ExitCode(), host)
		require.Equal(t, ReasonHostInvalid, env.Reason, host)
		require.Contains(t, env.Msg, "bare IPv4 or IPv6 literal", host)
	}
}

// TestAuthorityForTargetLoopbackPolicies pins D2's default and explicit
// loopback rows: the standard set (localhost, 127.0.0.1, ::1), the explicit
// loopback's extra literal, and the advertised host list.
func TestAuthorityForTargetLoopbackPolicies(t *testing.T) {
	def := authorityForTarget(bindTarget{addr: v4Loopback, mode: modeLoopback}, nil)
	require.False(t, def.Network(), "the default bind does not expose beyond loopback")
	require.Equal(t, mustAddr(t, "127.0.0.1"), def.Bind())
	require.Equal(t, []string{"127.0.0.1"}, def.Hosts())
	require.True(t, admitsName(t, def, "localhost"))
	require.True(t, admitsIP(t, def, "127.0.0.1"))
	require.True(t, admitsIP(t, def, "::1"))
	require.False(t, admitsName(t, def, "127.0.0.1"), "an IP literal is not a name")
	require.False(t, admitsName(t, def, "LOCALHOST"), "names match canonical lowercase")
	require.False(t, admitsName(t, def, "localhost.attacker.example"))
	require.False(t, admitsIP(t, def, "127.0.0.2"), "another loopback literal is not the default")
	require.False(t, admitsIP(t, def, "10.0.0.1"))

	// A second subtest per explicit loopback literal: 127.0.0.2 and ::1.
	for _, tc := range []struct {
		addr string
	}{
		{addr: "127.0.0.2"},
		{addr: "::1"},
	} {
		t.Run("explicit "+tc.addr, func(t *testing.T) {
			a := authorityForTarget(bindTarget{addr: mustAddr(t, tc.addr), mode: modeExplicitLoopback}, nil)
			require.False(t, a.Network())
			require.Equal(t, mustAddr(t, tc.addr), a.Bind())
			require.Equal(t, []string{tc.addr}, a.Hosts())
			require.True(t, admitsName(t, a, "localhost"), "the standard set survives")
			require.True(t, admitsIP(t, a, "127.0.0.1"), "the standard set survives")
			require.True(t, admitsIP(t, a, "::1"), "the standard set survives")
			require.True(t, admitsIP(t, a, tc.addr), "the bound literal is admitted")
			require.False(t, admitsIP(t, a, "10.0.0.1"), "nothing beyond the standard set")
		})
	}
}

// TestAuthorityForTargetWildcard4 pins D2's IPv4 wildcard row over an
// injected snapshot: duplicates collapse, the unusable classes (unspecified,
// opposite family, mapped, multicast, zoned link-local, limited broadcast)
// are dropped, the standard loopback is preserved even when the snapshot
// omits it, and the host list is loopback first, then numerically sorted.
func TestAuthorityForTargetWildcard4(t *testing.T) {
	snap := snapshotAddrs(t,
		"192.168.1.5",     // usable
		"192.168.1.5",     // duplicate collapses
		"0.0.0.0",         // unspecified
		"::",              // opposite family
		"::ffff:10.0.0.9", // IPv4-mapped
		"224.0.0.2",       // multicast
		"fe80::1%eth0",    // zoned link-local, opposite family
		"169.254.1.1",     // IPv4 link-local
		"255.255.255.255", // limited broadcast
		"10.1.2.3",        // usable
		"10.1.2.11",       // numeric, not lexical, sorting
	)
	a := authorityForTarget(bindTarget{addr: mustAddr(t, "0.0.0.0"), mode: modeWildcard4}, snap)
	require.True(t, a.Network())
	require.Equal(t, mustAddr(t, "0.0.0.0"), a.Bind())
	require.Equal(t, []string{"127.0.0.1", "10.1.2.3", "10.1.2.11", "192.168.1.5"}, a.Hosts())

	require.True(t, admitsName(t, a, "localhost"))
	require.False(t, admitsName(t, a, "mitbox.local"), "machine hostnames are not admitted")
	for _, ip := range []string{"127.0.0.1", "192.168.1.5", "10.1.2.3", "10.1.2.11"} {
		require.True(t, admitsIP(t, a, ip), ip)
	}
	for _, ip := range []string{"0.0.0.0", "10.0.0.9", "169.254.1.1", "224.0.0.2", "255.255.255.255", "192.168.1.6", "8.8.8.8"} {
		require.False(t, admitsIP(t, a, ip), ip)
	}
	// No IPv6 address from the snapshot is admitted by an IPv4 wildcard.
	for _, ip := range []string{"::1", "fe80::1", "fe80::1%eth0", "fd12::1"} {
		require.False(t, admitsIP(t, a, ip), ip)
	}
}

// TestAuthorityForTargetWildcard6 pins D2's IPv6 wildcard row: only usable
// family addresses are admitted, the standard ::1 is preserved and leads the
// host list, and no IPv4 literal appears.
func TestAuthorityForTargetWildcard6(t *testing.T) {
	snap := snapshotAddrs(t,
		"fd12::5",         // usable unique-local
		"fd12::2",         // usable unique-local
		"fd12::10",        // numeric, not lexical, sorting
		"fd12::9%eth0",    // global/unique-local zones are also excluded
		"::",              // unspecified
		"ff02::1",         // multicast
		"fe80::1",         // unzoned link-local
		"fe80::1%eth0",    // zoned link-local
		"192.168.1.5",     // opposite family
		"::ffff:10.0.0.9", // IPv4-mapped
	)
	snap = append(snap, netip.Addr{}) // invalid addresses must not be advertised
	a := authorityForTarget(bindTarget{addr: mustAddr(t, "::"), mode: modeWildcard6}, snap)
	require.True(t, a.Network())
	require.Equal(t, mustAddr(t, "::"), a.Bind())
	require.Equal(t, []string{"::1", "fd12::2", "fd12::5", "fd12::10"}, a.Hosts())

	require.True(t, admitsName(t, a, "localhost"))
	for _, ip := range []string{"::1", "fd12::2", "fd12::5", "fd12::10"} {
		require.True(t, admitsIP(t, a, ip), ip)
	}
	for _, ip := range []string{"::", "fd12::9%eth0", "ff02::1", "fe80::1", "fe80::1%eth0", "10.0.0.9", "192.168.1.5", "127.0.0.1"} {
		require.False(t, admitsIP(t, a, ip), ip)
	}
}

// TestAuthorityForTargetSpecific pins D2's specific non-loopback row: exactly
// the bound IP, no names, and the injected snapshot is ignored because only
// wildcard binds enumerate interfaces.
func TestAuthorityForTargetSpecific(t *testing.T) {
	// Garbage snapshot: a specific bind must not consult it.
	a := authorityForTarget(bindTarget{addr: mustAddr(t, "192.168.1.5"), mode: modeSpecific},
		snapshotAddrs(t, "10.9.9.9", "127.0.0.1"))
	require.True(t, a.Network())
	require.Equal(t, mustAddr(t, "192.168.1.5"), a.Bind())
	require.Equal(t, []string{"192.168.1.5"}, a.Hosts())

	require.True(t, admitsIP(t, a, "192.168.1.5"))
	require.False(t, admitsName(t, a, "localhost"), "no name aliases for a specific bind")
	require.False(t, admitsIP(t, a, "10.9.9.9"), "the snapshot is not consulted")
	require.False(t, admitsIP(t, a, "127.0.0.1"), "no loopback aliases for a specific bind")
}

func TestAuthorityForBindEnumeratesOnlyUpInterfaces(t *testing.T) {
	for _, tc := range []struct {
		host  string
		hosts []string
	}{
		{"0.0.0.0", []string{"127.0.0.1", "10.0.0.2", "192.168.1.5"}},
		{"::", []string{"::1", "2001:db8::2", "fd12::5"}},
	} {
		t.Run(tc.host, func(t *testing.T) {
			interfaceCalls := 0
			var addressCalls []string
			a, err := authorityForBind(tc.host,
				func() ([]net.Interface, error) {
					interfaceCalls++
					return []net.Interface{
						{Name: "down", Flags: net.FlagBroadcast},
						{Name: "vpn", Flags: net.FlagUp},
						{Name: "lan", Flags: net.FlagUp | net.FlagBroadcast},
					}, nil
				},
				func(iface net.Interface) ([]net.Addr, error) {
					addressCalls = append(addressCalls, iface.Name)
					switch iface.Name {
					case "down":
						return nil, errors.New("a down interface must not be read")
					case "vpn":
						return []net.Addr{
							&net.IPNet{IP: net.ParseIP("192.168.1.5"), Mask: net.CIDRMask(24, 32)},
							&net.IPNet{IP: net.ParseIP("fd12::5"), Mask: net.CIDRMask(64, 128)},
						}, nil
					default:
						return []net.Addr{
							&net.IPNet{IP: net.ParseIP("10.0.0.2"), Mask: net.CIDRMask(24, 32)},
							&net.IPAddr{IP: net.ParseIP("192.168.1.5")},
							&net.IPAddr{IP: net.ParseIP("2001:db8::2")},
							&net.IPAddr{IP: net.ParseIP("fd12::9"), Zone: "lan"},
							&net.IPAddr{IP: net.ParseIP("169.254.1.1")},
							&net.IPAddr{IP: net.ParseIP("ff02::1")},
							&net.IPAddr{},
						}, nil
					}
				})
			require.NoError(t, err)
			require.Equal(t, 1, interfaceCalls, "enumeration is a startup snapshot")
			require.Equal(t, []string{"vpn", "lan"}, addressCalls)
			require.Equal(t, tc.hosts, a.Hosts())
			require.True(t, admitsName(t, a, "localhost"))
			for _, host := range tc.hosts {
				require.True(t, admitsIP(t, a, host), "advertised IPs must be admitted")
			}
			for _, host := range []string{"10.0.0.99", "fd12::99", "fd12::9%lan", "169.254.1.1", "ff02::1"} {
				require.False(t, admitsIP(t, a, host), host)
			}
			require.False(t, admitsName(t, a, "machine.local"))
		})
	}
}

func TestAuthorityForBindEnumerationFailures(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "::"} {
		for _, failure := range []string{"interfaces", "addresses"} {
			t.Run(host+"/"+failure, func(t *testing.T) {
				a, err := authorityForBind(host,
					func() ([]net.Interface, error) {
						if failure == "interfaces" {
							return nil, errors.New("interface visibility denied")
						}
						return []net.Interface{{Name: "lan", Flags: net.FlagUp}}, nil
					},
					func(net.Interface) ([]net.Addr, error) {
						if failure == "interfaces" {
							t.Fatal("failed interface enumeration must stop before address reads")
						}
						return nil, errors.New("address visibility denied")
					})
				require.Nil(t, a, "failure must not return a partial policy")
				var env *store.EnvError
				require.ErrorAs(t, err, &env)
				require.Equal(t, 3, env.ExitCode())
				require.Equal(t, "host_interfaces_unavailable", env.Reason)
				require.Contains(t, env.Msg, host)
				require.Contains(t, env.Msg, "visibility denied")
				require.Contains(t, env.Msg, "specific locally assigned address")
				if failure == "addresses" {
					require.Contains(t, env.Msg, "lan")
				}
			})
		}
	}
}

func TestAuthorityForBindSkipsEnumerationForNonwildcards(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "127.0.0.2", "::1", "192.168.1.5", "fd12::5", "", "localhost", "fe80::1"} {
		t.Run(host, func(t *testing.T) {
			a, err := authorityForBind(host,
				func() ([]net.Interface, error) {
					t.Fatal("nonwildcards must not enumerate interfaces")
					return nil, errors.New("interfaces unavailable")
				},
				func(net.Interface) ([]net.Addr, error) {
					t.Fatal("nonwildcards must not enumerate addresses")
					return nil, errors.New("addresses unavailable")
				})
			if host == "" || host == "localhost" || host == "fe80::1" {
				require.Nil(t, a)
				var env *store.EnvError
				require.ErrorAs(t, err, &env)
				require.Equal(t, 2, env.ExitCode())
				require.Equal(t, "host_invalid", env.Reason)
				return
			}
			require.NoError(t, err)
			require.Equal(t, []string{host}, a.Hosts())
		})
	}
}

func TestAuthorityOwnsItsSnapshotAndHostList(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "::", "127.0.0.1", "192.168.1.5"} {
		t.Run(host, func(t *testing.T) {
			target, err := validateBindHost(host)
			require.NoError(t, err)
			snapshot := snapshotAddrs(t, "192.168.1.5", "fd12::5")
			a := authorityForTarget(target, snapshot)
			original := append([]string(nil), a.Hosts()...)
			snapshot[0] = mustAddr(t, "10.9.9.9")
			hosts := a.Hosts()
			hosts[0] = "attacker.example"
			require.Equal(t, original, a.Hosts(), "returned lists must not mutate the policy")
			require.False(t, admitsName(t, a, "attacker.example"))
			require.False(t, admitsIP(t, a, "10.9.9.9"), "snapshot changes cannot add admitted addresses")
			for _, ip := range original {
				require.True(t, admitsIP(t, a, ip), "original destinations remain admitted")
			}
		})
	}
}
