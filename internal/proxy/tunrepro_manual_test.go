//go:build awgrepro

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

// Tunnel-mode bench. Needs administrator rights (TUN), so it is behind the same
// build tag as the proxy-mode reproducer and is never part of a normal run:
//
//	go test -tags "awgrepro,with_gvisor,with_utls,with_clash_api,with_quic,with_wireguard,with_naive_outbound,with_purego,with_grpc" \
//	  -run TestTunReproManual ./internal/proxy/ -v -timeout 20m
//
// Why it exists: proxy mode moves 109 Mbit/s through the same node on the same
// 1.14 engine, and tunnel mode collapses within a minute of real throughput. So
// the fault is somewhere in the TUN inbound, and the remaining suspects each
// have a switch. This runs them back to back against the same node and prints
// one table, instead of asking a person to reproduce a hang by hand each time.
//
// Every variant tears its tunnel down and clears leftovers before the next, and
// the whole thing clears leftovers again on the way out — a wedged adapter is
// exactly the failure being investigated, and it must not be left behind.

package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
)

type tunVariant struct {
	name string
	// apply mutates the engine config for this variant.
	apply func(cfg *EngineConfig)
}

func TestTunReproManual(t *testing.T) {
	node := reproFindAWGNode(t)
	// Connect pins the server's addresses before starting the engine, and the
	// tunnel depends on it: the route rule that keeps the node's own UDP out of
	// the tunnel is built from the pinned set, and without it every WireGuard
	// packet is routed into the TUN it is supposed to carry. The first version
	// of this bench skipped the pin and measured nothing — rx_bytes and
	// last_handshake_time_sec stayed at zero in all three variants because the
	// tunnel never came up at all, while the downloads ran outside it.
	if node.ResolvedIP == "" {
		node.ResolvedIP = resolvePinnedServerIP(node.IP)
	}
	if len(node.ResolvedIPs) == 0 {
		node.ResolvedIPs = resolveAllServerIPs(node.IP)
	}
	t.Logf("узел: %s %s:%d пин=%s все=%v", node.Type, node.IP, node.Port, node.ResolvedIP, node.ResolvedIPs)
	if node.ResolvedIP == "" {
		t.Fatal("адрес узла не разрешён — без пина стенд измеряет не туннель")
	}
	t.Log("ВНИМАНИЕ: закройте ResultV перед прогоном — адаптер один на всех")

	variants := []tunVariant{
		{
			// The shape the user runs today.
			name:  "как сейчас",
			apply: func(cfg *EngineConfig) { cfg.DNSLeakProtection = true },
		},
		{
			// BuildTunnelModeConfig skips route_exclude_address for WireGuard
			// nodes, so the node's own UDP enters the TUN and is let out again by
			// a routing rule — a loop the logs show plainly ("inbound packet
			// connection to <server>:3306"). It worked on 1.13; on 1.14 every one
			// of those packets now goes through a rewritten UDP NAT and flow
			// dispatcher, twice per byte carried. Excluding the server from the
			// tunnel removes the loop instead of making it cheaper.
			name: "с исключением сервера из TUN",
			apply: func(cfg *EngineConfig) {
				cfg.DNSLeakProtection = true
				t.Setenv("RESULTV_WG_ROUTE_EXCLUDE", "1")
			},
		},
	}

	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			runTunVariant(t, node, variant)
		})
		clearLeftovers(t)
		time.Sleep(2 * time.Second)
	}
}

