package proxy

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sagernet/sing-box/adapter"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type Socks5ExitCheck struct {
	OK        bool   `json:"ok"`
	IP        string `json:"ip,omitempty"`
	Country   string `json:"country,omitempty"`
	UDP       bool   `json:"udp"`
	Reason    string `json:"reason,omitempty"`
	Code      int    `json:"code,omitempty"`
	LatencyMs int64  `json:"latencyMs,omitempty"`
}

const (
	Socks5CheckInactive     = "inactive"
	Socks5CheckFirstHopDown = "first_hop_down"
	Socks5CheckNoResponse   = "socks_no_response"
	Socks5CheckBadAuth      = "bad_auth"
	Socks5CheckAuthMethod   = "auth_method"
	Socks5CheckRejected     = "rejected"
	Socks5CheckProbeFailed  = "probe_failed"
)

const (
	socks5ExitTraceTimeout = 8 * time.Second
	socks5ExitSTUNTimeout  = 3 * time.Second
)

var (
	socks5ExitTraceURL    = "https://www.cloudflare.com/cdn-cgi/trace"
	socks5ExitSTUNTargets = udpRelayProbeTargets
	socks5RejectCodeRE    = regexp.MustCompile(`request rejected, code=\s*(\d+)`)
)

type socks5ExitDial func(context.Context, M.Socksaddr) (net.Conn, error)

func (e *SingBoxEngine) socks5ExitCheckTargets() (*socks5ExitSlot, adapter.Outbound, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.instance == nil || !e.running.Load() {
		return nil, nil, false
	}
	out, _ := e.instance.Outbound().Outbound(socks5ExitSelector)
	selector, ok := out.(*socks5ExitSwitch)
	if !ok || selector.Now() == "proxy" {
		return nil, nil, false
	}
	out, _ = e.instance.Outbound().Outbound(selector.Now())
	slot, ok := out.(*socks5ExitSlot)
	if !ok {
		return nil, nil, false
	}
	first, ok := e.instance.Outbound().Outbound("proxy")
	return slot, first, ok
}

func (e *SingBoxEngine) CheckSocks5Exit(ctx context.Context) Socks5ExitCheck {
	slot, first, ok := e.socks5ExitCheckTargets()
	if !ok {
		return Socks5ExitCheck{Reason: Socks5CheckInactive}
	}
	gen := slot.generation()
	start := time.Now()
	body, err := fetchSocks5ExitTrace(ctx, slot.probeDial)
	if err != nil {
		slot.confirmUDP(gen, false)
		reason, code := classifySocks5ExitError(err)
		if reason == Socks5CheckNoResponse || reason == Socks5CheckProbeFailed {
			viaFirstHop := func(ctx context.Context, target M.Socksaddr) (net.Conn, error) {
				return first.DialContext(ctx, N.NetworkTCP, target)
			}
			if _, err := fetchSocks5ExitTrace(ctx, viaFirstHop); err != nil {
				reason, code = Socks5CheckFirstHopDown, 0
			}
		}
		return Socks5ExitCheck{Reason: reason, Code: code}
	}
	ip, country := parseCloudflareTrace(body)
	result := Socks5ExitCheck{OK: true, IP: ip, Country: country, LatencyMs: time.Since(start).Milliseconds()}
	result.UDP = socks5ExitCarriesUDP(ctx, slot)
	slot.confirmUDP(gen, result.UDP)
	return result
}

func fetchSocks5ExitTrace(ctx context.Context, dial socks5ExitDial) ([]byte, error) {
	client := &http.Client{
		Timeout: socks5ExitTraceTimeout,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
				return dial(ctx, M.ParseSocksaddr(addr))
			},
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, socks5ExitTraceURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trace answered %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4096))
}

func parseCloudflareTrace(body []byte) (ip, country string) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			continue
		}
		switch key {
		case "ip":
			ip = strings.TrimSpace(value)
		case "loc":
			country = strings.ToUpper(strings.TrimSpace(value))
		}
	}
	return ip, country
}

func classifySocks5ExitError(err error) (string, int) {
	text := err.Error()
	switch {
	case errors.Is(err, errSocks5NoResponse):
		return Socks5CheckNoResponse, 0
	case errors.Is(err, errSocks5UnadvertisedMethod), strings.Contains(text, "unsupported auth method"):
		return Socks5CheckAuthMethod, 0
	case strings.Contains(text, "incorrect user name or password"):
		return Socks5CheckBadAuth, 0
	}
	if m := socks5RejectCodeRE.FindStringSubmatch(text); m != nil {
		code, _ := strconv.Atoi(m[1])
		return Socks5CheckRejected, code
	}
	return Socks5CheckProbeFailed, 0
}

func socks5ExitCarriesUDP(ctx context.Context, slot *socks5ExitSlot) bool {
	for _, target := range socks5ExitSTUNTargets {
		if stunThroughSocks5Exit(ctx, slot, M.ParseSocksaddrHostPort(target.Host, uint16(target.Port))) == nil {
			return true
		}
	}
	return false
}

func stunThroughSocks5Exit(ctx context.Context, slot *socks5ExitSlot, target M.Socksaddr) error {
	ctx, cancel := context.WithTimeout(ctx, socks5ExitSTUNTimeout)
	defer cancel()
	packet, err := slot.probeListenPacket(ctx, target)
	if err != nil {
		return err
	}
	defer packet.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = packet.SetDeadline(deadline)
	}
	txID, request, err := newSTUNBindingRequest()
	if err != nil {
		return err
	}
	if _, err := packet.WriteTo(request, target); err != nil {
		return err
	}
	buf := make([]byte, 1500)
	for {
		n, _, err := packet.ReadFrom(buf)
		if err != nil {
			return err
		}
		if _, err := parseSTUNBindingSuccess(buf[:n], txID); err == nil {
			return nil
		}
	}
}
