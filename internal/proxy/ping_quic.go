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
	"crypto/tls"
	"net"
	"strconv"
	"time"

	quic "github.com/quic-go/quic-go"
)

// quicHandshakeTimeout bounds a single QUIC ping attempt: dial + Initial + handshake.
// Anything beyond this is treated as unreachable rather than waiting forever.
const quicHandshakeTimeout = 3 * time.Second

// hysteria2ALPN is the ALPN list we offer. "h3" is the modern default, "hysteria"
// covers older servers. The server picks one; if it picks neither, the handshake
// fails fast and we fall back to TCP.
var hysteria2ALPN = []string{"h3", "hysteria"}

// quicHandshakeProbe and quicHandshakeLANProbe are vars so tests can swap them out
// for deterministic fakes (real QUIC needs a real server).
var quicHandshakeProbe = quicHandshakePing
var quicHandshakeLANProbe = quicHandshakePingLANBind

// quicHandshakePing measures the time required for a full QUIC handshake to ip:port.
// The TLS layer is configured with InsecureSkipVerify because we only want a latency
// reading — auth and certificate validity belong to the actual proxy flow, not to ping.
// Returned latency is wall-clock from dial() to handshake-complete.
func quicHandshakePing(ip string, port int) (latencyMs int64, reachable bool, reason string) {
	addr := net.JoinHostPort(ip, strconv.Itoa(port))

	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         hysteria2ALPN,
		ServerName:         ip,
	}
	quicConf := &quic.Config{
		HandshakeIdleTimeout: quicHandshakeTimeout,
		MaxIdleTimeout:       quicHandshakeTimeout,
	}

	ctx, cancel := context.WithTimeout(context.Background(), quicHandshakeTimeout)
	defer cancel()

	start := time.Now()
	conn, err := quic.DialAddr(ctx, addr, tlsConf, quicConf)
	elapsed := time.Since(start)
	if err != nil {
		return 0, false, pingReasonFromError(err)
	}
	_ = conn.CloseWithError(0, "")
	return elapsed.Milliseconds(), true, ""
}

// quicHandshakePingLANBind is quicHandshakePing pinned to a LAN interface — used
// when a system tunnel is up so the probe doesn't get pulled back through the
// proxy that's being pinged.
func quicHandshakePingLANBind(ip string, port int) (latencyMs int64, reachable bool, reason string) {
	local, err := pickLANBindIPv4()
	if err != nil {
		return 0, false, "lan_bind_unavailable"
	}

	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: local, Port: 0})
	if err != nil {
		return 0, false, pingReasonFromError(err)
	}
	defer udpConn.Close()

	remote, err := net.ResolveUDPAddr("udp", net.JoinHostPort(ip, strconv.Itoa(port)))
	if err != nil {
		return 0, false, pingReasonFromError(err)
	}

	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         hysteria2ALPN,
		ServerName:         ip,
	}
	quicConf := &quic.Config{
		HandshakeIdleTimeout: quicHandshakeTimeout,
		MaxIdleTimeout:       quicHandshakeTimeout,
	}

	ctx, cancel := context.WithTimeout(context.Background(), quicHandshakeTimeout)
	defer cancel()

	start := time.Now()
	conn, err := quic.Dial(ctx, udpConn, remote, tlsConf, quicConf)
	elapsed := time.Since(start)
	if err != nil {
		return 0, false, pingReasonFromError(err)
	}
	_ = conn.CloseWithError(0, "")
	return elapsed.Milliseconds(), true, ""
}

// PingHysteria2QUICStrict is PingHysteria2QUIC without the TCP fallback, for
// callers that are choosing a node rather than reporting host liveness to the
// UI.
//
// Hysteria2 speaks UDP only, so a successful TCP connect to its port proves
// nothing about the node — and while a TUN is up it proves less than nothing:
// sing-tun's system stack answers the SYN locally, so the fallback succeeds in
// a couple of milliseconds for every node in the group, reachable or not. Since
// a real QUIC handshake costs hundreds of milliseconds, the fallback's fake
// reading always wins an RTT-ordered ranking. Failing outright is the only
// answer that keeps the two measurements on one scale.
func PingHysteria2QUICStrict(ip string, port int) (latencyMs int64, reachable bool, reason, checkType string) {
	latency, ok, r := quicHandshakeProbe(ip, port)
	if ok {
		return latency, true, "", "quic_handshake"
	}
	if r == "" {
		r = "quic_handshake_failed"
	}
	return 0, false, r, "quic_handshake"
}
