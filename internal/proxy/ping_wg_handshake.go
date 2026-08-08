/*
 * Copyright (C) 2026 ResultV
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package proxy

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sagernet/wireguard-go/conn"
	"github.com/sagernet/wireguard-go/device"
	"github.com/sagernet/wireguard-go/tun"

	"resultproxy-wails/internal/config"
)

// wgHandshakeTimeout bounds a single WireGuard/AmneziaWG handshake probe. One
// network round-trip is all a healthy server needs to answer the initiation;
// WireGuard retransmits the initiation every ~5s if the first is lost, so the
// budget is kept just under that to avoid double-counting a retransmit.
const wgHandshakeTimeout = 5 * time.Second

// wgProbeHardCap bounds the WHOLE probe — endpoint resolution, device setup,
// the handshake poll loop and teardown — not just the poll loop. The poll
// budget alone never applies if a setup step wedges: on Android net.ResolveUDPAddr
// on a .conf hostname endpoint (no context, no deadline) can block indefinitely,
// and device Close() can stall, leaving the gomobile Ping binding hung forever
// (the "infinite ping" symptom). This outer cap guarantees the binding always
// returns. A var (not const) so tests can shrink it.
var wgProbeHardCap = wgHandshakeTimeout + 3*time.Second

// wgHandshakeProbe runs the actual device-backed handshake probe. It is a var so
// tests can substitute a hung body to verify the hard cap returns.
var wgHandshakeProbe = realWGHandshakeProbe

// PingWireGuardHandshake measures the real round-trip latency to a WireGuard /
// AmneziaWG endpoint by performing an actual Noise handshake against it.
//
// Unlike a TCP connect or a bare UDP byte (both of which a WireGuard server
// silently drops), a valid handshake initiation provokes a handshake response,
// and the time from sending the initiation to the peer being marked
// established is ~1 RTT. This is the only transport-faithful latency signal on
// Android, where unprivileged ICMP is unavailable.
//
// entryJSON is the marshaled config.ProxyEntry (carrying the private/public
// keys, endpoint and any AmneziaWG obfuscation knobs in Extra). On success the
// returned latency is in milliseconds; on failure reason carries the kind
// ("timeout", "connection_refused", …) using the same vocabulary as the other
// probes.
func PingWireGuardHandshake(entryJSON string) (latencyMs int64, reachable bool, reason string) {
	var entry config.ProxyEntry
	if err := json.Unmarshal([]byte(entryJSON), &entry); err != nil {
		return 0, false, "probe_error"
	}

	uapi, endpoint, err := buildWGUAPI(entry)
	if err != nil {
		return 0, false, "probe_error"
	}

	// The device-backed probe runs under an overall hard cap: every step below
	// (resolve, setup, poll, teardown) is otherwise unbounded, and a single one
	// hanging on Android would wedge the binding forever. If the cap fires we
	// abandon the in-flight goroutine (it cleans itself up on Close) and report
	// a timeout so the UI shows a result instead of an endless spinner.
	type probeResult struct {
		ms     int64
		ok     bool
		reason string
	}
	ch := make(chan probeResult, 1)
	go func() {
		ms, ok, r := wgHandshakeProbe(uapi, endpoint)
		ch <- probeResult{ms, ok, r}
	}()
	select {
	case res := <-ch:
		return res.ms, res.ok, res.reason
	case <-time.After(wgProbeHardCap):
		return 0, false, "timeout"
	}
}

// realWGHandshakeProbe resolves the endpoint, brings up a wireguard-go device
// against it, and polls for handshake completion. Bounded by the poll loop for
// the network round-trip; PingWireGuardHandshake wraps it in an overall hard cap
// so resolution and teardown can't hang the caller.
func realWGHandshakeProbe(uapi, endpoint string) (latencyMs int64, reachable bool, reason string) {
	// Resolve the endpoint up front so a dead/unroutable host surfaces a
	// precise reason instead of a flat handshake timeout. Bound it with a
	// context: the bare net.ResolveUDPAddr has no deadline and can block
	// indefinitely on Android when the system resolver is unavailable.
	resolveCtx, cancel := context.WithTimeout(context.Background(), wgHandshakeTimeout)
	defer cancel()
	host, port, splitErr := net.SplitHostPort(endpoint)
	if splitErr != nil {
		return 0, false, "probe_error"
	}
	addrs, err := net.DefaultResolver.LookupNetIP(resolveCtx, "ip", host)
	if err != nil {
		return 0, false, pingReasonFromError(err)
	}

	// The vendored wireguard-go fork's Bind.ParseEndpoint accepts only a
	// numeric IP:port (netip.ParseAddrPort on Unix; GetAddrInfoW with
	// AI_NUMERICHOST on Windows) — it never resolves hostnames itself. Splice
	// the resolved literal IP into the UAPI endpoint= line so IpcSet doesn't
	// reject a hostname endpoint that the real (sing-box) connect path
	// resolves fine.
	resolvedEndpoint := net.JoinHostPort(addrs[0].String(), port)
	uapi = strings.Replace(uapi, "endpoint="+endpoint+"\n", "endpoint="+resolvedEndpoint+"\n", 1)

	dev := device.NewDevice(
		context.Background(),
		newStubTUN(),
		conn.NewDefaultBind(nil),
		device.NewLogger(device.LogLevelSilent, ""),
		0,     // workers: 0 → runtime default
		0,     // preallocatedBuffersPerPool: 0 → default
		false, // disablePauses
	)
	defer dev.Close()

	if err := dev.IpcSet(uapi); err != nil {
		return 0, false, "probe_error"
	}
	if err := dev.Up(); err != nil {
		return 0, false, "probe_error"
	}

	// The stub TUN feeds one dummy packet on first Read; allowed_ip=0.0.0.0/0
	// routes it to our single peer, which kicks off the handshake initiation.
	start := time.Now()
	deadline := start.Add(wgHandshakeTimeout)
	for time.Now().Before(deadline) {
		if hs := lastHandshakeSecs(dev); hs != 0 {
			return time.Since(start).Milliseconds(), true, ""
		}
		time.Sleep(50 * time.Millisecond)
	}
	return 0, false, "timeout"
}

// lastHandshakeSecs reads the peer's last_handshake_time_sec from the device's
// UAPI snapshot. A non-zero value means the Noise handshake completed.
func lastHandshakeSecs(dev *device.Device) int64 {
	snap, err := dev.IpcGet()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(snap, "\n") {
		if v, ok := strings.CutPrefix(line, "last_handshake_time_sec="); ok {
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
				return n
			}
		}
	}
	return 0
}

// buildWGUAPI converts a WireGuard/AmneziaWG ProxyEntry into the cross-platform
// wireguard-go UAPI config string and the resolved endpoint. Device-level keys
// (private_key + any AmneziaWG knobs) are emitted before the peer block, which
// begins at the first public_key= line — the order the UAPI parser requires.
//
// WireGuard keys travel as standard base64 in the config but the UAPI expects
// hex, so each is decoded and re-encoded here.
func buildWGUAPI(entry config.ProxyEntry) (uapi string, endpoint string, err error) {
	var extra map[string]any
	if len(entry.Extra) > 0 {
		if err := json.Unmarshal(entry.Extra, &extra); err != nil {
			return "", "", fmt.Errorf("parsing extra: %w", err)
		}
	}
	if extra == nil {
		extra = map[string]any{}
	}

	privHex, err := keyToHex(asString(extra["private_key"]))
	if err != nil {
		return "", "", fmt.Errorf("private_key: %w", err)
	}
	pubHex, err := keyToHex(asString(extra["public_key"]))
	if err != nil {
		return "", "", fmt.Errorf("public_key: %w", err)
	}
	if entry.IP == "" || entry.Port <= 0 {
		return "", "", fmt.Errorf("missing endpoint")
	}
	endpoint = net.JoinHostPort(entry.IP, strconv.Itoa(entry.Port))

	var b strings.Builder
	// ── Device-level keys ──
	fmt.Fprintf(&b, "private_key=%s\n", privHex)
	writeAmneziaUAPI(&b, extra)

	// ── Peer block ──
	fmt.Fprintf(&b, "public_key=%s\n", pubHex)
	if psk := asString(extra["pre_shared_key"]); psk != "" {
		if pskHex, err := keyToHex(psk); err == nil {
			fmt.Fprintf(&b, "preshared_key=%s\n", pskHex)
		}
	}
	fmt.Fprintf(&b, "endpoint=%s\n", endpoint)
	fmt.Fprintf(&b, "allowed_ip=0.0.0.0/0\n")

	return b.String(), endpoint, nil
}

// writeAmneziaUAPI emits the AmneziaWG obfuscation knobs present in extra.
// Integer knobs (jc/jmin/jmax/s1-s4) and the magic-header / junk-packet knobs
// (h1-h4, i1-i5 — which may be ranges or templates carried as strings) are
// passed through in the form the wireguard-go fork's UAPI parser accepts.
//
// This builder speaks to wireguard-go directly rather than through sing-box's
// option struct, so it is the one path that can carry the full AmneziaWG 3.0
// knob set — see awg3DeviceKnobs for why the tunnel cannot. A probe against a
// server that requires header protection only completes if we send them.
func writeAmneziaUAPI(b *strings.Builder, extra map[string]any) {
	amRaw, ok := extra["amnezia"].(map[string]any)
	if !ok {
		return
	}
	intKeys := []string{"jc", "jmin", "jmax", "s1", "s2", "s3", "s4"}
	for _, k := range intKeys {
		if v, ok := amRaw[k]; ok {
			if n := asInt(v); n > 0 {
				fmt.Fprintf(b, "%s=%d\n", k, n)
			}
		}
	}
	// h1-h4 and i1-i5 are emitted only when present; the fork supplies WG-1.0
	// defaults for any header left unset.
	strKeys := []string{"h1", "h2", "h3", "h4", "i1", "i2", "i3", "i4", "i5"}
	for _, k := range strKeys {
		if v, ok := amRaw[k]; ok {
			s := amneziaScalar(v)
			if s != "" && s != "0" {
				fmt.Fprintf(b, "%s=%s\n", k, s)
			}
		}
	}
	// ── AmneziaWG 3.0 (wireguard-go extended-1.5.0) ──
	// The key is the only one needing a conversion; the rest are ranges
	// ("a" or "a-b") that UintRange.FromString takes verbatim.
	//
	// headerProtectionUsable gates the key for the same reason the config path
	// does: with S1-S4 below the nonce size the device refuses to start at all,
	// so sending it would turn every probe into a flat probe_error. Keeping the
	// two paths on one rule means ping and connect agree about a given config.
	if headerProtectionUsable(amRaw) {
		if h := amneziaKeyHex(amRaw["header_protection_key"]); h != "" {
			fmt.Fprintf(b, "header_protection_key=%s\n", h)
		}
	}
	for _, k := range awg3DeviceKnobs {
		if k == "header_protection_key" {
			continue
		}
		if v, ok := amRaw[k]; ok {
			s := amneziaScalar(v)
			if s != "" && s != "0" {
				fmt.Fprintf(b, "%s=%s\n", k, s)
			}
		}
	}
}

// amneziaKeyHex normalizes a header-protection key to the lowercase hex the
// UAPI expects. `awg genkey` emits base64 like every other WireGuard key, but
// HeaderCipherKey.FromHex only accepts hex, so accept either spelling.
func amneziaKeyHex(v any) string {
	s := amneziaScalar(v)
	if s == "" {
		return ""
	}
	if len(s) == 64 {
		if _, err := hex.DecodeString(s); err == nil {
			return strings.ToLower(s)
		}
	}
	if h, err := keyToHex(s); err == nil {
		return h
	}
	return ""
}

// amneziaScalar renders an AmneziaWG knob value (which may arrive as a number
// or a string range like "1-5") as the string the UAPI parser expects.
func amneziaScalar(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	default:
		return ""
	}
}

// keyToHex decodes a standard- or raw-base64 WireGuard key into lowercase hex.
func keyToHex(b64 string) (string, error) {
	b64 = strings.TrimSpace(b64)
	if b64 == "" {
		return "", fmt.Errorf("empty key")
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if raw, err := enc.DecodeString(b64); err == nil && len(raw) == 32 {
			return hex.EncodeToString(raw), nil
		}
	}
	return "", fmt.Errorf("not a 32-byte base64 key")
}

// ──────────────────────── stub TUN ────────────────────────

// stubTUNRetryInterval is how often the stub re-offers its synthetic packet.
// A var so tests can shrink it.
var stubTUNRetryInterval = 250 * time.Millisecond

// stubTUN is a no-op tun.Device used solely to drive a handshake probe. It
// repeatedly surfaces a synthetic IPv4 packet (so the device routes it to the
// peer and initiates the handshake); every Write is discarded.
type stubTUN struct {
	events chan tun.Event
	closed chan struct{}
	first  bool
}

func newStubTUN() *stubTUN {
	t := &stubTUN{
		events: make(chan tun.Event, 1),
		closed: make(chan struct{}),
		first:  true,
	}
	t.events <- tun.EventUp
	return t
}

// dummyIPv4Packet is a minimal, well-formed IPv4 header (no payload) addressed
// to a routable destination so the device picks our 0.0.0.0/0 peer and starts a
// handshake. The contents are never sent on the wire — only the handshake is.
var dummyIPv4Packet = []byte{
	0x45, 0x00, 0x00, 0x14, // version/IHL, DSCP, total length 20
	0x00, 0x00, 0x00, 0x00, // id, flags/frag
	0x40, 0x00, 0x00, 0x00, // TTL 64, proto 0, checksum 0
	10, 0, 0, 1, // src 10.0.0.1
	10, 0, 0, 2, // dst 10.0.0.2
}

// Read re-offers the synthetic packet on a slow tick rather than exactly once.
//
// device.NewDevice starts RoutineReadFromTUN before the caller gets a chance to
// run IpcSet, so the very first Read is served while the device still has no
// peers: allowedips.Lookup returns nil and the packet is dropped on the floor.
// A stub that yields one packet and then blocks therefore leaves the device
// with nothing to send — no handshake initiation is ever queued and the probe
// reports a flat "timeout" having transmitted zero bytes. (It succeeded only
// when the reader goroutine happened to lose the scheduling race and read after
// IpcSet, which is why the probe looked flaky rather than broken.)
//
// Repeating also gives a lost initiation another trigger, at the cost of a
// handful of packets the device discards once the handshake is under way.
func (t *stubTUN) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	if t.first {
		t.first = false
	} else {
		select {
		case <-t.closed:
			return 0, os.ErrClosed
		case <-time.After(stubTUNRetryInterval):
		}
	}
	select {
	case <-t.closed:
		return 0, os.ErrClosed
	default:
	}
	n := copy(bufs[0][offset:], dummyIPv4Packet)
	sizes[0] = n
	return 1, nil
}

func (t *stubTUN) Write(bufs [][]byte, offset int) (int, error) { return len(bufs), nil }
func (t *stubTUN) Flush() error                                 { return nil }
func (t *stubTUN) MTU() (int, error)                            { return 1420, nil }
func (t *stubTUN) Name() (string, error)                        { return "rvprobe", nil }
func (t *stubTUN) File() *os.File                               { return nil }
func (t *stubTUN) Events() <-chan tun.Event                     { return t.events }
func (t *stubTUN) BatchSize() int                               { return 1 }

func (t *stubTUN) Close() error {
	select {
	case <-t.closed:
	default:
		close(t.closed)
	}
	return nil
}
