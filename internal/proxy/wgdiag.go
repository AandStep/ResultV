// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/sagernet/gvisor/pkg/tcpip"
	"github.com/sagernet/sing-box/adapter"
	wgprotocol "github.com/sagernet/sing-box/protocol/wireguard"
)

// The question a WireGuard log cannot answer on its own: when a tunnel goes
// quiet, did our packets leave, and did anything come back?
//
// The core logs handshakes and keepalives, and nothing at all for data — so a
// session that stops carrying traffic while the device still believes it is up
// produces total silence, with the browser hammering the tunnel the whole time.
//
// The device's own counters do answer it. tx_bytes rising with rx_bytes flat
// means our packets go out and nothing returns — a path or peer problem. Both
// flat means the packets never reached the device at all, and the fault is
// above WireGuard, in the stack that feeds it. last_handshake_time_sec says
// whether the device ever tried to re-establish the session.
//
// Sampling is driven from Kotlin and only while the verbose log is on, so it
// costs nothing in an ordinary session.
const (
	wgStatsPrefix = "wg-stats"
	wgStackPrefix = "wg-stack"
)

// wgStatsFields is what gets written down. Deliberately a whitelist: the UAPI
// dump also carries private_key and preshared_key, and this line goes into a
// log the user is asked to hand to someone.
var wgStatsFields = []string{
	"endpoint",
	"last_handshake_time_sec",
	"tx_bytes",
	"rx_bytes",
	"persistent_keepalive_interval",
	"protocol_version",
}

// ipcGetter is the read half of the device's UAPI.
type ipcGetter interface {
	IpcGet() (string, error)
}

// WGDiagLine returns one line with the WireGuard device counters and, when the
// endpoint runs on the gVisor stack, its stack counters after them.
//
// An endpoint on the system stack has no stack to read; that half is dropped
// silently rather than failing the whole line, because the device counters are
// still the answer to the first question.
func WGDiagLine(manager adapter.EndpointManager) (string, error) {
	device, err := wgStatsLine(manager)
	if err != nil {
		return "", err
	}
	line := wgStatsPrefix + " " + device
	if stack, stackErr := wgStackStatsLine(manager); stackErr == nil {
		line += " | " + wgStackPrefix + " " + stack
	}
	return line, nil
}

// wgStatsLine reads the device's UAPI dump and keeps the fields worth having.
func wgStatsLine(manager adapter.EndpointManager) (string, error) {
	endpoint, err := wgEndpointFrom(manager)
	if err != nil {
		return "", err
	}
	deviceValue, err := awg31DeviceInterface(endpoint)
	if err != nil {
		return "", err
	}
	getter, ok := deviceValue.(ipcGetter)
	if !ok {
		return "", &awg31Error{"устройство не отдаёт UAPI"}
	}
	dump, err := getter.IpcGet()
	if err != nil {
		return "", &awg31Error{"UAPI не прочитан: " + err.Error()}
	}
	return filterWGStats(dump), nil
}

// filterWGStats keeps only the whitelisted keys, in one line.
func filterWGStats(dump string) string {
	var kept []string
	for _, line := range strings.Split(dump, "\n") {
		line = strings.TrimSpace(line)
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		for _, want := range wgStatsFields {
			if key == want {
				kept = append(kept, line)
				break
			}
		}
	}
	if len(kept) == 0 {
		return "(пусто)"
	}
	return strings.Join(kept, " ")
}

// The counters above stop at the WireGuard device: they say packets left and
// packets came back, and nothing about what happened to them afterwards. The
// next step is the gVisor stack that sits on top of the device inside the
// endpoint. Received-but-invalid segments, packets dropped for a destination
// the stack does not recognise, failed connection attempts and resets each
// point at a different cause, and they are counted whether or not anything is
// logged.

// statsProvider is the gVisor stack's own accounting.
type statsProvider interface {
	Stats() tcpip.Stats
}

// wgStackStatsLine reads the endpoint's gVisor stack counters.
func wgStackStatsLine(manager adapter.EndpointManager) (string, error) {
	endpoint, err := wgEndpointFrom(manager)
	if err != nil {
		return "", err
	}
	stackValue, err := wgStackFrom(endpoint)
	if err != nil {
		return "", err
	}
	stats := stackValue.Stats()
	fields := []struct {
		name    string
		counter *tcpip.StatCounter
	}{
		{"ip_received", stats.IP.PacketsReceived},
		{"ip_delivered", stats.IP.PacketsDelivered},
		{"ip_sent", stats.IP.PacketsSent},
		{"ip_bad_dst", stats.IP.InvalidDestinationAddressesReceived},
		{"ip_malformed", stats.IP.MalformedPacketsReceived},
		{"ip_out_err", stats.IP.OutgoingPacketErrors},
		{"tcp_opened", stats.TCP.ActiveConnectionOpenings},
		{"tcp_established", stats.TCP.CurrentEstablished},
		{"tcp_failed", stats.TCP.FailedConnectionAttempts},
		{"tcp_valid_in", stats.TCP.ValidSegmentsReceived},
		{"tcp_invalid_in", stats.TCP.InvalidSegmentsReceived},
		{"tcp_sent", stats.TCP.SegmentsSent},
		{"tcp_send_err", stats.TCP.SegmentSendErrors},
		{"tcp_rst_in", stats.TCP.ResetsReceived},
		{"tcp_rst_out", stats.TCP.ResetsSent},
		{"tcp_retransmit", stats.TCP.Retransmits},
		{"tcp_timeout", stats.TCP.EstablishedTimedout},
	}
	var parts []string
	for _, field := range fields {
		if field.counter == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%d", field.name, field.counter.Value()))
	}
	return strings.Join(parts, " "), nil
}

// wgStackFrom walks the endpoint down to the gVisor stack inside its device.
func wgStackFrom(wgEndpoint *wgprotocol.Endpoint) (statsProvider, error) {
	transportEndpoint, err := unexportedField(reflect.ValueOf(wgEndpoint), "endpoint")
	if err != nil {
		return nil, &awg31Error{"protocol/wireguard.Endpoint: " + err.Error()}
	}
	if transportEndpoint.Kind() == reflect.Pointer && transportEndpoint.IsNil() {
		return nil, &awg31Error{"устройство WireGuard ещё не создано"}
	}
	tunDevice, err := unexportedField(transportEndpoint, "tunDevice")
	if err != nil {
		return nil, &awg31Error{"transport/wireguard.Endpoint: " + err.Error()}
	}
	if !tunDevice.IsValid() || tunDevice.IsZero() {
		return nil, &awg31Error{"устройство WireGuard ещё не создано"}
	}
	// tunDevice is an interface holding *stackDevice for a gVisor endpoint, and
	// something else entirely for a system one — where there is no stack to read
	// and nothing to report.
	stackField, err := unexportedField(reflect.ValueOf(tunDevice.Interface()), "stack")
	if err != nil {
		return nil, &awg31Error{"устройство без gVisor-стека: " + err.Error()}
	}
	provider, ok := stackField.Interface().(statsProvider)
	if !ok {
		return nil, &awg31Error{"стек не отдаёт статистику"}
	}
	return provider, nil
}
