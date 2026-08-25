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

	node := config.ProxyEntry{IP: "1.1.1.1", Port: 443, Type: "VLESS"}
	rtt, ok, stage, _ := probeTransport(node, node.IP)
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

	node := config.ProxyEntry{IP: "1.1.1.1", Port: 443, Type: "VLESS"}
	rtt, ok, _, _ := probeTransport(node, node.IP)
	if !ok || rtt != 77 {
		t.Fatalf("ожидали 77ms/OK через обычную пробу, получили rtt=%d ok=%v", rtt, ok)
	}
}

func TestProbeTransport_UsesLANBindForHysteria2AndWireGuard(t *testing.T) {
	oldPick := pickLANBindIPv4
	oldHY, oldHYLAN := pingHysteria2StrictProbe, pingHysteria2StrictLANProbe
	oldWG, oldWGLAN := pingWireGuardProbe, pingWireGuardLANProbe
	defer func() {
		pickLANBindIPv4 = oldPick
		pingHysteria2StrictProbe, pingHysteria2StrictLANProbe = oldHY, oldHYLAN
		pingWireGuardProbe, pingWireGuardLANProbe = oldWG, oldWGLAN
	}()

	pickLANBindIPv4 = func() (net.IP, error) { return net.IPv4(192, 168, 1, 5), nil }
	pingHysteria2StrictProbe = func(_ string, _ int, _ string) (int64, bool, string, string) {
		t.Error("HYSTERIA2: ожидали LAN-bind пробу")
		return 0, false, "", ""
	}
	pingWireGuardProbe = func(_ string, _ int) (int64, bool, string) {
		t.Error("WIREGUARD: ожидали LAN-bind пробу")
		return 0, false, ""
	}
	pingHysteria2StrictLANProbe = func(_ string, _ int, _ string) (int64, bool, string, string) {
		return 11, true, "", "quic_handshake_lan_bind"
	}
	pingWireGuardLANProbe = func(_ string, _ int) (int64, bool, string) { return 22, true, "" }

	hyNode := config.ProxyEntry{IP: "1.1.1.1", Port: 443, Type: "HYSTERIA2"}
	if rtt, ok, stage, _ := probeTransport(hyNode, hyNode.IP); !ok || rtt != 11 || stage != "quic_handshake_lan_bind" {
		t.Fatalf("HYSTERIA2: получили rtt=%d ok=%v stage=%q", rtt, ok, stage)
	}
	wgNode := config.ProxyEntry{IP: "1.1.1.1", Port: 443, Type: "AMNEZIAWG"}
	if rtt, ok, stage, _ := probeTransport(wgNode, wgNode.IP); !ok || rtt != 22 || stage != "udp" {
		t.Fatalf("AMNEZIAWG: получили rtt=%d ok=%v stage=%q", rtt, ok, stage)
	}
}

// TLS-фаза ходит тем же путём и по той же причине должна уходить с физического
// адаптера.
func TestAutoProbeDialer_BindsToLANAddressWhenAvailable(t *testing.T) {
	oldPick := pickLANBindIPv4
	defer func() { pickLANBindIPv4 = oldPick }()

	pickLANBindIPv4 = func() (net.IP, error) { return net.IPv4(192, 168, 1, 5), nil }
	d := autoProbeDialer(4*time.Second, "203.0.113.10")
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
	if d := autoProbeDialer(4*time.Second, "203.0.113.10"); d.LocalAddr != nil {
		t.Fatalf("без bind-адреса LocalAddr должен остаться nil, получили %#v", d.LocalAddr)
	}
}

// Windows отклоняет соединение к 127.0.0.1 с не-loopback источника
// («connectex: The requested address is not valid in its context»), поэтому
// привязка превратила бы достижимый локальный узел в недостижимый. Сама
// привязка нужна, чтобы уйти от маршрута через TUN, а loopback через TUN не
// ходит — пропуск ничего не стоит.
func TestAutoProbeDialer_SkipsLANBindForLoopbackTargets(t *testing.T) {
	oldPick := pickLANBindIPv4
	defer func() { pickLANBindIPv4 = oldPick }()

	pickLANBindIPv4 = func() (net.IP, error) { return net.IPv4(192, 168, 1, 5), nil }

	for _, host := range []string{"127.0.0.1", "127.0.0.53", "::1", "localhost", "LocalHost"} {
		if d := autoProbeDialer(4*time.Second, host); d.LocalAddr != nil {
			t.Errorf("для %q привязка должна быть пропущена, получили %#v", host, d.LocalAddr)
		}
	}
}

