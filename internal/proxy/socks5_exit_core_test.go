package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/protocol/socks"

	"resultproxy-wails/internal/config"
)

const (
	testExitGroup = socks5ExitSelector
	testExitUUID  = "90e4099f-aaf7-452d-9bc5-f9f2bb82c5b0"
)

type testExitSwitch interface {
	adapter.Outbound
	Now() string
	SelectOutbound(string) bool
}

func TestSocks5ExitCoreSwitch(t *testing.T) {
	target := testExitEcho(t)
	targetUDP := testExitUDPEcho(t)
	control := testExitEcho(t)
	secondHop := testExitPort(t)
	delayedHop := testExitHoldGreeting(t, secondHop)
	firstHop := testExitPort(t)

	// Only the VLESS server can map a synthetic SOCKS port to the real one.
	// Reaching the authenticated SOCKS listener proves the VLESS hop was used.
	const syntheticPort = 1
	testExitBox(t, fmt.Sprintf(`{
		"log":{"disabled":true},
		"inbounds":[
			{"type":"vless","listen":"127.0.0.1","listen_port":%d,"users":[{"uuid":%q}]},
			{"type":"socks","listen":"127.0.0.1","listen_port":%d,"users":[{"username":"test-user","password":"test-password"}]}
		],
		"route":{"rules":[{"port":%d,"action":"route-options","override_port":%d}]}
	}`, firstHop, testExitUUID, secondHop, syntheticPort, delayedHop.port))

	cachePath := filepath.Join(t.TempDir(), "cache.db")
	startClient := func(t *testing.T, selectorType, initial, password string) (*box.Box, testExitSwitch, *socks.Client) {
		t.Helper()
		localPort := testExitPort(t)
		instance := testExitBox(t, fmt.Sprintf(`{
			"log":{"disabled":true},
			"experimental":{"cache_file":{"enabled":true,"path":%s}},
			"inbounds":[{"type":"socks","listen":"127.0.0.1","listen_port":%d}],
			"outbounds":[
				{"type":"vless","tag":"proxy","server":"127.0.0.1","server_port":%d,"uuid":%q},
				{"type":%q,"tag":"socks5-exit","detour":"proxy","network":"tcp","server":"127.0.0.1","server_port":%d,
				 "version":"5","username":"test-user","password":%q},
				{"type":%q,"tag":"socks5-exit-next","detour":"proxy","network":"tcp","version":"5"},
				{"type":%q,"tag":%q,"outbounds":["proxy","socks5-exit","socks5-exit-next"],"default":%q,"interrupt_exist_connections":true}
			],
			"route":{"rules":[{"port":[%d,%d],"action":"route","outbound":%q}],"final":"proxy"}
		}`, testExitJSONString(cachePath), localPort, firstHop, testExitUUID, socks5ExitOutboundType, syntheticPort, password, socks5ExitOutboundType, selectorType, testExitGroup, initial, target.Port, targetUDP.Port, testExitGroup))
		out, ok := instance.Outbound().Outbound(testExitGroup)
		if !ok {
			t.Fatal("exit selector missing")
		}
		return instance, out.(testExitSwitch), socks.NewClient(N.SystemDialer, testExitAddress(localPort), socks.Version5, "", "")
	}

	t.Run("routes and closes only selected flows", func(t *testing.T) {
		instance, selector, client := startClient(t, socks5ExitSelectorType, "proxy", "test-password")
		if _, unwrapsToEndpoint := selector.(adapter.OutboundGroup); unwrapsToEndpoint {
			t.Fatal("the TUN router can bypass live switching by caching raw endpoint flows")
		}
		selected := testExitDial(t, client, N.NetworkTCP, target)
		unselected := testExitDial(t, client, N.NetworkTCP, control)
		testExitRoundTrip(t, selected)
		testExitRoundTrip(t, unselected)
		testExitRoundTrip(t, testExitDial(t, client, N.NetworkUDP, targetUDP))

		if !selector.SelectOutbound("socks5-exit") {
			t.Fatal("cannot select SOCKS5")
		}
		testExitClosed(t, selected)
		testExitRoundTrip(t, unselected)
		chained := testExitDial(t, client, N.NetworkTCP, target)
		testExitRoundTrip(t, chained)
		// The router can have checked the previous selection's network
		// support. The exit itself must refuse a late UDP dial.
		out, _ := instance.Outbound().Outbound("socks5-exit")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		packet, err := out.ListenPacket(ctx, targetUDP)
		cancel()
		if packet != nil {
			_ = packet.Close()
		}
		if !errors.Is(err, N.ErrUnknownNetwork) {
			t.Fatalf("SOCKS5 exit did not reject UDP at its dial boundary: %v", err)
		}
		udp := testExitDial(t, client, N.NetworkUDP, targetUDP)
		if _, err := udp.Write([]byte("x")); err == nil {
			_ = udp.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
			if _, err := udp.Read(make([]byte, 1)); err == nil {
				t.Fatal("selected UDP reached the target via a TCP-only exit")
			}
		}

		release := make(chan struct{})
		hold := make(chan struct{})
		delayedHop.holds <- testExitGreetingHold{entered: hold, release: release}
		t.Cleanup(func() {
			select {
			case <-release:
			default:
				close(release)
			}
		})
		pending := make(chan error, 1)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, err := client.DialContext(ctx, N.NetworkTCP, target)
			if conn != nil {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				if err == nil {
					_, err = conn.Write([]byte("x"))
					if err == nil {
						_, err = io.ReadFull(conn, make([]byte, 1))
					}
				}
			}
			pending <- err
		}()
		select {
		case <-hold:
		case <-time.After(3 * time.Second):
			t.Fatal("pending dial did not reach the second hop")
		}
		if !selector.SelectOutbound("proxy") {
			t.Fatal("cannot restore VPN-only exit")
		}
		close(release)
		select {
		case err := <-pending:
			if err == nil {
				t.Fatal("dial initiated on the previous exit exchanged data after switching")
			}
		case <-time.After(4 * time.Second):
			t.Fatal("pending dial did not terminate")
		}
		testExitClosed(t, chained)
		testExitRoundTrip(t, unselected)
		testExitRoundTrip(t, testExitDial(t, client, N.NetworkTCP, target))
	})

	t.Run("invalid SOCKS credentials fail closed", func(t *testing.T) {
		_, _, client := startClient(t, socks5ExitSelectorType, "socks5-exit", "wrong-password")
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		conn, err := client.DialContext(ctx, N.NetworkTCP, target)
		if conn != nil {
			_ = conn.Close()
		}
		if err == nil {
			t.Fatal("target was reachable with invalid SOCKS5 credentials")
		}
	})

	t.Run("enable first and edit live", func(t *testing.T) {
		instance, _, client := startClient(t, socks5ExitSelectorType, "proxy", "test-password")
		engine := &SingBoxEngine{instance: instance}
		engine.running.Store(true)
		requireBlocked := func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			conn, err := client.DialContext(ctx, N.NetworkTCP, target)
			if conn != nil {
				_ = conn.Close()
			}
			if err == nil {
				t.Fatal("an enabled but invalid exit must fail closed")
			}
		}
		if err := engine.SetSocks5Exit(&config.Socks5ExitConfig{Enabled: true}); err != nil {
			t.Fatal(err)
		}
		requireBlocked()
		profile := config.Socks5ExitConfig{Enabled: true, Host: "127.0.0.1", Port: syntheticPort, Username: "test-user", Password: "test-password"}
		if err := engine.SetSocks5Exit(&profile); err != nil {
			t.Fatal(err)
		}
		flow := testExitDial(t, client, N.NetworkTCP, target)
		testExitRoundTrip(t, flow)
		bad := profile
		bad.Password = "wrong-password"
		if err := engine.SetSocks5Exit(&bad); err != nil {
			t.Fatal(err)
		}
		testExitClosed(t, flow)
		requireBlocked()
		profile.Enabled = false
		if err := engine.SetSocks5Exit(&profile); err != nil {
			t.Fatal(err)
		}
		testExitRoundTrip(t, testExitDial(t, client, N.NetworkTCP, target))
	})

	t.Run("rejects an unadvertised auth method", func(t *testing.T) {
		listener := testExitListener(t)
		connectSent := make(chan bool, 1)
		go func() {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
			greeting := make([]byte, 3)
			if _, err := io.ReadFull(conn, greeting); err != nil || !slices.Equal(greeting, []byte{5, 1, 2}) {
				connectSent <- true
				return
			}
			_, _ = conn.Write([]byte{5, 0})
			n, _ := conn.Read(make([]byte, 1))
			connectSent <- n != 0
		}()
		instance, _, client := startClient(t, socks5ExitSelectorType, "proxy", "test-password")
		engine := &SingBoxEngine{instance: instance}
		engine.running.Store(true)
		profile := config.Socks5ExitConfig{
			Enabled: true, Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port,
			Username: "test-user", Password: "test-password",
		}
		if err := engine.SetSocks5Exit(&profile); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		conn, err := client.DialContext(ctx, N.NetworkTCP, target)
		if conn != nil {
			_ = conn.Close()
		}
		if err == nil {
			t.Fatal("unexpectedly connected with an unadvertised auth method")
		}
		select {
		case sent := <-connectSent:
			if sent {
				t.Fatal("SOCKS5 accepted a no-auth response when only password auth was offered")
			}
		case <-ctx.Done():
			t.Fatal("SOCKS5 server never saw the greeting")
		}
	})

	for _, scenario := range []struct {
		name, stale, initial string
	}{
		{"new session ignores stale SOCKS5 choice", "socks5-exit", "proxy"},
		{"restored session ignores stale VPN-only choice", "proxy", "socks5-exit"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			instance, selector, _ := startClient(t, "selector", scenario.initial, "test-password")
			if !selector.SelectOutbound(scenario.stale) {
				t.Fatal("cannot seed the old selector choice")
			}
			if err := instance.Close(); err != nil {
				t.Fatal(err)
			}
			_, fresh, _ := startClient(t, socks5ExitSelectorType, scenario.initial, "test-password")
			if got := fresh.Now(); got != scenario.initial {
				t.Fatalf("session started on %s, want %s", got, scenario.initial)
			}
		})
	}
}

