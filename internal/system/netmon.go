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
	"sort"
	"strings"
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



// InterfaceChangeHandler is called when the set of local IP addresses changes:
// a Wi-Fi roam, a switch to a phone hotspot, a cable pulled, a tunnel coming
// up. Unlike StatusChangeHandler this says nothing about reachability — the
// machine can stay online across the whole transition — it only reports that
// the local network identity is no longer the one anything cached earlier was
// based on.
type InterfaceChangeHandler func()

type NetMonitor struct {
	mu        sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
	interval  time.Duration
	handler   StatusChangeHandler
	ifHandler InterfaceChangeHandler
	ifSig     string
	last      NetworkStatus
	running   bool
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


// SetInterfaceChangeHandler installs the handler for local-address changes.
// Optional: with none installed the signature is still tracked but nothing is
// notified.
func (nm *NetMonitor) SetInterfaceChangeHandler(h InterfaceChangeHandler) {
	nm.mu.Lock()
	nm.ifHandler = h
	nm.mu.Unlock()
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
	sig := localAddrSignature()

	nm.mu.Lock()
	changed := status.Online != nm.last.Online
	nm.last = status
	handler := nm.handler
	// The very first tick establishes the baseline rather than reporting a
	// change: at startup nothing has cached a previous network identity yet, so
	// firing here would only produce a spurious invalidation on every launch.
	ifChanged := nm.ifSig != "" && sig != "" && sig != nm.ifSig
	if sig != "" {
		nm.ifSig = sig
	}
	ifHandler := nm.ifHandler
	nm.mu.Unlock()


	if changed && handler != nil {
		handler(status)
	}
	if ifChanged && ifHandler != nil {
		ifHandler()
	}
}

// localAddrSignature fingerprints the machine's current local addressing. The
// enumeration costs one GetAdaptersAddresses per tick on Windows — the same
// call preferLANBindIPv4 makes — which is affordable at the monitor's 5s
// cadence and is what lets everything downstream cache an adapter choice for
// much longer than that.
//
// Interface names are folded in alongside the addresses so that the same IP
// reappearing on a different adapter (a hotspot handing out the same
// 192.168.1.x the home router did) still reads as a change. Returns "" if the
// adapters cannot be enumerated at all, which the caller treats as "no news"
// rather than as a change.
func localAddrSignature() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	parts := make([]string, 0, len(ifaces))
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			parts = append(parts, ifi.Name+"="+a.String())
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
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
