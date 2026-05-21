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
	"net/netip"
	"strconv"
	"strings"
)

const (
	DefaultWireGuardAddress   = "10.0.0.2/32"
	DefaultWireGuardAllowedIP = "0.0.0.0/0"
)

func normalizeWireGuardLocalPrefix(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty")
	}
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return "", err
		}
		return p.String(), nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return "", err
	}
	a = a.Unmap()
	if a.Is4() {
		return netip.PrefixFrom(a, 32).String(), nil
	}
	return netip.PrefixFrom(a, 128).String(), nil
}

func normalizeWireGuardLocalPrefixes(addrs []string) ([]string, error) {
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		n, err := normalizeWireGuardLocalPrefix(a)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", a, err)
		}
		out = append(out, n)
	}
	return out, nil
}

func buildEndpoints(proxy ProxyConfig) ([]SBEndpoint, error) {
	pt := strings.ToUpper(strings.TrimSpace(proxy.Type))
	if pt != "WIREGUARD" && pt != "AMNEZIAWG" {
		return nil, nil
	}
	extra := parseExtra(proxy)

	address := stringListFromExtra(extra, "address", "local_address", "localAddress")
	if len(address) == 0 {
		address = []string{DefaultWireGuardAddress}
	} else {
		var err error
		address, err = normalizeWireGuardLocalPrefixes(address)
		if err != nil {
			return nil, fmt.Errorf("wireguard local address: %w", err)
		}
	}

	privateKey := getStringField(extra, "private_key", "")
	if privateKey == "" {
		privateKey = getStringField(extra, "privateKey", "")
	}
	publicKey := getStringField(extra, "public_key", "")
	if publicKey == "" {
		publicKey = getStringField(extra, "publicKey", "")
	}
	psk := getStringField(extra, "pre_shared_key", "")
	if psk == "" {
		psk = getStringField(extra, "preSharedKey", "")
	}

	peerAllowed := stringListFromExtra(extra, "allowed_ips", "allowedIps")
	if len(peerAllowed) == 0 {
		peerAllowed = []string{DefaultWireGuardAllowedIP}
	}

	peer := SBWireGuardPeer{
		Address:                     proxy.IP,
		Port:                        proxy.Port,
		PublicKey:                   publicKey,
		PreSharedKey:                psk,
		AllowedIPs:                  peerAllowed,
		PersistentKeepaliveInterval: intFromExtra(extra, "persistent_keepalive_interval", "persistentKeepaliveInterval"),
		Reserved:                    intListFromExtra(extra, "reserved"),
	}

	ep := SBEndpoint{
		Type:          "wireguard",
		Tag:           "proxy",
		Detour:        "direct",
		System:        getBoolField(extra, "system"),
		Name:          getStringField(extra, "name", ""),
		MTU:           intFromExtra(extra, "mtu", "MTU"),
		Address:       address,
		PrivateKey:    privateKey,
		ListenPort:    intFromExtra(extra, "listen_port", "listenPort"),
		Peers:         []SBWireGuardPeer{peer},
		UDPTimeout:    getStringField(extra, "udp_timeout", ""),
		Workers:       intFromExtra(extra, "workers", "Workers"),
		DisablePauses: getBoolField(extra, "disable_pauses"),
	}

	if pt == "AMNEZIAWG" {
		am := amneziaFromExtra(extra)
		if am != nil {
			ep.Amnezia = am
		}
	}

	return []SBEndpoint{ep}, nil
}

func stringListFromExtra(extra map[string]interface{}, keys ...string) []string {
	for _, k := range keys {
		if v, ok := extra[k]; ok && v != nil {
			if out := stringListFromAny(v); len(out) > 0 {
				return out
			}
		}
	}
	return nil
}

