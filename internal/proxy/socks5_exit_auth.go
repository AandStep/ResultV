package proxy

import (
	"bufio"
	"context"
	"errors"
	"net"

	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type socks5MethodDialer struct {
	N.Dialer
	method byte
}

func (d socks5MethodDialer) DialContext(ctx context.Context, network string, addr M.Socksaddr) (net.Conn, error) {
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
			return 0, err
		}
		if reply[0] != 5 || reply[1] != c.method {
			return 0, errors.New("SOCKS5 server selected an unadvertised authentication method")
		}
		c.checked = true
	}
	return c.reader.Read(data)
}
