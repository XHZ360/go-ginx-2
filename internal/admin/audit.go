package admin

import (
	"context"
	"errors"
	"strings"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

// AuditRecorder is the narrow audit dependency shared by admin facades.
type AuditRecorder interface {
	Record(ctx context.Context, actorID, resourceType, resourceID, action string) error
}

type storeAuditRecorder struct {
	store store.Store
}

func (recorder storeAuditRecorder) Record(ctx context.Context, actorID, resourceType, resourceID, action string) error {
	if recorder.store == nil {
		return errors.New("store is required")
	}
	if strings.TrimSpace(actorID) == "" {
		actorID = "system"
	}
	event := domain.AuditEvent{
		ID:           newID("audit"),
		ActorUserID:  actorID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       action,
		Result:       "success",
	}
	return recorder.store.AuditEvents().Create(ctx, event)
}

var _ AuditRecorder = storeAuditRecorder{}
