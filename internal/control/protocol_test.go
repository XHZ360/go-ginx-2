package control

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/control/controlpb"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"google.golang.org/protobuf/proto"
)

func TestWriteReadMessageRoundTrip(t *testing.T) {
	request := AuthRequest{
		ClientID:   "client-1",
		Credential: "secret",
		Nonce:      "nonce",
		Timestamp:  time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC),
		Version:    "test",
		Protocols:  []domain.Protocol{domain.ProtocolQUIC},
	}

	var buffer bytes.Buffer
	if err := WriteMessage(&buffer, MessageAuthRequest, request); err != nil {
		t.Fatalf("write message: %v", err)
	}

	envelope, err := ReadMessage(&buffer)
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	if envelope.Type != MessageAuthRequest {
		t.Fatalf("expected %s, got %s", MessageAuthRequest, envelope.Type)
	}

	decoded, err := DecodePayload[AuthRequest](envelope)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if decoded.ClientID != request.ClientID || decoded.Protocols[0] != domain.ProtocolQUIC {
		t.Fatalf("unexpected decoded request: %+v", decoded)
	}
}

func TestReadMessageRejectsEmptyFrame(t *testing.T) {
	buffer := bytes.NewBuffer([]byte{0, 0, 0, 0})

	if _, err := ReadMessage(buffer); err == nil {
		t.Fatal("expected empty frame error")
	}
}

func TestWriteMessageUsesProtobufEnvelope(t *testing.T) {
	var buffer bytes.Buffer
	if err := WriteMessage(&buffer, MessageHeartbeat, Heartbeat{SessionID: "session-1", ClientID: "client-1", ObservedAt: time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("write message: %v", err)
	}
	frame := buffer.Bytes()
	if len(frame) < 5 {
		t.Fatalf("expected length-prefixed frame, got %d bytes", len(frame))
	}
	frameSize := binary.BigEndian.Uint32(frame[:4])
	if int(frameSize) != len(frame)-4 {
		t.Fatalf("unexpected frame size %d for %d bytes", frameSize, len(frame))
	}
	var envelope controlpb.Envelope
	if err := proto.Unmarshal(frame[4:], &envelope); err != nil {
		t.Fatalf("unmarshal protobuf envelope: %v", err)
	}
	if envelope.GetType() != controlpb.MessageType_MESSAGE_TYPE_HEARTBEAT {
		t.Fatalf("unexpected envelope type: %s", envelope.GetType())
	}
	var heartbeat controlpb.Heartbeat
	if err := proto.Unmarshal(envelope.GetPayload(), &heartbeat); err != nil {
		t.Fatalf("unmarshal protobuf heartbeat: %v", err)
	}
	if heartbeat.GetSessionId() != "session-1" || heartbeat.GetClientId() != "client-1" {
		t.Fatalf("unexpected heartbeat: %+v", &heartbeat)
	}
}

func TestProxySnapshotRoundTrip(t *testing.T) {
	snapshot := ProxySnapshot{Version: 3, Proxies: []domain.Proxy{{ID: "p1", UserID: "u1", ClientID: "c1", Name: "web", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080}}}

	var buffer bytes.Buffer
	if err := WriteMessage(&buffer, MessageProxySnapshot, snapshot); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	envelope, err := ReadMessage(&buffer)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	decoded, err := DecodePayload[ProxySnapshot](envelope)
	if err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if decoded.Version != snapshot.Version || len(decoded.Proxies) != 1 || decoded.Proxies[0].ID != "p1" {
		t.Fatalf("unexpected snapshot: %+v", decoded)
	}
}
