package control

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestTCPTLSMuxOpenDataClose(t *testing.T) {
	leftConn, rightConn := net.Pipe()
	left := newTCPTLSMux(leftConn, 1)
	right := newTCPTLSMux(rightConn, 2)
	left.Start()
	right.Start()
	t.Cleanup(func() { _ = left.Close() })
	t.Cleanup(func() { _ = right.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	leftStream, err := left.OpenStream(ctx)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	rightStream, err := right.AcceptStream(ctx)
	if err != nil {
		t.Fatalf("accept stream: %v", err)
	}

	go func() { _, _ = leftStream.Write([]byte("ping")) }()
	payload := make([]byte, 4)
	if _, err := io.ReadFull(rightStream, payload); err != nil {
		t.Fatalf("read payload: %v", err)
	}
	if string(payload) != "ping" {
		t.Fatalf("unexpected payload %q", string(payload))
	}

	if err := leftStream.Close(); err != nil {
		t.Fatalf("close stream: %v", err)
	}
	if _, err := rightStream.Read(payload); err != io.EOF {
		t.Fatalf("expected EOF after close, got %v", err)
	}
}

func TestTCPTLSMuxControlStream(t *testing.T) {
	leftConn, rightConn := net.Pipe()
	left := newTCPTLSMux(leftConn, 1)
	right := newTCPTLSMux(rightConn, 2)
	left.Start()
	right.Start()
	t.Cleanup(func() { _ = left.Close() })
	t.Cleanup(func() { _ = right.Close() })

	go func() {
		_ = WriteMessage(left.ControlStream(), MessageHeartbeat, Heartbeat{SessionID: "session-1", ClientID: "client-1", ObservedAt: time.Now().UTC()})
	}()
	envelope, err := ReadMessage(right.ControlStream())
	if err != nil {
		t.Fatalf("read control message: %v", err)
	}
	if envelope.Type != MessageHeartbeat {
		t.Fatalf("unexpected control message %s", envelope.Type)
	}
}

func TestTCPTLSMuxOpenWaitsForReady(t *testing.T) {
	leftConn, rightConn := net.Pipe()
	left := newTCPTLSMux(leftConn, 1)
	right := newTCPTLSMux(rightConn, 2)
	t.Cleanup(func() { _ = left.Close() })
	t.Cleanup(func() { _ = right.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := left.OpenStream(ctx); err == nil {
		t.Fatal("expected open stream to wait for mux readiness")
	}
}

func TestTCPTLSMuxUsesPartitionedStreamIDs(t *testing.T) {
	leftConn, rightConn := net.Pipe()
	left := newTCPTLSMux(leftConn, 1)
	right := newTCPTLSMux(rightConn, 2)
	left.Start()
	right.Start()
	t.Cleanup(func() { _ = left.Close() })
	t.Cleanup(func() { _ = right.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	leftStream, err := left.OpenStream(ctx)
	if err != nil {
		t.Fatalf("open left stream: %v", err)
	}
	rightStream, err := right.OpenStream(ctx)
	if err != nil {
		t.Fatalf("open right stream: %v", err)
	}
	if leftStream.(*muxStream).id%2 == rightStream.(*muxStream).id%2 {
		t.Fatalf("expected partitioned stream ids, got %d and %d", leftStream.(*muxStream).id, rightStream.(*muxStream).id)
	}
}

func TestTCPTLSMuxRejectsPeerLocalParityStreamID(t *testing.T) {
	leftConn, rightConn := net.Pipe()
	left := newTCPTLSMux(leftConn, 1)
	left.Start()
	t.Cleanup(func() { _ = left.Close() })
	t.Cleanup(func() { _ = rightConn.Close() })

	if err := WriteMuxFrame(rightConn, MuxFrame{StreamID: 1, Type: MuxFrameOpen}); err != nil {
		t.Fatalf("write invalid open: %v", err)
	}
	frame, err := ReadMuxFrame(rightConn)
	if err != nil {
		t.Fatalf("read reset: %v", err)
	}
	if frame.Type != MuxFrameReset {
		t.Fatalf("expected reset for invalid stream id, got %+v", frame)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := left.AcceptStream(ctx); err == nil {
		t.Fatal("expected invalid local-parity stream id to be rejected")
	}
}

func TestTCPTLSMuxOpenSkipsPreclaimedStreamID(t *testing.T) {
	leftConn, rightConn := net.Pipe()
	left := newTCPTLSMux(leftConn, 1)
	left.Start()
	t.Cleanup(func() { _ = left.Close() })
	t.Cleanup(func() { _ = rightConn.Close() })

	left.mu.Lock()
	left.streams[1] = newMuxStream(left, 1)
	left.mu.Unlock()

	openDone := make(chan io.ReadWriteCloser, 1)
	go func() {
		stream, _ := left.OpenStream(context.Background())
		openDone <- stream
	}()
	frame, err := ReadMuxFrame(rightConn)
	if err != nil {
		t.Fatalf("read open frame: %v", err)
	}
	stream := <-openDone
	if frame.StreamID != 3 || stream.(*muxStream).id != 3 {
		t.Fatalf("expected stream id 3 after preclaimed id, got frame=%d stream=%d", frame.StreamID, stream.(*muxStream).id)
	}
}

func TestTCPTLSMuxOpenFailsWhenClosedBeforeReady(t *testing.T) {
	leftConn, rightConn := net.Pipe()
	left := newTCPTLSMux(leftConn, 1)
	t.Cleanup(func() { _ = rightConn.Close() })
	_ = left.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := left.OpenStream(ctx); !errors.Is(err, errMuxClosed) {
		t.Fatalf("expected closed mux error, got %v", err)
	}
}

func TestTCPTLSMuxAcceptQueueOverflowResets(t *testing.T) {
	leftConn, rightConn := net.Pipe()
	left := newTCPTLSMux(leftConn, 1)
	left.Start()
	t.Cleanup(func() { _ = left.Close() })
	t.Cleanup(func() { _ = rightConn.Close() })

	for streamID := uint64(2); streamID < 2+uint64(cap(left.accept))*2; streamID += 2 {
		if err := WriteMuxFrame(rightConn, MuxFrame{StreamID: streamID, Type: MuxFrameOpen}); err != nil {
			t.Fatalf("write open %d: %v", streamID, err)
		}
	}
	if err := WriteMuxFrame(rightConn, MuxFrame{StreamID: 2 + uint64(cap(left.accept))*2, Type: MuxFrameOpen}); err != nil {
		t.Fatalf("write overflow open: %v", err)
	}
	frame, err := ReadMuxFrame(rightConn)
	if err != nil {
		t.Fatalf("read reset: %v", err)
	}
	if frame.Type != MuxFrameReset {
		t.Fatalf("expected reset for accept overflow, got %+v", frame)
	}
}

func TestTCPTLSMuxWriteAfterCloseFails(t *testing.T) {
	leftConn, rightConn := net.Pipe()
	left := newTCPTLSMux(leftConn, 1)
	right := newTCPTLSMux(rightConn, 2)
	left.Start()
	right.Start()
	t.Cleanup(func() { _ = left.Close() })
	t.Cleanup(func() { _ = right.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stream, err := left.OpenStream(ctx)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("close stream: %v", err)
	}
	if _, err := stream.Write([]byte("late")); err == nil {
		t.Fatal("expected write after close to fail")
	}
}
