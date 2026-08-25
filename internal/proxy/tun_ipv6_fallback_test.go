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
		// Dropping IPv6 from the TUN without strict_route would send the host's
		// IPv6 traffic straight out the physical adapter — a real-address leak.
		// strict_route's WFP filters blackhole it instead.
		if !in.StrictRoute {
			t.Fatal("dropping IPv6 must force strict_route so IPv6 is blackholed, not leaked")
		}
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
