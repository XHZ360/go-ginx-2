package daemon

import (
	"context"
	"log"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/session"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

func runSessionExpiryLoop(ctx context.Context, sessions *session.Manager, db store.Store, timeout time.Duration) {
	if sessions == nil || timeout <= 0 {
		return
	}
	interval := max(timeout/2, time.Second)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			expireSessions(ctx, sessions, db, timeout)
		}
	}
}

func expireSessions(ctx context.Context, sessions *session.Manager, db store.Store, timeout time.Duration) {
	for _, expired := range sessions.MarkExpired(timeout) {
		log.Printf("control session expired: client_id=%s protocol=%s session_id=%s", expired.ClientID, expired.Protocol, expired.ID)
		if db == nil || db.Clients() == nil {
			continue
		}
		client, err := db.Clients().ByID(ctx, expired.ClientID)
		if err != nil {
			log.Printf("control client status lookup failed: client_id=%s status=%s error=%v", expired.ClientID, domain.ClientOffline, err)
			continue
		}
		if client.Status == domain.ClientDisabled {
			continue
		}
		if err := db.Clients().SetStatus(ctx, expired.ClientID, domain.ClientOffline); err != nil {
			log.Printf("control client status update failed: client_id=%s status=%s error=%v", expired.ClientID, domain.ClientOffline, err)
		}
	}
}
