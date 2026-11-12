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
	"strings"
)

// AmneziaWG 3.1 adds two device-wide switches on top of 3.0:
//
//   - random_trailers — appends a random-length tail of random bytes to the
//     handshake initiation, the response and the cookie reply. It is symmetric:
//     the receiver only accepts an oversized packet when the switch is on, so a
//     peer that sends tails to a client without it has its handshakes dropped
//     for being the wrong size (wireguard-go device/receive.go).
//   - disable_cookies — stops the device from answering with cookie replies,
//     the message a loaded server uses to make an initiator prove its address.
const (
	awg31RandomTrailersKey = "random_trailers"
	awg31DisableCookiesKey = "disable_cookies"
)

// awg31Keys is the pair in the order the URI parsers carry them across.
var awg31Keys = []string{awg31RandomTrailersKey, awg31DisableCookiesKey}

// awg31Knobs carries the two switches. Nil means "not stated by the config".
type awg31Knobs struct {
	RandomTrailers *bool
	DisableCookies *bool
}

func (k awg31Knobs) empty() bool {
	return k.RandomTrailers == nil && k.DisableCookies == nil
}

func (k awg31Knobs) anyOn() bool {
	return isOn(k.RandomTrailers) || isOn(k.DisableCookies)
}

// applyTo writes the switches into the endpoint's amnezia block. The device
// default for both is off, so an unstated switch and a stated "off" are the
// same field.
func (k awg31Knobs) applyTo(am *SBWireGuardAmnezia) {
	am.RandomTrailers = isOn(k.RandomTrailers)
	am.DisableCookies = isOn(k.DisableCookies)
}

func isOn(v *bool) bool { return v != nil && *v }

// awg31FromExtra reads the switches out of a node's amnezia block.
//
// Spellings are folded the same way the 3.0 knobs are (normalizeAWGKey), so
// "RandomTrailers" from an .conf-style provider and "random_trailers" from a
// JSON subscription are one key. Values follow the AmneziaWG config file, which
// writes on/off.
func awg31FromExtra(m map[string]interface{}) awg31Knobs {
	var knobs awg31Knobs
	if len(m) == 0 {
		return knobs
	}
	for rawKey, rawVal := range m {
		switch normalizeAWGKey(rawKey) {
		case normalizeAWGKey(awg31RandomTrailersKey):
			knobs.RandomTrailers = awgBoolFromAny(rawVal)
		case normalizeAWGKey(awg31DisableCookiesKey):
			knobs.DisableCookies = awgBoolFromAny(rawVal)
		}
	}
	return knobs
}

// awgBoolFromAny accepts every spelling seen in the wild for these switches,
// and returns nil for anything it cannot read — an unreadable value must not
// silently become "off", which is a different protocol.
func awgBoolFromAny(v interface{}) *bool {
	switch t := v.(type) {
	case nil:
		return nil
	case bool:
		return &t
	case float64:
		b := t != 0
		return &b
	case int:
		b := t != 0
		return &b
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "on", "true", "1", "yes", "enabled", "enable":
			b := true
			return &b
		case "off", "false", "0", "no", "disabled", "disable":
			b := false
			return &b
		}
	}
	return nil
}

// awg31KnobsFor extracts the switches for a node, or an empty set when the node
// is not WireGuard-shaped or says nothing about them.
func awg31KnobsFor(proxy ProxyConfig) awg31Knobs {
	pt := strings.ToUpper(strings.TrimSpace(proxy.Type))
	if pt != "WIREGUARD" && pt != "AMNEZIAWG" {
		return awg31Knobs{}
	}
	return awg31FromExtra(amneziaBlock(parseExtra(proxy)))
}

// describe renders the stated switches for the user-visible log.
func (k awg31Knobs) describe() string {
	var parts []string
	if k.RandomTrailers != nil {
		parts = append(parts, "случайные хвосты "+awgOnOff(*k.RandomTrailers))
	}
	if k.DisableCookies != nil {
		parts = append(parts, "cookie-ответы "+awgOnOff(!*k.DisableCookies))
	}
	return strings.Join(parts, ", ")
}

func awgOnOff(v bool) string {
	if v {
		return "вкл"
	}
	return "выкл"
}
