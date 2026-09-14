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
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unsafe"

	"github.com/sagernet/sing-box/adapter"
	wgprotocol "github.com/sagernet/sing-box/protocol/wireguard"
	"github.com/sagernet/sing/service"

	"resultproxy-wails/internal/logger"
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
// the 3.0 knobs — so through the config file these two are unreachable, in
// 1.13 and 1.14 alike. They are therefore applied here, after the core has
// built the device, with a second IpcSet.
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

// awg31FromExtra reads the switches out of a node's amnezia block.
//
// Spellings are folded the same way the 3.0 knobs are (normalizeAWGKey), so
// "RandomTrailers" from an .conf-style provider and "random_trailers" from a
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

// ipcSetter is the one method needed off the core's WireGuard device. Declared
// here rather than imported so this file does not pull wireguard-go in for a
// single signature.
type ipcSetter interface {
	IpcSet(string) error
}

// errAWG31Unreachable means the path from the endpoint to its device did not
// look the way it does in the engine version this was written against.
type awg31Error struct{ what string }

func (e *awg31Error) Error() string { return e.what }

// applyAWG31 pushes the 3.1 switches into the running device.
//
// Why this is reached through unexported fields: the core builds the UAPI
// string itself inside transport/wireguard.Endpoint.Start and exposes neither
// the device nor a hook, so the only seam left is the object graph —
// protocol/wireguard.Endpoint.endpoint → transport/wireguard.Endpoint.device.
// Every step is verified by name and type, and a mismatch is reported instead
// of panicking: an engine bump that renames a field must degrade to "3.1 not
// applied" with a line in the log, never to a crash on connect. The compile-
// time-ish guard is TestAWG31DeviceFieldsStillExist, which fails on the next
// engine bump that moves them.
func applyAWG31(boxCtx context.Context, knobs awg31Knobs, log *logger.Logger) error {
	if knobs.empty() {
		return nil
	}
	if boxCtx == nil {
		return &awg31Error{"нет контекста ядра"}
	}
	manager := service.FromContext[adapter.EndpointManager](boxCtx)
	if manager == nil {
		return &awg31Error{"ядро не отдало менеджер эндпоинтов"}
	}
	ep, loaded := manager.Get(wireguardEndpointTag)
	if !loaded {
		return &awg31Error{"эндпоинт " + wireguardEndpointTag + " не найден"}
	}
	wgEndpoint, ok := ep.(*wgprotocol.Endpoint)
	if !ok {
		return &awg31Error{fmt.Sprintf("эндпоинт %s не WireGuard (%T)", wireguardEndpointTag, ep)}
	}
	device, err := awg31Device(wgEndpoint)
	if err != nil {
		return err
	}
	if err := device.IpcSet(knobs.ipcLines()); err != nil {
		return &awg31Error{"ядро отклонило параметры: " + err.Error()}
	}
	if log != nil {
		log.Info("[PROXY] AmneziaWG 3.1: " + knobs.describe() + " применены")
	}
	return nil
}

// describe renders the applied switches for the user-visible log.
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

// awg31Device walks the endpoint down to the wireguard-go device.
func awg31Device(wgEndpoint *wgprotocol.Endpoint) (ipcSetter, error) {
	transportEndpoint, err := unexportedField(reflect.ValueOf(wgEndpoint), "endpoint")
	if err != nil {
		return nil, &awg31Error{"protocol/wireguard.Endpoint: " + err.Error()}
	}
	// Not-yet-built and built-but-deviceless are one state to a caller, and it
	// is a legitimate one rather than a defect: the device is created by
	// transport.Endpoint.Start, which for a domain-addressed peer runs in the
	// post-start stage.
	if transportEndpoint.Kind() == reflect.Pointer && transportEndpoint.IsNil() {
		return nil, &awg31Error{"устройство WireGuard ещё не создано"}
	}
	deviceValue, err := unexportedField(transportEndpoint, "device")
	if err != nil {
		return nil, &awg31Error{"transport/wireguard.Endpoint: " + err.Error()}
	}
	if !deviceValue.IsValid() || deviceValue.IsZero() {
		return nil, &awg31Error{"устройство WireGuard ещё не создано"}
	}
	device, ok := deviceValue.Interface().(ipcSetter)
	if !ok {
		return nil, &awg31Error{fmt.Sprintf("устройство не принимает UAPI (%s)", deviceValue.Type())}
	}
	return device, nil
}

// unexportedField reads an unexported struct field by name.
//
// reflect refuses to hand over unexported values through Interface(), so the
// field is re-created at its own address — the standard escape hatch, and the
// reason every caller here checks the field exists first instead of trusting
// the layout.
func unexportedField(v reflect.Value, name string) (reflect.Value, error) {
	if !v.IsValid() {
		return reflect.Value{}, fmt.Errorf("нет значения для поля %s", name)
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return reflect.Value{}, fmt.Errorf("нулевой указатель вместо структуры с полем %s", name)
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("%s: ожидалась структура, получено %s", name, v.Kind())
	}
	field := v.FieldByName(name)
	if !field.IsValid() {
		return reflect.Value{}, fmt.Errorf("поле %s исчезло из ядра", name)
	}
	if !field.CanAddr() {
		return reflect.Value{}, fmt.Errorf("поле %s недоступно по адресу", name)
	}
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem(), nil
}
