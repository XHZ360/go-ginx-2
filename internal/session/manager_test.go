package session

import (
	"errors"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
)

func TestRegisterReplacesPreviousClientSession(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	manager := NewManager()
	manager.now = func() time.Time { return now }

	first, replaced, err := manager.Register(RegisterInput{SessionID: "s1", ClientID: "c1", UserID: "u1", Protocol: domain.ProtocolQUIC, ConfigVersion: 1})
	if err != nil {
		t.Fatalf("register first: %v", err)
	}
	if replaced != nil {
		t.Fatal("first session should not replace anything")
	}

	second, replaced, err := manager.Register(RegisterInput{SessionID: "s2", ClientID: "c1", UserID: "u1", Protocol: domain.ProtocolQUIC, ConfigVersion: 2})
	if err != nil {
		t.Fatalf("register second: %v", err)
	}
	if replaced == nil || replaced.ID != first.ID || replaced.ReplacedAt == nil {
		t.Fatalf("expected first session replacement, got %+v", replaced)
	}
	latest, ok := manager.Latest("c1")
	if !ok || latest.ID != second.ID {
		t.Fatalf("expected latest session %s, got %+v", second.ID, latest)
	}
}

func TestHeartbeatUpdatesSessionStats(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	manager := NewManager()
	manager.now = func() time.Time { return now }
	registered, _, err := manager.Register(RegisterInput{SessionID: "s1", ClientID: "c1", UserID: "u1", Protocol: domain.ProtocolQUIC, ConfigVersion: 1})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	now = now.Add(10 * time.Second)
	updated, err := manager.Heartbeat(HeartbeatInput{SessionID: registered.ID, ConfigVersion: 3, Stats: HeartbeatStats{ActiveProxies: 2, ActiveStreams: 4, UploadBytes: 128, DownloadBytes: 256}})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if updated.ConfigVersion != 3 || updated.Stats.ActiveStreams != 4 || !updated.LastHeartbeat.Equal(now) {
		t.Fatalf("unexpected heartbeat update: %+v", updated)
	}
}

func TestCloseRemovesOnlyLatestSession(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	manager := NewManager()
	manager.now = func() time.Time { return now }
	first, _, err := manager.Register(RegisterInput{SessionID: "s1", ClientID: "c1", UserID: "u1", Protocol: domain.ProtocolQUIC})
	if err != nil {
		t.Fatalf("register first: %v", err)
	}
	second, _, err := manager.Register(RegisterInput{SessionID: "s2", ClientID: "c1", UserID: "u1", Protocol: domain.ProtocolQUIC})
	if err != nil {
		t.Fatalf("register second: %v", err)
	}

	closed, latest, err := manager.Close(first.ID)
	if err != nil {
		t.Fatalf("close first: %v", err)
	}
	if latest || closed.ClosedAt != nil {
		t.Fatalf("replaced session should not close latest state: latest=%v closed=%+v", latest, closed)
	}
	if found, ok := manager.Latest("c1"); !ok || found.ID != second.ID {
		t.Fatalf("expected second session to remain latest, got %+v", found)
	}

	now = now.Add(time.Second)
	closed, latest, err = manager.Close(second.ID)
	if err != nil {
		t.Fatalf("close second: %v", err)
	}
	if !latest || closed.ClosedAt == nil {
		t.Fatalf("expected latest session to close, latest=%v closed=%+v", latest, closed)
	}
	if _, ok := manager.Latest("c1"); ok {
		t.Fatal("expected latest session to be removed")
	}
}

func TestMarkExpiredClosesStaleSessions(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	manager := NewManager()
	manager.now = func() time.Time { return now }
	registered, _, err := manager.Register(RegisterInput{SessionID: "s1", ClientID: "c1", UserID: "u1", Protocol: domain.ProtocolQUIC})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	now = now.Add(46 * time.Second)
	expired := manager.MarkExpired(45 * time.Second)
	if len(expired) != 1 || expired[0].ID != registered.ID || expired[0].ClosedAt == nil {
		t.Fatalf("expected expired session, got %+v", expired)
	}
	if _, ok := manager.Latest("c1"); ok {
		t.Fatal("expected latest session to be removed")
	}
	if _, err := manager.Heartbeat(HeartbeatInput{SessionID: registered.ID}); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected session not found, got %v", err)
	}
}

func TestMarkExpiredKeepsPersistentVirtualSession(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	manager := NewManager()
	manager.now = func() time.Time { return now }
	registered, _, err := manager.Register(RegisterInput{SessionID: "server-local-virtual", ClientID: "server-local", UserID: "server-local-system", Protocol: domain.ProtocolTCPTLS, Persistent: true})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(24 * time.Hour)
	if expired := manager.MarkExpired(time.Second); len(expired) != 0 {
		t.Fatalf("persistent session must not expire: %+v", expired)
	}
	if latest, ok := manager.Latest(registered.ClientID); !ok || latest.ID != registered.ID {
		t.Fatalf("persistent session should remain latest: %+v", latest)
	}
}