func testExitBox(t *testing.T, raw string) *box.Box {
	t.Helper()
	ctx := extendedBoxContext(context.Background())
	var opts option.Options
	if err := singjson.UnmarshalContext(ctx, []byte(raw), &opts); err != nil {
		t.Fatal(err)
	}
	instance, err := box.New(box.Options{Context: ctx, Options: opts})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = instance.Close() })
	if err := instance.Start(); err != nil {
		t.Fatal(err)
	}
	return instance
}

func testExitAddress(port uint16) M.Socksaddr {
	return M.Socksaddr{Addr: netip.MustParseAddr("127.0.0.1"), Port: port}
}

func testExitJSONString(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

func testExitPort(t *testing.T) uint16 {
	t.Helper()
	listener := testExitListener(t)
	_ = listener.Close()
	return uint16(listener.Addr().(*net.TCPAddr).Port)
}

func testExitListener(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func testExitEcho(t *testing.T) M.Socksaddr {
	t.Helper()
	listener := testExitListener(t)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return testExitAddress(uint16(listener.Addr().(*net.TCPAddr).Port))
}

func testExitUDPEcho(t *testing.T) M.Socksaddr {
	t.Helper()
	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = udp.Close() })
	go func() {
		data := make([]byte, 64)
		for {
			n, addr, err := udp.ReadFromUDP(data)
			if err != nil {
				return
			}
			_, _ = udp.WriteToUDP(data[:n], addr)
		}
	}()
	return testExitAddress(uint16(udp.LocalAddr().(*net.UDPAddr).Port))
}

