package control

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/control/controlpb"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"google.golang.org/protobuf/proto"
)

const maxFrameSize = 1 << 20

const maxMuxPayloadSize = 32 * 1024

const MaxDatagramFrameSize = maxFrameSize

type MessageType string

const (
	MessageAuthRequest       MessageType = "auth_request"
	MessageAuthResponse      MessageType = "auth_response"
	MessageHeartbeat         MessageType = "heartbeat"
	MessageOpenStream        MessageType = "open_stream"
	MessageProxySnapshot     MessageType = "proxy_snapshot"
	MessageProxyListRequest  MessageType = "proxy_list_request"
	MessageProxyListResponse MessageType = "proxy_list_response"
)

type Envelope struct {
	Type    MessageType
	Payload []byte
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

type ProxyListRequest struct {
	ConfigVersion int64 `json:"config_version"`
}

type ProxyListResponse struct {
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

type MuxFrameType string

const (
	MuxFrameOpen  MuxFrameType = "open"
	MuxFrameData  MuxFrameType = "data"
	MuxFrameClose MuxFrameType = "close"
	MuxFrameReset MuxFrameType = "reset"
)

type MuxFrame struct {
	StreamID uint64
	Type     MuxFrameType
	Payload  []byte
	Reason   string
}

func WriteMessage(w io.Writer, messageType MessageType, payload any) error {
	protoPayload, err := marshalPayload(messageType, payload)
	if err != nil {
		return err
	}
	encodedEnvelope, err := proto.Marshal(&controlpb.Envelope{Type: toProtoMessageType(messageType), Payload: protoPayload})
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
	var envelope controlpb.Envelope
	if err := proto.Unmarshal(frame, &envelope); err != nil {
		return Envelope{}, err
	}
	messageType := fromProtoMessageType(envelope.GetType())
	if messageType == "" {
		return Envelope{}, errors.New("message type is required")
	}
	return Envelope{Type: messageType, Payload: envelope.GetPayload()}, nil
}

func WriteMuxFrame(w io.Writer, frame MuxFrame) error {
	if frame.StreamID == 0 && frame.Type == MuxFrameOpen {
		return errors.New("control mux stream cannot be opened")
	}
	if len(frame.Payload) > maxMuxPayloadSize {
		return fmt.Errorf("mux payload too large: %d", len(frame.Payload))
	}
	encodedFrame, err := proto.Marshal(muxFrameToProto(frame))
	if err != nil {
		return err
	}
	if len(encodedFrame) > maxFrameSize {
		return fmt.Errorf("mux frame too large: %d", len(encodedFrame))
	}
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(encodedFrame)))
	if _, err := w.Write(length[:]); err != nil {
		return err
	}
	_, err = w.Write(encodedFrame)
	return err
}

func ReadMuxFrame(r io.Reader) (MuxFrame, error) {
	var length [4]byte
	if _, err := io.ReadFull(r, length[:]); err != nil {
		return MuxFrame{}, err
	}
	frameSize := binary.BigEndian.Uint32(length[:])
	if frameSize == 0 {
		return MuxFrame{}, errors.New("empty mux frame")
	}
	if frameSize > maxFrameSize {
		return MuxFrame{}, fmt.Errorf("mux frame too large: %d", frameSize)
	}
	encodedFrame := make([]byte, frameSize)
	if _, err := io.ReadFull(r, encodedFrame); err != nil {
		return MuxFrame{}, err
	}
	var frame controlpb.MuxFrame
	if err := proto.Unmarshal(encodedFrame, &frame); err != nil {
		return MuxFrame{}, err
	}
	decoded := muxFrameFromProto(&frame)
	if decoded.Type == "" {
		return MuxFrame{}, errors.New("mux frame type is required")
	}
	if len(decoded.Payload) > maxMuxPayloadSize {
		return MuxFrame{}, fmt.Errorf("mux payload too large: %d", len(decoded.Payload))
	}
	return decoded, nil
}

