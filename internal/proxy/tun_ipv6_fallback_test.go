// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package proxy

import (
	"context"
	"errors"
	"net"
	"runtime"
	"strings"
	"testing"

	"resultproxy-wails/internal/logger"
)

const ipv6SetErr = "start inbound/tun[tun-in]: configure tun interface: set ipv6 address: Element not found."

func TestIsTunIPv6Error(t *testing.T) {
	if !isTunIPv6Error(errors.New(ipv6SetErr)) {
		t.Fatal("the sing-tun ipv6 address failure must be recognised")
	}
	// An IPv4-side failure of the same shape must NOT trigger the IPv6 fallback:
	// dropping IPv6 cannot fix it, and retrying without IPv6 would hide the real
	// cause behind a degraded tunnel.
	if isTunIPv6Error(errors.New("configure tun interface: set ipv4 address: Element not found.")) {
		t.Fatal("an ipv4 address failure must not be treated as the ipv6 case")
	}
	if isTunIPv6Error(nil) {
		t.Fatal("nil is not an ipv6 error")
	}
}

// TunDisableIPv6 is the retry-only switch startEngine flips after the adapter
// refuses an IPv6 address. It must win over BOTH the explicit TunIPv6 override
// and systemHasIPv6(), or the retry rebuilds the identical config and fails
// identically — which is exactly the wedge this fixes.
func TestBuildTunnelModeConfig_DisableIPv6DropsTheULA(t *testing.T) {
	cfg := EngineConfig{
		Proxy:          ProxyConfig{Type: "vless", IP: "203.0.113.7", Port: 443, Password: "p"},
		Mode:           ProxyModeTunnel,
		TunIPv6:        "fdfe:dcba:9876::1/126",
		TunDisableIPv6: true,
	}
	sb := mustBuildTunnelModeConfig(t, cfg)
	for _, in := range sb.Inbounds {
		if in.Tag != "tun-in" {
			continue
		}
		for _, addr := range in.Address {
			if strings.Contains(addr, ":") {
				t.Fatalf("TunDisableIPv6 must drop every IPv6 address, got %q", in.Address)
			}
		}
		// Whether that also forces strict_route depends on the host actually
		// having routable IPv6 to leak — covered by
		// TestBuildTunnelModeConfig_ForcesStrictRouteWhenIPv6WouldLeak and its
		// negative twin, not here.
		return
	}
	t.Fatal("no tun-in inbound")
}

func TestBuildTunnelModeConfig_DisableIPv6KeepsIPv4(t *testing.T) {
	sb := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy:          ProxyConfig{Type: "vless", IP: "203.0.113.7", Port: 443, Password: "p"},
		Mode:           ProxyModeTunnel,
		TunDisableIPv6: true,
	})
	for _, in := range sb.Inbounds {
		if in.Tag != "tun-in" {
			continue
		}
		if len(in.Address) != 1 || !strings.HasPrefix(in.Address[0], "172.19.0.1") {
			t.Fatalf("expected IPv4-only tun address, got %q", in.Address)
		}
	}
	assertCoreAcceptsConfig(t, sb)
}

// The wedge: the adapter refuses IPv6, so removing a ghost device changes
// nothing and a retry with the SAME config fails identically. The retry must
// change the config.
func TestStartEngine_RetriesWithoutIPv6WhenAdapterRefusesIt(t *testing.T) {
	fastTunRetry(t)
	fakeAdmin(t, true)
	stubRemoveStaleTunAdapter(t)

	eng := &recordingEngine{errs: []error{errors.New(ipv6SetErr)}}
	m := NewManager(logger.New())
	m.engine = eng

	err, _, _, _ := m.startEngine(context.Background(), EngineConfig{Mode: ProxyModeTunnel})
	if err != nil {
		t.Fatalf("expected the IPv4-only retry to succeed, got %v", err)
	}
	if len(eng.cfgs) != 2 {
		t.Fatalf("expected exactly one retry, got %d starts", len(eng.cfgs))
	}
	if eng.cfgs[0].TunDisableIPv6 {
		t.Fatal("the first attempt must keep IPv6")
	}
	if !eng.cfgs[1].TunDisableIPv6 {
		t.Fatal("the retry must drop IPv6 — retrying the identical config cannot help")
	}
}

