package certmanager

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	OriginCARequestTypeECC = "origin-ecc"
	OriginCARequestTypeRSA = "origin-rsa"
)

type OriginCAClient interface {
	Create(ctx context.Context, token string, request OriginCACreateRequest) (OriginCACertificate, error)
	Get(ctx context.Context, token string, certificateID string) (OriginCACertificate, error)
	List(ctx context.Context, token string) ([]OriginCACertificate, error)
	Revoke(ctx context.Context, token string, certificateID string) error
	VerifyToken(ctx context.Context, token string) error
}

type OriginCACreateRequest struct {
	CSR               string
	Hostnames         []string
	RequestType       string
	RequestedValidity int
}

type OriginCACertificate struct {
	ID                string
	CertificatePEM    []byte
	Hostnames         []string
	RequestType       string
	RequestedValidity int
	ExpiresOn         *time.Time
	RevokedAt         *time.Time
	Status            string
}

type CloudflareOriginCAClient struct {
	HTTPClient *http.Client
	BaseURL    string
}

func BuildOriginCACSR(hostnames []string, requestType string) (csrPEM []byte, keyPEM []byte, err error) {
	hostnames = normalizeOriginCAHostnames(hostnames)
	if len(hostnames) == 0 {
		return nil, nil, errors.New("origin ca hostnames are required")
	}
	if strings.TrimSpace(requestType) == "" {
		requestType = OriginCARequestTypeECC
	}
	template := &x509.CertificateRequest{Subject: pkix.Name{CommonName: hostnames[0]}, DNSNames: hostnames}
	switch requestType {
	case OriginCARequestTypeECC:
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, key)
		if err != nil {
			return nil, nil, err
		}
		keyDER, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return nil, nil, err
		}
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), nil
	case OriginCARequestTypeRSA:
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, nil, err
		}
		csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, key)
		if err != nil {
			return nil, nil, err
		}
		keyDER, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return nil, nil, err
		}
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), nil
	default:
		return nil, nil, fmt.Errorf("unsupported origin ca request type %q", requestType)
	}
}

func (client CloudflareOriginCAClient) Create(ctx context.Context, token string, request OriginCACreateRequest) (OriginCACertificate, error) {
	if strings.TrimSpace(request.CSR) == "" {
		return OriginCACertificate{}, errors.New("origin ca csr is required")
	}
	request.Hostnames = normalizeOriginCAHostnames(request.Hostnames)
	if len(request.Hostnames) == 0 {
		return OriginCACertificate{}, errors.New("origin ca hostnames are required")
	}
	if request.RequestType == "" {
		request.RequestType = OriginCARequestTypeECC
	}
	body := map[string]any{
		"csr":                request.CSR,
		"hostnames":          request.Hostnames,
		"request_type":       request.RequestType,
		"requested_validity": request.RequestedValidity,
	}
	data, err := client.request(ctx, token, http.MethodPost, "/certificates", nil, body)
	if err != nil {
		return OriginCACertificate{}, err
	}
	return parseOriginCACertificate(data)
}

func (client CloudflareOriginCAClient) Get(ctx context.Context, token string, certificateID string) (OriginCACertificate, error) {
	certificateID = strings.TrimSpace(certificateID)
	if certificateID == "" {
		return OriginCACertificate{}, errors.New("cloudflare certificate id is required")
	}
	data, err := client.request(ctx, token, http.MethodGet, "/certificates/"+url.PathEscape(certificateID), nil, nil)
	if err != nil {
		return OriginCACertificate{}, err
	}
	return parseOriginCACertificate(data)
}

func (client CloudflareOriginCAClient) List(ctx context.Context, token string) ([]OriginCACertificate, error) {
	data, err := client.request(ctx, token, http.MethodGet, "/certificates", nil, nil)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Result []originCACertificateJSON `json:"result"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	items := make([]OriginCACertificate, 0, len(envelope.Result))
	for _, item := range envelope.Result {
		items = append(items, item.toDomain())
	}
	return items, nil
}

func (client CloudflareOriginCAClient) Revoke(ctx context.Context, token string, certificateID string) error {
	certificateID = strings.TrimSpace(certificateID)
	if certificateID == "" {
		return errors.New("cloudflare certificate id is required")
	}
	_, err := client.request(ctx, token, http.MethodDelete, "/certificates/"+url.PathEscape(certificateID), nil, nil)
	return err
}

func (client CloudflareOriginCAClient) VerifyToken(ctx context.Context, token string) error {
	_, err := client.request(ctx, token, http.MethodGet, "/user/tokens/verify", nil, nil)
	return err
}

func (client CloudflareOriginCAClient) request(ctx context.Context, token string, method string, path string, query url.Values, body any) ([]byte, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("cloudflare api token is required")
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	baseURL := strings.TrimRight(client.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.cloudflare.com/client/v4"
	}
	requestURL := baseURL + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL, reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	httpClient := client.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode == http.StatusNotFound {
		return nil, ErrOriginCACertificateMissing
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("cloudflare origin ca request failed: status %d", response.StatusCode)
	}
	var envelope struct {
		Success bool            `json:"success"`
		Errors  json.RawMessage `json:"errors"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	if !envelope.Success {
		return nil, errors.New("cloudflare origin ca request failed")
	}
	return data, nil
}

var ErrOriginCACertificateMissing = errors.New("cloudflare origin ca certificate is missing")

type originCACertificateJSON struct {
	ID                string   `json:"id"`
	Certificate       string   `json:"certificate"`
	Hostnames         []string `json:"hostnames"`
	RequestType       string   `json:"request_type"`
	RequestedValidity int      `json:"requested_validity"`
	ExpiresOn         string   `json:"expires_on"`
	RevokedAt         string   `json:"revoked_at"`
	Status            string   `json:"status"`
}

func parseOriginCACertificate(data []byte) (OriginCACertificate, error) {
	var envelope struct {
		Result originCACertificateJSON `json:"result"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return OriginCACertificate{}, err
	}
	certificate := envelope.Result.toDomain()
	if certificate.ID == "" {
		return OriginCACertificate{}, errors.New("cloudflare origin ca response missing certificate id")
	}
	return certificate, nil
}

func (value originCACertificateJSON) toDomain() OriginCACertificate {
	certificate := OriginCACertificate{
		ID:                value.ID,
		CertificatePEM:    []byte(value.Certificate),
		Hostnames:         normalizeOriginCAHostnames(value.Hostnames),
		RequestType:       value.RequestType,
		RequestedValidity: value.RequestedValidity,
		Status:            value.Status,
	}
	certificate.ExpiresOn = parseCloudflareTime(value.ExpiresOn)
	certificate.RevokedAt = parseCloudflareTime(value.RevokedAt)
	return certificate
}

func parseCloudflareTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			parsed = parsed.UTC()
			return &parsed
		}
	}
	return nil
}

func normalizeOriginCAHostnames(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}
