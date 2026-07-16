package control

import (
	"context"
	"errors"
	"io"
	"sync"
)

var errMuxClosed = errors.New("tcp tls mux closed")

const maxMuxStreams = 256

type tcpTLSMux struct {
	conn    io.ReadWriteCloser
	writeMu sync.Mutex

	mu      sync.Mutex
	started bool
	closed  bool
	nextID  uint64
	localID uint64
	streams map[uint64]*muxStream
	accept  chan *muxStream
	ready   chan struct{}
	done    chan struct{}
}

type muxStream struct {
	mux      *tcpTLSMux
	id       uint64
	inbound  chan []byte
	readBuf  []byte
	readDone chan struct{}
	mu       sync.Mutex
	closed   bool
	close    sync.Once
}

func newTCPTLSMux(conn io.ReadWriteCloser, nextID uint64) *tcpTLSMux {
	mux := &tcpTLSMux{conn: conn, nextID: nextID, localID: nextID % 2, streams: make(map[uint64]*muxStream), accept: make(chan *muxStream, 128), ready: make(chan struct{}), done: make(chan struct{})}
	mux.streams[0] = newMuxStream(mux, 0)
	return mux
}

func newMuxStream(mux *tcpTLSMux, id uint64) *muxStream {
	return &muxStream{mux: mux, id: id, inbound: make(chan []byte, 32), readDone: make(chan struct{})}
}

func (mux *tcpTLSMux) Start() {
	mux.mu.Lock()
	if mux.started || mux.closed {
		mux.mu.Unlock()
		return
	}
	mux.started = true
	close(mux.ready)
	mux.mu.Unlock()
	go mux.readLoop()
}

func (mux *tcpTLSMux) ControlStream() io.ReadWriteCloser {
	mux.mu.Lock()
	defer mux.mu.Unlock()
	return mux.streams[0]
}

