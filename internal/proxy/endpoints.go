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
	"os"
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

// buildEndpoints builds the WireGuard/AmneziaWG endpoint, if the node is one.
// domainResolver comes from serverDomainResolverTag and answers for a peer
// addressed by a domain.
func buildEndpoints(proxy ProxyConfig, domainResolver string) ([]SBEndpoint, error) {
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
		Type:           "wireguard",
		Tag:            wireguardEndpointTag,
		Detour:         "direct",
		DomainResolver: domainResolver,
		System:         getBoolField(extra, "system"),
		Name:           getStringField(extra, "name", ""),
		MTU:            wireguardMTU(intFromExtra(extra, "mtu", "MTU")),
		Address:        address,
		PrivateKey:     privateKey,
		ListenPort:     intFromExtra(extra, "listen_port", "listenPort"),
		Peers:          []SBWireGuardPeer{peer},
		UDPTimeout:     getStringField(extra, "udp_timeout", ""),
		Workers:        intFromExtra(extra, "workers", "Workers"),
		DisablePauses:  getBoolField(extra, "disable_pauses"),
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

// wireguardMTU returns the endpoint MTU, letting RESULTV_WG_MTU override what
// the node's config asked for.
//
// The override exists because MTU is the one WireGuard parameter whose failure
// mode is invisible from the inside: small packets pass, large ones are dropped
// somewhere on the path, and the tunnel looks alive while carrying nothing.
// Sessions on 14.09.2026 show exactly that shape — every packet in the stack
// counters between 150 and 330 bytes, retransmits starting the moment a speed
// test asks for volume, and the device still exchanging keepalives afterwards.
// Answering "is it the packet size" needs one run at a smaller MTU, and a build
// flag beats hand-editing a subscription node.
//
// Out-of-range values are ignored rather than clamped: 576 is the IPv4 minimum
// any path must carry, and above 1500 the override would create the very
// problem it is meant to test for.
func wireguardMTU(configured int) int {
	raw := strings.TrimSpace(os.Getenv("RESULTV_WG_MTU"))
	if raw == "" {
		return configured
	}
	override, err := strconv.Atoi(raw)
	if err != nil || override < 576 || override > 1500 {
		return configured
	}
	return override
}

// wireguardEndpointTag is the tag a WireGuard/AmneziaWG node is given in the
// engine config. Named because applyAWG31 looks the endpoint up by it after
// start — the two must never drift apart.
const wireguardEndpointTag = "proxy"

// awg3Keys lists the AmneziaWG 3.0 device knobs in the order they are written
// into ipcConf. See appendAWG3Lines for how they reach the engine.
var awg3Keys = []string{
	"header_protection_key",
	"content_padding_addition",
	"rekey_after_time",
	"rekey_timeout",
	"reject_after_time",
	"keepalive_timeout",
	"max_handshake_attempts",
}

// normalizeAWGKey folds the spellings providers use for the same knob:
// "HeaderProtectionKey" (amneziawg .conf style), "header_protection_key"
// (JSON subscriptions) and "header-protection-key" all collapse to one form.
func normalizeAWGKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if r != '_' && r != '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// awg3FromExtra picks the AmneziaWG 3.0 knobs out of the amnezia block,
// keyed by their canonical UAPI name.
func awg3FromExtra(m map[string]interface{}) map[string]string {
	if len(m) == 0 {
		return nil
	}
	canonical := make(map[string]string, len(awg3Keys))
	for _, k := range awg3Keys {
		canonical[normalizeAWGKey(k)] = k
	}
	out := map[string]string{}
	for rawKey, rawVal := range m {
		name, ok := canonical[normalizeAWGKey(rawKey)]
		if !ok {
			continue
		}
		if v := stringFromExtraValue(rawVal); v != "" {
			out[name] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// applyAWG3Knobs copies the AmneziaWG 3.0 knobs onto the amnezia block.
func applyAWG3Knobs(am *SBWireGuardAmnezia, knobs map[string]string) {
	if am == nil || len(knobs) == 0 {
		return
	}
	for name, target := range map[string]*string{
		"header_protection_key":    &am.HeaderProtectionKey,
		"content_padding_addition": &am.ContentPaddingAddition,
		"rekey_after_time":         &am.RekeyAfterTime,
		"rekey_timeout":            &am.RekeyTimeout,
		"reject_after_time":        &am.RejectAfterTime,
		"keepalive_timeout":        &am.KeepaliveTimeout,
		"max_handshake_attempts":   &am.MaxHandshakeAttempts,
	} {
		if v, ok := knobs[name]; ok {
			*target = v
		}
	}
}

// amneziaBlock returns the raw "amnezia" sub-object of a proxy's extra data.
// Both the endpoint builder and the config validator read through it so they
// can never disagree about what the engine will be handed.
func amneziaBlock(extra map[string]interface{}) map[string]interface{} {
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
	return m
}

func amneziaFromExtra(extra map[string]interface{}) *SBWireGuardAmnezia {
	m := amneziaBlock(extra)
	if m == nil {
		return nil
	}
	am := &SBWireGuardAmnezia{
		JC:   intFromAny(m["jc"]),
		JMin: intFromAny(m["jmin"]),
		JMax: intFromAny(m["jmax"]),
		S1:   intFromAny(m["s1"]),
		S2:   intFromAny(m["s2"]),
		S3:   intFromAny(m["s3"]),
		S4:   intFromAny(m["s4"]),
		H1:   amneziaHeaderString(m["h1"]),
		H2:   amneziaHeaderString(m["h2"]),
		H3:   amneziaHeaderString(m["h3"]),
		H4:   amneziaHeaderString(m["h4"]),
		I1:   stringFromExtraValue(m["i1"]),
		I2:   stringFromExtraValue(m["i2"]),
		I3:   stringFromExtraValue(m["i3"]),
		I4:   stringFromExtraValue(m["i4"]),
		I5:   stringFromExtraValue(m["i5"]),
	}
	knobs := awg3FromExtra(m)
	if *am == (SBWireGuardAmnezia{}) && len(knobs) == 0 {
		return nil
	}
	normalizeAmnezia(am)
	applyAWG3Knobs(am, knobs)
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
}
