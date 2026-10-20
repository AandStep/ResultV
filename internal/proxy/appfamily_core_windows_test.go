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

//go:build windows

package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/service"

	mDNS "github.com/miekg/dns"

	"resultproxy-wails/internal/system/processtree"
)

const appDirectTestHost = "game.example"

// startFamilyCore runs a Global-mode config in-process, without the TUN, with
// the node unreachable and the system resolver replaced by a record for
// appDirectTestHost. A connection that reaches the echo server can only have
// gone through the family rule and app-direct.
func startFamilyCore(t *testing.T, excluded string, nodeAnswers map[string][]string) (int, context.Context) {
	t.Helper()
	cfg := EngineConfig{
		Mode:         ProxyModeTunnel,
		RoutingMode:  ModeGlobal,
		Proxy:        ProxyConfig{Type: "ss", IP: "203.0.113.7", Port: 443, Password: "p"},
		AppWhitelist: []string{excluded},
		DataDir:      t.TempDir(),
		LocalPort:    getFreeLocalPort(24081),
	}
	sb := mustBuildTunnelModeConfig(t, cfg)
	nodeResolver := firstDetourServerTag(sb.DNS.Servers, "proxy")
	for i, srv := range sb.DNS.Servers {
		if srv.Tag == nodeResolver && nodeAnswers != nil {
			sb.DNS.Servers[i] = SBDNSServer{Type: "hosts", Tag: nodeResolver, Predefined: nodeAnswers}
		}
		if srv.Tag == "local" {
			sb.DNS.Servers[i] = SBDNSServer{
				Type:       "hosts",
				Tag:        "local",
				Predefined: map[string][]string{appDirectTestHost: {"127.0.0.1"}},
			}
		}
	}
	var inbounds []SBInbound
	for _, in := range sb.Inbounds {
		if in.Type != "tun" {
			inbounds = append(inbounds, in)
		}
	}
	sb.Inbounds = inbounds
	sb.Experimental = nil

	raw, err := json.Marshal(sb)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	lists := newAppLists(cfg.AppWhitelist, appFamilyVPNList(cfg))
	boxCtx := service.ContextWith[adapter.PlatformInterface](ctx, newFamilyPlatform(newFamilyResolver(lists, processtree.NewLive())))
	boxCtx = extendedBoxContext(boxCtx)
	var opts option.Options
	if err := singjson.UnmarshalContext(boxCtx, raw, &opts); err != nil {
		t.Fatalf("core rejected the config: %v\n%s", err, raw)
	}
	instance, err := box.New(box.Options{Context: boxCtx, Options: opts})
	if err != nil {
		t.Fatalf("box.New: %v", err)
	}
	if err := instance.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		done := make(chan struct{})
		go func() { _ = instance.Close(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
		}
		cancel()
	})
	return cfg.LocalPort, boxCtx
}

func echoServer(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

// echoThrough opens target through the core's HTTP inbound and reports
// whether the echo came back.
func echoThrough(proxyPort int, target string) error {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort), 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
	if _, err := fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target); err != nil {
		return err
	}
	r := bufio.NewReader(conn)
	status, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.Contains(status, " 200") {
		return fmt.Errorf("CONNECT answered %q", strings.TrimSpace(status))
	}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return err
		}
		if line == "\r\n" {
			break
		}
	}
	if _, err := conn.Write([]byte("ping")); err != nil {
		return err
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	if string(buf) != "ping" {
		return fmt.Errorf("echo returned %q", buf)
	}
	return nil
}

// The test process stands in for a game: once its own executable is on the
// exclusion list, its connection must leave through app-direct, resolved by
// the system resolver, while the node is unreachable.
func TestFamilyCore_ExcludedProcessLeavesThroughAppDirect(t *testing.T) {
	saved := selfPID
	selfPID = 0
	defer func() { selfPID = saved }()

	exe, _ := os.Executable()
	echo := echoServer(t)
	port, _ := startFamilyCore(t, filepath.Base(exe), nil)
	if err := echoThrough(port, fmt.Sprintf("%s:%d", appDirectTestHost, echo)); err != nil {
		t.Fatalf("excluded process did not get through app-direct: %v", err)
	}
}

// The control: the same connection from a process outside every family goes
// to the node and fails, so the success above means something.
func TestFamilyCore_OtherProcessStaysOnTheNode(t *testing.T) {
	saved := selfPID
	selfPID = 0
	defer func() { selfPID = saved }()

	echo := echoServer(t)
	port, _ := startFamilyCore(t, "steam.exe", nil)
	if err := echoThrough(port, fmt.Sprintf("%s:%d", appDirectTestHost, echo)); err == nil {
		t.Fatal("a process outside the family reached the echo server without the node")
	}
}

// Inheritance through the real core: curl, started by the test process, is
// not on any list, but its parent is.
func TestFamilyCore_ChildInheritsTheExclusion(t *testing.T) {
	curl, err := exec.LookPath("curl.exe")
	if err != nil {
		t.Skip("curl.exe not found")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("reached"))
	}))
	defer srv.Close()
	srvPort := srv.Listener.Addr().(*net.TCPAddr).Port

	exe, _ := os.Executable()
	port, _ := startFamilyCore(t, filepath.Base(exe), nil)
	out, err := exec.Command(curl, "-s", "--max-time", "5",
		"-x", fmt.Sprintf("http://127.0.0.1:%d", port),
		fmt.Sprintf("http://%s:%d/", appDirectTestHost, srvPort)).Output()
	if err != nil || string(out) != "reached" {
		t.Fatalf("child process did not inherit the exclusion: %v, output %q", err, out)
	}
}

// The Global-mode case end to end: the lookup is answered through the node with
// an address the excluded app cannot use, the app connects to that address, and
// app-direct takes it to the local answer instead.
func TestFamilyCore_AddressFromTheNodeResolverIsLocalized(t *testing.T) {
	saved := selfPID
	selfPID = 0
	defer func() { selfPID = saved }()

	const nodeRegion = "203.0.113.50"
	exe, _ := os.Executable()
	echo := echoServer(t)
	port, boxCtx := startFamilyCore(t, filepath.Base(exe), map[string][]string{appDirectTestHost: {nodeRegion}})

	msg := new(mDNS.Msg)
	msg.SetQuestion(mDNS.Fqdn(appDirectTestHost), mDNS.TypeA)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := service.FromContext[adapter.DNSRouter](boxCtx).Exchange(ctx, msg, adapter.DNSQueryOptions{})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(resp.Answer) != 1 || resp.Answer[0].(*mDNS.A).A.String() != nodeRegion {
		t.Fatalf("the node resolver answered %v, want %s", resp.Answer, nodeRegion)
	}

	if err := echoThrough(port, fmt.Sprintf("%s:%d", nodeRegion, echo)); err != nil {
		t.Fatalf("the address from the node resolver was not swapped for the local one: %v", err)
	}
}
