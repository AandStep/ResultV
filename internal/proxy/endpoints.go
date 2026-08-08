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
	"strconv"
	"strings"
)

const (
	DefaultWireGuardAddress   = "10.0.0.2/32"
	DefaultWireGuardAllowedIP = "0.0.0.0/0"
)



func buildEndpoints(proxy ProxyConfig) []SBEndpoint {
	pt := strings.ToUpper(strings.TrimSpace(proxy.Type))
	if pt != "WIREGUARD" && pt != "AMNEZIAWG" {
		return nil
	}
	extra := parseExtra(proxy)

	address := stringListFromExtra(extra, "address", "local_address", "localAddress")
	if len(address) == 0 {
		address = []string{DefaultWireGuardAddress}
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

	return []SBEndpoint{ep}
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

// amneziaMapFromExtra pulls the "amnezia" sub-object out of an entry's Extra,
// accepting both an already-decoded map and a raw JSON blob.
func amneziaMapFromExtra(extra map[string]interface{}) map[string]interface{} {
	v, ok := extra["amnezia"]
	if !ok || v == nil {
		return nil
	}
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	if raw, ok := v.(json.RawMessage); ok && len(raw) > 0 {
		var mm map[string]interface{}
		if json.Unmarshal(raw, &mm) == nil {
			return mm
		}
	}
	return nil
}

func amneziaFromExtra(extra map[string]interface{}) *SBWireGuardAmnezia {
	m := amneziaMapFromExtra(extra)
	if m == nil {
		return nil
	}
	am := &SBWireGuardAmnezia{
		JC: intFromAny(m["jc"]),
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

		HeaderProtectionKey:    amneziaHeaderProtectionKey(m),
		ContentPaddingAddition: amneziaRangeString(m["content_padding_addition"]),
		RekeyAfterTime:         amneziaRangeString(m["rekey_after_time"]),
		RekeyTimeout:           amneziaRangeString(m["rekey_timeout"]),
		RejectAfterTime:        amneziaRangeString(m["reject_after_time"]),
		KeepaliveTimeout:       amneziaRangeString(m["keepalive_timeout"]),
		MaxHandshakeAttempts:   amneziaRangeString(m["max_handshake_attempts"]),
	}
	// J1-J3 / ITime are simply never read into the struct: there is nowhere to
	// put them since 2.6.1 dropped the option fields. unsupportedAmneziaKnobs
	// still reports them from the raw extra so the user learns they were lost.
	if *am == (SBWireGuardAmnezia{}) {
		return nil
	}
	normalizeAmnezia(am)
	return am
}

// headerProtectionUsable reports whether an amnezia block may carry
// header_protection_key — the key is present and every S1-S4 padding is large
// enough to serve as its nonce (see awgHeaderCipherNonceSize).
//
// The config path does not use this: validateAmneziaOptions rejects the bad
// combination outright, with a message naming the offending padding. Silently
// dropping the key there would trade a clear error for a mysterious handshake
// timeout, since a server demanding header protection will not answer an
// unprotected peer anyway.
//
// The probe path does use it, because PingEntry is reached straight from the UI
// and never passes through validation. There the question is not "is this
// config acceptable" but "can this probe produce a meaningful answer" — and a
// device that refuses to start would just report probe_error.
func headerProtectionUsable(m map[string]interface{}) bool {
	if strings.TrimSpace(stringFromExtraValue(m["header_protection_key"])) == "" {
		return false
	}
	for _, k := range []string{"s1", "s2", "s3", "s4"} {
		if intFromAny(m[k]) < awgHeaderCipherNonceSize {
			return false
		}
	}
	return true
}

// amneziaHeaderProtectionKey returns the base64 header-protection key as-is.
// The core base64-decodes it itself, so it must not be pre-converted to hex
// here — unlike the probe's UAPI path, which hands wireguard-go hex directly.
func amneziaHeaderProtectionKey(m map[string]interface{}) string {
	return strings.TrimSpace(stringFromExtraValue(m["header_protection_key"]))
}

// amneziaRangeString renders an AmneziaWG 3.0 range knob the way upstream's
// *badoption.Range[uint32] expects it. Its UnmarshalJSON takes a bare number,
// "N", or "low-high", so a string is always safe — and a literal 0 means
// "unset" to AmneziaWG, which must stay out of the config entirely.
func amneziaRangeString(v interface{}) string {
	s := strings.TrimSpace(stringFromExtraValue(v))
	if s == "" || s == "0" {
		return ""
	}
	return s
}

// awg3DeviceKnobs are the AmneziaWG 3.0 device-level knobs, in the order the
// user-facing ordering, and the order they are written to the IPC string.
//
// Supported end to end since sing-box-extended v1.13.16-extended-2.6.1: the
// fork (wireguard-go extended-1.5.0) implements every one of these, and 2.6.1
// finally widened option.WireGuardAmnezia and taught
// transport/wireguard/endpoint.go to write them. Before 2.6.1 the fork had them
// but the core could not express them, so they reached the handshake probe —
// which builds its own UAPI — but never the tunnel.
var awg3DeviceKnobs = []string{
	"header_protection_key",
	"content_padding_addition",
	"rekey_after_time",
	"rekey_timeout",
	"reject_after_time",
	"keepalive_timeout",
	"max_handshake_attempts",
}

// unsupportedAmneziaKnobs lists, in a stable order, the AmneziaWG knobs present
// in extra that the engine cannot honour. Pure — intended for user-facing
// warnings. Returns nil when there is nothing to report.
//
// Only the junk-packet knobs remain: no wireguard-go fork ever implemented
// j1-j3 / itime, and 2.6.1 removed the matching option fields from the core, so
// they now have nowhere to go on either side. Emitting them would be actively
// harmful — the root decoder runs with DisallowUnknownFields, so one unknown
// key fails the whole config rather than a single endpoint.
//
// Dropping them costs DPI resistance, not connectivity: junk packets are
// outbound decoration, unlike H1-H4 / S1-S4 which the peer needs to classify
// and de-pad packets. The warning exists so a config that genuinely depends on
// them is visible rather than silently degraded.
func unsupportedAmneziaKnobs(extra map[string]interface{}) []string {
	m := amneziaMapFromExtra(extra)
	if m == nil {
		return nil
	}
	var out []string
	for _, k := range []string{"j1", "j2", "j3"} {
		if stringFromExtraValue(m[k]) != "" {
			out = append(out, k)
		}
	}
	if intFromAny(m["itime"]) > 0 {
		out = append(out, "itime")
	}
	// header_protection_key is deliberately absent: an unusable one is a hard
	// error from validateAmneziaOptions now, not a knob we quietly drop, so
	// reporting it here would describe a connection that never happened.
	return out
}

// UnsupportedAmneziaKnobs reports the AmneziaWG knobs in a marshaled
// config.ProxyEntry's Extra that the engine will drop, as a comma-separated
// list ("j1,itime"). Empty when there is nothing to warn about, including for
// non-AmneziaWG entries.
func UnsupportedAmneziaKnobs(entryJSON string) string {
	var entry struct {
		Type  string          `json:"type"`
		Extra json.RawMessage `json:"extra"`
	}
	if json.Unmarshal([]byte(entryJSON), &entry) != nil {
		return ""
	}
	if strings.ToUpper(strings.TrimSpace(entry.Type)) != "AMNEZIAWG" {
		return ""
	}
	var extra map[string]interface{}
	if len(entry.Extra) == 0 || json.Unmarshal(entry.Extra, &extra) != nil {
		return ""
	}
	return strings.Join(unsupportedAmneziaKnobs(extra), ",")
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

// wireguard-go extended-1.5.0 tightened the UAPI parsers: jc/jmin/jmax now go
// through ParseUint(_, 10, 32) and s1-s4 through ParseUint(_, 10, 16), where
// -1.4.3 used Atoi and only rejected non-positive values. A knob past those
// bounds no longer degrades — it fails the whole IpcSet and the endpoint never
// starts, so clamp instead of forwarding a value we know will be rejected.
const (
	maxAmneziaPadding = 1<<16 - 1
	maxAmneziaJunk    = 1<<32 - 1
)

// clampKnob folds a knob into [0, max]. Takes int64 so the uint32 ceiling is
// representable on 32-bit builds (armeabi-v7a), where int is 32 bits wide.
func clampKnob(v int, max int64) int {
	if v < 0 {
		return 0
	}
	if int64(v) > max {
		return int(max)
	}
	return v
}

func normalizeAmnezia(am *SBWireGuardAmnezia) {
	if am == nil {
		return
	}
	am.JC = clampKnob(am.JC, maxAmneziaJunk)
	am.JMin = clampKnob(am.JMin, maxAmneziaJunk)
	am.JMax = clampKnob(am.JMax, maxAmneziaJunk)
	if am.JMin > 0 && am.JMax > 0 && am.JMin > am.JMax {
		am.JMin, am.JMax = am.JMax, am.JMin
	}
	am.S1 = clampKnob(am.S1, maxAmneziaPadding)
	am.S2 = clampKnob(am.S2, maxAmneziaPadding)
	am.S3 = clampKnob(am.S3, maxAmneziaPadding)
	am.S4 = clampKnob(am.S4, maxAmneziaPadding)
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
