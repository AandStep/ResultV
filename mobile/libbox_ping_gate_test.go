// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package mobile

import (
	"encoding/json"
	"testing"
)

// awgEntryFixture points at TEST-NET-1 (RFC 5737) so no probe can reach a real
// host. Carries the AmneziaWG knobs a 2.0 profile would, so the keyed path has
// everything it needs when it is allowed to run.
const awgEntryFixture = `{
  "name":"awg","type":"AMNEZIAWG","ip":"192.0.2.1","port":51820,
  "extra":{
    "private_key":"aAXFScHA5tAA9mUwp1aBDV9cAbHj1mSfwdc1ISTsbm8=",
    "public_key":"WpE32HIFCmunopfbfcuwwgOqdGxmuu04tdZmFQdTBTE=",
    "address":["10.8.1.2/24"],"allowed_ips":["0.0.0.0/0"],
    "amnezia":{"jc":8,"jmin":64,"jmax":900,"s1":88,"s2":155,"s3":32,"s4":16,
      "h1":"122163117-125750861","h2":"708517833-711678982",
      "h3":"1169962535-1174185828","h4":"2133918740-2137138052"}
  }
}`

func TestSetTunnelActiveGatesKeyedWGProbe(t *testing.T) {
	t.Cleanup(func() { SetTunnelActive(false) })

	SetTunnelActive(false)
	if !keyedWGProbeAllowed() {
		t.Error("keyed WG probe must be allowed while the tunnel is down")
	}

	SetTunnelActive(true)
	if keyedWGProbeAllowed() {
		t.Error("keyed WG probe must be refused while the tunnel is up")
	}

	// Idempotent and reversible — a reconnect flips this repeatedly.
	SetTunnelActive(false)
	if !keyedWGProbeAllowed() {
		t.Error("keyed WG probe must be allowed again after disconnect")
	}
}

// The keyed probe speaks Noise with the profile's own private key, so to the
// server it is the same peer as the live tunnel: it hijacks the peer endpoint,
// trips the handshake flood guard and clobbers the single per-peer handshake
// slot. PingEntry must fall back to the keyless liveness probe while connected.
func TestPingEntryWhileTunnelActiveSkipsHandshakeProbe(t *testing.T) {
	if testing.Short() {
		t.Skip("probes an unroutable host; costs a few seconds of timeouts")
	}
	t.Cleanup(func() { SetTunnelActive(false) })
	SetTunnelActive(true)

	raw, err := PingEntry(awgEntryFixture)
	if err != nil {
		t.Fatalf("PingEntry: %v", err)
	}
	var res PingResult
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if res.CheckType == "wg_handshake" {
		t.Fatalf("keyed handshake probe ran while the tunnel was up: %+v", res)
	}
	if res.CheckType != "wg_liveness" {
		t.Errorf("checkType = %q, want %q", res.CheckType, "wg_liveness")
	}
}

func TestPingEntryWhileTunnelIdleUsesHandshakeProbe(t *testing.T) {
	if testing.Short() {
		t.Skip("probes an unroutable host; costs a few seconds of timeouts")
	}
	t.Cleanup(func() { SetTunnelActive(false) })
	SetTunnelActive(false)

	raw, err := PingEntry(awgEntryFixture)
	if err != nil {
		t.Fatalf("PingEntry: %v", err)
	}
	var res PingResult
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if res.CheckType != "wg_handshake" {
		t.Errorf("checkType = %q, want %q", res.CheckType, "wg_handshake")
	}
}
