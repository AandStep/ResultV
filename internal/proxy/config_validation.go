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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const (
	ConnectErrorInvalidConfig = "invalid_config"
	ConnectErrorTunPrivileges = "tun_privileges"
	ConnectErrorEngineStart   = "engine_start_failed"
)

func validateEngineConfig(cfg EngineConfig) (string, error) {
	sb := BuildProxyModeConfig(cfg)
	if cfg.Mode == ProxyModeTunnel {
		sb = BuildTunnelModeConfig(cfg)
	}

	if err := validateRouteFinalTarget(sb); err != nil {
		return ConnectErrorInvalidConfig, err
	}
	if err := validateDNSConfig(cfg, sb); err != nil {
		return ConnectErrorInvalidConfig, err
	}
	if err := validateProtocolRequiredFields(cfg.Proxy); err != nil {
		return ConnectErrorInvalidConfig, err
	}
	if err := validateAmneziaOptions(parseExtra(cfg.Proxy)); err != nil {
		return ConnectErrorInvalidConfig, err
	}
	return "", nil
}

// awgHeaderCipherNonceSize mirrors device.HeaderCipherNonceSize in the
// wireguard-go fork. The header cipher uses the S1-S4 crypto padding as its
// nonce, so every padding must be at least this large once a header protection
// key is in play, or mergeWithDevice refuses the device outright.
const awgHeaderCipherNonceSize = 12

// awgKeySize is device.HeaderCipherKeySize: the header protection key must
// base64-decode to exactly this many bytes.
const awgKeySize = 32

// validateAmneziaOptions checks the AmneziaWG knobs before the engine sees
// them. Without it the failures surface as an opaque "setup wireguard" error,
// which tells the user nothing about which knob was wrong.
//
// Ported from the dev branch so both products reject the same configs with the
// same wording; keep the messages in sync when either side changes.
func validateAmneziaOptions(extra map[string]interface{}) error {
	m := amneziaMapFromExtra(extra)
	if m == nil {
		return nil
	}

	// sing-box-extended writes I1-I5 and the AWG 3.0 knobs into ipcConf
	// verbatim, and ipcConf is newline-separated "key=value". A subscription
	// that smuggles a line break through one of these slots would get to set
	// arbitrary WireGuard device keys — including ones we deliberately refuse
	// to emit, like j1 — so reject them outright. Clipping the length does not
	// help: the injection needs only a few bytes.
	slots := append([]string{"i1", "i2", "i3", "i4", "i5"}, awg3Keys...)
	for _, slot := range slots {
		if strings.ContainsAny(stringFromExtraValue(m[slot]), "\r\n") {
			return fmt.Errorf("amneziawg %s must not contain line breaks", slot)
		}
	}

	if err := validateAWGHeaderRanges(m); err != nil {
		return err
	}
	if err := validateAWG31Switches(m); err != nil {
		return err
	}

	for _, name := range awg3Keys {
		value := strings.TrimSpace(stringFromExtraValue(m[name]))
		if value == "" || value == "0" {
			continue
		}
		if name == "header_protection_key" {
			if err := validateAWGHeaderProtectionKey(value); err != nil {
				return err
			}
			continue
		}
		if err := validateAWGRange(value); err != nil {
			return fmt.Errorf("amneziawg %s: %w", name, err)
		}
	}

	if strings.TrimSpace(stringFromExtraValue(m["header_protection_key"])) == "" {
		return nil
	}
	for _, p := range []struct {
		name  string
		value int
	}{{"s1", intFromAny(m["s1"])}, {"s2", intFromAny(m["s2"])},
		{"s3", intFromAny(m["s3"])}, {"s4", intFromAny(m["s4"])}} {
		if p.value < awgHeaderCipherNonceSize {
			return fmt.Errorf(
				"amneziawg header_protection_key requires %s to be at least %d, got %d",
				p.name, awgHeaderCipherNonceSize, p.value)
		}
	}
	return nil
}