func (mux *tcpTLSMux) OpenStream(ctx context.Context) (io.ReadWriteCloser, error) {
	select {
	case <-mux.ready:
	case <-mux.done:
		return nil, errMuxClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	mux.mu.Lock()
	if mux.closed {
		mux.mu.Unlock()
		return nil, errMuxClosed
	}
	if len(mux.streams) >= maxMuxStreams {
		mux.mu.Unlock()
		return nil, errors.New("too many mux streams")
	}
	streamID := mux.nextAvailableStreamID()
	stream := newMuxStream(mux, streamID)
	mux.streams[streamID] = stream
	mux.mu.Unlock()
	if err := mux.writeFrame(ctx, MuxFrame{StreamID: streamID, Type: MuxFrameOpen}); err != nil {
		mux.removeStream(streamID)
		return nil, err
	}
	return stream, nil
}

func (mux *tcpTLSMux) AcceptStream(ctx context.Context) (io.ReadWriteCloser, error) {
	select {
	case stream := <-mux.accept:
		return stream, nil
	case <-mux.done:
		return nil, errMuxClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (mux *tcpTLSMux) Close() error {
	mux.closeAll()
	return mux.conn.Close()
}

func (mux *tcpTLSMux) readLoop() {
	defer mux.closeAll()
	for {
		frame, err := ReadMuxFrame(mux.conn)
		if err != nil {
			return
		}
		switch frame.Type {
		case MuxFrameOpen:
			mux.handleOpen(frame.StreamID)
		case MuxFrameData:
			if stream := mux.stream(frame.StreamID); stream != nil {
				stream.deliver(frame.Payload)
			}
		case MuxFrameClose, MuxFrameReset:
			if stream := mux.stream(frame.StreamID); stream != nil {
				stream.remoteClose()
			}
		}
	}
}

func (mux *tcpTLSMux) handleOpen(streamID uint64) {
	if streamID == 0 || streamID%2 == mux.localID {
		_ = mux.writeFrame(context.Background(), MuxFrame{StreamID: streamID, Type: MuxFrameReset, Reason: "invalid stream id"})
		return
	}
	stream := newMuxStream(mux, streamID)
	mux.mu.Lock()
	if mux.closed || len(mux.streams) >= maxMuxStreams {
		mux.mu.Unlock()
		return
	}
	if _, exists := mux.streams[streamID]; exists {
		mux.mu.Unlock()
		_ = mux.writeFrame(context.Background(), MuxFrame{StreamID: streamID, Type: MuxFrameReset, Reason: "duplicate stream id"})
		return
	}
	mux.streams[streamID] = stream
	mux.mu.Unlock()
	select {
	case mux.accept <- stream:
	case <-mux.done:
	default:
		mux.removeStream(streamID)
		stream.remoteClose()
		_ = mux.writeFrame(context.Background(), MuxFrame{StreamID: streamID, Type: MuxFrameReset, Reason: "accept queue full"})
	}
}

func (mux *tcpTLSMux) nextAvailableStreamID() uint64 {
	for {
		streamID := mux.nextID
		mux.nextID += 2
		if _, exists := mux.streams[streamID]; !exists {
			return streamID
		}
	}
}

func (mux *tcpTLSMux) stream(streamID uint64) *muxStream {
	mux.mu.Lock()
	defer mux.mu.Unlock()
	return mux.streams[streamID]
}

func (mux *tcpTLSMux) removeStream(streamID uint64) {
	if streamID == 0 {
		return
	}
	mux.mu.Lock()
	delete(mux.streams, streamID)
	mux.mu.Unlock()
}

func (mux *tcpTLSMux) writeFrame(ctx context.Context, frame MuxFrame) error {
	select {
	case <-mux.done:
		return errMuxClosed
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	mux.writeMu.Lock()
	defer mux.writeMu.Unlock()
	return WriteMuxFrame(mux.conn, frame)
}

func (mux *tcpTLSMux) closeAll() {
	mux.mu.Lock()
	if mux.closed {
		mux.mu.Unlock()
		return
	}
	mux.closed = true
	close(mux.done)
	streams := make([]*muxStream, 0, len(mux.streams))
	for _, stream := range mux.streams {
		streams = append(streams, stream)
	}
	mux.streams = make(map[uint64]*muxStream)
	mux.mu.Unlock()
	for _, stream := range streams {
		stream.remoteClose()
	}
}

func (stream *muxStream) Read(p []byte) (int, error) {
	for len(stream.readBuf) == 0 {
		select {
		case payload := <-stream.inbound:
			stream.readBuf = payload
		case <-stream.readDone:
			return 0, io.EOF
		}
	}
	n := copy(p, stream.readBuf)
	stream.readBuf = stream.readBuf[n:]
	return n, nil
}

func (stream *muxStream) Write(p []byte) (int, error) {
	if stream.isClosed() {
		return 0, io.ErrClosedPipe
	}
	for written := 0; written < len(p); {
		end := min(written+maxMuxPayloadSize, len(p))
		if stream.isClosed() {
			return written, io.ErrClosedPipe
		}
		if err := stream.mux.writeFrame(context.Background(), MuxFrame{StreamID: stream.id, Type: MuxFrameData, Payload: p[written:end]}); err != nil {
			return written, err
		}
		written = end
	}
	return len(p), nil
}

func (stream *muxStream) Close() error {
	_ = stream.mux.writeFrame(context.Background(), MuxFrame{StreamID: stream.id, Type: MuxFrameClose})
	stream.finish()
	stream.mux.removeStream(stream.id)
	return nil
}

func (stream *muxStream) deliver(payload []byte) {
	if stream.isClosed() {
		return
	}
	select {
	case stream.inbound <- append([]byte(nil), payload...):
	case <-stream.readDone:
	}
}

func (stream *muxStream) remoteClose() {
	stream.finish()
	stream.mux.removeStream(stream.id)
}

func (stream *muxStream) finish() {
	stream.close.Do(func() {
		stream.mu.Lock()
		stream.closed = true
		stream.mu.Unlock()
		close(stream.readDone)
	})
}

func (stream *muxStream) isClosed() bool {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	return stream.closed
}
