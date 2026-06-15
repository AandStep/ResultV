package tun

import (
	"context"
	"net/netip"
	"sync"
	"time"

	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type TCPNat struct {
	timeout    time.Duration
	portIndex  uint16
	portAccess sync.RWMutex
	addrAccess sync.RWMutex
	addrMap    map[netip.AddrPort]uint16
	portMap    map[uint16]*TCPSession
}

type TCPSession struct {
	sync.Mutex
	Source      netip.AddrPort
	Destination netip.AddrPort
	LastActive  time.Time
}

func NewNat(ctx context.Context, timeout time.Duration) *TCPNat {
	natMap := &TCPNat{
		timeout:   timeout,
		portIndex: 10000,
		addrMap:   make(map[netip.AddrPort]uint16),
		portMap:   make(map[uint16]*TCPSession),
	}
	go natMap.loopCheckTimeout(ctx)
	return natMap
}

func (n *TCPNat) loopCheckTimeout(ctx context.Context) {
	ticker := time.NewTicker(n.timeout)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			n.checkTimeout()
		case <-ctx.Done():
			return
		}
	}
}

func (n *TCPNat) checkTimeout() {
	now := time.Now()
	n.portAccess.Lock()
	defer n.portAccess.Unlock()
	n.addrAccess.Lock()
	defer n.addrAccess.Unlock()
	for natPort, session := range n.portMap {
		session.Lock()
		if now.Sub(session.LastActive) > n.timeout {
			delete(n.addrMap, session.Source)
			delete(n.portMap, natPort)
		}
		session.Unlock()
	}
}

func (n *TCPNat) LookupBack(port uint16) *TCPSession {
	n.portAccess.RLock()
	session := n.portMap[port]
	n.portAccess.RUnlock()
	if session != nil {
		session.Lock()
		if time.Since(session.LastActive) > time.Second {
			session.LastActive = time.Now()
		}
		session.Unlock()
	}
	return session
}

func (n *TCPNat) Lookup(source netip.AddrPort, destination netip.AddrPort, handler Handler) (uint16, error) {
	n.addrAccess.RLock()
	port, loaded := n.addrMap[source]
	n.addrAccess.RUnlock()
	if loaded {
		return port, nil
	}
	_, pErr := handler.PrepareConnection(N.NetworkTCP, M.SocksaddrFromNetIP(source), M.SocksaddrFromNetIP(destination), nil, 0)
	if pErr != nil {
		return 0, pErr
	}
	// Allocate a NAT port that is NOT currently held by a live session.
	//
	// The previous implementation handed out n.portIndex and incremented it
	// monotonically (10000..65535, wrapping back to 10000 on uint16 overflow),
	// overwriting n.portMap[nextPort] unconditionally. Under high connection
	// churn the counter wraps after ~55k flows and starts reusing the NAT ports
	// of still-alive long-lived connections (streaming / websocket / push),
	// corrupting their reverse mapping (LookupBack) and tearing every one of
	// them down at the same instant — the "all connections drop at once,
	// WSAECONNABORTED" storm seen under agentic/go-test load.
	//
	// Scan forward from the rolling cursor for a slot no live session occupies.
	// Closed sessions are reaped within the NAT timeout, so in practice far
	// fewer than the ~55k available ports are held at any moment and this
	// resolves on the first probe. Genuine simultaneous-flow exhaustion now
	// fails loudly instead of silently killing an established connection.
	//
	// Lock order (portAccess then addrAccess) matches checkTimeout to avoid a
	// lock-ordering deadlock.
	n.portAccess.Lock()
	defer n.portAccess.Unlock()
	n.addrAccess.Lock()
	defer n.addrAccess.Unlock()
	const minNatPort = 10000
	nextPort := n.portIndex
	if nextPort < minNatPort {
		nextPort = minNatPort
	}
	var allocated bool
	for i := 0; i < 65535-minNatPort+1; i++ {
		if _, used := n.portMap[nextPort]; !used {
			allocated = true
			break
		}
		if nextPort == 65535 {
			nextPort = minNatPort
		} else {
			nextPort++
		}
	}
	if !allocated {
		return 0, E.New("system: tcp nat port space exhausted")
	}
	if nextPort == 65535 {
		n.portIndex = minNatPort
	} else {
		n.portIndex = nextPort + 1
	}
	n.addrMap[source] = nextPort
	n.portMap[nextPort] = &TCPSession{
		Source:      source,
		Destination: destination,
		LastActive:  time.Now(),
	}
	return nextPort, nil
}
