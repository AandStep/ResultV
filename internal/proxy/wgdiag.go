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
	"reflect"
	"strings"
	"time"

	wgprotocol "github.com/sagernet/sing-box/protocol/wireguard"
)

// The question a WireGuard log cannot answer on its own: when a tunnel goes
// quiet, did our packets leave, and did anything come back?
//
// The core logs handshakes and keepalives, and nothing at all for data — so a
// session that stops carrying traffic while the device still believes it is up
// produces total silence, which is what the 14.09.2026 logs show: one handshake
// at connect, then not a single line until teardown a minute and a half later,
// with the browser hammering the tunnel the whole time.
//
// The device's own counters do answer it. tx_bytes rising with rx_bytes flat
// means our packets go out and nothing returns — a path or peer problem.
// Both flat means the packets never reached the device at all, and the fault is
// above WireGuard, in the stack that feeds it. last_handshake_time_sec says
// whether the device ever tried to re-establish the session.
//
// Sampling runs only while the diagnostic core log is open, so it costs nothing
// in an ordinary session.
const (
	wgStatsInterval = 5 * time.Second
	// wgStatsFields is what gets written down. Deliberately a whitelist: the
	// UAPI dump also carries private_key and preshared_key, and this file is
	// opened to be sent to someone.
	wgStatsPrefix = "wg-stats"
)

var wgStatsFields = []string{
	"endpoint",
	"last_handshake_time_sec",
	"tx_bytes",
	"rx_bytes",
	"persistent_keepalive_interval",
	"protocol_version",
}

// ipcGetter is the read half of the device's UAPI.
type ipcGetter interface {
	IpcGet() (string, error)
}

// startWGStatsSampler logs the WireGuard device counters every few seconds for
// as long as ctx lives. No-op unless there is somewhere to write and a
// WireGuard device to read.
func startWGStatsSampler(ctx context.Context, boxCtx context.Context, core *coreLogFile) {
	if core == nil || boxCtx == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(wgStatsInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				line, err := wgStatsLine(boxCtx)
				if err != nil {
					// Reported once and then dropped: a node that is not
					// WireGuard has nothing to sample, and repeating that every
					// five seconds would bury the log this exists to fill.
					core.writeLine(wgStatsPrefix + " недоступны: " + err.Error())
					return
				}
				core.writeLine(wgStatsPrefix + " " + line)
			}
		}
	}()
}

// wgStatsLine reads the device's UAPI dump and keeps the fields worth having.
func wgStatsLine(boxCtx context.Context) (string, error) {
	endpoint, err := wgEndpointFrom(boxCtx)
	if err != nil {
		return "", err
	}
	deviceValue, err := awg31Device(endpoint)
	if err != nil {
		return "", err
	}
	getter, ok := deviceValue.(ipcGetter)
	if !ok {
		return "", &awg31Error{"устройство не отдаёт UAPI"}
	}
	dump, err := getter.IpcGet()
	if err != nil {
		return "", &awg31Error{"UAPI не прочитан: " + err.Error()}
	}
	return filterWGStats(dump), nil
}

// filterWGStats keeps only the whitelisted keys, in one line.
func filterWGStats(dump string) string {
	var kept []string
	for _, line := range strings.Split(dump, "\n") {
		line = strings.TrimSpace(line)
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		for _, want := range wgStatsFields {
			if key == want {
				kept = append(kept, line)
				break
			}
		}
	}
	if len(kept) == 0 {
		return "(пусто)"
	}
	return strings.Join(kept, " ")
}

// wgEndpointFrom resolves the WireGuard endpoint the session is running on.
func wgEndpointFrom(boxCtx context.Context) (*wgprotocol.Endpoint, error) {
	manager := endpointManagerFrom(boxCtx)
	if manager == nil {
		return nil, &awg31Error{"ядро не отдало менеджер эндпоинтов"}
	}
	ep, loaded := manager.Get(wireguardEndpointTag)
	if !loaded {
		return nil, &awg31Error{"эндпоинт " + wireguardEndpointTag + " не найден"}
	}
	wgEndpoint, ok := ep.(*wgprotocol.Endpoint)
	if !ok {
		return nil, &awg31Error{"узел не WireGuard"}
	}
	return wgEndpoint, nil
}

// awg31DeviceValue exposes the device as an untyped value so both the 3.1
// writer and the stats reader can ask it for their own interface.
func awg31DeviceInterface(wgEndpoint *wgprotocol.Endpoint) (any, error) {
	transportEndpoint, err := unexportedField(reflect.ValueOf(wgEndpoint), "endpoint")
	if err != nil {
		return nil, &awg31Error{"protocol/wireguard.Endpoint: " + err.Error()}
	}
	if transportEndpoint.Kind() == reflect.Pointer && transportEndpoint.IsNil() {
		return nil, &awg31Error{"устройство WireGuard ещё не создано"}
	}
	deviceValue, err := unexportedField(transportEndpoint, "device")
	if err != nil {
		return nil, &awg31Error{"transport/wireguard.Endpoint: " + err.Error()}
	}
	if !deviceValue.IsValid() || deviceValue.IsZero() {
		return nil, &awg31Error{"устройство WireGuard ещё не создано"}
	}
	return deviceValue.Interface(), nil
}
