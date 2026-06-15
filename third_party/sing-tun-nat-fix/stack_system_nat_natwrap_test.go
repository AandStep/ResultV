package tun

import (
	"context"
	"net/netip"
	"testing"
	"time"

	M "github.com/sagernet/sing/common/metadata"
)

// stubHandler satisfies tun.Handler by embedding the (nil) interface and
// overriding only PrepareConnection, which is the sole method TCPNat.Lookup
// invokes. Any other call would panic, surfacing an unexpected code path.
type stubHandler struct {
	Handler
}

func (stubHandler) PrepareConnection(
	network string,
	source M.Socksaddr,
	destination M.Socksaddr,
	routeContext DirectRouteContext,
	timeout time.Duration,
) (DirectRouteDestination, error) {
	return nil, nil
}

// TestTCPNatDoesNotReuseLivePorts reproduces the "all connections drop at once"
// bug: under high connection churn the monotonic uint16 port counter wraps past
// 65535 and reuses the NAT ports of still-alive long-lived flows.
//
// RED (original Lookup): fails — a churned allocation eventually returns a port
// still held by one of the long-lived sessions.
// GREEN (free-port-scan Lookup): passes — live ports are never reused.
func TestTCPNatDoesNotReuseLivePorts(t *testing.T) {
	// Hour-long timeout: the background reaper never fires during the test, so
	// the only sessions removed are the ones we explicitly delete below.
	nat := NewNat(context.Background(), time.Hour)
	h := stubHandler{}
	dest := netip.MustParseAddrPort("1.2.3.4:443")

	mkSource := func(i int) netip.AddrPort {
		return netip.AddrPortFrom(
			netip.AddrFrom4([4]byte{10, byte(i >> 16), byte(i >> 8), byte(i)}),
			12345,
		)
	}

	// 50 long-lived flows (streaming / websocket / push). They stay live for the
	// whole test and must never have their NAT port reused.
	live := make(map[uint16]netip.AddrPort, 50)
	for i := 0; i < 50; i++ {
		src := mkSource(i)
		p, err := nat.Lookup(src, dest, h)
		if err != nil {
			t.Fatalf("long-lived Lookup %d failed: %v", i, err)
		}
		if prev, dup := live[p]; dup {
			t.Fatalf("two long-lived sessions got the same port %d (%v / %v)", p, prev, src)
		}
		live[p] = src
	}

	// Churn ~3.6x the usable port range. Each short-lived flow is immediately
	// reaped (delete simulates the connection closing + the timeout sweep), so
	// the port space never genuinely exhausts — exactly the real-world steady
	// state where held ports stay far below 55k.
	const churn = 200_000
	for i := 50; i < 50+churn; i++ {
		src := mkSource(i)
		p, err := nat.Lookup(src, dest, h)
		if err != nil {
			t.Fatalf("churn Lookup %d unexpectedly failed (false exhaustion): %v", i, err)
		}
		if liveSrc, clash := live[p]; clash {
			t.Fatalf("NAT reused port %d still held by live session %v for new session %v (iteration %d)",
				p, liveSrc, src, i)
		}
		// Simulate the short-lived flow closing and being reaped.
		nat.portAccess.Lock()
		nat.addrAccess.Lock()
		delete(nat.portMap, p)
		delete(nat.addrMap, src)
		nat.addrAccess.Unlock()
		nat.portAccess.Unlock()
	}

	// All 50 long-lived sessions must still be intact and correctly mapped.
	if got := len(nat.portMap); got != 50 {
		t.Fatalf("expected 50 live sessions remaining, got %d", got)
	}
	for p, src := range live {
		nat.portAccess.RLock()
		sess := nat.portMap[p]
		nat.portAccess.RUnlock()
		if sess == nil || sess.Source != src {
			t.Fatalf("live session on port %d corrupted: want source %v, got %+v", p, src, sess)
		}
	}
}
