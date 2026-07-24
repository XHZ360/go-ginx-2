package admin

import (
	"context"
	"errors"
	"strings"

	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

// AuditRecorder is the narrow audit dependency shared by admin facades.
type AuditRecorder interface {
	Record(ctx context.Context, actorID, resourceType, resourceID, action string) error
	RecordResult(ctx context.Context, actorID, resourceType, resourceID, action, result, errorSummary string) error
}

type storeAuditRecorder struct {
	store store.Store
}

func (recorder storeAuditRecorder) Record(ctx context.Context, actorID, resourceType, resourceID, action string) error {
	return recorder.RecordResult(ctx, actorID, resourceType, resourceID, action, "success", "")
}

func (recorder storeAuditRecorder) RecordResult(ctx context.Context, actorID, resourceType, resourceID, action, result, errorSummary string) error {
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
		Result:       result,
		ErrorSummary: errorSummary,
	}
	return recorder.store.AuditEvents().Create(ctx, event)
}

func finishAudit(ctx context.Context, recorder AuditRecorder, actorID, resourceType, resourceID, action string, operationErr error) error {
	if recorder == nil {
		return operationErr
	}
	result := "success"
	summary := ""
	if operationErr != nil {
		result = "failure"
		summary = contracterr.CodeInternal
		var contractError *contracterr.Error
		if errors.As(operationErr, &contractError) && strings.TrimSpace(contractError.Code) != "" {
			summary = contractError.Code
			if contractError.Code == contracterr.CodeForbidden {
				result = "forbidden"
			}
		}
	}
	auditErr := recorder.RecordResult(ctx, actorID, resourceType, resourceID, action, result, summary)
	if operationErr != nil {
		return operationErr
	}
	return auditErr
}

func recordRejectedAudit(ctx context.Context, recorder AuditRecorder, actorID, resourceType, resourceID, action string, operationErr error) {
	_ = finishAudit(ctx, recorder, actorID, resourceType, resourceID, action, operationErr)
}

var _ AuditRecorder = storeAuditRecorder{}
