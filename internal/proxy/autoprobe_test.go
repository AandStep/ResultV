package proxy

import (
	"context"
	"sync/atomic"
	"testing"

	"resultproxy-wails/internal/config"
)

func TestProbeAutoNodes_FastReturnsResultPerNodeInInputOrder(t *testing.T) {
	old := pingTCPProbe
	defer func() { pingTCPProbe = old }()

	pingTCPProbe = func(ip string, _ int) (int64, bool, string) {
		switch ip {
		case "1.1.1.1":
			return 40, true, ""
		case "2.2.2.2":
			return 0, false, "timeout"
		}
		return 90, true, ""
	}

	nodes := []config.ProxyEntry{
		{ID: "a", IP: "1.1.1.1", Port: 443, Type: "VLESS"},
		{ID: "b", IP: "2.2.2.2", Port: 443, Type: "TROJAN"},
		{ID: "c", IP: "3.3.3.3", Port: 443, Type: "VLESS"},
	}

	got := ProbeAutoNodes(context.Background(), nodes, DepthFast)

	if len(got) != 3 {
		t.Fatalf("ожидали 3 результата, получили %d", len(got))
	}
	if got[0].RTTms != 40 || !got[0].OK {
		t.Errorf("узел a: ожидали 40ms/OK, получили %+v", got[0])
	}
	if got[1].OK || got[1].Reason != "timeout" {
		t.Errorf("узел b: ожидали отказ с timeout, получили %+v", got[1])
	}
	if got[2].RTTms != 90 {
		t.Errorf("узел c: ожидали 90ms, получили %+v", got[2])
	}
	if got[0].Stage != "tcp" {
		t.Errorf("ожидали stage=tcp, получили %q", got[0].Stage)
	}
}

func TestProbeAutoNodes_SkipsSectionAndAddresslessEntries(t *testing.T) {
	old := pingTCPProbe
	defer func() { pingTCPProbe = old }()

	var calls int32
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		atomic.AddInt32(&calls, 1)
		return 10, true, ""
	}

	nodes := []config.ProxyEntry{
		{ID: "s", Type: "SECTION", Name: "Когда глушат"},
		{ID: "z", IP: "", Port: 0, Type: "VLESS"},
		{ID: "ok", IP: "1.1.1.1", Port: 443, Type: "VLESS"},
	}

	got := ProbeAutoNodes(context.Background(), nodes, DepthFast)

	if len(got) != 1 || got[0].Key == "" {
		t.Fatalf("ожидали один результат для единственного адресуемого узла, получили %+v", got)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("проба должна вызываться ровно один раз, вызвана %d раз", n)
	}
}

func TestProbeAutoNodes_UsesHysteria2ProbeForHysteria2(t *testing.T) {
	oldTCP, oldHY := pingTCPProbe, pingHysteria2Probe
	defer func() { pingTCPProbe, pingHysteria2Probe = oldTCP, oldHY }()

	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		t.Error("для HYSTERIA2 должна использоваться QUIC-проба, а не TCP")
		return 0, false, ""
	}
	pingHysteria2Probe = func(_ string, _ int) (int64, bool, string, string) {
		return 25, true, "", "quic_handshake"
	}

	got := ProbeAutoNodes(context.Background(),
		[]config.ProxyEntry{{ID: "h", IP: "1.1.1.1", Port: 443, Type: "HYSTERIA2"}},
		DepthFast)

	if len(got) != 1 || got[0].RTTms != 25 || got[0].Stage != "quic_handshake" {
		t.Fatalf("ожидали 25ms через quic_handshake, получили %+v", got)
	}
}
