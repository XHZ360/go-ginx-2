package localproxy

import (
	"context"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/control"
)

func TestVirtualSessionConnectsAllowedTCPTarget(t *testing.T) {
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	go func() {
		conn, acceptErr := target.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()

	repo := &memoryAllowlistRepository{entries: DefaultAllowlist}
	policy, err := LoadPolicy(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	virtual := Session{Dialer: Dialer{Policy: policy, Timeout: time.Second}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stream, err := virtual.OpenStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	host, rawPort, _ := net.SplitHostPort(target.Addr().String())
	port, _ := strconv.Atoi(rawPort)
	if err := control.WriteMessage(stream, control.MessageOpenStream, control.OpenStream{ProxyID: "proxy-1", TargetHost: host, TargetPort: port}); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 4)
	if _, err := io.ReadFull(stream, buffer); err != nil {
		t.Fatal(err)
	}
	if string(buffer) != "ping" {
		t.Fatalf("unexpected echo: %q", buffer)
	}
}

func TestVirtualSessionClosesDeniedTarget(t *testing.T) {
	repo := &memoryAllowlistRepository{entries: DefaultAllowlist}
	policy, err := LoadPolicy(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := (Session{Dialer: Dialer{Policy: policy, Timeout: time.Second}}).OpenStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if err := control.WriteMessage(stream, control.MessageOpenStream, control.OpenStream{ProxyID: "proxy-1", TargetHost: "192.0.2.1", TargetPort: 80}); err != nil {
		t.Fatal(err)
	}
	_ = stream.(net.Conn).SetReadDeadline(time.Now().Add(time.Second))
	if _, err := stream.Read(make([]byte, 1)); err == nil {
		t.Fatal("expected denied stream to close")
	}
}

func TestVirtualSessionClosesUnreachableAllowedTarget(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host, rawPort, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(rawPort)
	_ = listener.Close()
	repo := &memoryAllowlistRepository{entries: DefaultAllowlist}
	policy, err := LoadPolicy(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := (Session{Dialer: Dialer{Policy: policy, Timeout: 200 * time.Millisecond}}).OpenStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if err := control.WriteMessage(stream, control.MessageOpenStream, control.OpenStream{ProxyID: "proxy-1", TargetHost: host, TargetPort: port}); err != nil {
		t.Fatal(err)
	}
	_ = stream.(net.Conn).SetReadDeadline(time.Now().Add(time.Second))
	if _, err := stream.Read(make([]byte, 1)); err == nil {
		t.Fatal("expected unreachable target stream to close")
	}
}