func TestProbeTransport_SkipsLANBindForLoopbackTargets(t *testing.T) {
	oldBind := autoProbeBindsToLAN
	oldTCP, oldLAN := pingTCPProbe, pingLANProbe
	defer func() {
		autoProbeBindsToLAN = oldBind
		pingTCPProbe, pingLANProbe = oldTCP, oldLAN
	}()

	autoProbeBindsToLAN = func() bool { return true }
	pingLANProbe = func(_ string, _ int) (int64, bool, string) {
		t.Error("для loopback-цели LAN-bind проба использоваться не должна")
		return 0, false, ""
	}
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) { return 1, true, "" }

	node := config.ProxyEntry{IP: "127.0.0.1", Port: 1080, Type: "SS"}
	if rtt, ok, _, _ := probeTransport(node, "127.0.0.1"); !ok || rtt != 1 {
		t.Fatalf("ожидали успех через обычную пробу, получили rtt=%d ok=%v", rtt, ok)
	}
}

// probeTransport обязан звать именно строгие варианты — иначе фикс живёт в
// функции, до которой отбор не доходит.
func TestProbeTransport_UsesStrictHysteria2Probes(t *testing.T) {
	oldBind := autoProbeBindsToLAN
	oldStrict, oldStrictLAN := pingHysteria2StrictProbe, pingHysteria2StrictLANProbe
	oldLoose, oldLooseLAN := pingHysteria2Probe, pingHysteria2LANProbe
	defer func() {
		autoProbeBindsToLAN = oldBind
		pingHysteria2StrictProbe, pingHysteria2StrictLANProbe = oldStrict, oldStrictLAN
		pingHysteria2Probe, pingHysteria2LANProbe = oldLoose, oldLooseLAN
	}()

	pingHysteria2Probe = func(_ string, _ int) (int64, bool, string, string) {
		t.Error("в отборе не должна использоваться проба с TCP-fallback")
		return 0, false, "", ""
	}
	pingHysteria2LANProbe = func(_ string, _ int) (int64, bool, string, string) {
		t.Error("в отборе не должна использоваться LAN-проба с TCP-fallback")
		return 0, false, "", ""
	}
	pingHysteria2StrictProbe = func(_ string, _ int, _ string) (int64, bool, string, string) {
		return 100, true, "", "quic_handshake"
	}
	pingHysteria2StrictLANProbe = func(_ string, _ int, _ string) (int64, bool, string, string) {
		return 200, true, "", "quic_handshake_lan_bind"
	}

	node := config.ProxyEntry{IP: "1.1.1.1", Port: 443, Type: "HYSTERIA2"}

	autoProbeBindsToLAN = func() bool { return true }
	if rtt, _, stage, _ := probeTransport(node, node.IP); rtt != 200 || stage != "quic_handshake_lan_bind" {
		t.Fatalf("с bind-адресом: rtt=%d stage=%q", rtt, stage)
	}

	autoProbeBindsToLAN = func() bool { return false }
	if rtt, _, stage, _ := probeTransport(node, node.IP); rtt != 100 || stage != "quic_handshake" {
		t.Fatalf("без bind-адреса: rtt=%d stage=%q", rtt, stage)
	}
}

// HYSTERIA2-ветка обязана дозваниваться по резолвнутому литералу (dialHost),
// но нести в SNI исходное имя узла — перепутанные местами аргументы
// проверяются раздельно, чтобы транспозиция host<->sni не прошла тесты молча.
func TestProbeTransport_HYSTERIA2DialsResolvedLiteralWithOriginalHostnameSNI(t *testing.T) {
	oldBind := autoProbeBindsToLAN
	oldStrict := pingHysteria2StrictProbe
	defer func() {
		autoProbeBindsToLAN = oldBind
		pingHysteria2StrictProbe = oldStrict
	}()
	autoProbeBindsToLAN = func() bool { return false }

	var gotHost, gotSNI string
	pingHysteria2StrictProbe = func(host string, _ int, sni string) (int64, bool, string, string) {
		gotHost, gotSNI = host, sni
		return 10, true, "", "quic_handshake"
	}

	node := config.ProxyEntry{IP: "hy2.example.test", Port: 443, Type: "HYSTERIA2"}
	probeTransport(node, "203.0.113.9")

	if gotHost != "203.0.113.9" {
		t.Fatalf("ожидали дозвон по резолвнутому литералу, получили %q", gotHost)
	}
	if gotSNI != "hy2.example.test" {
		t.Fatalf("SNI должен остаться исходным именем узла, получили %q", gotSNI)
	}
}
