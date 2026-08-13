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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"testing"
	"time"

	quic "github.com/quic-go/quic-go"
)

func TestPingHysteria2QUIC_UsesQUICHandshakeOnSuccess(t *testing.T) {
	oldQUIC := quicHandshakeProbe
	defer func() { quicHandshakeProbe = oldQUIC }()

	quicCalls := 0
	quicHandshakeProbe = func(_ string, _ int) (int64, bool, string) {
		quicCalls++
		return 42, true, ""
	}

	latency, reachable, reason, checkType := PingHysteria2QUIC("1.2.3.4", 443)
	if !reachable || latency != 42 || reason != "" || checkType != "quic_handshake" {
		t.Fatalf("unexpected result: latency=%d reachable=%v reason=%q checkType=%q",
			latency, reachable, reason, checkType)
	}
	if quicCalls != 1 {
		t.Fatalf("expected exactly one QUIC probe call, got %d", quicCalls)
	}
}

func TestPingHysteria2QUIC_FallsBackToTCPWhenQUICFails(t *testing.T) {
	oldQUIC := quicHandshakeProbe
	oldTCP := pingTCPProbe
	defer func() {
		quicHandshakeProbe = oldQUIC
		pingTCPProbe = oldTCP
	}()

	quicHandshakeProbe = func(_ string, _ int) (int64, bool, string) {
		return 0, false, "timeout"
	}
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		return 17, true, ""
	}

	latency, reachable, reason, checkType := PingHysteria2QUIC("1.2.3.4", 443)
	if !reachable || latency != 17 || reason != "" || checkType != "tcp_fallback" {
		t.Fatalf("unexpected result: latency=%d reachable=%v reason=%q checkType=%q",
			latency, reachable, reason, checkType)
	}
}

func TestPingHysteria2QUIC_BothFail_ReturnsQUICReason(t *testing.T) {
	oldQUIC := quicHandshakeProbe
	oldTCP := pingTCPProbe
	defer func() {
		quicHandshakeProbe = oldQUIC
		pingTCPProbe = oldTCP
	}()

	quicHandshakeProbe = func(_ string, _ int) (int64, bool, string) {
		return 0, false, "timeout"
	}
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		return 0, false, "connection_refused"
	}

	latency, reachable, reason, checkType := PingHysteria2QUIC("1.2.3.4", 443)
	if reachable || latency != 0 || reason != "timeout" || checkType != "quic_handshake" {
		t.Fatalf("unexpected result: latency=%d reachable=%v reason=%q checkType=%q",
			latency, reachable, reason, checkType)
	}
}

func TestPingHysteria2QUIC_QUICEmptyReason_UsesTCPReason(t *testing.T) {
	oldQUIC := quicHandshakeProbe
	oldTCP := pingTCPProbe
	defer func() {
		quicHandshakeProbe = oldQUIC
		pingTCPProbe = oldTCP
	}()

	quicHandshakeProbe = func(_ string, _ int) (int64, bool, string) {
		return 0, false, ""
	}
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		return 0, false, "no_route"
	}

	_, reachable, reason, checkType := PingHysteria2QUIC("1.2.3.4", 443)
	if reachable || reason != "no_route" || checkType != "quic_handshake" {
		t.Fatalf("unexpected result: reachable=%v reason=%q checkType=%q", reachable, reason, checkType)
	}
}

func TestPingHysteria2QUICLANBind_UsesQUICHandshakeOnSuccess(t *testing.T) {
	oldQUIC := quicHandshakeLANProbe
	defer func() { quicHandshakeLANProbe = oldQUIC }()

	quicHandshakeLANProbe = func(_ string, _ int) (int64, bool, string) {
		return 33, true, ""
	}

	latency, reachable, reason, checkType := PingHysteria2QUICLANBind("1.2.3.4", 443)
	if !reachable || latency != 33 || reason != "" || checkType != "quic_handshake_lan_bind" {
		t.Fatalf("unexpected result: latency=%d reachable=%v reason=%q checkType=%q",
			latency, reachable, reason, checkType)
	}
}

func TestPingHysteria2QUICLANBind_FallsBackToTCPLanWhenQUICFails(t *testing.T) {
	oldQUIC := quicHandshakeLANProbe
	oldTCP := pingLANProbe
	defer func() {
		quicHandshakeLANProbe = oldQUIC
		pingLANProbe = oldTCP
	}()

	_ = oldTCP
	quicHandshakeLANProbe = func(_ string, _ int) (int64, bool, string) {
		return 0, false, "timeout"
	}

	oldPingLAN := PingProxyLANBind
	_ = oldPingLAN

	oldPick := pickLANBindIPv4
	defer func() { pickLANBindIPv4 = oldPick }()
	pickLANBindIPv4 = func() (net.IP, error) {
		return nil, errors.New("no iface")
	}

	latency, reachable, reason, checkType := PingHysteria2QUICLANBind("1.2.3.4", 443)
	if reachable || latency != 0 || reason != "timeout" || checkType != "quic_handshake_lan_bind" {
		t.Fatalf("unexpected result: latency=%d reachable=%v reason=%q checkType=%q",
			latency, reachable, reason, checkType)
	}
}

