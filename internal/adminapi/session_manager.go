package adminapi

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

const (
	defaultSessionIdleTimeout      = 15 * time.Minute
	defaultSessionAbsoluteLifetime = 8 * time.Hour
)

type administratorSession struct {
	ID         string
	Username   string
	CSRFToken  string
	CreatedAt  time.Time
	LastSeenAt time.Time
}

type sessionManager struct {
	guard            sync.Mutex
	sessions         map[string]administratorSession
	idleTimeout      time.Duration
	absoluteLifetime time.Duration
	now              func() time.Time
}

func newSessionManager(idleTimeout time.Duration, absoluteLifetime time.Duration, now func() time.Time) *sessionManager {
	if idleTimeout <= 0 {
		idleTimeout = defaultSessionIdleTimeout
	}
	if absoluteLifetime <= 0 {
		absoluteLifetime = defaultSessionAbsoluteLifetime
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &sessionManager{
		sessions:         make(map[string]administratorSession),
		idleTimeout:      idleTimeout,
		absoluteLifetime: absoluteLifetime,
		now:              now,
	}
}

func (manager *sessionManager) Create(username string) (administratorSession, error) {
	sessionID, err := randomToken(32)
	if err != nil {
		return administratorSession{}, err
	}
	csrfToken, err := randomToken(32)
	if err != nil {
		return administratorSession{}, err
	}
	now := manager.now()
	session := administratorSession{ID: sessionID, Username: username, CSRFToken: csrfToken, CreatedAt: now, LastSeenAt: now}
	manager.guard.Lock()
	defer manager.guard.Unlock()
	manager.sessions[session.ID] = session
	return session, nil
}

func (manager *sessionManager) Get(sessionID string) (administratorSession, bool) {
	manager.guard.Lock()
	defer manager.guard.Unlock()
	session, ok := manager.sessions[sessionID]
	if !ok {
		return administratorSession{}, false
	}
	now := manager.now()
	if now.Sub(session.LastSeenAt) > manager.idleTimeout || now.Sub(session.CreatedAt) > manager.absoluteLifetime {
		delete(manager.sessions, sessionID)
		return administratorSession{}, false
	}
	session.LastSeenAt = now
	manager.sessions[sessionID] = session
	return session, true
}

func (manager *sessionManager) Invalidate(sessionID string) {
	if sessionID == "" {
		return
	}
	manager.guard.Lock()
	defer manager.guard.Unlock()
	delete(manager.sessions, sessionID)
}

func randomToken(length int) (string, error) {
	buffer := make([]byte, length)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
