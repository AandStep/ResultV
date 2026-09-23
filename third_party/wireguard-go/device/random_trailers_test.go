package device_test

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sagernet/wireguard-go/conn"
	"github.com/sagernet/wireguard-go/device"
	"github.com/sagernet/wireguard-go/tun"
)

// TestRandomTrailersHandshake proves that a handshake completes when both sides
// append random trailers to handshake messages. The MACs belong at the end of
// the message itself, not at the end of the padded datagram.
func TestRandomTrailersHandshake(t *testing.T) {
	for _, withTrailers := range []bool{false, true} {
		name := "plain"
		if withTrailers {
			name = "trailers"
		}
		t.Run(name, func(t *testing.T) {
			serverPrivate, serverPublic := generateTestKeyPair(t)
			clientPrivate, clientPublic := generateTestKeyPair(t)
			network := newMemNetwork()

			server, _ := startMemDevice(t, network, "server", 1000, withTrailers,
				"private_key="+serverPrivate+"\npublic_key="+clientPublic+"\nallowed_ip=10.0.0.2/32")
			defer server.Close()
			client, clientTUN := startMemDevice(t, network, "client", 2000, withTrailers,
				"private_key="+clientPrivate+"\npublic_key="+serverPublic+"\nallowed_ip=10.0.0.1/32\nendpoint=127.0.0.1:1000")
			defer client.Close()

			for attempt := 0; attempt < 3; attempt++ {
				clientTUN.inbound <- buildTestPacket()
				if waitForHandshake(client, 6*time.Second) {
					return
				}
			}
			t.Fatal("handshake did not complete")
		})
	}
}

func startMemDevice(t *testing.T, network *memNetwork, name string, port uint16, trailers bool, config string) (*device.Device, *testTUN) {
	tunDevice := &testTUN{
		name:    name,
		inbound: make(chan []byte, 16),
		events:  make(chan tun.Event, 1),
		done:    make(chan struct{}),
	}
	bind := &memBind{network: network, port: port}
	wgDevice := device.NewDevice(context.Background(), tunDevice, bind, device.NewLogger(device.LogLevelError, name+": "), 0, 0, false)
	if trailers {
		config = "random_trailers=true\n" + config
	}
	config = "listen_port=" + strconv.Itoa(int(port)) + "\n" + config
	if err := wgDevice.IpcSet(config); err != nil {
		t.Fatal(err)
	}
	if err := wgDevice.Up(); err != nil {
		t.Fatal(err)
	}
	return wgDevice, tunDevice
}