// A plain transient TUN error must still take the ghost-removal path, not the
// IPv6 one: dropping IPv6 there would degrade the tunnel for no reason.
func TestStartEngine_TransientErrorDoesNotDropIPv6(t *testing.T) {
	fastTunRetry(t)
	fakeAdmin(t, true)
	calls := stubRemoveStaleTunAdapter(t)

	eng := &recordingEngine{errs: []error{errors.New("start inbound/tun[tun-in]: configure tun interface: Access is denied.")}}
	m := NewManager(logger.New())
	m.engine = eng

	err, _, _, _ := m.startEngine(context.Background(), EngineConfig{Mode: ProxyModeTunnel})
	if err != nil {
		t.Fatalf("expected success after the ghost-removal retry, got %v", err)
	}
	if *calls != 1 {
		t.Fatalf("ghost removal must run exactly once, got %d", *calls)
	}
	for i, c := range eng.cfgs {
		if c.TunDisableIPv6 {
			t.Fatalf("attempt %d must keep IPv6 for a non-IPv6 failure", i)
		}
	}
}

// recordingEngine fails with errs[i] on start i and records the config it was
// handed, so a test can assert WHAT the retry changed, not just that it retried.
type recordingEngine struct {
	Engine
	errs []error
	cfgs []EngineConfig
}

func (e *recordingEngine) Start(ctx context.Context, cfg EngineConfig) error {
	e.cfgs = append(e.cfgs, cfg)
	if len(e.cfgs) <= len(e.errs) {
		return e.errs[len(e.cfgs)-1]
	}
	return nil
}

func (e *recordingEngine) Stop() error     { return nil }
func (e *recordingEngine) IsRunning() bool { return false }

// stubRoutableIPv6 controls the "would IPv6 leak if we do not tunnel it" probe,
// which otherwise enumerates the real host's interfaces.
func stubRoutableIPv6(t *testing.T, v bool) {
	t.Helper()
	prev := hasRoutableIPv6Fn
	hasRoutableIPv6Fn = func() bool { return v }
	t.Cleanup(func() { hasRoutableIPv6Fn = prev })
}

// Without an IPv6 address on the TUN (the adapter refused it) a host with a
// globally routable IPv6 would route that traffic out the physical adapter. Our
// ipv4_only resolver keeps domain traffic off IPv6, but any app with its own DoH
// resolver (every modern browser) gets AAAA independently, so the leak is real.
func TestBuildTunnelModeConfig_ForcesStrictRouteWhenIPv6WouldLeak(t *testing.T) {
	stubRoutableIPv6(t, true)
	sb := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy:             ProxyConfig{Type: "vless", IP: "203.0.113.7", Port: 443, Password: "p"},
		Mode:              ProxyModeTunnel,
		DNSLeakProtection: false,
		TunDisableIPv6:    true,
	})
	if !tunInboundOf(t, sb).StrictRoute {
		t.Fatal("a host with routable IPv6 and no IPv6 on the TUN must get strict_route")
	}
}

// With the address on the TUN, routable IPv6 is routed into the tunnel, so there
// is nothing to force — and forcing it would bring back the blanket IPv6 block.
func TestBuildTunnelModeConfig_NoForcedStrictRouteWhenTunHasIPv6(t *testing.T) {
	stubHostSupportsIPv6(t, true)
	stubRoutableIPv6(t, true)
	sb := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy:             ProxyConfig{Type: "vless", IP: "203.0.113.7", Port: 443, Password: "p"},
		Mode:              ProxyModeTunnel,
		DNSLeakProtection: false,
	})
	if tunInboundOf(t, sb).StrictRoute {
		t.Fatal("IPv6 routed into the TUN cannot leak — strict_route must stay off")
	}
}

