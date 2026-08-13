package proxy

import (
	"errors"
	"net"
	"testing"
	"time"

	"resultproxy-wails/internal/config"
)

// Свип авторежима часто идёт при поднятом туннеле (переключение на AUTO без
// паузы). Стек sing-tun завершает TCP-рукопожатие локально, поэтому не-LAN-bind
// проба рапортует «живой, 4 мс» о сервере, до которого нет связи. Проба обязана
// уходить с физического адаптера.
func TestProbeTransport_UsesLANBindProbesWhenBindAddressAvailable(t *testing.T) {
	oldPick := pickLANBindIPv4
	oldTCP, oldLAN := pingTCPProbe, pingLANProbe
	defer func() {
		pickLANBindIPv4 = oldPick
		pingTCPProbe, pingLANProbe = oldTCP, oldLAN
	}()

	pickLANBindIPv4 = func() (net.IP, error) { return net.IPv4(192, 168, 1, 5), nil }
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		t.Error("при доступном bind-адресе должна вызываться LAN-bind проба")
		return 0, false, ""
	}
	pingLANProbe = func(_ string, _ int) (int64, bool, string) { return 42, true, "" }

	rtt, ok, stage, _ := probeTransport(config.ProxyEntry{IP: "1.1.1.1", Port: 443, Type: "VLESS"})
	if !ok || rtt != 42 || stage != "tcp" {
		t.Fatalf("ожидали 42ms/OK/tcp через LAN-bind, получили rtt=%d ok=%v stage=%q", rtt, ok, stage)
	}
}

// Без физического адаптера (например, единственный интерфейс — туннельный)
// привязка невозможна; проба обязана деградировать к обычной, а не отказать.
func TestProbeTransport_FallsBackToPlainProbesWithoutBindAddress(t *testing.T) {
	oldPick := pickLANBindIPv4
	oldTCP, oldLAN := pingTCPProbe, pingLANProbe
	defer func() {
		pickLANBindIPv4 = oldPick
		pingTCPProbe, pingLANProbe = oldTCP, oldLAN
	}()

	pickLANBindIPv4 = func() (net.IP, error) { return nil, errors.New("no suitable LAN IPv4 for bind") }
	pingLANProbe = func(_ string, _ int) (int64, bool, string) {
		t.Error("без bind-адреса LAN-bind проба вызываться не должна")
		return 0, false, ""
	}
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) { return 77, true, "" }

	rtt, ok, _, _ := probeTransport(config.ProxyEntry{IP: "1.1.1.1", Port: 443, Type: "VLESS"})
	if !ok || rtt != 77 {
		t.Fatalf("ожидали 77ms/OK через обычную пробу, получили rtt=%d ok=%v", rtt, ok)
	}
}

func TestProbeTransport_UsesLANBindForHysteria2AndWireGuard(t *testing.T) {
	oldPick := pickLANBindIPv4
	oldHY, oldHYLAN := pingHysteria2Probe, pingHysteria2LANProbe
	oldWG, oldWGLAN := pingWireGuardProbe, pingWireGuardLANProbe
	defer func() {
		pickLANBindIPv4 = oldPick
		pingHysteria2Probe, pingHysteria2LANProbe = oldHY, oldHYLAN
		pingWireGuardProbe, pingWireGuardLANProbe = oldWG, oldWGLAN
	}()

	pickLANBindIPv4 = func() (net.IP, error) { return net.IPv4(192, 168, 1, 5), nil }
	pingHysteria2Probe = func(_ string, _ int) (int64, bool, string, string) {
		t.Error("HYSTERIA2: ожидали LAN-bind пробу")
		return 0, false, "", ""
	}
	pingWireGuardProbe = func(_ string, _ int) (int64, bool, string) {
		t.Error("WIREGUARD: ожидали LAN-bind пробу")
		return 0, false, ""
	}
	pingHysteria2LANProbe = func(_ string, _ int) (int64, bool, string, string) {
		return 11, true, "", "quic_handshake_lan_bind"
	}
	pingWireGuardLANProbe = func(_ string, _ int) (int64, bool, string) { return 22, true, "" }

	if rtt, ok, stage, _ := probeTransport(config.ProxyEntry{IP: "1.1.1.1", Port: 443, Type: "HYSTERIA2"}); !ok || rtt != 11 || stage != "quic_handshake_lan_bind" {
		t.Fatalf("HYSTERIA2: получили rtt=%d ok=%v stage=%q", rtt, ok, stage)
	}
	if rtt, ok, stage, _ := probeTransport(config.ProxyEntry{IP: "1.1.1.1", Port: 443, Type: "AMNEZIAWG"}); !ok || rtt != 22 || stage != "udp" {
		t.Fatalf("AMNEZIAWG: получили rtt=%d ok=%v stage=%q", rtt, ok, stage)
	}
}

// TLS-фаза ходит тем же путём и по той же причине должна уходить с физического
// адаптера.
func TestAutoProbeDialer_BindsToLANAddressWhenAvailable(t *testing.T) {
	oldPick := pickLANBindIPv4
	defer func() { pickLANBindIPv4 = oldPick }()

	pickLANBindIPv4 = func() (net.IP, error) { return net.IPv4(192, 168, 1, 5), nil }
	d := autoProbeDialer(4 * time.Second)
	local, ok := d.LocalAddr.(*net.TCPAddr)
	if !ok || !local.IP.Equal(net.IPv4(192, 168, 1, 5)) {
		t.Fatalf("ожидали привязку к 192.168.1.5, получили %#v", d.LocalAddr)
	}
	if d.Timeout != 4*time.Second {
		t.Fatalf("таймаут не проброшен: %v", d.Timeout)
	}
}

func TestAutoProbeDialer_LeavesLocalAddrNilWithoutBindAddress(t *testing.T) {
	oldPick := pickLANBindIPv4
	defer func() { pickLANBindIPv4 = oldPick }()

	pickLANBindIPv4 = func() (net.IP, error) { return nil, errors.New("no suitable LAN IPv4 for bind") }
	if d := autoProbeDialer(4 * time.Second); d.LocalAddr != nil {
		t.Fatalf("без bind-адреса LocalAddr должен остаться nil, получили %#v", d.LocalAddr)
	}
}
