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

package proxy

import (
	"strings"
	"testing"
)

func hasDomain(list []string, want string) bool {
	for _, d := range list {
		if d == want {
			return true
		}
	}
	return false
}

// Measured on the user's live Smart config (2026-08-27) by replaying it
// through a real sing-box: aistudio.google.com and its API host matched the
// block-list and went to the proxy, while accounts.google.com, ogs.google.com,
// apis.google.com and myaccount.google.com matched nothing and fell through to
// Final=direct — i.e. Google saw the product request from the exit node and the
// account/session requests from the user's real address. That split is what
// makes an account-gated Google product answer "not available in your country".
// The upstream service lists carry the product hosts only, so the floor has to
// supply the account layer whatever the list source happened to contain.
func TestRouterBlockedDomains_GoogleAccountFloorPresentForEverySource(t *testing.T) {
	sources := map[string][]string{
		"remote-list": {"aistudio.google.com", "youtube.com"},
		"empty-list":  {},
	}
	for name, domains := range sources {
		t.Run(name, func(t *testing.T) {
			r := NewRouter()
			r.SetBlockedDomains(domains)
			got := r.GetBlockedDomains()
			for _, want := range blockedDomainFloor() {
				if !hasDomain(got, want) {
					t.Fatalf("floor entry %q missing from GetBlockedDomains (%d entries)", want, len(got))
				}
				if !r.IsBlockedDomain(want) {
					t.Fatalf("IsBlockedDomain(%q) = false, floor must apply to the router's own decisions too", want)
				}
			}
		})
	}
}

// The floor is a union, never a replacement: a list source's own entries must
// survive it, and the custom "route via VPN" domains must keep working.
func TestRouterBlockedDomains_FloorDoesNotDisplaceSources(t *testing.T) {
	r := NewRouter()
	r.SetBlockedDomains([]string{"aistudio.google.com"})
	r.SetCustomBlockedDomains([]string{"example.org"})
	got := r.GetBlockedDomains()
	for _, want := range []string{"aistudio.google.com", "example.org"} {
		if !hasDomain(got, want) {
			t.Fatalf("%q lost after the floor was unioned in", want)
		}
	}
	seen := make(map[string]int, len(got))
	for _, d := range got {
		seen[d]++
	}
	for d, n := range seen {
		if n > 1 {
			t.Fatalf("duplicate entry %q (%d times) — the floor must dedupe", d, n)
		}
	}
}

// The floor is emitted as a sing-box domain_suffix, which matches sub-domains
// too, so every entry must be a real host under Google's account layer. A bare
// 2LD here would drag unrelated traffic through the tunnel.
func TestBlockedDomainFloor_EntriesAreSpecificHosts(t *testing.T) {
	for _, d := range blockedDomainFloor() {
		if n := strings.Count(d, "."); n < 2 {
			t.Fatalf("floor entry %q has %d dots — too broad for a domain_suffix rule", d, n)
		}
		if d != normalizeRule(d) {
			t.Fatalf("floor entry %q is not in normalized form (%q)", d, normalizeRule(d))
		}
	}
}

// Measured against the live claude.ai login page (2026-09-02): claude.ai,
// claude.com and anthropic.com are all carried by the RU block-list sources and
// leave through the tunnel, while the invisible hCaptcha the same page loads for
// client attestation (js.hcaptcha.com → api.hcaptcha.com) and Cloudflare's
// managed-challenge platform are in no source at all and fell through to
// Final=direct. That is the Google account-layer split again: the product is
// served to the exit node, the check that decides whether the product trusts
// you runs from the user's real address.
func TestRouterBlockedDomains_AttestationFloorCoversAnthropicList(t *testing.T) {
	r := NewRouter()
	// What the RU sources actually ship for Anthropic — product hosts only.
	r.SetBlockedDomains([]string{"claude.ai", "claude.com", "anthropic.com"})
	for _, host := range []string{
		"js.hcaptcha.com",
		"newassets.hcaptcha.com",
		"api.hcaptcha.com",
		"api2.hcaptcha.com",
		"pst-issuer.hcaptcha.com",
		"challenges.cloudflare.com",
	} {
		if !r.IsBlockedDomain(host) {
			t.Fatalf("IsBlockedDomain(%q) = false — the attestation layer would leave from the real address while claude.ai leaves through the tunnel", host)
		}
	}
}
