package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
)

var ErrSessionNotFound = errors.New("session not found")

type Manager struct {
	mu             sync.RWMutex
	now            func() time.Time
	nextGeneration int64
	byID           map[string]Session
	latestByClient map[string]string
}

type Session struct {
	ID            string
	ClientID      string
	UserID        string
	ClientKind    domain.ClientKind
	Protocol      domain.Protocol
	Generation    int64
	ConfigVersion int64
	ConnectedAt   time.Time
	LastHeartbeat time.Time
	ReplacedAt    *time.Time
	ClosedAt      *time.Time
	Stats         HeartbeatStats
	StreamOpener  StreamOpener
}

type StreamOpener interface {
	OpenStream(ctx context.Context) (io.ReadWriteCloser, error)
}

// StreamAcceptor extends StreamOpener with the ability to accept inbound streams
// opened by the remote peer. Consumer SDK connections use this to receive streams
// that the consumer initiates on the control channel.
type StreamAcceptor interface {
	StreamOpener
	AcceptStream(ctx context.Context) (io.ReadWriteCloser, error)
}

type HeartbeatStats struct {
	ActiveProxies int
	ActiveStreams int
	UploadBytes   int64
	DownloadBytes int64
	ErrorSummary  string
}

type RegisterInput struct {
	SessionID     string
	ClientID      string
	UserID        string
	ClientKind    domain.ClientKind
	Protocol      domain.Protocol
	ConfigVersion int64
	StreamOpener  StreamOpener
}

type HeartbeatInput struct {
	SessionID     string
	ConfigVersion int64
	Stats         HeartbeatStats
}

func NewManager() *Manager {
	return &Manager{
		now:            func() time.Time { return time.Now().UTC() },
		byID:           make(map[string]Session),
		latestByClient: make(map[string]string),
	}
}

func (manager *Manager) Register(input RegisterInput) (Session, *Session, error) {
	if input.SessionID == "" {
		return Session{}, nil, errors.New("session id is required")
	}
	if input.ClientID == "" {
		return Session{}, nil, errors.New("client id is required")
	}
	if input.UserID == "" {
		return Session{}, nil, errors.New("user id is required")
	}
	if !input.Protocol.Valid() {
		return Session{}, nil, errors.New("protocol is invalid")
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()

	now := manager.now()
	manager.nextGeneration++
	session := Session{
		ID:            input.SessionID,
		ClientID:      input.ClientID,
		UserID:        input.UserID,
		ClientKind:    domain.NormalizeClientKind(input.ClientKind),
		Protocol:      input.Protocol,
		Generation:    manager.nextGeneration,
		ConfigVersion: input.ConfigVersion,
		ConnectedAt:   now,
		LastHeartbeat: now,
		StreamOpener:  input.StreamOpener,
	}

	var replaced *Session
	if previousID, ok := manager.latestByClient[input.ClientID]; ok {
		previous := manager.byID[previousID]
		previous.ReplacedAt = &now
		manager.byID[previousID] = previous
		previousCopy := previous
		replaced = &previousCopy
	}

	manager.byID[session.ID] = session
	manager.latestByClient[session.ClientID] = session.ID
	return session, replaced, nil
}

func (manager *Manager) Heartbeat(input HeartbeatInput) (Session, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session, ok := manager.byID[input.SessionID]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	if session.ReplacedAt != nil || session.ClosedAt != nil {
		return Session{}, fmt.Errorf("%w: session is not active", ErrSessionNotFound)
	}
	session.LastHeartbeat = manager.now()
	session.ConfigVersion = input.ConfigVersion
	session.Stats = input.Stats
	manager.byID[input.SessionID] = session
	return session, nil
}

func (manager *Manager) Close(sessionID string) (Session, bool, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session, ok := manager.byID[sessionID]
	if !ok {
		return Session{}, false, ErrSessionNotFound
	}
	if session.ReplacedAt != nil {
		return session, false, nil
	}
	if session.ClosedAt == nil {
		now := manager.now()
		session.ClosedAt = &now
		manager.byID[sessionID] = session
	}
	isLatest := manager.latestByClient[session.ClientID] == sessionID
	if isLatest {
		delete(manager.latestByClient, session.ClientID)
	}
	return session, isLatest, nil
}

func (manager *Manager) Latest(clientID string) (Session, bool) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	sessionID, ok := manager.latestByClient[clientID]
	if !ok {
		return Session{}, false
	}
	session := manager.byID[sessionID]
	return session, true
}

func (manager *Manager) SnapshotLatest() []Session {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	sessions := make([]Session, 0, len(manager.latestByClient))
	for _, sessionID := range manager.latestByClient {
		session, ok := manager.byID[sessionID]
		if !ok || session.ReplacedAt != nil || session.ClosedAt != nil {
			continue
		}
		sessions = append(sessions, session)
	}
	return sessions
}

func (manager *Manager) MarkExpired(timeout time.Duration) []Session {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	now := manager.now()
	expired := make([]Session, 0)
	for id, current := range manager.byID {
		if current.ReplacedAt != nil || current.ClosedAt != nil {
			continue
		}
		if now.Sub(current.LastHeartbeat) <= timeout {
			continue
		}
		current.ClosedAt = &now
		manager.byID[id] = current
		if manager.latestByClient[current.ClientID] == id {
			delete(manager.latestByClient, current.ClientID)
		}
		expired = append(expired, current)
	}
	return expired
}
