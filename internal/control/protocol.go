package control

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
)

const maxFrameSize = 1 << 20

type MessageType string

const (
	MessageAuthRequest   MessageType = "auth_request"
	MessageAuthResponse  MessageType = "auth_response"
	MessageHeartbeat     MessageType = "heartbeat"
	MessageOpenStream    MessageType = "open_stream"
	MessageProxySnapshot MessageType = "proxy_snapshot"
)

type Envelope struct {
	Type    MessageType     `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type AuthRequest struct {
	ClientID   string            `json:"client_id"`
	Credential string            `json:"credential"`
	Nonce      string            `json:"nonce"`
	Timestamp  time.Time         `json:"timestamp"`
	Version    string            `json:"version"`
	Protocols  []domain.Protocol `json:"protocols"`
}

type AuthResponse struct {
	Accepted          bool            `json:"accepted"`
	SessionID         string          `json:"session_id,omitempty"`
	SelectedProtocol  domain.Protocol `json:"selected_protocol,omitempty"`
	HeartbeatInterval time.Duration   `json:"heartbeat_interval,omitempty"`
	ConfigVersion     int64           `json:"config_version,omitempty"`
	Reason            string          `json:"reason,omitempty"`
}

type Heartbeat struct {
	SessionID     string    `json:"session_id"`
	ClientID      string    `json:"client_id"`
	ObservedAt    time.Time `json:"observed_at"`
	ConfigVersion int64     `json:"config_version"`
	ActiveProxies int       `json:"active_proxies"`
	ActiveStreams int       `json:"active_streams"`
	UploadBytes   int64     `json:"upload_bytes"`
	DownloadBytes int64     `json:"download_bytes"`
	ErrorSummary  string    `json:"error_summary,omitempty"`
}

type ProxySnapshot struct {
	Version int64          `json:"version"`
	Proxies []domain.Proxy `json:"proxies"`
}

type OpenStream struct {
	Kind         string `json:"kind"`
	ProxyID      string `json:"proxy_id"`
	ConnectionID string `json:"connection_id"`
	TargetHost   string `json:"target_host"`
	TargetPort   int    `json:"target_port"`
}

func WriteMessage(w io.Writer, messageType MessageType, payload any) error {
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	envelope := Envelope{Type: messageType, Payload: encodedPayload}
	encodedEnvelope, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	if len(encodedEnvelope) > maxFrameSize {
		return fmt.Errorf("message frame too large: %d", len(encodedEnvelope))
	}
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(encodedEnvelope)))
	if _, err := w.Write(length[:]); err != nil {
		return err
	}
	_, err = w.Write(encodedEnvelope)
	return err
}

func ReadMessage(r io.Reader) (Envelope, error) {
	var length [4]byte
	if _, err := io.ReadFull(r, length[:]); err != nil {
		return Envelope{}, err
	}
	frameSize := binary.BigEndian.Uint32(length[:])
	if frameSize == 0 {
		return Envelope{}, errors.New("empty message frame")
	}
	if frameSize > maxFrameSize {
		return Envelope{}, fmt.Errorf("message frame too large: %d", frameSize)
	}
	frame := make([]byte, frameSize)
	if _, err := io.ReadFull(r, frame); err != nil {
		return Envelope{}, err
	}
	var envelope Envelope
	if err := json.Unmarshal(frame, &envelope); err != nil {
		return Envelope{}, err
	}
	if envelope.Type == "" {
		return Envelope{}, errors.New("message type is required")
	}
	return envelope, nil
}

func DecodePayload[T any](envelope Envelope) (T, error) {
	var payload T
	if len(envelope.Payload) == 0 {
		return payload, errors.New("message payload is required")
	}
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return payload, err
	}
	return payload, nil
}
