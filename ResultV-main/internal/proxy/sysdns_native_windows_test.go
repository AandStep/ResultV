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

//go:build windows

package proxy

import (
	"os"
	"testing"
)

// Host-dependent smoke test for the native (iphlpapi) DNS read path: it confirms
// GetAdaptersAddresses + struct parsing agree with the PowerShell lister and that
// index→GUID conversion succeeds for every listed interface. Read-only, no admin.
// Guarded so CI (no/standard adapters) stays green; run on demand:
//
//	RESULTVPC_DNS_SMOKE=1 go test ./internal/proxy/ -run TestNativeDNSList -v
func TestNativeDNSList_MatchesPowerShell(t *testing.T) {
	if os.Getenv("RESULTVPC_DNS_SMOKE") == "" {
		t.Skip("set RESULTVPC_DNS_SMOKE=1 to run the host-dependent native DNS smoke test")
	}

	native, err := listAdapterDNSNative()
	if err != nil {
		t.Fatalf("listAdapterDNSNative: %v", err)
	}
	ps, err := listAdapterDNSPowerShell()
	if err != nil {
		t.Fatalf("listAdapterDNSPowerShell: %v", err)
	}
	t.Logf("native:     %+v", native)
	t.Logf("powershell: %+v", ps)

	index := func(list []adapterDNS) map[int]map[string]bool {
		m := map[int]map[string]bool{}
		for _, a := range list {
			set := map[string]bool{}
			for _, s := range a.ServerAddresses {
				set[s] = true
			}
			m[a.InterfaceIndex] = set
		}
		return m
	}
	n, p := index(native), index(ps)
	if len(n) != len(p) {
		t.Fatalf("adapter count differs: native=%d powershell=%d", len(n), len(p))
	}
	for i, pset := range p {
		nset, ok := n[i]
		if !ok {
			t.Fatalf("native missing interface %d present in powershell", i)
		}
		for s := range pset {
			if !nset[s] {
				t.Errorf("interface %d: native missing server %s", i, s)
			}
		}
	}
	for i := range p {
		if _, err := interfaceGUIDForIndex(i); err != nil {
			t.Errorf("interfaceGUIDForIndex(%d): %v", i, err)
		}
	}
}
