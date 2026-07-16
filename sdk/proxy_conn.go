package sdk

import (
	"io"
	"net"
	"time"
)

// ProxyInfo describes a proxy accessible to the consumer.
type ProxyInfo struct {
	ID          string
	Name        string
	Type        string
	EntryHost   string
	EntryPort   int
	TargetHost  string
	TargetPort  int
	Description string
}

// ProxyConn wraps a multiplexed data stream to a proxy target. It implements
// io.ReadWriteCloser and can be adapted to net.Conn via DialTCP.
type ProxyConn struct {
	stream io.ReadWriteCloser
	addr   proxyAddr
}

func newProxyConn(stream io.ReadWriteCloser, proxyID string) *ProxyConn {
	return &ProxyConn{
		stream: stream,
		addr:   proxyAddr{proxyID: proxyID},
	}
}

func (c *ProxyConn) Read(p []byte) (int, error)  { return c.stream.Read(p) }
func (c *ProxyConn) Write(p []byte) (int, error) { return c.stream.Write(p) }
func (c *ProxyConn) Close() error                { return c.stream.Close() }

// LocalAddr returns a synthetic address identifying the proxy target.
func (c *ProxyConn) LocalAddr() net.Addr { return c.addr }

// RemoteAddr returns a synthetic address identifying the proxy target.
func (c *ProxyConn) RemoteAddr() net.Addr { return c.addr }

// SetDeadline is a no-op; the underlying stream does not support deadlines.
func (c *ProxyConn) SetDeadline(_ time.Time) error { return nil }

// SetReadDeadline is a no-op.
func (c *ProxyConn) SetReadDeadline(_ time.Time) error { return nil }

// SetWriteDeadline is a no-op.
func (c *ProxyConn) SetWriteDeadline(_ time.Time) error { return nil }

type proxyAddr struct {
	proxyID string
}

func (a proxyAddr) Network() string { return "goginx" }
func (a proxyAddr) String() string  { return a.proxyID }
