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
	"os"
	"strings"
	"testing"

	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
)

// TestDumpLinks is a diagnostic, not a test: it takes real node links and prints
// what the engine would actually receive for each one, so a parameter that gets
// lost between the link and the core is visible without connecting to anything.
//
// It never starts the engine and never touches the network — it stops at the
// exact step startInstance does before box.New(), which is also the step that
// decides whether the engine would start at all.
//
// Usage (PowerShell):
//
//	$env:RESULTV_LINKS = "C:\path\to\links.txt"   # one link per line, # comments allowed
//	go test ./internal/proxy -run TestDumpLinks -v
//
// A single link can be passed inline instead of a file path.
//
// For each link it prints:
//   - EXTRA:    the parsed extra map — what the parser salvaged from the link
//   - OUTBOUND: the outbound JSON handed to the core — what actually arrives
//   - CORE:     whether the pinned core accepts that config, or the exact error
//     it would fail the whole engine start with
//
// A field present in EXTRA but missing from OUTBOUND means the builder drops it.
// A field missing from both means the parser drops it. CORE=rejected means this
// link alone would prevent every node from connecting.
func TestDumpLinks(t *testing.T) {
	raw := strings.TrimSpace(os.Getenv("RESULTV_LINKS"))
	if raw == "" {
		t.Skip("set RESULTV_LINKS to a file path or a link to dump what the engine receives")
	}
	if data, err := os.ReadFile(raw); err == nil {
		raw = string(data)
	}

	var links []string
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		links = append(links, line)
	}
	if len(links) == 0 {
		t.Fatal("RESULTV_LINKS contained no links")
	}

	for i, link := range links {
		entry, err := ParseProxyURI(link)
		if err != nil {
			t.Errorf("[%d] PARSE FAILED: %v", i+1, err)
			continue
		}

		t.Logf("[%d] %s %s:%d (%s)", i+1, entry.Type, entry.IP, entry.Port, entry.Name)
		t.Logf("[%d] EXTRA: %s", i+1, indentJSON(entry.Extra))

		proxy := ProxyConfig{
			Type: entry.Type, IP: entry.IP, Port: entry.Port,
			Username: entry.Username, Password: entry.Password,
			URI: entry.URI, Extra: entry.Extra,
		}
		out := buildProxyOutbound(proxy)
		outJSON, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			t.Errorf("[%d] marshaling outbound: %v", i+1, err)
			continue
		}
		t.Logf("[%d] OUTBOUND: %s", i+1, outJSON)

		// A domain server needs a pinned IP to build in tunnel mode; the value is
		// irrelevant here since nothing dials, it only has to exist.
		proxy.ResolvedIPs = []string{"203.0.113.7"}
		cfg, err := BuildTunnelModeConfig(EngineConfig{Proxy: proxy, Mode: ProxyModeTunnel})
		if err != nil {
			t.Errorf("[%d] CORE: config could not be built: %v", i+1, err)
			continue
		}
		full, err := json.Marshal(cfg)
		if err != nil {
			t.Errorf("[%d] marshaling config: %v", i+1, err)
			continue
		}
		var opts option.Options
		if err := singjson.UnmarshalContext(include.Context(context.Background()), full, &opts); err != nil {
			t.Errorf("[%d] CORE: REJECTED — this link alone would stop the engine from starting: %v", i+1, err)
			continue
		}
		t.Logf("[%d] CORE: accepted", i+1)
	}
}

func indentJSON(raw []byte) string {
	if len(raw) == 0 {
		return "{}"
	}
	var pretty map[string]interface{}
	if err := json.Unmarshal(raw, &pretty); err != nil {
		return string(raw)
	}
	out, err := json.MarshalIndent(pretty, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(out)
}
