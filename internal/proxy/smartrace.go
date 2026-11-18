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
	"errors"
	"net"
	"os"
	"time"

	"github.com/sagernet/sing-box/adapter"
	E "github.com/sagernet/sing/common/exceptions"
)

// The numbers, and why each one is what it is.
const (
	// Give client-first protocols time to produce the opening message.
	smartOpeningReadTimeout = 700 * time.Millisecond
	// A direct connection gets this long to dial and produce a response.
	smartDirectTimeout = 2 * time.Second
	// The direct attempt and the VPN fallback share this total budget.
	smartConnectTimeout = 5 * time.Second
	// smartFirstReadBudget is how much of the server's first answer is read
	// before handing the connection on. One TLS record header is enough to
	// know the server spoke; reading more would only delay the handover.
	smartFirstReadBudget = 8 * 1024
	// Leave time to hand over a connected VPN fallback before the context
	// expires, even when its server has not replied yet.
	smartSilenceMargin = 250 * time.Millisecond
)

// raceTarget is what a failed race should be reported against: the name when
// there is one, the address otherwise. Without it the log carries a connection
// id and nothing a user or a reader can act on.
func raceTarget(metadata *adapter.InboundContext) string {
	if host := smartHost(metadata); host != "" {
		return host
	}
	return metadata.Destination.String()
}

// raceDialer opens one candidate connection.
type raceDialer func(ctx context.Context) (net.Conn, error)

// raceResult is the winner together with the first bytes already read off it.
// Head has to travel with Conn: those bytes are gone from the socket, and the
// caller has to replay them to the client or the stream is corrupt.
type raceResult struct {
	Conn     net.Conn
	Head     []byte
	ViaProxy bool
	// Unproven marks a connection handed over without the server ever having
	// spoken on it. It is the last live candidate, not a measurement: nothing
	// may be learned from it and it says nothing about the link.
	Unproven bool
	// Unfinished counts paths still dialing or still reading when the ceiling
	// hit. Their outcome is unknown, which is why such a result is no evidence
	// either way.
	Unfinished int
	Err        error
}

// raceLinkEvidence turns one finished race into what it says about the direct
// LINK, which is not the same question as what it says about the destination.
// Only a race both legs lost is evidence the link is broken; a race the node
// won is one censored destination, and filing it as a link failure is what let
// the breaker trip on the engine's own rescues.
func raceLinkEvidence(res raceResult) (report, ok bool) {
	switch {
	case res.Unfinished > 0 || res.Unproven:
		// Ambiguous by construction: direct may have refused because that one
		// address is censored while the node merely had not answered yet.
		// Counting it would trip the breaker on the very traffic it exists to
		// keep measuring.
		return false, false
	case res.Err != nil:
		return true, false
	case res.ViaProxy:
		return false, false
	default:
		return true, true
	}
}

// raceTeaches reports whether this outcome may be written to the verdict
// store. A verdict lives for days; only a server that actually answered earns
// one.
func raceTeaches(res raceResult) bool {
	return res.Err == nil && !res.Unproven
}

// connectSmartTCP tries the direct path through its response deadline. The node
// receives the opening message only after direct fails or stays silent.
func connectSmartTCP(ctx context.Context, first []byte, direct, proxy raceDialer) raceResult {
	ctx, cancel := context.WithTimeout(ctx, smartConnectTimeout)
	defer cancel()

	directCtx, directCancel := context.WithTimeout(ctx, smartDirectTimeout)
	result := attemptSmartTCP(directCtx, direct, first, false)
	directCancel()
	if result.Err == nil && !result.Unproven {
		return result
	}
	if result.Conn != nil {
		result.Conn.Close()
	}
	if err := ctx.Err(); err != nil {
		return raceResult{Err: err, Unfinished: 1}
	}

	result = attemptSmartTCP(ctx, proxy, first, true)
	if result.Err != nil && ctx.Err() != nil {
		result.Unfinished = 1
	}
	return result
}

func attemptSmartTCP(ctx context.Context, dial raceDialer, first []byte, viaProxy bool) raceResult {
	conn, err := dial(ctx)
	if err != nil {
		return raceResult{ViaProxy: viaProxy, Err: err}
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()

	deadline, _ := ctx.Deadline()
	_ = conn.SetWriteDeadline(deadline)
	if len(first) != 0 {
		if _, err = conn.Write(first); err != nil {
			conn.Close()
			return raceResult{ViaProxy: viaProxy, Err: err}
		}
	}
	if viaProxy {
		deadline = deadline.Add(-smartSilenceMargin)
	}
	_ = conn.SetReadDeadline(deadline)
	head := make([]byte, smartFirstReadBudget)
	n, err := conn.Read(head)
	if n == 0 && !isDeadlineError(err) {
		conn.Close()
		if err == nil {
			err = E.New("server closed without answering")
		}
		return raceResult{ViaProxy: viaProxy, Err: err}
	}
	if !stop() {
		conn.Close()
		return raceResult{ViaProxy: viaProxy, Err: ctx.Err()}
	}
	_ = conn.SetReadDeadline(time.Time{})
	_ = conn.SetWriteDeadline(time.Time{})
	return raceResult{Conn: conn, Head: head[:n], ViaProxy: viaProxy, Unproven: n == 0}
}

// isDeadlineError separates "the server stayed silent" from "the connection
// broke", which are the two ways a first read comes back empty.
func isDeadlineError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
