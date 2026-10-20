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

package main

import (
	"net/http"
	"time"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
)

const (
	startupOnlineWait  = 60 * time.Second
	trafficProbeURL    = "http://cp.cloudflare.com/generate_204"
	trafficProbeTries  = 3
	trafficProbePause  = 2 * time.Second
	trafficProbeTimout = 5 * time.Second
)

func startupConnectID(s config.AppSettings, resumeID string) string {
	if resumeID != "" {
		return resumeID
	}
	if s.ConnectOnStartup {
		return s.LastSelectedProxyID
	}
	return ""
}

// applyConnectOnStartup runs once the config is loaded: the pending connect
// gets the last server, and the window stays visible so a failure is seen.
func (a *App) applyConnectOnStartup() {
	if a.config == nil {
		return
	}
	s := a.config.GetConfig().Settings
	a.resumeMu.Lock()
	a.resumeProxyID = startupConnectID(s, a.resumeProxyID)
	a.resumeMu.Unlock()
	if s.ConnectOnStartup {
		a.startInTray = false
		if s.LastSelectedProxyID == "" {
			a.log.Warning("[СИСТЕМА] Подключение при запуске включено, но сервер не выбран")
		}
	}
}

func (a *App) waitForOnline(max time.Duration) {
	if a.netmon == nil {
		return
	}
	deadline := time.Now().Add(max)
	for st := a.netmon.GetStatus(); !st.Online || st.CheckedAt == 0; st = a.netmon.GetStatus() {
		if time.Now().After(deadline) {
			a.log.Warning("[СЕТЬ] Сеть так и не появилась — подключаемся всё равно")
			return
		}
		select {
		case <-a.ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// VerifyTunnelTraffic checks that traffic actually leaves through the live
// tunnel after a startup connect. Proxy mode is not checked.
func (a *App) VerifyTunnelTraffic() bool {
	if a.proxy == nil {
		return true
	}
	if st := a.proxy.GetStatus(); !st.IsConnected || st.Mode != proxy.ProxyModeTunnel {
		return true
	}
	client := &http.Client{
		Timeout:   trafficProbeTimout,
		Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true},
	}
	for i := 0; i < trafficProbeTries; i++ {
		if i > 0 {
			time.Sleep(trafficProbePause)
		}
		resp, err := client.Get(trafficProbeURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusNoContent {
				return true
			}
		}
	}
	a.log.Warning("[СИСТЕМА] Страховка автоподключения: трафик через туннель не идёт")
	return false
}
