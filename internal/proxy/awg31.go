// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"strconv"
	"strings"
)

// AmneziaWG 3.1 adds two device-wide switches on top of 3.0:
//
//   - random_trailers — appends a random-length tail of random bytes to the
//     handshake initiation, the response and the cookie reply. It is symmetric:
//     the receiver only accepts an oversized packet when the switch is on, so a
//     peer that sends tails to a client without it has its handshakes dropped
//     for being the wrong size (wireguard-go device/receive.go). "Same value on
//     both sides" is not advice, it is the protocol.
//   - disable_cookies — stops the device from answering with cookie replies,
//     the message a loaded server uses to make an initiator prove its address.
//     Under probing, silence is a weaker fingerprint than a reply.
//
// The engine's WireGuard device (wireguard-go fork) implements both in its UAPI
// (device/uapi.go). sing-box-extended does not: option.WireGuardAmnezia has no
// field for either, and the ipcConf it builds in transport/wireguard stops at
// the 3.0 knobs — so through the config file these two are unreachable, in 1.13
// and 1.14 alike. They reach the tunnel by a second IpcSet on the running
// device (ApplyAWG31) and the ping probe by its own UAPI string
// (writeAmneziaUAPI).
//
// Deliberately NOT emitted into the sing-box JSON: the core parses its config
// strictly, and an unknown key under "amnezia" fails the whole start rather
// than being ignored.
const (
	awg31RandomTrailersKey = "random_trailers"
	awg31DisableCookiesKey = "disable_cookies"
)

// awg31Keys is the pair in the order the URI parsers carry them across.
var awg31Keys = []string{awg31RandomTrailersKey, awg31DisableCookiesKey}

// awg31Knobs carries the two switches. Nil means "not stated by the config",
// which is not the same as false: a stated "off" is worth sending, because the
// device default can move and a config that says off must keep meaning off.
type awg31Knobs struct {
	RandomTrailers *bool
	DisableCookies *bool
}

func (k awg31Knobs) empty() bool {
	return k.RandomTrailers == nil && k.DisableCookies == nil
}

// ipcLines renders the knobs as UAPI lines. The device's IpcSet takes a partial
// configuration: it seeds itself from the live device and merges, and peers are
// only touched by a "public_key" line — which is why sending just these two is
// safe on a running session (wireguard-go device/uapi.go, IpcSetOperation).
func (k awg31Knobs) ipcLines() string {
	var b strings.Builder
	if k.RandomTrailers != nil {
		b.WriteString(awg31RandomTrailersKey + "=" + strconv.FormatBool(*k.RandomTrailers) + "\n")
	}
	if k.DisableCookies != nil {
		b.WriteString(awg31DisableCookiesKey + "=" + strconv.FormatBool(*k.DisableCookies) + "\n")
	}
	return b.String()
}

// describe renders the applied switches for the log. The UAPI spelling, not a
// translated phrase: this string is handed to Kotlin and printed into a log
// that exists in two languages, where a knob name is the same word in both.
func (k awg31Knobs) describe() string {
	var parts []string
	if k.RandomTrailers != nil {
		parts = append(parts, awg31RandomTrailersKey+"="+awgOnOff(*k.RandomTrailers))
	}
	if k.DisableCookies != nil {
		parts = append(parts, awg31DisableCookiesKey+"="+awgOnOff(*k.DisableCookies))
	}
	return strings.Join(parts, ", ")
}

func awgOnOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

// awg31FromExtra reads the switches out of a node's amnezia block.
//
// Spellings are folded the same way the 3.0 knobs are (normalizeAWGKey), so
// "RandomTrailers" from a .conf-style provider and "random_trailers" from a
// JSON subscription are one key. Values follow the AmneziaWG config file, which
// writes on/off — a spelling Go's ParseBool rejects, and the UAPI is ParseBool.
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
//
// int64 is in the list for a local reason: parseAmneziaWGURI stores numbers
// from a link with strconv.ParseInt, so a value that arrived through a query
// parameter is int64 where the same value from JSON is float64.
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
	case int64:
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
	return awg31FromExtra(amneziaMapFromExtra(parseExtra(proxy)))
}

// awg31Error is the error type for every step on the way to the device. It is
// declared here because ApplyAWG31 and the counters in wgdiag.go both raise it.
type awg31Error struct{ what string }

func (e *awg31Error) Error() string { return e.what }