func waitForHandshake(wgDevice *device.Device, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		config, err := wgDevice.IpcGet()
		if err == nil {
			for _, line := range strings.Split(config, "\n") {
				if value, ok := strings.CutPrefix(line, "last_handshake_time_sec="); ok && value != "0" {
					return true
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

type memNetwork struct {
	mu      sync.Mutex
	ports   map[uint16]chan memDatagram
	largest int
}

type memDatagram struct {
	data []byte
	from uint16
}

func newMemNetwork() *memNetwork {
	return &memNetwork{ports: make(map[uint16]chan memDatagram)}
}

type memEndpoint struct{ addr netip.AddrPort }

func (e memEndpoint) ClearSrc()           {}
func (e memEndpoint) SrcToString() string { return "" }
func (e memEndpoint) DstToString() string { return e.addr.String() }
func (e memEndpoint) DstToBytes() []byte  { b, _ := e.addr.MarshalBinary(); return b }
func (e memEndpoint) DstIP() netip.Addr   { return e.addr.Addr() }
func (e memEndpoint) SrcIP() netip.Addr   { return netip.Addr{} }

type memBind struct {
	network *memNetwork
	port    uint16
	inbox   chan memDatagram
	closed  chan struct{}
}

func (b *memBind) Open(uint16) ([]conn.ReceiveFunc, uint16, error) {
	b.inbox = make(chan memDatagram, 256)
	b.closed = make(chan struct{})
	b.network.mu.Lock()
	b.network.ports[b.port] = b.inbox
	b.network.mu.Unlock()
	inbox, closed := b.inbox, b.closed
	receive := func(packets [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
		select {
		case d := <-inbox:
			sizes[0] = copy(packets[0], d.data)
			eps[0] = memEndpoint{netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), d.from)}
			return 1, nil
		case <-closed:
			return 0, net.ErrClosed
		}
	}
	return []conn.ReceiveFunc{receive}, b.port, nil
}

func (b *memBind) Close() error {
	if b.closed != nil {
		b.network.mu.Lock()
		delete(b.network.ports, b.port)
		b.network.mu.Unlock()
		close(b.closed)
		b.closed = nil
	}
	return nil
}

func (b *memBind) SetMark(uint32) error { return nil }
func (b *memBind) BatchSize() int       { return 1 }

func (b *memBind) ParseEndpoint(s string) (conn.Endpoint, error) {
	addr, err := netip.ParseAddrPort(s)
	if err != nil {
		return nil, err
	}
	return memEndpoint{addr}, nil
}

func (b *memBind) Send(bufs [][]byte, ep conn.Endpoint, offset int) error {
	b.network.mu.Lock()
	inbox := b.network.ports[netip.MustParseAddrPort(ep.DstToString()).Port()]
	b.network.mu.Unlock()
	if inbox == nil {
		return nil
	}
	for _, buf := range bufs {
		data := append([]byte(nil), buf[offset:]...)
		b.network.mu.Lock()
		b.network.largest = max(b.network.largest, len(data))
		b.network.mu.Unlock()
		select {
		case inbox <- memDatagram{data: data, from: b.port}:
		default:
		}
	}
	return nil
}

// TestRandomTrailersInputPacket sends small packets through InputPacket, whose
// buffers are sized to the packet. A trailer must neither overrun that buffer
// nor put anything but the sealed packet on the wire.
func TestRandomTrailersInputPacket(t *testing.T) {
	serverPrivate, serverPublic := generateTestKeyPair(t)
	clientPrivate, clientPublic := generateTestKeyPair(t)
	network := newMemNetwork()

	server, _ := startMemDevice(t, network, "server", 1000, true,
		"private_key="+serverPrivate+"\npublic_key="+clientPublic+"\nallowed_ip=10.0.0.2/32")
	defer server.Close()
	client, clientTUN := startMemDevice(t, network, "client", 2000, true,
		"private_key="+clientPrivate+"\npublic_key="+serverPublic+"\nallowed_ip=10.0.0.1/32\nendpoint=127.0.0.1:1000")
	defer client.Close()

	clientTUN.inbound <- buildTestPacket()
	if !waitForHandshake(client, 10*time.Second) {
		t.Fatal("handshake did not complete")
	}
	before := rxBytes(t, server)
	network.mu.Lock()
	network.largest = 0
	network.mu.Unlock()

	const count = 200
	destination := netip.MustParseAddr("10.0.0.1").AsSlice()
	for i := 0; i < count; i++ {
		client.InputPacket(destination, [][]byte{buildTestPacket()})
	}

	want := before + count*len(buildTestPacket())
	deadline := time.Now().Add(5 * time.Second)
	for rxBytes(t, server) < want {
		if time.Now().After(deadline) {
			t.Fatalf("server decrypted %d bytes, want at least %d", rxBytes(t, server)-before, want-before)
		}
		time.Sleep(20 * time.Millisecond)
	}

	network.mu.Lock()
	largest := network.largest
	network.mu.Unlock()
	if plain := 12 + 16 + len(buildTestPacket()) + 16 + 16; largest <= plain {
		t.Fatalf("largest datagram %d bytes, no trailers beyond %d", largest, plain)
	}
}

func rxBytes(t *testing.T, wgDevice *device.Device) int {
	config, err := wgDevice.IpcGet()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(config, "\n") {
		if value, ok := strings.CutPrefix(line, "rx_bytes="); ok {
			n, _ := strconv.Atoi(value)
			return n
		}
	}
	return 0
}

// TestRandomTrailersEnabledMidHandshake covers trailers switched on after the
// first initiation already left without one: the answer to it carries a
// trailer the client drops, so the switch has to start a fresh handshake
// instead of leaving the peer to wait out the retry timeout.
func TestRandomTrailersEnabledMidHandshake(t *testing.T) {
	serverPrivate, serverPublic := generateTestKeyPair(t)
	clientPrivate, clientPublic := generateTestKeyPair(t)
	network := newMemNetwork()

	server, _ := startMemDevice(t, network, "server", 1000, true,
		"private_key="+serverPrivate+"\npublic_key="+clientPublic+"\nallowed_ip=10.0.0.2/32")
	defer server.Close()
	client, clientTUN := startMemDevice(t, network, "client", 2000, false,
		"private_key="+clientPrivate+"\npublic_key="+serverPublic+"\nallowed_ip=10.0.0.1/32\nendpoint=127.0.0.1:1000")
	defer client.Close()

	clientTUN.inbound <- buildTestPacket()
	time.Sleep(300 * time.Millisecond)
	if waitForHandshake(client, 50*time.Millisecond) {
		t.Skip("the server's answer happened to carry an empty trailer")
	}

	if err := client.IpcSet("random_trailers=true"); err != nil {
		t.Fatal(err)
	}
	if !waitForHandshake(client, 1500*time.Millisecond) {
		t.Fatal("handshake did not restart when trailers were switched on")
	}
}
