package admin

import (
	"context"
	"errors"
	"strings"

	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/systemclient"
)

func (service *UserService) CreateUser(ctx context.Context, input CreateUserInput) (domain.User, error) {
	if service.Store == nil {
		return domain.User{}, errors.New("store is required")
	}
	if err := validateCreateUserInput(input); err != nil {
		return domain.User{}, err
	}
	if strings.TrimSpace(input.ID) == "" {
		input.ID = newID("user")
	}
	if err := systemclient.ProtectUserMutation(input.ID); err != nil {
		recordRejectedAudit(ctx, service.Audit, input.ActorID, "user", input.ID, "create_user", err)
		return domain.User{}, err
	}
	if input.Username == systemclient.Username {
		return domain.User{}, &contracterr.Error{Code: contracterr.CodeForbidden, Message: "system username is reserved"}
	}
	if input.Role == "" {
		input.Role = domain.RoleUser
	}
	passwordHash := ""
	if strings.TrimSpace(input.Password) != "" {
		passwordHashValue, err := domain.HashPassword(input.Password)
		if err != nil {
			return domain.User{}, err
		}
		passwordHash = passwordHashValue
	}
	user := domain.User{ID: input.ID, Username: input.Username, PasswordHash: passwordHash, Role: input.Role, Status: domain.UserEnabled}
	if err := service.Store.Users().Create(ctx, user); err != nil {
		return domain.User{}, err
	}
	return user, service.Audit.Record(ctx, input.ActorID, "user", user.ID, "create_user")
}

func (service *UserService) DisableUser(ctx context.Context, userID string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	if err := systemclient.ProtectUserMutation(userID); err != nil {
		recordRejectedAudit(ctx, service.Audit, actorID, "user", userID, "disable_user", err)
		return err
	}
	if err := service.Store.Users().SetStatus(ctx, userID, domain.UserDisabled); err != nil {
		return err
	}
	return service.Audit.Record(ctx, actorID, "user", userID, "disable_user")
}

func (service *UserService) EnableUser(ctx context.Context, userID string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	if err := systemclient.ProtectUserMutation(userID); err != nil {
		recordRejectedAudit(ctx, service.Audit, actorID, "user", userID, "enable_user", err)
		return err
	}
	if err := service.Store.Users().SetStatus(ctx, userID, domain.UserEnabled); err != nil {
		return err
	}
	return service.Audit.Record(ctx, actorID, "user", userID, "enable_user")
}

func (service *UserService) SetUserPassword(ctx context.Context, userID string, password string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	if err := systemclient.ProtectUserMutation(userID); err != nil {
		recordRejectedAudit(ctx, service.Audit, actorID, "user", userID, "set_user_password", err)
		return err
	}
	if err := validateSetUserPassword(userID, password); err != nil {
		return err
	}
	passwordHash, err := domain.HashPassword(password)
	if err != nil {
		return contracterr.Validation("validation failed", map[string]string{"password": err.Error()})
	}
	if err := service.Store.Users().SetPassword(ctx, userID, passwordHash); err != nil {
		return err
	}
	return service.Audit.Record(ctx, actorID, "user", userID, "set_user_password")
}

func (service *UserService) DeleteUser(ctx context.Context, userID string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	if strings.TrimSpace(userID) == "" {
		return contracterr.Validation("validation failed", map[string]string{"id": "user id is required"})
	}
	if err := systemclient.ProtectUserMutation(userID); err != nil {
		recordRejectedAudit(ctx, service.Audit, actorID, "user", userID, "delete_user", err)
		return err
	}
	if _, err := service.Store.Users().ByID(ctx, userID); err != nil {
		return err
	}
	clients, err := service.Store.Clients().List(ctx)
	if err != nil {
		return err
	}
	for _, client := range clients {
		if client.UserID == userID {
			return contracterr.Conflict("user has clients; disable and delete client resources before deleting the user", nil)
		}
	}
	proxies, err := service.Store.Proxies().List(ctx)
	if err != nil {
		return err
	}
	for _, proxy := range proxies {
		if proxy.UserID == userID {
			return contracterr.Conflict("user has proxies; disable and delete proxy resources before deleting the user", nil)
		}
	}
	if err := service.Store.Users().Delete(ctx, userID); err != nil {
		return err
	}
	return service.Audit.Record(ctx, actorID, "user", userID, "delete_user")
}
