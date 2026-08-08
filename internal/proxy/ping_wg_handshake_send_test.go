// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sagernet/wireguard-go/conn"
	"github.com/sagernet/wireguard-go/device"

	"resultproxy-wails/internal/config"
)

// txBytes reads the peer's tx_bytes counter out of the device's UAPI snapshot.
// Non-zero means the device actually put a handshake initiation on the wire.
func txBytes(t *testing.T, dev *device.Device) int64 {
	t.Helper()
	snap, err := dev.IpcGet()
	if err != nil {
		t.Fatalf("IpcGet: %v", err)
	}
	for _, line := range strings.Split(snap, "\n") {
		if v, ok := strings.CutPrefix(line, "tx_bytes="); ok {
			n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			return n
		}
	}
	return 0
}

// The probe device starts RoutineReadFromTUN inside NewDevice — before IpcSet
// has added the peer. A stub TUN that surfaces its synthetic packet exactly
// once loses it to allowedips.Lookup (no peer yet, packet dropped), so nothing
// ever triggers a handshake and every probe reports a flat "timeout" without
// having sent a single byte. The stub must keep offering the packet until the
// device is configured.
func TestWGProbeActuallySendsHandshakeInitiation(t *testing.T) {
	entry := config.ProxyEntry{
		IP:   "192.0.2.1", // TEST-NET-1: nothing answers, but the send still counts
		Port: 51820,
		Type: "AMNEZIAWG",
		Extra: json.RawMessage(`{
			"private_key":"aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=",
			"public_key":"WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=",
			"amnezia":{"jc":8,"jmin":64,"jmax":900,"s1":88,"s2":155,"s3":32,"s4":16}
		}`),
	}
	uapi, _, err := buildWGUAPI(entry)
	if err != nil {
		t.Fatalf("buildWGUAPI: %v", err)
	}

	dev := device.NewDevice(
		context.Background(),
		newStubTUN(),
		conn.NewDefaultBind(nil),
		device.NewLogger(device.LogLevelSilent, ""),
		0, 0, false,
	)
	defer dev.Close()

	if err := dev.IpcSet(uapi); err != nil {
		t.Fatalf("IpcSet: %v", err)
	}
	if err := dev.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if txBytes(t, dev) > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("device sent nothing in 3s: the stub TUN's packet was dropped before the peer existed, so no handshake initiation was ever queued")
}
