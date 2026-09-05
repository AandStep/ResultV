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
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Why this probe exists: every other health check in the app answers "does the
// server take a TCP connection / complete a TLS or QUIC handshake". None of
// them answers "does this node relay UDP", and that is a genuinely separate
// property. A VLESS server configured with flow=xtls-rprx-vision rejects plain
// UDP outright (sing-vmess/vless/service.go: "xtls-rprx-vision flow does not
// support UDP"), some servers firewall outbound UDP, and some relay it fine.
// A node can therefore look perfectly healthy by every existing measure and
// still be unable to carry a Discord call — which is exactly the report that
// calls do not work on some servers.
//
// The probe drives the same path real UDP takes: SOCKS5 UDP ASSOCIATE against
// our own loopback inbound, a STUN Binding Request to a public STUN server
// named by DOMAIN (so the destination is resolved at the far end, the way a
// tunnelled client resolves), and a parsed Binding Success in return. The
// reflexive address STUN reports back is kept: it is positive proof the packet
// left through the node and not through the local uplink.

// udpRelayProbeDomains are the STUN hosts the probe targets. Addressed by
// domain on purpose: buildRoute pins exactly these names, scoped to the probe
// inbound, so the probe cannot silently measure the direct path in Smart mode
// (where Final=direct would otherwise swallow it).
var udpRelayProbeDomains = []string{
	"stun.l.google.com",
	"stun.cloudflare.com",
}

// udpRelayProbeTargets pairs each probe domain with its STUN port. Neither
// port is 443, so quicRejectRule never shadows the probe.
var udpRelayProbeTargets = []struct {
	Host string
	Port int
}{
	{"stun.l.google.com", 19302},
	{"stun.cloudflare.com", 3478},
}

const (
	udpRelayProbeTimeout = 4 * time.Second
	stunMagicCookie      = 0x2112A442
	stunBindingRequest   = 0x0001
	stunBindingSuccess   = 0x0101
	stunAttrMappedAddr   = 0x0001
	stunAttrXorMapped    = 0x0020
	stunHeaderLen        = 20
)

// UDPRelayResult is the verdict for one node. Reason is filled on failure and
// is meant to be readable in a log line, not parsed.
type UDPRelayResult struct {
	OK         bool
	MappedAddr string
	Target     string
	LatencyMs  int64
	Reason     string
}

// ProbeUDPRelay reports whether the running engine relays UDP end to end.
// socksAddr is the local mixed/SOCKS inbound ("127.0.0.1:14081"). Targets are
// tried in order and the first success wins; a node that answers on neither is
// reported as unable to carry UDP.
func ProbeUDPRelay(ctx context.Context, socksAddr string) UDPRelayResult {
	if strings.TrimSpace(socksAddr) == "" {
		return UDPRelayResult{Reason: "no local inbound address"}
	}
	var reasons []string
	for _, t := range udpRelayProbeTargets {
		if ctx.Err() != nil {
			return UDPRelayResult{Reason: "probe cancelled"}
		}
		start := time.Now()
		mapped, err := stunThroughSOCKS5(ctx, socksAddr, t.Host, t.Port)
		if err == nil {
			return UDPRelayResult{
				OK:         true,
				MappedAddr: mapped,
				Target:     net.JoinHostPort(t.Host, strconv.Itoa(t.Port)),
				LatencyMs:  time.Since(start).Milliseconds(),
			}
		}
		reasons = append(reasons, fmt.Sprintf("%s: %v", t.Host, err))
	}
	return UDPRelayResult{Reason: strings.Join(reasons, "; ")}
}

// stunThroughSOCKS5 runs one Binding Request over a SOCKS5 UDP association and
// returns the reflexive address the STUN server saw.
func stunThroughSOCKS5(ctx context.Context, socksAddr, host string, port int) (string, error) {
	deadline := time.Now().Add(udpRelayProbeTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}

	var dialer net.Dialer
	ctrl, err := dialer.DialContext(ctx, "tcp", socksAddr)
	if err != nil {
		return "", fmt.Errorf("dial local inbound: %w", err)
	}
	// The association lives exactly as long as this TCP control connection —
	// closing it early tears the UDP relay down mid-probe.
	defer ctrl.Close()
	_ = ctrl.SetDeadline(deadline)

	relayAddr, err := socks5UDPAssociate(ctrl, socksAddr)
	if err != nil {
		return "", err
	}

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("local udp socket: %w", err)
	}
	defer pc.Close()
	_ = pc.SetDeadline(deadline)

	txID, req, err := newSTUNBindingRequest()
	if err != nil {
		return "", err
	}
	if _, err := pc.WriteTo(encodeSOCKS5UDPDatagram(host, port, req), relayAddr); err != nil {
		return "", fmt.Errorf("send: %w", err)
	}

	buf := make([]byte, 1500)
	for {
		n, _, err := pc.ReadFrom(buf)
		if err != nil {
			return "", fmt.Errorf("no UDP answer through the node: %w", err)
		}
		payload, decErr := decodeSOCKS5UDPDatagram(buf[:n])
		if decErr != nil {
			// A malformed relay frame is not a reason to give up on the
			// association: keep reading until the deadline says otherwise.
			continue
		}
		mapped, parseErr := parseSTUNBindingSuccess(payload, txID)
		if parseErr != nil {
			continue
		}
		return mapped, nil
	}
}

