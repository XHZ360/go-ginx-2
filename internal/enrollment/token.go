package enrollment

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/jointoken"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

const TokenPrefix = "goginx_join_"

type TokenPayload struct {
	Version          int               `json:"version"`
	EnrollmentID     string            `json:"enrollment_id"`
	Secret           string            `json:"secret"`
	EnrollmentURL    string            `json:"enrollment_url"`
	ServerAddress    string            `json:"server_address"`
	ServerTLSAddress string            `json:"server_tls_address,omitempty"`
	ServerName       string            `json:"server_name"`
	CAPEM            string            `json:"ca_pem"`
	ClientID         string            `json:"client_id"`
	Credential       string            `json:"credential"`
	AllowedProtocols []domain.Protocol `json:"allowed_protocols"`
	Reconnect        config.Reconnect  `json:"reconnect"`
	ExpiresAt        time.Time         `json:"expires_at"`
}

type RedeemRequest struct {
	Token string `json:"token"`
}

type RedeemResponse struct {
	ServerAddress    string            `json:"server_address"`
	ServerTLSAddress string            `json:"server_tls_address,omitempty"`
	ServerName       string            `json:"server_name"`
	CAPEM            string            `json:"ca_pem"`
	ClientID         string            `json:"client_id"`
	Credential       string            `json:"credential"`
	AllowedProtocols []domain.Protocol `json:"allowed_protocols"`
	Reconnect        config.Reconnect  `json:"reconnect"`
}

type Service struct {
	Store store.Store
	Now   func() time.Time
}

func EncodeToken(payload TokenPayload) (string, error) {
	payload.Version = 1
	content, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return TokenPrefix + base64.RawURLEncoding.EncodeToString(content), nil
}

func DecodeToken(token string) (TokenPayload, error) {
	raw := jointoken.Normalize(token)
	if !strings.HasPrefix(raw, TokenPrefix) {
		return TokenPayload{}, errors.New("join token has invalid prefix")
	}
	content, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(raw, TokenPrefix))
	if err != nil {
		return TokenPayload{}, err
	}
	var payload TokenPayload
	if err := json.Unmarshal(content, &payload); err != nil {
		return TokenPayload{}, err
	}
	if payload.Version != 1 {
		return TokenPayload{}, errors.New("join token version is unsupported")
	}
	return payload, payload.Validate()
}

func (payload TokenPayload) Validate() error {
	if strings.TrimSpace(payload.EnrollmentID) == "" {
		return errors.New("enrollment id is required")
	}
	if strings.TrimSpace(payload.Secret) == "" {
		return errors.New("enrollment secret is required")
	}
	if strings.TrimSpace(payload.EnrollmentURL) == "" {
		return errors.New("enrollment url is required")
	}
	if strings.TrimSpace(payload.ServerAddress) == "" {
		return errors.New("server address is required")
	}
	if strings.TrimSpace(payload.ServerName) == "" {
		return errors.New("server name is required")
	}
	if strings.TrimSpace(payload.CAPEM) == "" {
		return errors.New("ca pem is required")
	}
	if strings.TrimSpace(payload.ClientID) == "" {
		return errors.New("client id is required")
	}
	if strings.TrimSpace(payload.Credential) == "" {
		return errors.New("client credential is required")
	}
	if len(payload.AllowedProtocols) == 0 {
		return errors.New("allowed protocols are required")
	}
	if payload.ExpiresAt.IsZero() {
		return errors.New("enrollment expiry is required")
	}
	return nil
}

func (service Service) Redeem(ctx context.Context, token string) (RedeemResponse, error) {
	if service.Store == nil {
		return RedeemResponse{}, errors.New("store is required")
	}
	payload, err := DecodeToken(token)
	if err != nil {
		return RedeemResponse{}, err
	}
	record, err := service.Store.ClientEnrollments().ByID(ctx, payload.EnrollmentID)
	if err != nil {
		return RedeemResponse{}, err
	}
	now := service.now()
	if record.UsedAt != nil {
		return RedeemResponse{}, fmt.Errorf("%w: enrollment token already used", store.ErrConflict)
	}
	if !record.ExpiresAt.After(now) || payload.ExpiresAt.Before(now) {
		return RedeemResponse{}, fmt.Errorf("%w: enrollment token expired", store.ErrNotFound)
	}
	if subtle.ConstantTimeCompare([]byte(record.SecretHash), []byte(HashSecret(payload.Secret))) != 1 {
		return RedeemResponse{}, fmt.Errorf("%w: enrollment secret mismatch", store.ErrNotFound)
	}
	if subtle.ConstantTimeCompare([]byte(record.TokenHash), []byte(HashToken(token))) != 1 {
		return RedeemResponse{}, fmt.Errorf("%w: enrollment token mismatch", store.ErrNotFound)
	}
	if err := service.Store.ClientEnrollments().MarkUsed(ctx, record.ID, now); err != nil {
		return RedeemResponse{}, err
	}
	return RedeemResponse{
		ServerAddress:    payload.ServerAddress,
		ServerTLSAddress: payload.ServerTLSAddress,
		ServerName:       payload.ServerName,
		CAPEM:            payload.CAPEM,
		ClientID:         payload.ClientID,
		Credential:       payload.Credential,
		AllowedProtocols: append([]domain.Protocol(nil), payload.AllowedProtocols...),
		Reconnect:        payload.Reconnect,
	}, nil
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func ConfigFromResponse(response RedeemResponse, caFile string) config.Client {
	cfg := config.DefaultClient()
	cfg.ServerAddress = response.ServerAddress
	cfg.ServerTLSAddress = response.ServerTLSAddress
	cfg.ServerName = response.ServerName
	cfg.ServerCAFile = caFile
	cfg.ClientID = response.ClientID
	cfg.Credential = response.Credential
	cfg.AllowedProtocols = append([]domain.Protocol(nil), response.AllowedProtocols...)
	cfg.Reconnect = response.Reconnect
	return cfg
}

func HashSecret(secret string) string {
	return hashString(secret)
}

func HashToken(token string) string {
	return hashString(jointoken.Normalize(token))
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func HTTPStatusForError(err error) int {
	if errors.Is(err, store.ErrConflict) {
		return http.StatusConflict
	}
	if errors.Is(err, store.ErrNotFound) {
		return http.StatusUnauthorized
	}
	return http.StatusBadRequest
}
