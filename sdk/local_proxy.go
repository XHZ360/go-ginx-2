package sdk

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
)

// StartLocalProxy starts a local listener on addr that forwards all accepted
// connections to the specified proxy ID's fixed target. The listener stops
// when ctx is cancelled. It supports direct TCP, SOCKS5, and HTTP CONNECT
// handshakes as compatibility layers, but the remote target is always the
// proxy's server-configured fixed target — client-supplied destinations in
// SOCKS5 or CONNECT requests are not used.
//
// This is NOT an arbitrary-destination forward proxy. All connections are
// tunneled to the same fixed proxy target.
func (c *Client) StartLocalProxy(ctx context.Context, addr string, proxyID string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("sdk: local proxy listen: %w", err)
	}

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("sdk: local proxy accept: %w", err)
		}
		go c.handleLocalConn(ctx, conn, proxyID)
	}
}

func (c *Client) handleLocalConn(ctx context.Context, conn net.Conn, proxyID string) {
	defer conn.Close()

	// Read the first byte to detect protocol.
	buf := make([]byte, 1)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return
	}

	if buf[0] == 0x05 {
		// SOCKS5 handshake.
		c.handleLocalSOCKS5(ctx, conn, proxyID)
		return
	}

	if buf[0] == 'C' || buf[0] == 'c' {
		// Possible HTTP CONNECT. Read the rest of the first line from conn
		// (the first byte is already consumed).
		connReader := bufio.NewReaderSize(conn, 256)
		line, err := connReader.ReadString('\n')
		if err != nil {
			return
		}
		if strings.HasPrefix(strings.ToUpper(string(buf[:])+line), "CONNECT ") {
			c.handleLocalHTTPCONNECT(ctx, conn, connReader, proxyID)
			return
		}
		// Not CONNECT — fall through. Forward the first byte then the rest.
		buffered := append([]byte{}, buf[0])
		buffered = append(buffered, []byte(line)...)
		if n := connReader.Buffered(); n > 0 {
			peeked, err := connReader.Peek(n)
			if err == nil {
				buffered = append(buffered, peeked...)
				_, _ = connReader.Discard(n)
			}
		}
		c.forwardLocalDirect(ctx, conn, bytes.NewReader(buffered), proxyID)
		return
	}

	// Direct TCP forwarding: forward the first byte then the rest of the connection.
	c.forwardLocalDirect(ctx, conn, bytes.NewReader(buf), proxyID)
}

func (c *Client) forwardLocalDirect(ctx context.Context, local net.Conn, buffered io.Reader, proxyID string) {
	remote, err := c.Dial(ctx, proxyID)
	if err != nil {
		return
	}
	defer remote.Close()

	// Flush only bytes already consumed during protocol detection, then switch
	// to bidirectional copying so request/response protocols do not wait for EOF.
	if _, err := io.Copy(remote, buffered); err != nil {
		return
	}

	copyBidirectional(local, remote)
}

func (c *Client) handleLocalSOCKS5(ctx context.Context, conn net.Conn, proxyID string) {
	// Read SOCKS5 greeting: version (already read), nmethods, methods.
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return
	}
	nmethods := int(header[1])
	methods := make([]byte, nmethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}
	// Reply: no authentication required.
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// Read SOCKS5 request: version, cmd, rsv, atyp, dst.addr, dst.port.
	reqHeader := make([]byte, 4)
	if _, err := io.ReadFull(conn, reqHeader); err != nil {
		return
	}
	// Read the address based on address type (we don't use it, but must consume it).
	switch reqHeader[3] {
	case 0x01: // IPv4
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return
		}
	case 0x03: // Domain
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return
		}
		domain := make([]byte, lenBuf[0])
		if _, err := io.ReadFull(conn, domain); err != nil {
			return
		}
	case 0x04: // IPv6
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return
		}
	default:
		// Send failure reply.
		_, _ = conn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	// Read port (2 bytes).
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return
	}

	// Connect to the fixed proxy target.
	remote, err := c.Dial(ctx, proxyID)
	if err != nil {
		// SOCKS5 general failure.
		_, _ = conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer remote.Close()

	// SOCKS5 success reply (bind address 0.0.0.0:0).
	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	copyBidirectional(conn, remote)
}

func (c *Client) handleLocalHTTPCONNECT(ctx context.Context, conn net.Conn, reader *bufio.Reader, proxyID string) {
	// Drain remaining headers until empty line. Use ReadSlice which operates
	// on the existing buffer without calling fill() unless the buffer is empty.
	for {
		line, err := reader.ReadSlice('\n')
		if err != nil {
			return
		}
		if string(line) == "\r\n" || string(line) == "\n" {
			break
		}
	}

	// Connect to the fixed proxy target.
	remote, err := c.Dial(ctx, proxyID)
	if err != nil {
		_, _ = conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer remote.Close()

	// HTTP 200 Connection Established.
	if _, err := conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		return
	}

	copyBidirectional(conn, remote)
}

func copyBidirectional(a net.Conn, b io.ReadWriteCloser) {
	var wg sync.WaitGroup
	wg.Add(2)
	closeBoth := func() {
		_ = a.Close()
		_ = b.Close()
	}
	go func() {
		defer wg.Done()
		_, _ = io.Copy(b, a)
		closeBoth()
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(a, b)
		closeBoth()
	}()
	wg.Wait()
}