// socks5UDPAssociate performs the SOCKS5 greeting and UDP ASSOCIATE exchange,
// returning the address datagrams must be sent to.
func socks5UDPAssociate(ctrl net.Conn, socksAddr string) (*net.UDPAddr, error) {
	if _, err := ctrl.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return nil, fmt.Errorf("socks greeting: %w", err)
	}
	resp := make([]byte, 2)
	if _, err := readFullConn(ctrl, resp); err != nil {
		return nil, fmt.Errorf("socks greeting reply: %w", err)
	}
	if resp[0] != 0x05 || resp[1] != 0x00 {
		return nil, fmt.Errorf("socks greeting rejected: %v", resp)
	}

	// UDP ASSOCIATE with a 0.0.0.0:0 request address: we do not know which
	// local port we will bind until the association exists.
	if _, err := ctrl.Write([]byte{0x05, 0x03, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return nil, fmt.Errorf("udp associate: %w", err)
	}
	head := make([]byte, 4)
	if _, err := readFullConn(ctrl, head); err != nil {
		return nil, fmt.Errorf("udp associate reply: %w", err)
	}
	if head[0] != 0x05 {
		return nil, fmt.Errorf("bad socks version in reply: %d", head[0])
	}
	if head[1] != 0x00 {
		return nil, fmt.Errorf("node refused UDP ASSOCIATE (socks reply %d)", head[1])
	}
	ip, err := readSOCKS5Addr(ctrl, head[3])
	if err != nil {
		return nil, err
	}
	portBuf := make([]byte, 2)
	if _, err := readFullConn(ctrl, portBuf); err != nil {
		return nil, fmt.Errorf("udp associate port: %w", err)
	}
	bndPort := int(binary.BigEndian.Uint16(portBuf))
	if bndPort == 0 {
		return nil, errors.New("udp associate returned port 0")
	}
	// A bound address of 0.0.0.0 means "same host as the control connection",
	// which is how sing-box answers on a loopback inbound.
	if ip == nil || ip.IsUnspecified() {
		host, _, splitErr := net.SplitHostPort(socksAddr)
		if splitErr != nil {
			return nil, fmt.Errorf("resolve relay host: %w", splitErr)
		}
		ip = net.ParseIP(host)
		if ip == nil {
			return nil, fmt.Errorf("relay host %q is not an IP", host)
		}
	}
	return &net.UDPAddr{IP: ip, Port: bndPort}, nil
}

// readSOCKS5Addr consumes the BND.ADDR field. A nil IP with a nil error means
// "the reply named a host we should not resolve ourselves" and the caller
// falls back to the control connection's address.
func readSOCKS5Addr(conn net.Conn, atyp byte) (net.IP, error) {
	switch atyp {
	case 0x01:
		b := make([]byte, 4)
		if _, err := readFullConn(conn, b); err != nil {
			return nil, fmt.Errorf("read bnd addr: %w", err)
		}
		return net.IP(b), nil
	case 0x04:
		b := make([]byte, 16)
		if _, err := readFullConn(conn, b); err != nil {
			return nil, fmt.Errorf("read bnd addr6: %w", err)
		}
		return net.IP(b), nil
	case 0x03:
		l := make([]byte, 1)
		if _, err := readFullConn(conn, l); err != nil {
			return nil, fmt.Errorf("read bnd domain len: %w", err)
		}
		b := make([]byte, int(l[0]))
		if _, err := readFullConn(conn, b); err != nil {
			return nil, fmt.Errorf("read bnd domain: %w", err)
		}
		// Resolving a domain here locally would defeat the point of the probe.
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown socks address type %d", atyp)
	}
}