// validateAWGHeaderProtectionKey mirrors what sing-box-extended does with the
// value: base64-decode it and hand the bytes to wireguard-go, which requires
// exactly HeaderCipherKeySize of them.
//
// base64 only, deliberately. A 64-character hex key is also valid base64, so
// accepting hex anywhere means a key that passes one check and fails the other
// — which is exactly the bug this replaced.
func validateAWGHeaderProtectionKey(value string) error {
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return fmt.Errorf("amneziawg header_protection_key is not valid base64: %w", err)
	}
	if len(raw) != awgKeySize {
		return fmt.Errorf("amneziawg header_protection_key must decode to %d bytes, got %d", awgKeySize, len(raw))
	}
	return nil
}

// validateAWGRange accepts the "a" and "a-b" forms wireguard-go's
// UintRange.FromString parses.
func validateAWGRange(value string) error {
	parse := func(part string) (uint64, error) {
		part = strings.TrimSpace(part)
		if part == "" {
			return 0, fmt.Errorf("empty bound in %q", value)
		}
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return 0, fmt.Errorf("invalid value %q", value)
		}
		return n, nil
	}
	low, high, isRange := strings.Cut(value, "-")
	lowN, err := parse(low)
	if err != nil {
		return err
	}
	if !isRange {
		return nil
	}
	highN, err := parse(high)
	if err != nil {
		return err
	}
	if lowN > highN {
		return fmt.Errorf("range %q is inverted", value)
	}
	return nil
}

func validateRouteFinalTarget(cfg SingBoxConfig) error {
	if cfg.Route == nil {
		return fmt.Errorf("route section is missing")
	}
	final := strings.TrimSpace(cfg.Route.Final)
	if final == "" {
		return fmt.Errorf("route final target is empty")
	}
	for _, o := range cfg.Outbounds {
		if strings.EqualFold(strings.TrimSpace(o.Tag), final) {
			return nil
		}
	}
	for _, e := range cfg.Endpoints {
		if strings.EqualFold(strings.TrimSpace(e.Tag), final) {
			return nil
		}
	}
	return fmt.Errorf("route final target %q is not defined in outbounds or endpoints", final)
}

func validateDNSConfig(engineCfg EngineConfig, cfg SingBoxConfig) error {
	if engineCfg.Mode != ProxyModeTunnel {
		return nil
	}
	if cfg.DNS == nil || len(cfg.DNS.Servers) == 0 {
		return fmt.Errorf("dns servers are not configured for tunnel mode")
	}
	hasHijack := false
	if cfg.Route == nil {
		return fmt.Errorf("route section is missing")
	}
	for _, r := range cfg.Route.Rules {
		if r.Action == "hijack-dns" {
			hasHijack = true
			break
		}
	}
	if !hasHijack {
		return fmt.Errorf("tunnel mode requires hijack-dns route rule")
	}
	return nil
}

func validateProtocolRequiredFields(proxyCfg ProxyConfig) error {
	pt := strings.ToUpper(strings.TrimSpace(proxyCfg.Type))
	extra := parseExtra(proxyCfg)
	switch pt {
	case "WIREGUARD", "AMNEZIAWG":
		if strings.TrimSpace(getStringField(extra, "private_key", getStringField(extra, "privateKey", ""))) == "" {
			return fmt.Errorf("%s requires private_key", strings.ToLower(pt))
		}
		if strings.TrimSpace(getStringField(extra, "public_key", getStringField(extra, "publicKey", ""))) == "" {
			return fmt.Errorf("%s requires public_key", strings.ToLower(pt))
		}
		if len(stringListFromExtra(extra, "address", "local_address", "localAddress")) == 0 {
			return fmt.Errorf("%s requires address", strings.ToLower(pt))
		}
		if len(stringListFromExtra(extra, "allowed_ips", "allowedIps")) == 0 {
			return fmt.Errorf("%s requires allowed_ips", strings.ToLower(pt))
		}
	case "HYSTERIA2":
		if strings.TrimSpace(getStringField(extra, "password", strings.TrimSpace(proxyCfg.Password))) == "" {
			return fmt.Errorf("hysteria2 requires password")
		}
	case "NAIVEPROXY", "NAIVE":
		if strings.TrimSpace(proxyCfg.IP) == "" || proxyCfg.Port <= 0 {
			return fmt.Errorf("naiveproxy requires host and port")
		}
		if strings.TrimSpace(proxyCfg.Username) == "" || strings.TrimSpace(proxyCfg.Password) == "" {
			return fmt.Errorf("naiveproxy requires username and password")
		}
		if getBoolField(extra, "insecure") {
			return fmt.Errorf("naiveproxy: sing-box naive outbound does not support insecure TLS; use a publicly trusted certificate or remove insecure")
		}
	}
	if proxyCfg.Extra != nil && len(proxyCfg.Extra) > 0 {
		var js map[string]interface{}
		if err := json.Unmarshal(proxyCfg.Extra, &js); err != nil {
			return fmt.Errorf("invalid extra json: %w", err)
		}
	}
	return nil
}

