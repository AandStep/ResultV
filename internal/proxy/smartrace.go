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
	"net"
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
	Err      error
}

type raceAttempt struct {
	conn     net.Conn
	head     []byte
	viaProxy bool
	err      error
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
	var once sync.Once
	launchProxy := func() {
		once.Do(func() { go attempt(ctx, proxy, first, true, results) })
	}

	directCtx, directCancel := context.WithTimeout(ctx, smartDirectDialTimeout)
	defer directCancel()
	go attempt(directCtx, direct, first, false, results)

	timer := time.NewTimer(smartRaceHeadStart)
	defer timer.Stop()

	pending := 1
	var lastErr error
	for {
		select {
		case <-timer.C:
			// The server has not spoken. On a working path that means the
			// request is still in flight; on a censored one it means it never
			// will. The two are indistinguishable from here, so stop guessing
			// and try the other path as well.
			pending++
			launchProxy()
		case res := <-results:
			if res.err == nil {
				// Whoever is still running has lost and must not be left
				// holding a socket: cancel unblocks their read and the attempt
				// goroutine closes what it opened.
				go drainLosers(results, pending-1)
				return raceResult{Conn: res.conn, Head: res.head, ViaProxy: res.viaProxy}
			}
			lastErr = res.err
			pending--
			if !res.viaProxy {
				// A refusal is evidence now, not in 700 ms.
				pending++
				launchProxy()
			}
			if pending == 0 {
				return raceResult{Err: lastErr}
			}
		case <-ctx.Done():
			go drainLosers(results, pending)
			if lastErr == nil {
				lastErr = ctx.Err()
			}
			return raceResult{Err: E.Cause(lastErr, "smart: neither path answered")}
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
	head := make([]byte, smartFirstReadBudget)
	n, err := conn.Read(head)
	if err != nil || n == 0 {
		conn.Close()
		if err == nil {
			err = E.New("server closed without answering")
		}
		out <- raceAttempt{viaProxy: viaProxy, err: err}
		return
	}
	select {
	case out <- raceAttempt{conn: conn, head: head[:n], viaProxy: viaProxy}:
	case <-ctx.Done():
		// Someone else already won while we were reading.
		conn.Close()
	}
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
