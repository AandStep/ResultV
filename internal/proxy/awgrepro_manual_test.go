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

// Manual reproducer for the AmneziaWG collapse under load. Behind a build tag
// because it runs a real tunnel against the user's own node and would be a
// terrible thing to fire from an ordinary `go test`.
//
//	go test -tags awgrepro -run TestAWGReproManual ./internal/proxy/ -v -timeout 10m
//
// It deliberately runs in PROXY mode: no TUN, no admin rights, no system DNS
// override, nothing touched outside this process. That is also what makes it an
// experiment rather than a reenactment — if the collapse reproduces here, the
// TUN inbound and everything around it is ruled out, and the fault is inside
// the WireGuard endpoint. If it does not, the fault needs the TUN path.

package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"

	"resultproxy-wails/internal/config"
)

const (
	reproUserData    = `C:\Users\andbe\AppData\Roaming\ResultV`
	reproLocalPort   = 24081
	reproDownloadURL = "https://speed.cloudflare.com/__down?bytes=25000000"
	reproSampleEvery = 2 * time.Second
)

func TestAWGReproManual(t *testing.T) {
	node := reproFindAWGNode(t)
	t.Logf("узел: %s %s:%d", node.Type, node.IP, node.Port)

	engineCfg := EngineConfig{
		Proxy:     node,
		Mode:      ProxyModeProxy,
		LocalPort: reproLocalPort,
		DataDir:   t.TempDir(),
	}
	sbCfg, err := BuildProxyModeConfig(engineCfg)
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
	if err := instance.Start(); err != nil {
		t.Fatalf("старт: %v", err)
	}
	defer func() {
		done := make(chan struct{})
		go func() {
			_ = instance.Close()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Log("Close не уложился в 10s — тот самый висящий teardown")
		}
	}()

	stop := make(chan struct{})
	defer close(stop)
	go reproSample(t, boxCtx, stop)

	// Give the handshake a moment, then pull volume through the tunnel: every
	// log so far shows the collapse starting the second real throughput is
	// asked for.
	time.Sleep(3 * time.Second)
	t.Log("=== качаем 25 МБ через туннель ===")
	reproDownload(t)

	t.Log("=== качаем второй раз (после нагрузки) ===")
	reproDownload(t)

	t.Log("=== мелкие запросы после нагрузки ===")
	for i := 0; i < 5; i++ {
		reproSmallRequest(t, i)
		time.Sleep(2 * time.Second)
	}
}

func reproFindAWGNode(t *testing.T) ProxyConfig {
	t.Helper()
	crypto, err := config.NewCryptoService(reproUserData)
	if err != nil {
		t.Fatalf("crypto: %v", err)
	}
	manager := config.NewManager(crypto)
	if err := manager.Init(reproUserData); err != nil {
		t.Fatalf("конфиг не прочитан: %v", err)
	}
	appCfg := manager.GetConfig()
	for _, entry := range appCfg.Proxies {
		if !strings.EqualFold(strings.TrimSpace(entry.Type), "amneziawg") &&
			!strings.EqualFold(strings.TrimSpace(entry.Type), "wireguard") {
			continue
		}
		return ProxyConfig{
			ID: entry.ID, IP: entry.IP, Port: entry.Port, Type: entry.Type,
			Username: entry.Username, Password: entry.Password,
			URI: entry.URI, Extra: entry.Extra, SubscriptionURL: entry.SubscriptionURL,
		}
	}
	t.Skip("в конфиге нет WireGuard/AmneziaWG-узла")
	return ProxyConfig{}
}

func reproClient() *http.Client {
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", reproLocalPort))
	return &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   90 * time.Second,
	}
}

func reproDownload(t *testing.T) {
	t.Helper()
	started := time.Now()
	resp, err := reproClient().Get(reproDownloadURL)
	if err != nil {
		t.Logf("ЗАГРУЗКА ПРОВАЛИЛАСЬ через %s: %v", time.Since(started).Round(time.Millisecond), err)
		return
	}
	defer resp.Body.Close()
	written, copyErr := io.Copy(io.Discard, resp.Body)
	elapsed := time.Since(started)
	speed := float64(written) / elapsed.Seconds() / 125000
	if copyErr != nil {
		t.Logf("ОБРЫВ на %d байт за %s (%.2f Мбит/с): %v", written, elapsed.Round(time.Millisecond), speed, copyErr)
		return
	}
	t.Logf("скачано %d байт за %s (%.2f Мбит/с)", written, elapsed.Round(time.Millisecond), speed)
}

func reproSmallRequest(t *testing.T, index int) {
	t.Helper()
	started := time.Now()
	resp, err := reproClient().Get("http://connectivitycheck.gstatic.com/generate_204")
	if err != nil {
		t.Logf("мелкий запрос %d ПРОВАЛЕН через %s: %v", index, time.Since(started).Round(time.Millisecond), err)
		return
	}
	resp.Body.Close()
	t.Logf("мелкий запрос %d: %s за %s", index, resp.Status, time.Since(started).Round(time.Millisecond))
}

func reproSample(t *testing.T, boxCtx context.Context, stop <-chan struct{}) {
	ticker := time.NewTicker(reproSampleEvery)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			line, err := wgStatsLine(boxCtx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "wg-stats: %v\n", err)
				continue
			}
			stack, stackErr := wgStackStatsLine(boxCtx)
			if stackErr != nil {
				fmt.Fprintf(os.Stderr, "%s wg-stats %s\n", time.Now().Format("15:04:05"), line)
				continue
			}
			fmt.Fprintf(os.Stderr, "%s wg-stats %s\n%s wg-stack %s\n",
				time.Now().Format("15:04:05"), line, time.Now().Format("15:04:05"), stack)
		}
	}
}