// awgDefaultHeaders are the packet-type values WireGuard uses when H1-H4 say
// nothing: initiation 1, response 2, cookie 3, transport 4.
var awgDefaultHeaders = [4]uint64{1, 2, 3, 4}

// validateAWGHeaderRanges rejects H1-H4 that overlap.
//
// The engine checks this itself, inside IpcSet (wireguard-go device/uapi.go,
// mergeWithDevice → "headers must not overlap"), and a failure there aborts the
// whole connect with the entire ipcConf as its message. The same verdict here
// names the two headers that collide.
//
// Unset slots are checked at their defaults rather than skipped: a config that
// sets only H1 = 3 collides with the default H3, and the engine would see the
// collision even though the config never mentions H3.
func validateAWGHeaderRanges(m map[string]interface{}) error {
	type headerRange struct {
		name      string
		low, high uint64
	}
	ranges := make([]headerRange, 0, 4)
	for i, name := range []string{"h1", "h2", "h3", "h4"} {
		raw := amneziaHeaderString(m[name])
		if raw == "" {
			ranges = append(ranges, headerRange{name: name, low: awgDefaultHeaders[i], high: awgDefaultHeaders[i]})
			continue
		}
		if err := validateAWGRange(raw); err != nil {
			return fmt.Errorf("amneziawg %s: %w", name, err)
		}
		lowRaw, highRaw, isRange := strings.Cut(raw, "-")
		low, err := strconv.ParseUint(strings.TrimSpace(lowRaw), 10, 32)
		if err != nil {
			return fmt.Errorf("amneziawg %s: invalid value %q", name, raw)
		}
		high := low
		if isRange {
			high, err = strconv.ParseUint(strings.TrimSpace(highRaw), 10, 32)
			if err != nil {
				return fmt.Errorf("amneziawg %s: invalid value %q", name, raw)
			}
		}
		ranges = append(ranges, headerRange{name: name, low: low, high: high})
	}
	for i := 0; i < len(ranges); i++ {
		for j := i + 1; j < len(ranges); j++ {
			if ranges[i].low <= ranges[j].high && ranges[j].low <= ranges[i].high {
				return fmt.Errorf("amneziawg %s and %s overlap: the engine rejects overlapping packet-type headers",
					ranges[i].name, ranges[j].name)
			}
		}
	}
	return nil
}

// validateAWG31Switches rejects a stated-but-unreadable random_trailers or
// disable_cookies.
//
// Silence would be worse than a refusal here. random_trailers has to match the
// peer — with it off against a peer that has it on, every handshake the peer
// sends is dropped for being the wrong size, and the session simply never comes
// up, with nothing in the log to say why. A config that means to set it must
// either be understood or rejected out loud.
func validateAWG31Switches(m map[string]interface{}) error {
	for _, name := range awg31Keys {
		for rawKey, rawVal := range m {
			if normalizeAWGKey(rawKey) != normalizeAWGKey(name) {
				continue
			}
			if rawVal == nil {
				continue
			}
			if awgBoolFromAny(rawVal) == nil {
				return fmt.Errorf("amneziawg %s: unreadable value %v, expected on/off", name, rawVal)
			}
		}
	}
	return nil
}
