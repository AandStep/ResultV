package device_test

import (
	"context"

	"github.com/sagernet/wireguard-go/device"
	"github.com/sagernet/wireguard-go/tun"

	"testing"
	"time"
)

// A rebind closes the socket an initiation left through, so its answer has
// nowhere to arrive. The handshake must start over on the new socket at once
// instead of sitting out the retry timeout.
func TestBindUpdateRestartsHandshakeInFlight(t *testing.T) {
	serverPrivate, serverPublic := generateTestKeyPair(t)
	clientPrivate, clientPublic := generateTestKeyPair(t)
	network := newMemNetwork()

	client, clientTUN := startMemDevice(t, network, "client", 2000, false,
		"private_key="+clientPrivate+"\npublic_key="+serverPublic+"\nallowed_ip=10.0.0.1/32\nendpoint=127.0.0.1:1000")
	defer client.Close()

	clientTUN.inbound <- buildTestPacket()
	time.Sleep(300 * time.Millisecond)

	server, _ := startMemDevice(t, network, "server", 1000, false,
		"private_key="+serverPrivate+"\npublic_key="+clientPublic+"\nallowed_ip=10.0.0.2/32")
	defer server.Close()

	if err := client.BindUpdate(); err != nil {
		t.Fatal(err)
	}
	if !waitForHandshake(client, 1500*time.Millisecond) {
		t.Fatal("после BindUpdate рукопожатие ждёт таймаута повтора")
	}
}

// The server has already consumed the first initiation, so a second one sent
// within the same timestamp tick or the flood window is dropped as a replay.
// The restart must leave enough of a gap for the server to take it.
func TestBindUpdateRestartIsNotAReplay(t *testing.T) {
	serverPrivate, serverPublic := generateTestKeyPair(t)
	clientPrivate, clientPublic := generateTestKeyPair(t)
	network := newMemNetwork()

	server, _ := startMemDevice(t, network, "server", 1000, false,
		"private_key="+serverPrivate+"\npublic_key="+clientPublic+"\nallowed_ip=10.0.0.2/32")
	defer server.Close()
	client, clientTUN := startMemDevice(t, network, "client", 2000, false,
		"private_key="+clientPrivate+"\npublic_key="+serverPublic+"\nallowed_ip=10.0.0.1/32\nendpoint=127.0.0.1:1000")
	defer client.Close()

	network.mu.Lock()
	network.dropTo = map[uint16]bool{2000: true}
	network.mu.Unlock()

	clientTUN.inbound <- buildTestPacket()
	time.Sleep(5 * time.Millisecond)

	network.mu.Lock()
	network.dropTo = nil
	network.mu.Unlock()

	if err := client.BindUpdate(); err != nil {
		t.Fatal(err)
	}
	if !waitForHandshake(client, 1500*time.Millisecond) {
		t.Fatal("повторная инициация отброшена сервером как повтор")
	}
}

// A network that drops one UDP flow drops every retry sent through it. With
// no fixed listen port the retry has to leave from a new one.
func TestHandshakeRetryLeavesBlackholedFlow(t *testing.T) {
	serverPrivate, serverPublic := generateTestKeyPair(t)
	clientPrivate, clientPublic := generateTestKeyPair(t)
	network := newMemNetwork()

	server, _ := startMemDevice(t, network, "server", 1000, false,
		"private_key="+serverPrivate+"\npublic_key="+clientPublic+"\nallowed_ip=10.0.0.2/32")
	defer server.Close()

	const firstPort = 2001
	network.mu.Lock()
	network.dropFrom = map[uint16]bool{firstPort: true}
	network.mu.Unlock()

	clientTUN := &testTUN{
		name:    "client",
		inbound: make(chan []byte, 16),
		events:  make(chan tun.Event, 1),
		done:    make(chan struct{}),
	}
	bind := &memBind{network: network, port: firstPort - 1, ephemeral: true}
	client := device.NewDevice(context.Background(), clientTUN, bind, device.NewLogger(device.LogLevelError, "client: "), 0, 0, false)
	defer client.Close()
	if err := client.IpcSet("private_key=" + clientPrivate + "\nrekey_timeout=1-1\npublic_key=" + serverPublic + "\nallowed_ip=10.0.0.1/32\nendpoint=127.0.0.1:1000"); err != nil {
		t.Fatal(err)
	}
	if err := client.Up(); err != nil {
		t.Fatal(err)
	}

	clientTUN.inbound <- buildTestPacket()
	if !waitForHandshake(client, 2500*time.Millisecond) {
		t.Fatal("повтор рукопожатия ушёл через тот же заглушённый поток")
	}
}