// ...but only then. Forcing the WFP filters on every host would silently
// re-enable a feature the user deliberately turned off, for no gain: a host with
// only fe80::/fd00:: has no IPv6 that can reach the internet, so none can leak.
func TestBuildTunnelModeConfig_NoForcedStrictRouteWithoutRoutableIPv6(t *testing.T) {
	stubRoutableIPv6(t, false)
	sb := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy:             ProxyConfig{Type: "vless", IP: "203.0.113.7", Port: 443, Password: "p"},
		Mode:              ProxyModeTunnel,
		DNSLeakProtection: false,
	})
	if tunInboundOf(t, sb).StrictRoute {
		t.Fatal("no routable IPv6 means nothing can leak — strict_route must stay off")
	}
}

func TestIsLeakableIPv6(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"2001:4860:4860::8888", true},  // global unicast
		{"2a00:1450:4001:80f::200e", true},
		{"fe80::5fdd:2162:71f1:268e", false}, // link-local: cannot reach the internet
		{"fdfd::1a7a:9a83", false},           // ULA: ditto
		{"fc00::1", false},                   // ULA lower half
		{"::1", false},                       // loopback
		{"ff02::1", false},                   // multicast
		{"::", false},                        // unspecified
		{"192.168.1.10", false},              // IPv4 is not IPv6
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			if got := isLeakableIPv6(net.ParseIP(tc.ip)); got != tc.want {
				t.Fatalf("isLeakableIPv6(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
	if isLeakableIPv6(nil) {
		t.Fatal("nil is not a leakable address")
	}
}

func tunInboundOf(t *testing.T, sb SingBoxConfig) SBInbound {
	t.Helper()
	for _, in := range sb.Inbounds {
		if in.Tag == "tun-in" {
			return in
		}
	}
	t.Fatal("no tun-in inbound")
	return SBInbound{}
}

// stubHostSupportsIPv6 controls the "can this box take an IPv6 address at all"
// probe — a different question from hasRoutableIPv6 ("could IPv6 leak"), which is
// why they are separate seams: a link-local-only host answers yes to the first
// and no to the second.
func stubHostSupportsIPv6(t *testing.T, v bool) {
	t.Helper()
	prev := hostSupportsIPv6Fn
	hostSupportsIPv6Fn = func() bool { return v }
	t.Cleanup(func() { hostSupportsIPv6Fn = prev })
}

func ipv6ToggleConfig(enable bool) EngineConfig {
	return EngineConfig{
		Proxy:      ProxyConfig{Type: "vless", IP: "203.0.113.7", Port: 443, Password: "p"},
		Mode:       ProxyModeTunnel,
		EnableIPv6: enable,
	}
}

// The toggle switches the resolver off ipv4_only; the address is there either way.
func TestBuildTunnelModeConfig_EnableIPv6AttachesAddressAndSwitchesDNS(t *testing.T) {
	stubHostSupportsIPv6(t, true)
	stubRoutableIPv6(t, true)
	sb := mustBuildTunnelModeConfig(t, ipv6ToggleConfig(true))
	in := tunInboundOf(t, sb)
	if len(in.Address) != 2 || in.Address[1] != "fdfe:dcba:9876::1/126" {
		t.Fatalf("enabled IPv6 must attach the ULA, got %q", in.Address)
	}
	if sb.DNS == nil || sb.DNS.Strategy != "prefer_ipv4" {
		t.Fatalf("enabled IPv6 must switch DNS off ipv4_only, got %+v", sb.DNS)
	}
	assertCoreAcceptsConfig(t, sb)
}

func TestBuildTunnelModeConfig_DisabledIPv6KeepsIPv4OnlyDNS(t *testing.T) {
	stubHostSupportsIPv6(t, true)
	stubRoutableIPv6(t, false)
	sb := mustBuildTunnelModeConfig(t, ipv6ToggleConfig(false))
	if in := tunInboundOf(t, sb); len(in.Address) != 2 {
		t.Fatalf("the TUN keeps its IPv6 address with the toggle off, got %q", in.Address)
	}
	if sb.DNS == nil || sb.DNS.Strategy != "ipv4_only" {
		t.Fatalf("disabled IPv6 must keep ipv4_only, got %+v", sb.DNS)
	}
}

// The guard the whole feature hangs on: a user can tick the box on a machine that
// has no IPv6 at all. Attaching the address there is what fails with
// "set ipv6 address: ..." and takes the entire inbound down, so we must behave
// exactly as if the box were unticked — including the DNS half, or apps would get
// AAAA answers with no IPv6 path to use them.
func TestBuildTunnelModeConfig_EnableIPv6IsIgnoredWhenHostHasNoIPv6(t *testing.T) {
	stubHostSupportsIPv6(t, false)
	stubRoutableIPv6(t, false)
	sb := mustBuildTunnelModeConfig(t, ipv6ToggleConfig(true))
	if in := tunInboundOf(t, sb); len(in.Address) != 1 || strings.Contains(in.Address[0], ":") {
		t.Fatalf("host without IPv6 must get an IPv4-only tun, got %q", in.Address)
	}
	if sb.DNS == nil || sb.DNS.Strategy != "ipv4_only" {
		t.Fatalf("guard must revert the DNS half too, got %+v", sb.DNS)
	}
	assertCoreAcceptsConfig(t, sb)
}

// TunDisableIPv6 is set by startEngine only after the adapter has already refused
// the address, so it has to win over the user's opt-in — otherwise the retry
// rebuilds the config that just failed.
func TestBuildTunnelModeConfig_TunDisableIPv6BeatsTheToggle(t *testing.T) {
	stubHostSupportsIPv6(t, true)
	stubRoutableIPv6(t, false)
	cfg := ipv6ToggleConfig(true)
	cfg.TunDisableIPv6 = true
	sb := mustBuildTunnelModeConfig(t, cfg)
	if in := tunInboundOf(t, sb); len(in.Address) != 1 {
		t.Fatalf("TunDisableIPv6 must strip IPv6 despite the toggle, got %q", in.Address)
	}
	if sb.DNS == nil || sb.DNS.Strategy != "ipv4_only" {
		t.Fatalf("TunDisableIPv6 must revert the DNS half too, got %+v", sb.DNS)
	}
}

// A custom address overrides the default ULA whether or not the toggle is on.
func TestBuildTunnelModeConfig_CustomTunIPv6IgnoresTheToggle(t *testing.T) {
	stubHostSupportsIPv6(t, true)
	stubRoutableIPv6(t, false)
	for _, enable := range []bool{false, true} {
		cfg := ipv6ToggleConfig(enable)
		cfg.TunIPv6 = "fd00:dead:beef::1/64"
		in := tunInboundOf(t, mustBuildTunnelModeConfig(t, cfg))
		if len(in.Address) != 2 || in.Address[1] != "fd00:dead:beef::1/64" {
			t.Fatalf("EnableIPv6=%v: the custom address must win, got %q", enable, in.Address)
		}
	}
}

func loopbackWarned(m *Manager) bool {
	for _, e := range m.log.GetAll() {
		if strings.Contains(e.Msg, "::1") {
			return true
		}
	}
	return false
}

// strict_route on a TUN without an IPv6 address blocks ::1 (issue #15); the
// user must be told, and only then.
func TestWarnIPv6LoopbackBlocked(t *testing.T) {
	stubRoutableIPv6(t, false)
	cases := []struct {
		name        string
		hostIPv6    bool
		disableIPv6 bool
		leakProt    bool
		want        bool
	}{
		{"address on tun", true, false, true, false},
		{"system ipv6 off", false, false, true, true},
		{"adapter refused", true, true, true, true},
		{"no strict_route", false, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubHostSupportsIPv6(t, tc.hostIPv6)
			m := NewManager(logger.New())
			m.warnIPv6LoopbackBlocked(EngineConfig{
				Mode:              ProxyModeTunnel,
				DNSLeakProtection: tc.leakProt,
				TunDisableIPv6:    tc.disableIPv6,
			})
			want := tc.want && runtime.GOOS == "windows"
			if got := loopbackWarned(m); got != want {
				t.Fatalf("warned = %v, want %v", got, want)
			}
		})
	}
}
