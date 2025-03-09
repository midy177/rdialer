package rdialer

import (
	"net"
	"strings"
	"time"

	"github.com/quic-go/webtransport-go"
)

type connection struct {
	addr   *addr
	stream webtransport.Stream
}

func newConnection(conn webtransport.Stream, proto, address string) (net.Conn, error) {
	return &connection{
		stream: conn,
		addr:   &addr{proto, address},
	}, nil
}

func (c *connection) Close() error {
	return c.stream.Close()
}

func (c *connection) SetDeadline(t time.Time) error {
	return c.stream.SetDeadline(t)
}

func (c *connection) SetReadDeadline(t time.Time) error {
	return c.stream.SetReadDeadline(t)
}

func (c *connection) SetWriteDeadline(t time.Time) error {
	return c.stream.SetWriteDeadline(t)
}

func (c *connection) LocalAddr() net.Addr {
	return c.addr
}

func (c *connection) RemoteAddr() net.Addr {
	return c.addr
}

func (c *connection) Read(p []byte) (int, error) {
	n, err := c.stream.Read(p)
	if err != nil {
		if strings.HasPrefix(err.Error(), "stream reset") {
			return n, nil
		}
	}
	return n, err
}

func (c *connection) Write(p []byte) (int, error) {
	// 直接使用 Stream 的写入方法，依赖 QUIC 的内置流控制
	return c.stream.Write(p)
}

type addr struct {
	proto   string
	address string
}

func (a *addr) Network() string {
	return a.proto
}

func (a *addr) String() string {
	return a.address
}
