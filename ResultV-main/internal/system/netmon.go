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


var checkHosts = []string{
	"dns.google:443",          
	"one.one.one.one:443",     
	"208.67.222.222:443",      
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