func readFullConn(conn net.Conn, b []byte) (int, error) {
	total := 0
	for total < len(b) {
		n, err := conn.Read(b[total:])
		if n > 0 {
			total += n
		}
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// encodeSOCKS5UDPDatagram wraps a payload in the RFC 1928 UDP request header,
// addressing the target by domain so the far end does the resolving.
func encodeSOCKS5UDPDatagram(host string, port int, payload []byte) []byte {
	out := make([]byte, 0, 4+1+len(host)+2+len(payload))
	out = append(out, 0x00, 0x00, 0x00, 0x03, byte(len(host)))
	out = append(out, host...)
	out = append(out, byte(port>>8), byte(port))
	return append(out, payload...)
}

// decodeSOCKS5UDPDatagram strips the RFC 1928 UDP header and returns the
// payload. FRAG != 0 is rejected: we never fragment, so a fragmented reply is
// not ours.
func decodeSOCKS5UDPDatagram(b []byte) ([]byte, error) {
	if len(b) < 10 {
		return nil, errors.New("short socks udp datagram")
	}
	if b[2] != 0x00 {
		return nil, errors.New("fragmented socks udp datagram")
	}
	off := 4
	switch b[3] {
	case 0x01:
		off += 4
	case 0x04:
		off += 16
	case 0x03:
		off += 1 + int(b[4])
	default:
		return nil, fmt.Errorf("unknown socks udp address type %d", b[3])
	}
	off += 2 // port
	if off > len(b) {
		return nil, errors.New("socks udp header overruns datagram")
	}
	return b[off:], nil
}

func newSTUNBindingRequest() ([12]byte, []byte, error) {
	var txID [12]byte
	if _, err := rand.Read(txID[:]); err != nil {
		return txID, nil, fmt.Errorf("stun transaction id: %w", err)
	}
	msg := make([]byte, stunHeaderLen)
	binary.BigEndian.PutUint16(msg[0:2], stunBindingRequest)
	binary.BigEndian.PutUint16(msg[2:4], 0)
	binary.BigEndian.PutUint32(msg[4:8], stunMagicCookie)
	copy(msg[8:20], txID[:])
	return txID, msg, nil
}

// parseSTUNBindingSuccess validates a Binding Success for our transaction and
// returns the reflexive address it reports.
func parseSTUNBindingSuccess(b []byte, txID [12]byte) (string, error) {
	if len(b) < stunHeaderLen {
		return "", errors.New("short stun message")
	}
	if binary.BigEndian.Uint16(b[0:2]) != stunBindingSuccess {
		return "", errors.New("not a binding success")
	}
	if binary.BigEndian.Uint32(b[4:8]) != stunMagicCookie {
		return "", errors.New("bad stun magic cookie")
	}
	if string(b[8:20]) != string(txID[:]) {
		return "", errors.New("stun transaction id mismatch")
	}
	bodyLen := int(binary.BigEndian.Uint16(b[2:4]))
	if stunHeaderLen+bodyLen > len(b) {
		return "", errors.New("stun length overruns message")
	}
	body := b[stunHeaderLen : stunHeaderLen+bodyLen]

	for len(body) >= 4 {
		attrType := binary.BigEndian.Uint16(body[0:2])
		attrLen := int(binary.BigEndian.Uint16(body[2:4]))
		if 4+attrLen > len(body) {
			return "", errors.New("stun attribute overruns body")
		}
		value := body[4 : 4+attrLen]
		switch attrType {
		case stunAttrXorMapped:
			if addr, err := decodeSTUNAddress(value, true); err == nil {
				return addr, nil
			}
		case stunAttrMappedAddr:
			if addr, err := decodeSTUNAddress(value, false); err == nil {
				return addr, nil
			}
		}
		// Attributes are padded to a 4-byte boundary.
		advance := 4 + attrLen
		if pad := attrLen % 4; pad != 0 {
			advance += 4 - pad
		}
		if advance > len(body) {
			break
		}
		body = body[advance:]
	}
	return "", errors.New("stun success carried no mapped address")
}

func decodeSTUNAddress(v []byte, xored bool) (string, error) {
	if len(v) < 8 || v[1] != 0x01 {
		// IPv6 mapped addresses are valid STUN but tell us nothing extra here.
		return "", errors.New("not an IPv4 mapped address")
	}
	port := binary.BigEndian.Uint16(v[2:4])
	ip := make(net.IP, 4)
	copy(ip, v[4:8])
	if xored {
		port ^= uint16(stunMagicCookie >> 16)
		var cookie [4]byte
		binary.BigEndian.PutUint32(cookie[:], stunMagicCookie)
		for i := range ip {
			ip[i] ^= cookie[i]
		}
	}
	return net.JoinHostPort(ip.String(), strconv.Itoa(int(port))), nil
}
