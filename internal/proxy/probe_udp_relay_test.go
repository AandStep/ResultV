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

package proxy

import (
	"context"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeSOCKSBehaviour selects how the stand-in inbound answers UDP ASSOCIATE.
type fakeSOCKSBehaviour int

const (
	fakeSOCKSRelays fakeSOCKSBehaviour = iota
	// fakeSOCKSRefusesUDP models the node this probe exists to catch: the
	// association is rejected outright, which is what a VLESS server running
	// flow=xtls-rprx-vision does with plain UDP.
	fakeSOCKSRefusesUDP
	// fakeSOCKSBlackHoles models the other shape: the association is granted
	// and the datagram then vanishes.
	fakeSOCKSBlackHoles
)

// startFakeSOCKSRelay stands in for the local mixed inbound. It speaks just
// enough SOCKS5 to drive the probe and, when relaying, answers a STUN Binding
// Request with a Binding Success carrying reflexiveAddr.
func startFakeSOCKSRelay(t *testing.T, behaviour fakeSOCKSBehaviour, reflexiveIP string, reflexivePort int) string {
	t.Helper()

	udpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("fake relay udp listen: %v", err)
	}
	t.Cleanup(func() { udpConn.Close() })
	udpPort := udpConn.LocalAddr().(*net.UDPAddr).Port

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("fake relay tcp listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveFakeSOCKS(conn, behaviour, udpPort)
		}
	}()

	if behaviour == fakeSOCKSRelays {
		go func() {
			buf := make([]byte, 1500)
			for {
				n, from, err := udpConn.ReadFrom(buf)
				if err != nil {
					return
				}
				payload, err := decodeSOCKS5UDPDatagram(buf[:n])
				if err != nil || len(payload) < stunHeaderLen {
					continue
				}
				var txID [12]byte
				copy(txID[:], payload[8:20])
				reply := encodeSOCKS5UDPDatagram("stun.example", 19302,
					buildSTUNBindingSuccess(txID, reflexiveIP, reflexivePort))
				if _, err := udpConn.WriteTo(reply, from); err != nil {
					return
				}
			}
		}()
	}

	return ln.Addr().String()
}

func serveFakeSOCKS(conn net.Conn, behaviour fakeSOCKSBehaviour, udpPort int) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	greeting := make([]byte, 3)
	if _, err := readFullConn(conn, greeting); err != nil {
		return
	}
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}
	req := make([]byte, 10)
	if _, err := readFullConn(conn, req); err != nil {
		return
	}
	if behaviour == fakeSOCKSRefusesUDP {
		// 0x07 = command not supported.
		_, _ = conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	reply := []byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, byte(udpPort >> 8), byte(udpPort)}
	if _, err := conn.Write(reply); err != nil {
		return
	}
	// Hold the association open the way a real inbound does.
	hold := make([]byte, 1)
	_, _ = conn.Read(hold)
}

func buildSTUNBindingSuccess(txID [12]byte, ip string, port int) []byte {
	addr := net.ParseIP(ip).To4()
	value := make([]byte, 8)
	value[0] = 0x00
	value[1] = 0x01
	binary.BigEndian.PutUint16(value[2:4], uint16(port)^uint16(stunMagicCookie>>16))
	var cookie [4]byte
	binary.BigEndian.PutUint32(cookie[:], stunMagicCookie)
	for i := range 4 {
		value[4+i] = addr[i] ^ cookie[i]
	}

	msg := make([]byte, stunHeaderLen, stunHeaderLen+4+len(value))
	binary.BigEndian.PutUint16(msg[0:2], stunBindingSuccess)
	binary.BigEndian.PutUint16(msg[2:4], uint16(4+len(value)))
	binary.BigEndian.PutUint32(msg[4:8], stunMagicCookie)
	copy(msg[8:20], txID[:])

	attr := make([]byte, 4)
	binary.BigEndian.PutUint16(attr[0:2], stunAttrXorMapped)
	binary.BigEndian.PutUint16(attr[2:4], uint16(len(value)))
	return append(append(msg, attr...), value...)
}

// useSingleProbeTarget narrows the target list so a test drives exactly one
// exchange instead of walking the real STUN hosts.
func useSingleProbeTarget(t *testing.T) {
	t.Helper()
	saved := udpRelayProbeTargets
	udpRelayProbeTargets = udpRelayProbeTargets[:1]
	t.Cleanup(func() { udpRelayProbeTargets = saved })
}

func TestProbeUDPRelay_ReportsMappedAddressWhenNodeRelaysUDP(t *testing.T) {
	useSingleProbeTarget(t)
	addr := startFakeSOCKSRelay(t, fakeSOCKSRelays, "203.0.113.9", 51820)

	res := ProbeUDPRelay(context.Background(), addr)
	if !res.OK {
		t.Fatalf("expected a working relay to pass, reason=%s", res.Reason)
	}
	if res.MappedAddr != "203.0.113.9:51820" {
		t.Fatalf("reflexive address lost: got %q", res.MappedAddr)
	}
	if res.Target != "stun.l.google.com:19302" {
		t.Fatalf("unexpected target: %q", res.Target)
	}
}

// The case this probe exists for: the node takes TCP happily and refuses the
// UDP association. Every other health check in the app would call it healthy.
func TestProbeUDPRelay_FailsWhenNodeRefusesUDPAssociate(t *testing.T) {
	useSingleProbeTarget(t)
	addr := startFakeSOCKSRelay(t, fakeSOCKSRefusesUDP, "203.0.113.9", 51820)

	res := ProbeUDPRelay(context.Background(), addr)
	if res.OK {
		t.Fatal("a node refusing UDP ASSOCIATE must not be reported as UDP-capable")
	}
	if !strings.Contains(res.Reason, "refused UDP ASSOCIATE") {
		t.Fatalf("reason should name the refusal, got %q", res.Reason)
	}
}

