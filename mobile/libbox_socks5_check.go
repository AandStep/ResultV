// SPDX-License-Identifier: GPL-3.0-or-later
// Proprietary license and copyright notice at https://vpncore.org/legal

package mobile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/pion/stun/v3"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/protocol/group"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

const socks5ExitTraceURL = "https://www.cloudflare.com/cdn-cgi/trace"

var socks5ExitCheckMu sync.Mutex

type socks5ExitCheckResult struct {
	IP      string `json:"ip"`
	Country string `json:"country"`
	UDP     bool   `json:"udp"`
}

// CheckSocks5Exit checks the exit through the running tunnel and selects it
// for UDP only after a STUN binding succeeds through that same exit.
func CheckSocks5Exit(server *libbox.CommandServer) (string, error) {
	if server == nil || server.StartedService == nil {
		return "", errors.New("VPN is not running")
	}
	socks5ExitCheckMu.Lock()
	defer socks5ExitCheckMu.Unlock()
	instance := server.Instance()
	if instance == nil || instance.Box() == nil {
		return "", errors.New("VPN is not running")
	}
	manager := instance.Box().Outbound()
	selection, ok := manager.Outbound(socks5ExitUDPTag)
	selector, isSelector := selection.(*group.Selector)
	if !ok || !isSelector {
		return "", errors.New("SOCKS5 exit UDP selector is unavailable")
	}
	if !selector.SelectOutbound("proxy") {
		return "", errors.New("SOCKS5 exit UDP fallback is unavailable")
	}
	exit, ok := manager.Outbound(socks5ExitTag)
	if !ok {
		return "", errors.New("SOCKS5 exit is not active")
	}
	ip, country, err := fetchSocks5ExitTrace(exit)
	if err != nil {
		return "", err
	}
	udp := probeSocks5ExitUDP(exit, M.ParseSocksaddr("stun.l.google.com:19302")) ||
		probeSocks5ExitUDP(exit, M.ParseSocksaddr("stun.cloudflare.com:3478"))
	if server.Instance() != instance {
		return "", errors.New("VPN changed during the SOCKS5 exit check")
	}
	if udp && !selector.SelectOutbound(socks5ExitTag) {
		return "", errors.New("could not enable validated SOCKS5 exit UDP")
	}
	result, err := json.Marshal(socks5ExitCheckResult{IP: ip, Country: country, UDP: udp})
	return string(result), err
}

func fetchSocks5ExitTrace(exit adapter.Outbound) (string, string, error) {
	client := &http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
				return exit.DialContext(ctx, N.NetworkTCP, M.ParseSocksaddr(addr))
			},
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Get(socks5ExitTraceURL)
	if err != nil {
		return "", "", fmt.Errorf("SOCKS5 exit probe: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("SOCKS5 exit probe returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		return "", "", err
	}
	return parseSocks5ExitTrace(body)
}

func parseSocks5ExitTrace(body []byte) (string, string, error) {
	var ip, country string
	for _, line := range strings.Split(string(body), "\n") {
		key, value, _ := strings.Cut(line, "=")
		switch key {
		case "ip":
			ip = strings.TrimSpace(value)
		case "loc":
			country = strings.ToUpper(strings.TrimSpace(value))
		}
	}
	address, err := netip.ParseAddr(ip)
	if err != nil {
		return "", "", errors.New("SOCKS5 exit probe did not return an IP address")
	}
	if len(country) != 2 || country[0] < 'A' || country[0] > 'Z' || country[1] < 'A' || country[1] > 'Z' {
		country = ""
	}
	return address.Unmap().String(), country, nil
}

func probeSocks5ExitUDP(exit adapter.Outbound, target M.Socksaddr) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, err := stun.Build(stun.TransactionID, stun.BindingRequest)
	if err != nil {
		return false
	}
	packet, err := exit.ListenPacket(ctx, target)
	if err != nil {
		return false
	}
	defer packet.Close()
	deadline, _ := ctx.Deadline()
	if packet.SetDeadline(deadline) != nil {
		return false
	}
	if _, err := packet.WriteTo(request.Raw, target); err != nil {
		return false
	}
	buf := make([]byte, 1500)
	for {
		n, _, err := packet.ReadFrom(buf)
		if err != nil {
			return false
		}
		if socks5ExitSTUNSuccess(buf[:n], request.TransactionID) {
			return true
		}
	}
}

func socks5ExitSTUNSuccess(data []byte, txID [stun.TransactionIDSize]byte) bool {
	var reply stun.Message
	if stun.Decode(data, &reply) != nil || reply.Type != stun.BindingSuccess || reply.TransactionID != txID {
		return false
	}
	var address stun.XORMappedAddress
	return address.GetFrom(&reply) == nil && address.Port > 0 && !address.IP.IsUnspecified()
}
