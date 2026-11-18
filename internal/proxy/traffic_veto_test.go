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
	"context"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	M "github.com/sagernet/sing/common/metadata"
	"resultproxy-wails/internal/logger"
)

func TestStatusCountsOnlyVPNTraffic(t *testing.T) {
	for _, tc := range []struct {
		name, tag, host string
		wantSent        int64
		wantReceived    int64
	}{
		{name: "app exclusion", tag: appDirectTag, host: "cache.steamcontent.com"},
		{name: "direct", tag: "direct", host: "cache.steamcontent.com"},
		{name: "adaptive decision pending", tag: smartOutboundTag, host: "cache.steamcontent.com"},
		{name: "repeated hostname", tag: "proxy", host: "cache.steamcontent.com", wantSent: 6, wantReceived: 14},
		{name: "IP only", tag: "proxy", wantSent: 6, wantReceived: 14},
		{name: "SOCKS exit behind VPN", tag: socks5ExitSelector, wantSent: 6, wantReceived: 14},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &SingBoxEngine{}
			tr := &trafficTracker{
				upload: &engine.uploadBytes, download: &engine.downloadBytes,
				proxyUpload: &engine.proxyUploadBytes, proxyDownload: &engine.proxyDownloadBytes,
				log: logger.New(),
			}
			for range 2 {
				exchangeTrackedTCP(t, func(conn net.Conn) net.Conn {
					return tr.RoutedConnection(context.Background(), conn, adapter.InboundContext{
						Domain: tc.host, Destination: M.ParseSocksaddr("192.0.2.1:443"),
					}, nil, stubOutbound{tag: tc.tag})
				})
			}
			manager := &Manager{connected: true, connectedAt: time.Now(), engine: engine}
			status := manager.GetStatus()
			if status.BytesSent != tc.wantSent || status.BytesReceived != tc.wantReceived {
				t.Fatalf("dashboard sent=%d received=%d, want sent=%d received=%d",
					status.BytesSent, status.BytesReceived, tc.wantSent, tc.wantReceived)
			}
		})
	}
}

func exchangeTrackedTCP(t *testing.T, wrap func(net.Conn) net.Conn) {
	t.Helper()
	inbound, client := net.Pipe()
	defer client.Close()
	conn := wrap(inbound)
	defer conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	if err := conn.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	if err := client.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := client.Write([]byte("ask"))
		if err == nil {
			_, err = io.ReadFull(client, make([]byte, len("payload")))
		}
		done <- err
	}()
	if _, err := io.ReadFull(conn, make([]byte, len("ask"))); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("payload")); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// TestTrafficTracker_ProxyCountersOnlyWhenTracked verifies the watchdog veto's
// liveness signal is sound: proxy-only byte counters are fed ONLY by the proxy
// outbound (shouldTrack), never by direct/split-tunnel traffic — otherwise
// direct traffic could mask a genuinely dead upstream and suppress a real trip.
func TestTrafficTracker_ProxyCountersOnlyWhenTracked(t *testing.T) {
	var up, down, pUp, pDown atomic.Int64
	tr := &trafficTracker{upload: &up, download: &down, proxyUpload: &pUp, proxyDownload: &pDown}

	// Proxy outbound: both the global and the proxy-only counters are incremented.
	dc := tr.downCounters(true)
	if len(dc) != 2 || dc[0] != &down || dc[1] != &pDown {
		t.Fatalf("tracked down counters = %v, want [global, proxy]", dc)
	}
	uc := tr.upCounters(true)
	if len(uc) != 2 || uc[0] != &up || uc[1] != &pUp {
		t.Fatalf("tracked up counters = %v, want [global, proxy]", uc)
	}

	// Direct/block outbound: only the global counter — the proxy-only counter is
	// never touched, so it reflects upstream traffic exclusively.
	if dc2 := tr.downCounters(false); len(dc2) != 1 || dc2[0] != &down {
		t.Fatalf("untracked down counters = %v, want [global] only", dc2)
	}
	if uc2 := tr.upCounters(false); len(uc2) != 1 || uc2[0] != &up {
		t.Fatalf("untracked up counters = %v, want [global] only", uc2)
	}
}