func runTunVariant(t *testing.T, node ProxyConfig, variant tunVariant) {
	t.Helper()
	cfg := EngineConfig{
		Proxy:   node,
		Mode:    ProxyModeTunnel,
		DataDir: t.TempDir(),
		// Routing stays out of it: the question is whether the TUN inbound can
		// carry throughput at all, and Smart's rule-set only adds noise.
		RoutingMode: ModeGlobal,
	}
	variant.apply(&cfg)

	sbCfg, err := BuildTunnelModeConfig(cfg)
	if err != nil {
		t.Fatalf("конфиг не собрался: %v", err)
	}
	configJSON, err := json.Marshal(sbCfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	boxCtx := extendedBoxContext(ctx)

	var options option.Options
	if err := singjson.UnmarshalContext(boxCtx, configJSON, &options); err != nil {
		t.Fatalf("ядро не приняло конфиг: %v", err)
	}
	instance, err := box.New(box.Options{Context: boxCtx, Options: options})
	if err != nil {
		t.Fatalf("box.New: %v", err)
	}
	started := false
	defer func() {
		closeStarted := time.Now()
		done := make(chan struct{})
		go func() {
			_ = instance.Close()
			close(done)
		}()
		select {
		case <-done:
			t.Logf("Close за %s", time.Since(closeStarted).Round(time.Millisecond))
		case <-time.After(15 * time.Second):
			t.Log("Close НЕ УЛОЖИЛСЯ в 15s — тот самый висящий teardown")
		}
	}()
	if err := instance.Start(); err != nil {
		t.Fatalf("старт (нужны права администратора): %v", err)
	}
	started = true
	_ = started

	stop := make(chan struct{})
	defer close(stop)
	go tunSample(t, boxCtx, stop)

	time.Sleep(4 * time.Second)
	// Through the tunnel the whole machine is routed, so the download goes out
	// directly rather than through a local proxy port.
	for attempt := 1; attempt <= 3; attempt++ {
		tunDownload(t, attempt)
	}
	t.Log("--- мелкие запросы после нагрузки ---")
	for i := 0; i < 3; i++ {
		tunSmallRequest(t, i)
		time.Sleep(3 * time.Second)
	}
}

func tunDownload(t *testing.T, attempt int) {
	t.Helper()
	client := &http.Client{Timeout: 60 * time.Second}
	started := time.Now()
	resp, err := client.Get(reproDownloadURL)
	if err != nil {
		t.Errorf("загрузка %d ПРОВАЛЕНА через %s: %v", attempt, time.Since(started).Round(time.Millisecond), err)
		return
	}
	defer resp.Body.Close()
	written, copyErr := io.Copy(io.Discard, resp.Body)
	elapsed := time.Since(started)
	speed := float64(written) / elapsed.Seconds() / 125000
	if copyErr != nil {
		t.Errorf("загрузка %d ОБОРВАНА на %d байт за %s (%.2f Мбит/с): %v",
			attempt, written, elapsed.Round(time.Millisecond), speed, copyErr)
		return
	}
	t.Logf("загрузка %d: %d байт за %s (%.2f Мбит/с)", attempt, written, elapsed.Round(time.Millisecond), speed)
}

func tunSmallRequest(t *testing.T, index int) {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	started := time.Now()
	resp, err := client.Get("http://connectivitycheck.gstatic.com/generate_204")
	if err != nil {
		t.Errorf("мелкий запрос %d ПРОВАЛЕН через %s: %v", index, time.Since(started).Round(time.Millisecond), err)
		return
	}
	resp.Body.Close()
	t.Logf("мелкий запрос %d: %s за %s", index, resp.Status, time.Since(started).Round(time.Millisecond))
}

func tunSample(t *testing.T, boxCtx context.Context, stop <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			line, err := wgStatsLine(boxCtx)
			if err != nil {
				continue
			}
			stack, stackErr := wgStackStatsLine(boxCtx)
			if stackErr != nil {
				t.Logf("wg-stats %s", line)
				continue
			}
			t.Logf("wg-stats %s", line)
			t.Logf("wg-stack %s", stack)
		}
	}
}

func clearLeftovers(t *testing.T) {
	t.Helper()
	if !hasLeftoverTun() {
		return
	}
	if err := clearLeftoverTun(); err != nil {
		t.Logf("остатки TUN не сняты: %v", err)
		return
	}
	t.Log("сняты остатки TUN")
}

// unusedInTunRepro keeps the proxy-mode helpers referenced when only this file
// is being read; both reproducers share reproFindAWGNode and reproDownloadURL.
var _ = url.Parse
var _ = fmt.Sprintf
