package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"

	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

var (
	errSocks5NoResponse         = errors.New("SOCKS5 server did not answer")
	errSocks5UnadvertisedMethod = errors.New("SOCKS5 server selected an unadvertised authentication method")
)

type socks5MethodDialer struct {
	N.Dialer
	method byte
	server M.Socksaddr
}

func (d socks5MethodDialer) DialContext(ctx context.Context, network string, addr M.Socksaddr) (net.Conn, error) {
	if N.NetworkName(network) == N.NetworkUDP {
		if addr.Addr.IsValid() && addr.Addr.IsUnspecified() {
			addr = M.Socksaddr{Addr: d.server.Addr, Fqdn: d.server.Fqdn, Port: addr.Port}
		}
		return d.Dialer.DialContext(ctx, network, addr)
	}
	conn, err := d.Dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	return &socks5MethodConn{Conn: conn, reader: bufio.NewReaderSize(conn, 16), method: d.method}, nil
}

// The core validates the server's method after this check, but does not check
// whether that method was offered. Keep the response buffered for its parser.
type socks5MethodConn struct {
	net.Conn
	reader  *bufio.Reader
	method  byte
	checked bool
}

func (c *socks5MethodConn) Read(data []byte) (int, error) {
	if !c.checked && len(data) != 0 {
		reply, err := c.reader.Peek(2)
		if err != nil {
			if c.reader.Buffered() == 0 {
				return 0, fmt.Errorf("%w: %w", errSocks5NoResponse, err)
			}
			return 0, err
		}
		if reply[0] != 5 || reply[1] != c.method {
			return 0, errSocks5UnadvertisedMethod
		}
		c.checked = true
	}
	return c.reader.Read(data)
}
