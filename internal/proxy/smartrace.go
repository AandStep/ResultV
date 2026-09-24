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
	"strconv"
	"sync"
	"time"

	E "github.com/sagernet/sing/common/exceptions"
)

// The numbers, and why each one is what it is.
const (
	// smartRaceHeadStart is how long the direct path gets alone before the
	// tunnel is dialled in parallel. Long enough that a healthy site anywhere
	// in Europe has answered (measured RTT to a working host: 47 ms), short
	// enough that a censored one does not cost the user a visible stall.
	smartRaceHeadStart = 700 * time.Millisecond
	// smartDirectDialTimeout caps the direct dial itself. The system deadline
	// is no use here: instagram.com on a Russian address takes ten seconds to
	// report "i/o timeout" because the SYN is dropped rather than refused.
	smartDirectDialTimeout = 2 * time.Second
	// smartRaceDeadline is the ceiling on the whole race. Past it the
	// connection fails the way it would have failed without this feature.
	smartRaceDeadline = 5 * time.Second
	// smartFirstReadBudget is how much of the server's first answer is read
	// before handing the connection on. One TLS record header is enough to
	// know the server spoke; reading more would only delay the handover.
	smartFirstReadBudget = 8 * 1024
	// smartSilenceMargin is how long before the ceiling an attempt stops
	// waiting for the server to speak. It exists so silence arrives as an
	// outcome the loop can still act on, instead of as the ceiling itself:
	// a connected but mute path is the only candidate left once the other one
	// has refused, and closing it there costs the user the whole request.
	smartSilenceMargin = 250 * time.Millisecond
)

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

type raceAttempt struct {
	conn     net.Conn
	head     []byte
	viaProxy bool
	// silent: connected, the client's bytes went out, the server said nothing
	// before the cut-off. The connection is alive and comes back unclosed.
	silent bool
	err    error
}

// runSmartRace sends the client's first bytes down both paths and keeps
// whichever one the server answers on first.
//
// Replaying `first` is safe for exactly one reason: it is the client's opening
// message and the server has said nothing back. A second connection through the
// node begins a fresh handshake, so the peer never sees a replay — it sees a
// first message, which is what it is.
func runSmartRace(ctx context.Context, first []byte, direct, proxy raceDialer) raceResult {
	ctx, cancel := context.WithTimeout(ctx, smartRaceDeadline)
	defer cancel()

	results := make(chan raceAttempt, 2)
	timer := time.NewTimer(smartRaceHeadStart)
	defer timer.Stop()

	pending := 1
	var once sync.Once
	// The counter is kept here rather than at the call sites because the
	// launch happens at most once while two different events ask for it. Both
	// used to increment, so on a refusal followed by the head start the loop
	// waited for a second attempt that was never running — and every doomed
	// race paid the full ceiling instead of ending when its last path failed.
	launchProxy := func() {
		once.Do(func() {
			pending++
			timer.Stop()
			go attempt(ctx, proxy, first, true, results)
		})
	}

	directCtx, directCancel := context.WithTimeout(ctx, smartDirectDialTimeout)
	defer directCancel()
	go attempt(directCtx, direct, first, false, results)

	var (
		lastErr error
		mute    *raceAttempt
	)
	// keepMute holds a connected-but-silent NODE connection as the candidate of
	// last resort. Direct's silence is not kept: it is the signature of the
	// black hole itself, and a tab hanging on the client's own minute-long
	// timeout is worse for the user than an honest refusal now.
	keepMute := func(res raceAttempt) {
		if !res.viaProxy || mute != nil {
			res.conn.Close()
			return
		}
		kept := res
		mute = &kept
	}
	handOver := func() raceResult {
		return raceResult{Conn: mute.conn, ViaProxy: mute.viaProxy, Unproven: true}
	}

	for {
		select {
		case <-timer.C:
			// The server has not spoken. On a working path that means the
			// request is still in flight; on a censored one it means it never
			// will. The two are indistinguishable from here, so stop guessing
			// and try the other path as well.
			launchProxy()
		case res := <-results:
			pending--
			switch {
			case res.err == nil && !res.silent:
				// Whoever is still running has lost and must not be left
				// holding a socket.
				go drainLosers(results, pending)
				if mute != nil {
					mute.conn.Close()
				}
				return raceResult{Conn: res.conn, Head: res.head, ViaProxy: res.viaProxy}
			case res.silent:
				keepMute(res)
			default:
				lastErr = res.err
			}
			if !res.viaProxy {
				// Direct refusing or going mute is evidence now, not in 700 ms.
				launchProxy()
			}
			if pending == 0 {
				if mute != nil {
					return handOver()
				}
				if lastErr == nil {
					lastErr = ctx.Err()
				}
				return raceResult{Err: lastErr}
			}
		case <-ctx.Done():
			go drainLosers(results, pending)
			if mute != nil {
				// Direct is known dead and the node is at least alive. Handing
				// its connection over costs nothing if the server never speaks
				// — the client then waits on its own terms instead of being
				// told no while the only live path was thrown away.
				return handOver()
			}
			if lastErr == nil {
				lastErr = ctx.Err()
			}
			return raceResult{
				Unfinished: pending,
				Err: E.Cause(lastErr, "no answer in ", smartRaceDeadline.String(), ", ",
					strconv.Itoa(pending), " path(s) unfinished"),
			}
		}
	}
}

// attempt dials, writes the client's opening bytes, and waits for the server to
// say something. Silence is not an outcome: only bytes are.
func attempt(ctx context.Context, dial raceDialer, first []byte, viaProxy bool, out chan<- raceAttempt) {
	conn, err := dial(ctx)
	if err != nil {
		out <- raceAttempt{viaProxy: viaProxy, err: err}
		return
	}
	if len(first) > 0 {
		if _, err = conn.Write(first); err != nil {
			conn.Close()
			out <- raceAttempt{viaProxy: viaProxy, err: err}
			return
		}
	}
	// Without a deadline here a mute server keeps this goroutine and its
	// socket for as long as it likes: cancelling the context does not unblock
	// a read on an established connection.
	if deadline, hasDeadline := ctx.Deadline(); hasDeadline {
		_ = conn.SetReadDeadline(deadline.Add(-smartSilenceMargin))
	}
	head := make([]byte, smartFirstReadBudget)
	n, err := conn.Read(head)
	if n == 0 && !isDeadlineError(err) {
		conn.Close()
		if err == nil {
			err = E.New("server closed without answering")
		}
		out <- raceAttempt{viaProxy: viaProxy, err: err}
		return
	}
	// Whatever happens next, the connection may end up in the client's hands,
	// and a deadline left on it would fail their very first read.
	_ = conn.SetReadDeadline(time.Time{})
	result := raceAttempt{conn: conn, viaProxy: viaProxy}
	if n > 0 {
		result.head = head[:n]
	} else {
		result.silent = true
	}
	select {
	case out <- result:
	case <-ctx.Done():
		// Someone else already won while we were reading.
		conn.Close()
	}
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

// drainLosers closes whatever the still-running attempts hand back.
func drainLosers(results <-chan raceAttempt, count int) {
	for i := 0; i < count; i++ {
		res, ok := <-results
		if !ok {
			return
		}
		if res.conn != nil {
			res.conn.Close()
		}
	}
}
