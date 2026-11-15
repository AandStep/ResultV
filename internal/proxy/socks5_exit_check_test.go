package proxy

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/logger"
)

func TestClassifySocks5ExitError(t *testing.T) {
	for _, tc := range []struct {
		err    error
		reason string
		code   int
	}{
		{errors.New("socks5: incorrect user name or password"), Socks5CheckBadAuth, 0},
		{fmt.Errorf("dial: %w", errSocks5UnadvertisedMethod), Socks5CheckAuthMethod, 0},
		{errors.New("socks5: unsupported auth method: 2"), Socks5CheckAuthMethod, 0},
		{errors.New("socks5: request rejected, code=5"), Socks5CheckRejected, 5},
		{fmt.Errorf("get: %w", fmt.Errorf("%w: EOF", errSocks5NoResponse)), Socks5CheckNoResponse, 0},
		{errors.New("tls: handshake failure"), Socks5CheckProbeFailed, 0},
	} {
		reason, code := classifySocks5ExitError(tc.err)
		if reason != tc.reason || code != tc.code {
			t.Errorf("%v: got %s/%d, want %s/%d", tc.err, reason, code, tc.reason, tc.code)
		}
	}
}

func TestParseCloudflareTrace(t *testing.T) {
	ip, country := parseCloudflareTrace([]byte("fl=1\nip=203.0.113.7\nloc=NL\n"))
	if ip != "203.0.113.7" || country != "NL" {
		t.Fatalf("got %q %q", ip, country)
	}
}

func testSTUNResponder(t *testing.T) int {
	t.Helper()
	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = udp.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := udp.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if n < 20 {
				continue
			}
			var txID [12]byte
			copy(txID[:], buf[8:20])
			_, _ = udp.WriteToUDP(buildSTUNBindingSuccess(txID, "203.0.113.7", 4242), from)
		}
	}()
	return udp.LocalAddr().(*net.UDPAddr).Port
}

func TestSocks5ExitCheck(t *testing.T) {
	trace := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ip=203.0.113.7\nloc=NL\n"))
	}))
	t.Cleanup(trace.Close)
	stunPort := testSTUNResponder(t)
	savedURL, savedSTUN := socks5ExitTraceURL, socks5ExitSTUNTargets
	socks5ExitTraceURL = trace.URL + "/cdn-cgi/trace"
	socks5ExitSTUNTargets = []struct {
		Host string
		Port int
	}{{"127.0.0.1", stunPort}}
	t.Cleanup(func() { socks5ExitTraceURL, socks5ExitSTUNTargets = savedURL, savedSTUN })

	firstHop := testExitPort(t)
	socksPort := testExitPort(t)
	server := testExitBox(t, fmt.Sprintf(`{"log":{"disabled":true},"inbounds":[
		{"type":"vless","listen":"127.0.0.1","listen_port":%d,"users":[{"uuid":%q}]},
		{"type":"socks","listen":"127.0.0.1","listen_port":%d,"users":[{"username":"test-user","password":"test-password"}]}]}`,
		firstHop, testExitUUID, socksPort))
	client := testExitBox(t, fmt.Sprintf(`{"log":{"disabled":true},"outbounds":[
		{"type":"vless","tag":"proxy","server":"127.0.0.1","server_port":%d,"uuid":%q},
		{"type":%q,"tag":"socks5-exit","detour":"proxy","network":"tcp","server":"127.0.0.1","server_port":%d,
		 "version":"5","username":"test-user","password":"test-password"},
		{"type":%q,"tag":"socks5-exit-next","detour":"proxy","network":"tcp","version":"5"},
		{"type":%q,"tag":%q,"outbounds":["proxy","socks5-exit","socks5-exit-next"],"default":"socks5-exit","interrupt_exist_connections":true}
	]}`, firstHop, testExitUUID, socks5ExitOutboundType, socksPort, socks5ExitOutboundType, socks5ExitSelectorType, socks5ExitSelector))
	engine := &SingBoxEngine{instance: client, log: logger.New()}
	engine.running.Store(true)
	check := func() Socks5ExitCheck {
		t.Helper()
		return engine.CheckSocks5Exit(t.Context())
	}

	got := check()
	if !got.OK || got.IP != "203.0.113.7" || got.Country != "NL" || !got.UDP {
		t.Fatalf("healthy exit: %+v", got)
	}
	out, _ := client.Outbound().Outbound(socks5ExitTag)
	if !contains(out.(*socks5ExitSlot).Network(), "udp") {
		t.Fatal("a confirmed check did not enable UDP")
	}

	profile := config.Socks5ExitConfig{Enabled: true, Host: "127.0.0.1", Port: int(socksPort), Username: "test-user", Password: "wrong"}
	if err := engine.SetSocks5Exit(&profile); err != nil {
		t.Fatal(err)
	}
	if got := check(); got.OK || got.Reason != Socks5CheckBadAuth {
		t.Fatalf("wrong password: %+v", got)
	}

	closed := testExitPort(t)
	profile.Port, profile.Password = int(closed), "test-password"
	if err := engine.SetSocks5Exit(&profile); err != nil {
		t.Fatal(err)
	}
	if got := check(); got.OK || got.Reason != Socks5CheckNoResponse {
		t.Fatalf("closed SOCKS5 port: %+v", got)
	}

	if err := engine.SetSocks5Exit(&config.Socks5ExitConfig{}); err != nil {
		t.Fatal(err)
	}
	if got := check(); got.Reason != Socks5CheckInactive {
		t.Fatalf("exit off: %+v", got)
	}

	profile.Port = int(socksPort)
	if err := engine.SetSocks5Exit(&profile); err != nil {
		t.Fatal(err)
	}
	_ = server.Close()
	if got := check(); got.OK || got.Reason != Socks5CheckFirstHopDown {
		t.Fatalf("first hop down: %+v", got)
	}
}

func TestManagerSocks5ExitState(t *testing.T) {
	m := NewManager(logger.New())
	listed := &config.Socks5ExitConfig{Enabled: true, Host: "192.0.2.2", Port: 1080, Sites: []string{"example.test"}}
	plain := &config.Socks5ExitConfig{Enabled: true, Host: "192.0.2.2", Port: 1080}
	for _, tc := range []struct {
		name              string
		connected, wired  bool
		profile, built    *config.Socks5ExitConfig
		active, coversAll bool
	}{
		{"disconnected", false, false, plain, nil, false, false},
		{"plain session", true, true, plain, plain, true, true},
		{"listed session", true, true, listed, listed, true, false},
		{"turned off live", true, true, &config.Socks5ExitConfig{Host: "192.0.2.2", Port: 1080}, plain, false, false},
	} {
		m.connected, m.socks5ExitWired, m.socks5Exit, m.socks5ExitBuilt = tc.connected, tc.wired, tc.profile, tc.built
		if m.Socks5ExitActive() != tc.active || m.Socks5ExitCoversAllVPN() != tc.coversAll {
			t.Errorf("%s: active=%t coversAll=%t", tc.name, m.Socks5ExitActive(), m.Socks5ExitCoversAllVPN())
		}
	}
	if !strings.Contains(m.CheckSocks5Exit(t.Context()).Reason, Socks5CheckInactive) {
		t.Fatal("a check without an engine must report the exit as inactive")
	}
}
