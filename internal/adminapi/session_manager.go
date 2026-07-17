package adminapi

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	defaultSessionAbsoluteLifetime = 7 * 24 * time.Hour
	adminJWTVersion                = 1
	adminJWTType                   = "admin"
	minAdminJWTSecretLength        = 32
	maxAdminJWTFutureIssuedSkew    = time.Minute
)

type administratorSession struct {
	ID        string
	Username  string
	CSRFToken string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type sessionManager struct {
	secret           []byte
	absoluteLifetime time.Duration
	now              func() time.Time
}

type adminJWTHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
}

type adminJWTClaims struct {
	Type      string `json:"typ"`
	Version   int    `json:"ver"`
	Subject   string `json:"sub"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	CSRFToken string `json:"csrf"`
}

func newSessionManager(secret []byte, absoluteLifetime time.Duration, now func() time.Time) (*sessionManager, error) {
	if len(secret) < minAdminJWTSecretLength {
		return nil, errors.New("admin jwt secret must be at least 32 bytes")
	}
	if absoluteLifetime <= 0 {
		absoluteLifetime = defaultSessionAbsoluteLifetime
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &sessionManager{secret: append([]byte(nil), secret...), absoluteLifetime: absoluteLifetime, now: now}, nil
}

func (manager *sessionManager) Create(username string) (administratorSession, error) {
	csrfToken, err := randomToken(32)
	if err != nil {
		return administratorSession{}, err
	}
	now := manager.now().UTC()
	session := administratorSession{Username: username, CSRFToken: csrfToken, CreatedAt: now, ExpiresAt: now.Add(manager.absoluteLifetime)}
	token, err := manager.sign(adminJWTClaims{Type: adminJWTType, Version: adminJWTVersion, Subject: username, IssuedAt: now.Unix(), ExpiresAt: session.ExpiresAt.Unix(), CSRFToken: csrfToken})
	if err != nil {
		return administratorSession{}, err
	}
	session.ID = token
	return session, nil
}

func (manager *sessionManager) Get(token string) (administratorSession, bool) {
	claims, ok := manager.verify(token)
	if !ok {
		return administratorSession{}, false
	}
	return administratorSession{
		ID:        token,
		Username:  claims.Subject,
		CSRFToken: claims.CSRFToken,
		CreatedAt: time.Unix(claims.IssuedAt, 0).UTC(),
		ExpiresAt: time.Unix(claims.ExpiresAt, 0).UTC(),
	}, true
}

func (manager *sessionManager) Invalidate(_ string) {}

func (manager *sessionManager) sign(claims adminJWTClaims) (string, error) {
	headerJSON, err := json.Marshal(adminJWTHeader{Algorithm: "HS256", Type: "JWT"})
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	return unsigned + "." + manager.signature(unsigned), nil
}

func (manager *sessionManager) verify(token string) (adminJWTClaims, bool) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return adminJWTClaims{}, false
	}
	unsigned := parts[0] + "." + parts[1]
	if !subtleConstantTimeEquals(parts[2], manager.signature(unsigned)) {
		return adminJWTClaims{}, false
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return adminJWTClaims{}, false
	}
	var header adminJWTHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return adminJWTClaims{}, false
	}
	if header.Algorithm != "HS256" || header.Type != "JWT" {
		return adminJWTClaims{}, false
	}
	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return adminJWTClaims{}, false
	}
	var claims adminJWTClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return adminJWTClaims{}, false
	}
	if !manager.validClaims(claims) {
		return adminJWTClaims{}, false
	}
	return claims, true
}

func (manager *sessionManager) validClaims(claims adminJWTClaims) bool {
	if claims.Type != adminJWTType || claims.Version != adminJWTVersion {
		return false
	}
	if strings.TrimSpace(claims.Subject) == "" || strings.TrimSpace(claims.CSRFToken) == "" {
		return false
	}
	if claims.IssuedAt <= 0 || claims.ExpiresAt <= 0 || claims.ExpiresAt <= claims.IssuedAt {
		return false
	}
	now := manager.now().UTC()
	issuedAt := time.Unix(claims.IssuedAt, 0).UTC()
	expiresAt := time.Unix(claims.ExpiresAt, 0).UTC()
	if issuedAt.After(now.Add(maxAdminJWTFutureIssuedSkew)) {
		return false
	}
	return expiresAt.After(now)
}

func (manager *sessionManager) signature(unsigned string) string {
	mac := hmac.New(sha256.New, manager.secret)
	_, _ = mac.Write([]byte(unsigned))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func randomToken(length int) (string, error) {
	buffer := make([]byte, length)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
