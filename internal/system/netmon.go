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

package system

import (
	"context"
	"net"
	"sync"
	"time"
)


type NetworkStatus struct {
	Online    bool   `json:"online"`
	Latency   int64  `json:"latency"`   
	CheckedAt int64  `json:"checkedAt"` 
	Error     string `json:"error,omitempty"`
}


type StatusChangeHandler func(status NetworkStatus)



type NetMonitor struct {
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	interval time.Duration
	handler  StatusChangeHandler
	last     NetworkStatus
	running  bool
}


// checkHosts are the reachability targets for the "is there internet at all"
// check. Literal IPs only, deliberately: a hostname here would be resolved by
// the OS resolver, and an active tunnel session breaks exactly that — the
// system-DNS override pins the physical adapters to resolvers reachable only
// inside the tunnel, while the app's own traffic is self-direct. The monitor
// then flapped "Интернет-соединение потеряно" while every browser tab worked.
// See netmon_test.go.
//
// All four must fail before the machine is called offline, so one target being
// throttled or blocked by a national filter can't produce a false negative.
var checkHosts = []string{
	"1.1.1.1:443",        // Cloudflare
	"8.8.8.8:443",        // Google
	"208.67.222.222:443", // OpenDNS
	"9.9.9.9:443",        // Quad9
}


func NewNetMonitor(handler StatusChangeHandler) *NetMonitor {
	return &NetMonitor{
		interval: 5 * time.Second,
		handler:  handler,
		last:     NetworkStatus{Online: true}, 
	}
}


func (nm *NetMonitor) Start(parentCtx context.Context) {
	nm.mu.Lock()
	if nm.running {
		nm.mu.Unlock()
		return
	}
	nm.ctx, nm.cancel = context.WithCancel(parentCtx)
	nm.running = true
	nm.mu.Unlock()

	go nm.loop()
}


func (nm *NetMonitor) Stop() {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if !nm.running {
		return
	}
	if nm.cancel != nil {
		nm.cancel()
	}
	nm.running = false
}


func (nm *NetMonitor) GetStatus() NetworkStatus {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	return nm.last
}


func (nm *NetMonitor) SetInterval(d time.Duration) {
	if d < time.Second {
		d = time.Second
	}
	nm.mu.Lock()
	nm.interval = d
	nm.mu.Unlock()
}

func (nm *NetMonitor) loop() {
	
	nm.check()

	nm.mu.Lock()
	interval := nm.interval
	nm.mu.Unlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-nm.ctx.Done():
			return
		case <-ticker.C:
			nm.check()
		}
	}
}

func (nm *NetMonitor) check() {
	status := checkConnectivity()

	nm.mu.Lock()
	changed := status.Online != nm.last.Online
	nm.last = status
	handler := nm.handler
	nm.mu.Unlock()

	
	if changed && handler != nil {
		handler(status)
	}
}



func checkConnectivity() NetworkStatus {
	for _, host := range checkHosts {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", host, 3*time.Second)
		if err == nil {
			conn.Close()
			return NetworkStatus{
				Online:    true,
				Latency:   time.Since(start).Milliseconds(),
				CheckedAt: time.Now().Unix(),
			}
		}
	}

	return NetworkStatus{
		Online:    false,
		CheckedAt: time.Now().Unix(),
		Error:     "all connectivity checks failed",
	}
}
