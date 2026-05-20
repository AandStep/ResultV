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
	"errors"
	"net"
	"strconv"
	"strings"
	"time"
)

var pickLANBindIPv4 = preferLANBindIPv4


func isEngineTunIPv4(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	return ip4[0] == 172 && ip4[1] == 19 && ip4[2] == 0 && ip4[3] <= 3
}

func looksLikeTunnelInterface(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "tun") ||
		strings.Contains(n, "tap") ||
		strings.Contains(n, "wintun") ||
		strings.Contains(n, "tailscale") ||
		strings.Contains(n, "wireguard") ||
		strings.Contains(n, "nordlynx") ||
		strings.Contains(n, "zerotier") ||
		strings.Contains(n, "sing-tun")
}

func isRFC1918(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	switch {
	case ip4[0] == 10:
		return true
	case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
		return true
	case ip4[0] == 192 && ip4[1] == 168:
		return true
	default:
		return false
	}
}



func preferLANBindIPv4() (net.IP, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var private, public []net.IP
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		if looksLikeTunnelInterface(ifi.Name) {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			ip4 := ip.To4()
			if ip4 == nil {
				continue
			}
			if isEngineTunIPv4(ip4) {
				continue
			}
			if isRFC1918(ip4) {
				private = append(private, append(net.IP(nil), ip4...))
			} else {
				public = append(public, append(net.IP(nil), ip4...))
			}
		}
	}
	if len(private) > 0 {
		return private[0], nil
	}
	if len(public) > 0 {
		return public[0], nil
	}
	return nil, errors.New("no suitable LAN IPv4 for bind")
}



func PingProxyLANBind(host string, port int) (latencyMs int64, reachable bool, reason string) {
	local, err := pickLANBindIPv4()
	if err != nil {
		return 0, false, "lan_bind_unavailable"
	}
	d := net.Dialer{
		Timeout:   5 * time.Second,
		LocalAddr: &net.TCPAddr{IP: local, Port: 0},
	}
	start := time.Now()
	conn, err := d.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	elapsed := time.Since(start)
	if err != nil {
		return 0, false, pingReasonFromError(err)
	}
	_ = conn.Close()
	return elapsed.Milliseconds(), true, ""
}

func PingProxyUDPLANBind(host string, port int) (latencyMs int64, reachable bool, reason string) {
	local, err := pickLANBindIPv4()
	if err != nil {
		return 0, false, "lan_bind_unavailable"
	}
	d := net.Dialer{
		Timeout:   3 * time.Second,
		LocalAddr: &net.UDPAddr{IP: local, Port: 0},
	}
	conn, err := d.Dial("udp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return 0, false, pingReasonFromError(err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	start := time.Now()
	_, _ = conn.Write([]byte{0x00})
	buf := make([]byte, 1)
	_, readErr := conn.Read(buf)
	elapsed := time.Since(start)
	if readErr != nil {
		if ne, ok := readErr.(net.Error); ok && ne.Timeout() {
			return -1, true, ""
		}
		msg := strings.ToLower(readErr.Error())
		if strings.Contains(msg, "refused") {
			return 0, false, "connection_refused"
		}
		return -1, true, ""
	}
	return elapsed.Milliseconds(), true, ""
}

func PingHysteria2QUICLANBind(host string, port int) (latencyMs int64, reachable bool, reason, checkType string) {
	latency, ok, r := PingProxyUDPLANBind(host, port)
	if ok {
		return latency, true, "", "udp_lan_bind"
	}

	tcpLatency, tcpOK, tcpReason := PingProxyLANBind(host, port)
	if tcpOK {
		return tcpLatency, true, "", "tcp_lan_fallback"
	}
	if r == "" {
		r = tcpReason
	}
	return 0, false, r, "udp_lan_bind"
}
