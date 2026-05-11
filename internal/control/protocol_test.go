package control

import (
	"bytes"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
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
