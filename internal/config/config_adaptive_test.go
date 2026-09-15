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

package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// The adaptive engine ships dark. A config written by an older build has no
// such key, and the zero value must mean "behave exactly as before".
func TestAdaptiveSmartDefaultsOff(t *testing.T) {
	var rules RoutingRules
	if err := json.Unmarshal([]byte(`{"mode":"smart"}`), &rules); err != nil {
		t.Fatal(err)
	}
	if rules.AdaptiveSmart || rules.AdaptiveSmartMemoryOnly || rules.AdaptiveSmartBlockBrowserDoH {
		t.Fatal("a config without the keys must leave every adaptive switch off")
	}
}

func TestAdaptiveSmartRoundTrip(t *testing.T) {
	in := RoutingRules{
		Mode:                         "smart",
		AdaptiveSmart:                true,
		AdaptiveSmartMemoryOnly:      true,
		AdaptiveSmartBlockBrowserDoH: true,
	}
	blob, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out RoutingRules
	if err := json.Unmarshal(blob, &out); err != nil {
		t.Fatal(err)
	}
	if !out.AdaptiveSmart || !out.AdaptiveSmartMemoryOnly || !out.AdaptiveSmartBlockBrowserDoH {
		t.Fatalf("round trip lost a switch: %+v", out)
	}
	for _, key := range []string{`"adaptiveSmart":true`, `"adaptiveSmartMemoryOnly":true`, `"adaptiveSmartBlockBrowserDoH":true`} {
		if !strings.Contains(string(blob), key) {
			t.Errorf("marshalled config lacks %s: %s", key, blob)
		}
	}
}
