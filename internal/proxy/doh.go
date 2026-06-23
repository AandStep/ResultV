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
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// dohEndpoints are queried by IP so a censored UDP/53 (or a blackholed OS
// resolver) can't stop us from resolving the VPN server's own domain at connect
// time. Cloudflare's and Google's certs are valid for these literal IPs, so the
// HTTPS handshake succeeds without needing DNS first. Overridable in tests.
var dohEndpoints = []string{
	"https://1.1.1.1/dns-query",
	"https://8.8.8.8/dns-query",
}

const dohQueryTimeout = 1500 * time.Millisecond

// parseDoHJSONAnswer extracts IPv4 (type 1 / A) records from a Cloudflare/Google
// JSON-DoH response, deduped and order-stable. AAAA (28) and CNAME (5) are
// skipped — the tunnel DNS strategy is ipv4_only and the outbound dials v4.
func parseDoHJSONAnswer(body []byte) []string {
	var resp struct {
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}
	seen := make(map[string]struct{}, len(resp.Answer))
	var out []string
	for _, a := range resp.Answer {
		if a.Type != 1 { // A record only
			continue
		}
		if ip := net.ParseIP(a.Data); ip == nil || ip.To4() == nil {
			continue
		}
		if _, dup := seen[a.Data]; dup {
			continue
		}
		seen[a.Data] = struct{}{}
		out = append(out, a.Data)
	}
	return out
}

// queryDoHEndpoint performs one JSON-DoH A-record lookup against endpoint.
// Returns nil on any error so callers can fall through to the next endpoint.
func queryDoHEndpoint(client *http.Client, endpoint, host string) []string {
	req, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%s?name=%s&type=A", endpoint, url.QueryEscape(host)), nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Accept", "application/dns-json")
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil
	}
	return parseDoHJSONAnswer(body)
}

// resolveServerIPsViaDoH resolves host to IPv4 via DoH-over-IP, trying each
// endpoint until one answers. Best-effort: an empty result leaves caller
// behaviour unchanged. Used as a fallback when the OS resolver returns nothing
// for a domain server (censored UDP/53) before the connect path gives up. The
// transport disables any inherited proxy so the lookup goes out the physical
// interface directly (this runs before the system-DNS override / TUN is up).
func resolveServerIPsViaDoH(host string) []string {
	host = strings.TrimSpace(host)
	if host == "" || net.ParseIP(host) != nil {
		return nil
	}
	client := &http.Client{
		Timeout:   dohQueryTimeout,
		Transport: &http.Transport{Proxy: nil},
	}
	for _, ep := range dohEndpoints {
		if ips := queryDoHEndpoint(client, ep, host); len(ips) > 0 {
			return ips
		}
	}
	return nil
}