func WriteDatagramFrame(w io.Writer, payload []byte) error {
	if len(payload) == 0 {
		return errors.New("empty datagram frame")
	}
	if len(payload) > MaxDatagramFrameSize {
		return fmt.Errorf("datagram frame too large: %d", len(payload))
	}
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(payload)))
	if _, err := w.Write(length[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func ReadDatagramFrame(r io.Reader) ([]byte, error) {
	var length [4]byte
	if _, err := io.ReadFull(r, length[:]); err != nil {
		return nil, err
	}
	frameSize := binary.BigEndian.Uint32(length[:])
	if frameSize == 0 {
		return nil, errors.New("empty datagram frame")
	}
	if frameSize > MaxDatagramFrameSize {
		return nil, fmt.Errorf("datagram frame too large: %d", frameSize)
	}
	payload := make([]byte, frameSize)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func DecodePayload[T any](envelope Envelope) (T, error) {
	var payload T
	switch target := any(&payload).(type) {
	case *AuthRequest:
		if len(envelope.Payload) == 0 {
			return payload, errors.New("message payload is required")
		}
		var message controlpb.AuthRequest
		if err := proto.Unmarshal(envelope.Payload, &message); err != nil {
			return payload, err
		}
		*target = authRequestFromProto(&message)
	case *AuthResponse:
		if len(envelope.Payload) == 0 {
			return payload, errors.New("message payload is required")
		}
		var message controlpb.AuthResponse
		if err := proto.Unmarshal(envelope.Payload, &message); err != nil {
			return payload, err
		}
		*target = authResponseFromProto(&message)
	case *Heartbeat:
		if len(envelope.Payload) == 0 {
			return payload, errors.New("message payload is required")
		}
		var message controlpb.Heartbeat
		if err := proto.Unmarshal(envelope.Payload, &message); err != nil {
			return payload, err
		}
		*target = heartbeatFromProto(&message)
	case *OpenStream:
		if len(envelope.Payload) == 0 {
			return payload, errors.New("message payload is required")
		}
		var message controlpb.OpenStream
		if err := proto.Unmarshal(envelope.Payload, &message); err != nil {
			return payload, err
		}
		*target = openStreamFromProto(&message)
	case *ProxySnapshot:
		var message controlpb.ProxySnapshot
		if err := proto.Unmarshal(envelope.Payload, &message); err != nil {
			return payload, err
		}
		*target = proxySnapshotFromProto(&message)
	case *ProxyListRequest:
		if len(envelope.Payload) == 0 {
			return payload, errors.New("message payload is required")
		}
		var message controlpb.ProxyListRequest
		if err := proto.Unmarshal(envelope.Payload, &message); err != nil {
			return payload, err
		}
		*target = proxyListRequestFromProto(&message)
	case *ProxyListResponse:
		var message controlpb.ProxyListResponse
		if err := proto.Unmarshal(envelope.Payload, &message); err != nil {
			return payload, err
		}
		*target = proxyListResponseFromProto(&message)
	default:
		return payload, fmt.Errorf("unsupported payload type %T", payload)
	}
	return payload, nil
}

func marshalPayload(messageType MessageType, payload any) ([]byte, error) {
	switch messageType {
	case MessageAuthRequest:
		value, ok := payload.(AuthRequest)
		if !ok {
			return nil, fmt.Errorf("%s payload must be AuthRequest", messageType)
		}
		return proto.Marshal(authRequestToProto(value))
	case MessageAuthResponse:
		value, ok := payload.(AuthResponse)
		if !ok {
			return nil, fmt.Errorf("%s payload must be AuthResponse", messageType)
		}
		return proto.Marshal(authResponseToProto(value))
	case MessageHeartbeat:
		value, ok := payload.(Heartbeat)
		if !ok {
			return nil, fmt.Errorf("%s payload must be Heartbeat", messageType)
		}
		return proto.Marshal(heartbeatToProto(value))
	case MessageOpenStream:
		value, ok := payload.(OpenStream)
		if !ok {
			return nil, fmt.Errorf("%s payload must be OpenStream", messageType)
		}
		return proto.Marshal(openStreamToProto(value))
	case MessageProxySnapshot:
		value, ok := payload.(ProxySnapshot)
		if !ok {
			return nil, fmt.Errorf("%s payload must be ProxySnapshot", messageType)
		}
		return proto.Marshal(proxySnapshotToProto(value))
	case MessageProxyListRequest:
		value, ok := payload.(ProxyListRequest)
		if !ok {
			return nil, fmt.Errorf("%s payload must be ProxyListRequest", messageType)
		}
		return proto.Marshal(proxyListRequestToProto(value))
	case MessageProxyListResponse:
		value, ok := payload.(ProxyListResponse)
		if !ok {
			return nil, fmt.Errorf("%s payload must be ProxyListResponse", messageType)
		}
		return proto.Marshal(proxyListResponseToProto(value))
	default:
		return nil, fmt.Errorf("unsupported message type %s", messageType)
	}
}

func toProtoMessageType(messageType MessageType) controlpb.MessageType {
	switch messageType {
	case MessageAuthRequest:
		return controlpb.MessageType_MESSAGE_TYPE_AUTH_REQUEST
	case MessageAuthResponse:
		return controlpb.MessageType_MESSAGE_TYPE_AUTH_RESPONSE
	case MessageHeartbeat:
		return controlpb.MessageType_MESSAGE_TYPE_HEARTBEAT
	case MessageOpenStream:
		return controlpb.MessageType_MESSAGE_TYPE_OPEN_STREAM
	case MessageProxySnapshot:
		return controlpb.MessageType_MESSAGE_TYPE_PROXY_SNAPSHOT
	case MessageProxyListRequest:
		return controlpb.MessageType_MESSAGE_TYPE_PROXY_LIST_REQUEST
	case MessageProxyListResponse:
		return controlpb.MessageType_MESSAGE_TYPE_PROXY_LIST_RESPONSE
	default:
		return controlpb.MessageType_MESSAGE_TYPE_UNSPECIFIED
	}
}

func fromProtoMessageType(messageType controlpb.MessageType) MessageType {
	switch messageType {
	case controlpb.MessageType_MESSAGE_TYPE_AUTH_REQUEST:
		return MessageAuthRequest
	case controlpb.MessageType_MESSAGE_TYPE_AUTH_RESPONSE:
		return MessageAuthResponse
	case controlpb.MessageType_MESSAGE_TYPE_HEARTBEAT:
		return MessageHeartbeat
	case controlpb.MessageType_MESSAGE_TYPE_OPEN_STREAM:
		return MessageOpenStream
	case controlpb.MessageType_MESSAGE_TYPE_PROXY_SNAPSHOT:
		return MessageProxySnapshot
	case controlpb.MessageType_MESSAGE_TYPE_PROXY_LIST_REQUEST:
		return MessageProxyListRequest
	case controlpb.MessageType_MESSAGE_TYPE_PROXY_LIST_RESPONSE:
		return MessageProxyListResponse
	default:
		return ""
	}
}

func authRequestToProto(request AuthRequest) *controlpb.AuthRequest {
	protocols := make([]string, 0, len(request.Protocols))
	for _, protocol := range request.Protocols {
		protocols = append(protocols, string(protocol))
	}
	return &controlpb.AuthRequest{ClientId: request.ClientID, Credential: request.Credential, Nonce: request.Nonce, TimestampUnixNano: request.Timestamp.UnixNano(), Version: request.Version, Protocols: protocols}
}

func authRequestFromProto(request *controlpb.AuthRequest) AuthRequest {
	protocols := make([]domain.Protocol, 0, len(request.GetProtocols()))
	for _, protocol := range request.GetProtocols() {
		protocols = append(protocols, domain.Protocol(protocol))
	}
	return AuthRequest{ClientID: request.GetClientId(), Credential: request.GetCredential(), Nonce: request.GetNonce(), Timestamp: unixNanoTime(request.GetTimestampUnixNano()), Version: request.GetVersion(), Protocols: protocols}
}

func authResponseToProto(response AuthResponse) *controlpb.AuthResponse {
	return &controlpb.AuthResponse{Accepted: response.Accepted, SessionId: response.SessionID, SelectedProtocol: string(response.SelectedProtocol), HeartbeatIntervalNanos: int64(response.HeartbeatInterval), ConfigVersion: response.ConfigVersion, Reason: response.Reason}
}

func authResponseFromProto(response *controlpb.AuthResponse) AuthResponse {
	return AuthResponse{Accepted: response.GetAccepted(), SessionID: response.GetSessionId(), SelectedProtocol: domain.Protocol(response.GetSelectedProtocol()), HeartbeatInterval: time.Duration(response.GetHeartbeatIntervalNanos()), ConfigVersion: response.GetConfigVersion(), Reason: response.GetReason()}
}

func heartbeatToProto(heartbeat Heartbeat) *controlpb.Heartbeat {
	return &controlpb.Heartbeat{SessionId: heartbeat.SessionID, ClientId: heartbeat.ClientID, ObservedAtUnixNano: heartbeat.ObservedAt.UnixNano(), ConfigVersion: heartbeat.ConfigVersion, ActiveProxies: int32(heartbeat.ActiveProxies), ActiveStreams: int32(heartbeat.ActiveStreams), UploadBytes: heartbeat.UploadBytes, DownloadBytes: heartbeat.DownloadBytes, ErrorSummary: heartbeat.ErrorSummary}
}

func heartbeatFromProto(heartbeat *controlpb.Heartbeat) Heartbeat {
	return Heartbeat{SessionID: heartbeat.GetSessionId(), ClientID: heartbeat.GetClientId(), ObservedAt: unixNanoTime(heartbeat.GetObservedAtUnixNano()), ConfigVersion: heartbeat.GetConfigVersion(), ActiveProxies: int(heartbeat.GetActiveProxies()), ActiveStreams: int(heartbeat.GetActiveStreams()), UploadBytes: heartbeat.GetUploadBytes(), DownloadBytes: heartbeat.GetDownloadBytes(), ErrorSummary: heartbeat.GetErrorSummary()}
}

func openStreamToProto(stream OpenStream) *controlpb.OpenStream {
	return &controlpb.OpenStream{Kind: stream.Kind, ProxyId: stream.ProxyID, ConnectionId: stream.ConnectionID, TargetHost: stream.TargetHost, TargetPort: int32(stream.TargetPort)}
}

func openStreamFromProto(stream *controlpb.OpenStream) OpenStream {
	return OpenStream{Kind: stream.GetKind(), ProxyID: stream.GetProxyId(), ConnectionID: stream.GetConnectionId(), TargetHost: stream.GetTargetHost(), TargetPort: int(stream.GetTargetPort())}
}

func proxyListRequestToProto(request ProxyListRequest) *controlpb.ProxyListRequest {
	return &controlpb.ProxyListRequest{ConfigVersion: request.ConfigVersion}
}

func proxyListRequestFromProto(request *controlpb.ProxyListRequest) ProxyListRequest {
	return ProxyListRequest{ConfigVersion: request.GetConfigVersion()}
}

func proxyListResponseToProto(response ProxyListResponse) *controlpb.ProxyListResponse {
	proxies := make([]*controlpb.Proxy, 0, len(response.Proxies))
	for _, proxy := range response.Proxies {
		proxies = append(proxies, proxyToProto(proxy))
	}
	return &controlpb.ProxyListResponse{Version: response.Version, Proxies: proxies}
}

func proxyListResponseFromProto(response *controlpb.ProxyListResponse) ProxyListResponse {
	proxies := make([]domain.Proxy, 0, len(response.GetProxies()))
	for _, proxy := range response.GetProxies() {
		proxies = append(proxies, proxyFromProto(proxy))
	}
	return ProxyListResponse{Version: response.GetVersion(), Proxies: proxies}
}

func proxySnapshotToProto(snapshot ProxySnapshot) *controlpb.ProxySnapshot {
	proxies := make([]*controlpb.Proxy, 0, len(snapshot.Proxies))
	for _, proxy := range snapshot.Proxies {
		proxies = append(proxies, proxyToProto(proxy))
	}
	return &controlpb.ProxySnapshot{Version: snapshot.Version, Proxies: proxies}
}

func proxySnapshotFromProto(snapshot *controlpb.ProxySnapshot) ProxySnapshot {
	proxies := make([]domain.Proxy, 0, len(snapshot.GetProxies()))
	for _, proxy := range snapshot.GetProxies() {
		proxies = append(proxies, proxyFromProto(proxy))
	}
	return ProxySnapshot{Version: snapshot.GetVersion(), Proxies: proxies}
}

func proxyToProto(proxy domain.Proxy) *controlpb.Proxy {
	return &controlpb.Proxy{Id: proxy.ID, UserId: proxy.UserID, ClientId: proxy.ClientID, Name: proxy.Name, Type: string(proxy.Type), Status: string(proxy.Status), EntryHost: proxy.EntryHost, EntryPort: int32(proxy.EntryPort), TargetHost: proxy.TargetHost, TargetPort: int32(proxy.TargetPort), Description: proxy.Description, CreatedAtUnixNano: proxy.CreatedAt.UnixNano(), UpdatedAtUnixNano: proxy.UpdatedAt.UnixNano()}
}

func proxyFromProto(proxy *controlpb.Proxy) domain.Proxy {
	return domain.Proxy{ID: proxy.GetId(), UserID: proxy.GetUserId(), ClientID: proxy.GetClientId(), Name: proxy.GetName(), Type: domain.ProxyType(proxy.GetType()), Status: domain.ProxyStatus(proxy.GetStatus()), EntryHost: proxy.GetEntryHost(), EntryPort: int(proxy.GetEntryPort()), TargetHost: proxy.GetTargetHost(), TargetPort: int(proxy.GetTargetPort()), Description: proxy.GetDescription(), CreatedAt: unixNanoTime(proxy.GetCreatedAtUnixNano()), UpdatedAt: unixNanoTime(proxy.GetUpdatedAtUnixNano())}
}

func muxFrameToProto(frame MuxFrame) *controlpb.MuxFrame {
	return &controlpb.MuxFrame{StreamId: frame.StreamID, Type: toProtoMuxFrameType(frame.Type), Payload: frame.Payload, Reason: frame.Reason}
}

func muxFrameFromProto(frame *controlpb.MuxFrame) MuxFrame {
	return MuxFrame{StreamID: frame.GetStreamId(), Type: fromProtoMuxFrameType(frame.GetType()), Payload: frame.GetPayload(), Reason: frame.GetReason()}
}

func toProtoMuxFrameType(frameType MuxFrameType) controlpb.MuxFrameType {
	switch frameType {
	case MuxFrameOpen:
		return controlpb.MuxFrameType_MUX_FRAME_TYPE_OPEN
	case MuxFrameData:
		return controlpb.MuxFrameType_MUX_FRAME_TYPE_DATA
	case MuxFrameClose:
		return controlpb.MuxFrameType_MUX_FRAME_TYPE_CLOSE
	case MuxFrameReset:
		return controlpb.MuxFrameType_MUX_FRAME_TYPE_RESET
	default:
		return controlpb.MuxFrameType_MUX_FRAME_TYPE_UNSPECIFIED
	}
}

func fromProtoMuxFrameType(frameType controlpb.MuxFrameType) MuxFrameType {
	switch frameType {
	case controlpb.MuxFrameType_MUX_FRAME_TYPE_OPEN:
		return MuxFrameOpen
	case controlpb.MuxFrameType_MUX_FRAME_TYPE_DATA:
		return MuxFrameData
	case controlpb.MuxFrameType_MUX_FRAME_TYPE_CLOSE:
		return MuxFrameClose
	case controlpb.MuxFrameType_MUX_FRAME_TYPE_RESET:
		return MuxFrameReset
	default:
		return ""
	}
}

func unixNanoTime(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(0, value).UTC()
}
