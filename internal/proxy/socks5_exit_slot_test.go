package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/protocol/socks/socks5"

	"resultproxy-wails/internal/config"
)

type testSocksServer struct {
	port      uint16
	requested chan M.Socksaddr
}

// testSocks5Server answers without authentication. CONNECT reports the
// requested address and hangs up; UDP ASSOCIATE hands out 0.0.0.0 as the relay
// address and echoes every datagram back.
func testSocks5Server(t *testing.T, closeAtOnce bool) *testSocksServer {
	t.Helper()
	listener := testExitListener(t)
	s := &testSocksServer{port: uint16(listener.Addr().(*net.TCPAddr).Port), requested: make(chan M.Socksaddr, 8)}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go s.serve(t, conn, closeAtOnce)
		}
	}()
	return s
}

func (s *testSocksServer) serve(t *testing.T, conn net.Conn, closeAtOnce bool) {
	defer conn.Close()
	if closeAtOnce {
		return
	}
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(conn)
	if _, err := socks5.ReadAuthRequest(reader); err != nil {
		return
	}
	if socks5.WriteAuthResponse(conn, socks5.AuthResponse{Method: socks5.AuthTypeNotRequired}) != nil {
		return
	}
	request, err := socks5.ReadRequest(reader)
	if err != nil {
		return
	}
	s.requested <- request.Destination
	if request.Command != socks5.CommandUDPAssociate {
		_ = socks5.WriteResponse(conn, socks5.Response{Bind: M.Socksaddr{Addr: netip.IPv4Unspecified()}})
		return
	}
	relay, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return
	}
	defer relay.Close()
	port := uint16(relay.LocalAddr().(*net.UDPAddr).Port)
	if socks5.WriteResponse(conn, socks5.Response{Bind: M.Socksaddr{Addr: netip.IPv4Unspecified(), Port: port}}) != nil {
		return
	}
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := relay.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = relay.WriteToUDP(buf[:n], from)
		}
	}()
	_, _ = reader.ReadByte()
}

func testExitSlot(t *testing.T, raw string) *socks5ExitSlot {
	t.Helper()
	instance := testExitBox(t, raw)
	out, ok := instance.Outbound().Outbound(socks5ExitTag)
	if !ok {
		t.Fatal("exit slot missing")
	}
	slot, ok := out.(*socks5ExitSlot)
	if !ok {
		t.Fatalf("exit slot has type %T", out)
	}
	return slot
}

func directSlotConfig(port uint16) string {
	return fmt.Sprintf(`{"log":{"disabled":true},"outbounds":[
		{"type":%q,"tag":%q,"server":"127.0.0.1","server_port":%d,"version":"5","network":"tcp"}]}`,
		socks5ExitOutboundType, socks5ExitTag, port)
}

func TestSocks5ExitSlotSendsSiteName(t *testing.T) {
	server := testSocks5Server(t, false)
	slot := testExitSlot(t, directSlotConfig(server.port))
	target := M.ParseSocksaddrHostPort("192.0.2.10", 443)
	for _, tc := range []struct {
		domain string
		want   string
	}{
		{"example.test", "example.test:443"},
		{"", "192.0.2.10:443"},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		ctx = adapter.WithContext(ctx, &adapter.InboundContext{Domain: tc.domain})
		conn, err := slot.DialContext(ctx, N.NetworkTCP, target)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.Close()
		if got := (<-server.requested).String(); got != tc.want {
			t.Errorf("domain %q: SOCKS5 server asked for %s, want %s", tc.domain, got, tc.want)
		}
	}
}

func TestSocks5ExitSlotUnspecifiedRelay(t *testing.T) {
	server := testSocks5Server(t, false)
	slot := testExitSlot(t, directSlotConfig(server.port))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	target := M.ParseSocksaddrHostPort("192.0.2.20", 3478)
	packet, err := slot.probeListenPacket(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	defer packet.Close()
	_ = packet.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := packet.WriteTo([]byte("ping"), target.UDPAddr()); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	n, _, err := packet.ReadFrom(buf)
	if err != nil || string(buf[:n]) != "ping" {
		t.Fatalf("relay at 0.0.0.0 was not reached: %q %v", buf[:n], err)
	}
}

func TestSocks5ExitSlotNoResponse(t *testing.T) {
	server := testSocks5Server(t, true)
	slot := testExitSlot(t, directSlotConfig(server.port))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := slot.probeDial(ctx, M.ParseSocksaddrHostPort("192.0.2.10", 443))
	if !errors.Is(err, errSocks5NoResponse) {
		t.Fatalf("err = %v, want errSocks5NoResponse", err)
	}
}

func TestSocks5ExitSlotUDPAfterConfirm(t *testing.T) {
	targetUDP := testExitUDPEcho(t)
	socksPort := testExitPort(t)
	firstHop := testExitPort(t)
	testExitBox(t, fmt.Sprintf(`{"log":{"disabled":true},"inbounds":[
		{"type":"vless","listen":"127.0.0.1","listen_port":%d,"users":[{"uuid":%q}]},
		{"type":"socks","listen":"127.0.0.1","listen_port":%d}]}`, firstHop, testExitUUID, socksPort))
	slot := testExitSlot(t, fmt.Sprintf(`{"log":{"disabled":true},"outbounds":[
		{"type":"vless","tag":"proxy","server":"127.0.0.1","server_port":%d,"uuid":%q},
		{"type":%q,"tag":%q,"detour":"proxy","server":"127.0.0.1","server_port":%d,"version":"5","network":"tcp"}]}`,
		firstHop, testExitUUID, socks5ExitOutboundType, socks5ExitTag, socksPort))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if contains(slot.Network(), N.NetworkUDP) {
		t.Fatal("UDP advertised before it was confirmed")
	}
	if _, err := slot.ListenPacket(ctx, targetUDP); !errors.Is(err, N.ErrUnknownNetwork) {
		t.Fatalf("unconfirmed UDP: %v", err)
	}
	stale := slot.generation()
	if err := slot.setProfile(&config.Socks5ExitConfig{Host: "127.0.0.1", Port: int(socksPort)}); err != nil {
		t.Fatal(err)
	}
	slot.confirmUDP(stale, true)
	if contains(slot.Network(), N.NetworkUDP) {
		t.Fatal("a check of the previous profile enabled UDP for the new one")
	}
	slot.confirmUDP(slot.generation(), true)
	if !contains(slot.Network(), N.NetworkUDP) {
		t.Fatal("confirmed UDP is not advertised")
	}
	packet, err := slot.ListenPacket(ctx, targetUDP)
	if err != nil {
		t.Fatal(err)
	}
	defer packet.Close()
	_ = packet.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := packet.WriteTo([]byte("x"), targetUDP.UDPAddr()); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 2048)
	if n, _, err := packet.ReadFrom(buf); err != nil || string(buf[:n]) != "x" {
		t.Fatalf("UDP through the chain: %q %v", buf[:n], err)
	}
}

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}