type testExitGreetingHold struct {
	entered chan struct{}
	release chan struct{}
}

type testExitDelayedHop struct {
	port  uint16
	holds chan testExitGreetingHold
}

func testExitHoldGreeting(t *testing.T, forwardPort uint16) testExitDelayedHop {
	t.Helper()
	listener := testExitListener(t)
	hop := testExitDelayedHop{port: uint16(listener.Addr().(*net.TCPAddr).Port), holds: make(chan testExitGreetingHold, 1)}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				select {
				case hold := <-hop.holds:
					close(hold.entered)
					<-hold.release
				default:
				}
				peer, err := net.DialTimeout("tcp", testExitAddress(forwardPort).String(), 2*time.Second)
				if err != nil {
					return
				}
				defer peer.Close()
				go func() {
					_, _ = io.Copy(peer, conn)
					_ = peer.Close()
				}()
				_, _ = io.Copy(conn, peer)
			}()
		}
	}()
	return hop
}

func testExitDial(t *testing.T, client *socks.Client, network string, dest M.Socksaddr) net.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := client.DialContext(ctx, network, dest)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func testExitRoundTrip(t *testing.T, conn net.Conn) {
	t.Helper()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if got, err := io.ReadFull(conn, make([]byte, 1)); got != 1 || err != nil {
		t.Fatalf("echo returned %d bytes: %v", got, err)
	}
}

func testExitClosed(t *testing.T, conn net.Conn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, err := conn.Read(make([]byte, 1))
	var timedOut net.Error
	if err == nil || errors.As(err, &timedOut) && timedOut.Timeout() {
		t.Fatalf("old flow was not closed: %v", err)
	}
}