func TestQUICHandshakePingLANBind_ReturnsLanBindUnavailableWhenNoInterface(t *testing.T) {
	oldPick := pickLANBindIPv4
	defer func() { pickLANBindIPv4 = oldPick }()
	pickLANBindIPv4 = func() (net.IP, error) {
		return nil, errors.New("no iface")
	}

	latency, reachable, reason := quicHandshakePingLANBind("1.2.3.4", 443)
	if reachable || latency != 0 || reason != "lan_bind_unavailable" {
		t.Fatalf("unexpected result: latency=%d reachable=%v reason=%q", latency, reachable, reason)
	}
}

// TestQUICHandshakePing_RealServer brings up a real in-process QUIC server and
// measures handshake latency against it. This proves the QUIC dialer actually
// completes a handshake and returns a real timing — not just a stubbed value.
func TestQUICHandshakePing_RealServer(t *testing.T) {
	tlsCert := generateSelfSignedCert(t)

	ln, err := quic.ListenAddr("127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"h3"},
	}, &quic.Config{
		HandshakeIdleTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("ListenAddr: %v", err)
	}
	defer ln.Close()

	// Accept handshakes in a goroutine so DialAddr can complete.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept(t.Context())
			if err != nil {
				return
			}
			_ = conn.CloseWithError(0, "")
		}
	}()

	addr := ln.Addr().(*net.UDPAddr)

	latency, reachable, reason := quicHandshakePing(addr.IP.String(), addr.Port)
	if !reachable {
		t.Fatalf("expected handshake to succeed, got reason=%q", reason)
	}
	if latency < 0 {
		t.Fatalf("expected non-negative latency, got %d", latency)
	}
	if latency > 2000 {
		t.Fatalf("loopback handshake should be fast, got %dms", latency)
	}
}

// Отбор кандидата не имеет права засчитывать TCP-коннект к UDP-порту Hysteria2:
// он ничего не говорит о пригодности узла, а по времени всегда выигрывает у
// честного QUIC-рукопожатия.
func TestPingHysteria2QUICStrict_ReportsDeadWhenQUICFails(t *testing.T) {
	oldQUIC, oldTCP := quicHandshakeProbe, pingTCPProbe
	defer func() { quicHandshakeProbe, pingTCPProbe = oldQUIC, oldTCP }()

	quicHandshakeProbe = func(_ string, _ int) (int64, bool, string) { return 0, false, "timeout" }
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		t.Error("строгая проба не должна откатываться на TCP")
		return 4, true, ""
	}

	latency, reachable, reason, checkType := PingHysteria2QUICStrict("1.2.3.4", 443)
	if reachable || latency != 0 {
		t.Fatalf("ожидали недоступность, получили latency=%d reachable=%v", latency, reachable)
	}
	if reason != "timeout" {
		t.Fatalf("ожидали причину timeout, получили %q", reason)
	}
	if checkType != "quic_handshake" {
		t.Fatalf("ожидали checkType=quic_handshake, получили %q", checkType)
	}
}

func TestPingHysteria2QUICStrict_ReportsHandshakeLatencyOnSuccess(t *testing.T) {
	oldQUIC := quicHandshakeProbe
	defer func() { quicHandshakeProbe = oldQUIC }()

	quicHandshakeProbe = func(_ string, _ int) (int64, bool, string) { return 130, true, "" }

	latency, reachable, reason, checkType := PingHysteria2QUICStrict("1.2.3.4", 443)
	if !reachable || latency != 130 || reason != "" || checkType != "quic_handshake" {
		t.Fatalf("получили latency=%d reachable=%v reason=%q checkType=%q", latency, reachable, reason, checkType)
	}
}

func TestPingHysteria2QUICStrictLANBind_ReportsDeadWhenQUICFails(t *testing.T) {
	oldQUIC, oldTCP := quicHandshakeLANProbe, pingLANProbe
	defer func() { quicHandshakeLANProbe, pingLANProbe = oldQUIC, oldTCP }()

	quicHandshakeLANProbe = func(_ string, _ int) (int64, bool, string) { return 0, false, "timeout" }
	pingLANProbe = func(_ string, _ int) (int64, bool, string) {
		t.Error("строгая LAN-bind проба не должна откатываться на TCP")
		return 4, true, ""
	}

	_, reachable, reason, checkType := PingHysteria2QUICStrictLANBind("1.2.3.4", 443)
	if reachable {
		t.Fatal("ожидали недоступность")
	}
	if reason != "timeout" || checkType != "quic_handshake_lan_bind" {
		t.Fatalf("получили reason=%q checkType=%q", reason, checkType)
	}
}

// Пустая причина от QUIC-слоя не должна превращаться в пустой Reason: он
// уходит в node_stats.json и в диагностику, где «» неотличимо от «не пробовали».
func TestPingHysteria2QUICStrict_SubstitutesDefaultReason(t *testing.T) {
	oldQUIC := quicHandshakeProbe
	defer func() { quicHandshakeProbe = oldQUIC }()

	quicHandshakeProbe = func(_ string, _ int) (int64, bool, string) { return 0, false, "" }

	if _, _, reason, _ := PingHysteria2QUICStrict("1.2.3.4", 443); reason != "quic_handshake_failed" {
		t.Fatalf("ожидали quic_handshake_failed, получили %q", reason)
	}
}

func generateSelfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}
}