func stringListFromAny(v interface{}) []string {
	switch t := v.(type) {
	case []string:
		var out []string
		for _, s := range t {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []interface{}:
		var out []string
		for _, it := range t {
			s, ok := it.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' })
		var out []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	default:
		return nil
	}
}

func intListFromExtra(extra map[string]interface{}, key string) []int {
	if extra == nil {
		return nil
	}
	v, ok := extra[key]
	if !ok || v == nil {
		return nil
	}
	switch t := v.(type) {
	case []int:
		return append([]int(nil), t...)
	case []interface{}:
		var out []int
		for _, it := range t {
			n := intFromAny(it)
			out = append(out, n)
		}
		return out
	default:
		return nil
	}
}

func amneziaFromExtra(extra map[string]interface{}) *SBWireGuardAmnezia {
	v, ok := extra["amnezia"]
	if !ok || v == nil {
		return nil
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		if raw, ok := v.(json.RawMessage); ok && len(raw) > 0 {
			var mm map[string]interface{}
			if json.Unmarshal(raw, &mm) == nil {
				m = mm
			}
		}
	}
	if m == nil {
		return nil
	}
	am := &SBWireGuardAmnezia{
		JC:    intFromAny(m["jc"]),
		JMin:  intFromAny(m["jmin"]),
		JMax:  intFromAny(m["jmax"]),
		S1:    intFromAny(m["s1"]),
		S2:    intFromAny(m["s2"]),
		S3:    intFromAny(m["s3"]),
		S4:    intFromAny(m["s4"]),
		H1:    amneziaHeaderString(m["h1"]),
		H2:    amneziaHeaderString(m["h2"]),
		H3:    amneziaHeaderString(m["h3"]),
		H4:    amneziaHeaderString(m["h4"]),
		I1:    stringFromExtraValue(m["i1"]),
		I2:    stringFromExtraValue(m["i2"]),
		I3:    stringFromExtraValue(m["i3"]),
		I4:    stringFromExtraValue(m["i4"]),
		I5:    stringFromExtraValue(m["i5"]),
		J1:    stringFromExtraValue(m["j1"]),
		J2:    stringFromExtraValue(m["j2"]),
		J3:    stringFromExtraValue(m["j3"]),
		ITime: int64(intFromAny(m["itime"])),
	}
	if *am == (SBWireGuardAmnezia{}) {
		return nil
	}
	normalizeAmnezia(am)
	return am
}

// amneziaHeaderString returns the H1-H4 value as a string in the form
// expected by the upstream sing-box-extended *Xbadoption.Range type:
// either "N" (AWG 1.0, fixed magic header) or "low-high" (AWG 2.0,
// header-range syntax). The upstream JSON unmarshaller picks a random
// value within the range per packet on the engine side.
func amneziaHeaderString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	if n := intFromAny(v); n > 0 {
		return strconv.Itoa(n)
	}
	return ""
}

func normalizeAmnezia(am *SBWireGuardAmnezia) {
	if am == nil {
		return
	}
	if am.JC < 0 {
		am.JC = 0
	}
	if am.JMin < 0 {
		am.JMin = 0
	}
	if am.JMax < 0 {
		am.JMax = 0
	}
	if am.JMin > 0 && am.JMax > 0 && am.JMin > am.JMax {
		am.JMin, am.JMax = am.JMax, am.JMin
	}
	if am.S1 < 0 {
		am.S1 = 0
	}
	if am.S2 < 0 {
		am.S2 = 0
	}
	if am.S3 < 0 {
		am.S3 = 0
	}
	if am.S4 < 0 {
		am.S4 = 0
	}
	if am.ITime < 0 {
		am.ITime = 0
	}
	const maxJunkLen = 4096
	clip := func(s string) string {
		if len(s) > maxJunkLen {
			return s[:maxJunkLen]
		}
		return s
	}
	am.I1 = clip(am.I1)
	am.I2 = clip(am.I2)
	am.I3 = clip(am.I3)
	am.I4 = clip(am.I4)
	am.I5 = clip(am.I5)
	am.J1 = clip(am.J1)
	am.J2 = clip(am.J2)
	am.J3 = clip(am.J3)
}