// The other shape: association granted, datagram black-holed. Bounded by the
// context deadline so a dead node cannot stall the caller.
func TestProbeUDPRelay_FailsWhenDatagramIsBlackHoled(t *testing.T) {
	useSingleProbeTarget(t)
	addr := startFakeSOCKSRelay(t, fakeSOCKSBlackHoles, "203.0.113.9", 51820)

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	start := time.Now()
	res := ProbeUDPRelay(ctx, addr)
	if res.OK {
		t.Fatal("a black-holed datagram must not be reported as success")
	}
	if elapsed := time.Since(start); elapsed > udpRelayProbeTimeout {
		t.Fatalf("probe ignored the context deadline, took %s", elapsed)
	}
	if !strings.Contains(res.Reason, "no UDP answer") {
		t.Fatalf("reason should say no answer came back, got %q", res.Reason)
	}
}

func TestProbeUDPRelay_EmptyInboundAddress(t *testing.T) {
	res := ProbeUDPRelay(context.Background(), "  ")
	if res.OK || res.Reason == "" {
		t.Fatalf("empty inbound must fail with a reason, got %+v", res)
	}
}

// A Binding Success for someone else's transaction must never be accepted:
// the association is shared, and a stale answer would fake a passing node.
func TestParseSTUNBindingSuccess_RejectsForeignTransaction(t *testing.T) {
	var mine, theirs [12]byte
	copy(mine[:], "aaaaaaaaaaaa")
	copy(theirs[:], "bbbbbbbbbbbb")
	if _, err := parseSTUNBindingSuccess(buildSTUNBindingSuccess(theirs, "198.51.100.7", 3478), mine); err == nil {
		t.Fatal("a foreign transaction id must be rejected")
	}
}

func TestParseSTUNBindingSuccess_DecodesXorMappedAddress(t *testing.T) {
	var txID [12]byte
	copy(txID[:], "0123456789ab")
	got, err := parseSTUNBindingSuccess(buildSTUNBindingSuccess(txID, "198.51.100.7", 3478), txID)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got != "198.51.100.7:3478" {
		t.Fatalf("XOR-MAPPED-ADDRESS decoded wrong: %q", got)
	}
}

// Round-trip the RFC 1928 UDP framing: the probe addresses its target by
// domain, so the header the far end sees must carry the name, not an IP.
func TestSOCKS5UDPDatagram_RoundTripsDomainTarget(t *testing.T) {
	payload := []byte("payload")
	frame := encodeSOCKS5UDPDatagram("stun.l.google.com", 19302, payload)
	if frame[3] != 0x03 {
		t.Fatalf("target must be addressed by domain, atyp=%d", frame[3])
	}
	got, err := decodeSOCKS5UDPDatagram(frame)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload mangled: %q", got)
	}
}

func TestDecodeSOCKS5UDPDatagram_RejectsFragments(t *testing.T) {
	frame := encodeSOCKS5UDPDatagram("stun.l.google.com", 19302, []byte("x"))
	frame[2] = 0x01 // FRAG
	if _, err := decodeSOCKS5UDPDatagram(frame); err == nil {
		t.Fatal("fragmented datagrams must be rejected")
	}
}

// The probe is only meaningful if its packets actually take the node. In Smart
// mode Final=direct, so without an explicit rule the STUN exchange would leave
// through the local uplink and report every node as UDP-capable.
func TestBuildRoute_TunnelMode_UDPProbeDomainsForcedThroughProxy(t *testing.T) {
	route := buildRoute(EngineConfig{
		Mode:        ProxyModeTunnel,
		RoutingMode: ModeSmart,
		Proxy:       ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
	})
	found := false
	for _, r := range route.Rules {
		if r.Outbound != "proxy" || len(r.Domain) == 0 {
			continue
		}
		has := map[string]bool{}
		for _, d := range r.Domain {
			has[d] = true
		}
		if !has["stun.l.google.com"] {
			continue
		}
		found = true
		if len(r.Inbound) != 1 || r.Inbound[0] != probeInboundTag {
			t.Fatalf("probe rule must be scoped to %s, got inbound=%v", probeInboundTag, r.Inbound)
		}
		if len(r.Network) != 1 || r.Network[0] != "udp" {
			t.Fatalf("probe rule must be udp-only, got %v", r.Network)
		}
	}
	if !found {
		t.Fatalf("no proxy route for the STUN probe hosts, rules=%+v", route.Rules)
	}
}

// Proxy mode shares its inbound with the user's apps, so the probe rule must
// not exist there: it would drag real WebRTC traffic through the proxy.
func TestBuildRoute_ProxyMode_NoUDPProbeRule(t *testing.T) {
	route := buildRoute(EngineConfig{
		Mode:        ProxyModeProxy,
		RoutingMode: ModeSmart,
		Proxy:       ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
	})
	for _, r := range route.Rules {
		for _, d := range r.Domain {
			if d == "stun.l.google.com" {
				t.Fatalf("proxy mode must not emit the UDP probe rule, rules=%+v", route.Rules)
			}
		}
	}
}

// The rule has to survive the pinned core's strict decoder, inbound tag and all.
func TestBuildTunnelModeConfig_UDPProbeRuleAcceptedByCore(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:        ProxyModeTunnel,
		RoutingMode: ModeSmart,
		Proxy:       ProxyConfig{Type: "TROJAN", IP: "1.2.3.4", Port: 443, Password: "p"},
	})
	assertCoreAcceptsConfig(t, cfg)
}
