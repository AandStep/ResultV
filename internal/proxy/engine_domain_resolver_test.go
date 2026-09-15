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
	"encoding/json"
	"testing"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
)

// A node addressed by a domain has to be resolved before it can be dialled, and
// sing-box 1.14 declared the path that decides how — walking dns.rules with no
// resolver named — deprecated and scheduled for removal. The node is the one
// dial where the right answer is known, so it is spelled out on the dial field:
// the same static hosts record the DNS rule already points at.
func TestDomainNodeOutboundResolvesThroughTheServerPin(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{IP: "node.example.com", Port: 443, Type: "vless", ResolvedIPs: []string{"203.0.113.7", "203.0.113.8"}},
	})

	out, ok := outboundByTag(cfg, "proxy")
	if !ok {
		t.Fatal("no proxy outbound built")
	}
	if out.DomainResolver != serverPinDNSTag {
		t.Fatalf("proxy outbound domain_resolver = %q, want %q", out.DomainResolver, serverPinDNSTag)
	}
	if !dnsServerExists(cfg.DNS, serverPinDNSTag) {
		t.Fatalf("domain_resolver names %q but no such DNS server is registered", serverPinDNSTag)
	}
}

// Without a pin there is no hosts record to point at, and the node must not be
// resolved through the tunnel it is supposed to open. That leaves the system
// resolver — the same server the DNS rule falls back to.
func TestUnpinnedDomainNodeFallsBackToTheSystemResolver(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{IP: "node.example.com", Port: 443, Type: "vless"},
	})

	out, _ := outboundByTag(cfg, "proxy")
	if out.DomainResolver != "local" {
		t.Fatalf("proxy outbound domain_resolver = %q, want local", out.DomainResolver)
	}
}

// A literal address needs no resolver, and the core builds no resolve dialer for
// it at all — naming one would be a claim the config does not need to make.
func TestLiteralIPNodeNamesNoDomainResolver(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{IP: "203.0.113.7", Port: 443, Type: "vless"},
	})

	out, _ := outboundByTag(cfg, "proxy")
	if out.DomainResolver != "" {
		t.Fatalf("a literal-IP node named a domain_resolver: %q", out.DomainResolver)
	}
}

// The WireGuard peer is the same dial by another name: its address is the node's
// address, and it has to survive the same way.
func TestWireGuardDomainPeerResolvesThroughTheServerPin(t *testing.T) {
	extra, err := json.Marshal(map[string]interface{}{
		"private_key": "aFq6pI5MPZBFPTGl0vPYCwxJZLbJbrjeJmC0VCFmIWM=",
		"public_key":  "uV0rXJfPMnZcG2Rr8hYqTxLpNkVqZxJmQbWcEtYuIoA=",
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{IP: "wg.example.com", Port: 51820, Type: "WIREGUARD", Extra: extra, ResolvedIPs: []string{"203.0.113.7"}},
	})

	if len(cfg.Endpoints) == 0 {
		t.Fatal("no endpoint built")
	}
	if got := cfg.Endpoints[0].DomainResolver; got != serverPinDNSTag {
		t.Fatalf("endpoint domain_resolver = %q, want %q", got, serverPinDNSTag)
	}
}

// Proxy mode has no TUN, so nothing redirects the system resolver and there is
// no hosts record either. Naming "local" there would move the node's lookup off
// the encrypted resolver it uses today and hand the node's domain to the ISP —
// so the dial field has to name what the core would have picked on its own.
func TestProxyModeDomainNodeKeepsTheEncryptedResolver(t *testing.T) {
	cfg := mustBuildProxyModeConfig(t, EngineConfig{
		Mode:      ProxyModeProxy,
		LocalPort: 24098,
		Proxy:     ProxyConfig{IP: "node.example.com", Port: 443, Type: "vless"},
	})

	out, _ := outboundByTag(cfg, "proxy")
	want := cfg.DNS.Servers[0].Tag
	if out.DomainResolver != want {
		t.Fatalf("proxy outbound domain_resolver = %q, want the first registered transport %q", out.DomainResolver, want)
	}
	if out.DomainResolver == "local" {
		t.Fatal("proxy mode must not push the node's own domain onto the plaintext system resolver")
	}
}

// A custom DNS server given as a hostname needs a resolver of its own, and the
// only one that cannot be circular is the system resolver.
func TestCustomDNSNamedByDomainCarriesABootstrapResolver(t *testing.T) {
	cfg := mustBuildProxyModeConfig(t, EngineConfig{
		Mode:       ProxyModeProxy,
		LocalPort:  24098,
		Proxy:      ProxyConfig{IP: "203.0.113.7", Port: 443, Type: "vless"},
		DNSServers: []string{"dns.example.com"},
	})

	srv, ok := dnsServerByTag(cfg.DNS, "custom-1")
	if !ok {
		t.Fatal("no custom DNS server built")
	}
	if srv.DomainResolver != "local" {
		t.Fatalf("custom DNS server domain_resolver = %q, want local", srv.DomainResolver)
	}
}

func TestCustomDNSNamedByIPNeedsNoBootstrapResolver(t *testing.T) {
	cfg := mustBuildProxyModeConfig(t, EngineConfig{
		Mode:       ProxyModeProxy,
		LocalPort:  24098,
		Proxy:      ProxyConfig{IP: "203.0.113.7", Port: 443, Type: "vless"},
		DNSServers: []string{"9.9.9.9"},
	})

	srv, _ := dnsServerByTag(cfg.DNS, "custom-1")
	if srv.DomainResolver != "" {
		t.Fatalf("an IP-addressed DNS server named a domain_resolver: %q", srv.DomainResolver)
	}
}

// The failure this guards is not a warning: sing-box 1.14 refuses to build the
// dialer for a DNS server addressed by a domain with no resolver named
// ("missing domain resolver for domain server address", common/dialer/
// dialer.go), and a refused dialer is an engine that never starts. Unmarshalling
// the config is not enough to catch it — the dialers are built by box.New.
func TestCoreBuildsDialersForACustomDNSServerNamedByDomain(t *testing.T) {
	cfg := mustBuildProxyModeConfig(t, EngineConfig{
		Mode:       ProxyModeProxy,
		LocalPort:  24098,
		Proxy:      ProxyConfig{IP: "node.example.com", Port: 443, Type: "vless"},
		DNSServers: []string{"dns.example.com"},
		DataDir:    t.TempDir(),
	})
	assertCoreBuildsConfig(t, cfg)
}

func outboundByTag(cfg SingBoxConfig, tag string) (SBOutbound, bool) {
	for _, out := range cfg.Outbounds {
		if out.Tag == tag {
			return out, true
		}
	}
	return SBOutbound{}, false
}

func dnsServerByTag(dns *SBDNS, tag string) (SBDNSServer, bool) {
	if dns == nil {
		return SBDNSServer{}, false
	}
	for _, srv := range dns.Servers {
		if srv.Tag == tag {
			return srv, true
		}
	}
	return SBDNSServer{}, false
}

func dnsServerExists(dns *SBDNS, tag string) bool {
	_, ok := dnsServerByTag(dns, tag)
	return ok
}

// assertCoreBuildsConfig goes one step further than assertCoreAcceptsConfig: it
// constructs the instance, which is where dialers, DNS transports and outbounds
// are actually built. Nothing is started, so no route and no adapter is touched.
func assertCoreBuildsConfig(t *testing.T, cfg SingBoxConfig) {
	t.Helper()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := coreBuildError(t, raw); err != nil {
		t.Fatalf("pinned core could not build the instance: %v\nconfig: %s", err, raw)
	}
}

// coreBuildError builds the instance and returns whatever the core said about
// it. An unmarshal failure is fatal rather than returned: that means the test's
// own config is malformed, which is never the thing under test.
func coreBuildError(t *testing.T, raw []byte) error {
	t.Helper()
	boxCtx := extendedBoxContext(context.Background())
	var opts option.Options
	if err := singjson.UnmarshalContext(boxCtx, raw, &opts); err != nil {
		t.Fatalf("pinned core rejected the config: %v\nconfig: %s", err, raw)
	}
	instance, err := box.New(box.Options{Context: boxCtx, Options: opts})
	if err != nil {
		return err
	}
	_ = instance.Close()
	return nil
}
