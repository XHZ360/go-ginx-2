package clientjoin

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"

	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/enrollment"
)

func Join(ctx context.Context, token string, httpClient *http.Client) (config.Client, []byte, error) {
	payload, err := enrollment.DecodeToken(token)
	if err != nil {
		return config.Client{}, nil, err
	}
	if httpClient == nil {
		httpClient, err = httpClientForPayload(payload)
		if err != nil {
			return config.Client{}, nil, err
		}
	}
	body, err := json.Marshal(enrollment.RedeemRequest{Token: token})
	if err != nil {
		return config.Client{}, nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, payload.EnrollmentURL, bytes.NewReader(body))
	if err != nil {
		return config.Client{}, nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := httpClient.Do(request)
	if err != nil {
		return config.Client{}, nil, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return config.Client{}, nil, err
	}
	if response.StatusCode != http.StatusOK {
		return config.Client{}, nil, fmt.Errorf("enrollment failed: status %d: %s", response.StatusCode, string(responseBody))
	}
	var decoded enrollment.RedeemResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return config.Client{}, nil, err
	}
	cfg := enrollment.ConfigFromResponse(decoded, config.DefaultClientCAFile)
	if err := cfg.Validate(); err != nil {
		return config.Client{}, nil, err
	}
	return cfg, []byte(decoded.CAPEM), nil
}

func httpClientForPayload(payload enrollment.TokenPayload) (*http.Client, error) {
	parsed, err := url.Parse(payload.EnrollmentURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "http" {
		return http.DefaultClient, nil
	}
	if payload.CAPEM == "" {
		return nil, errors.New("ca pem is required")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(payload.CAPEM)) {
		return nil, errors.New("ca pem contains no certificates")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: pool, ServerName: payload.ServerName, MinVersion: tls.VersionTLS13}
	transport.DialContext = (&net.Dialer{}).DialContext
	return &http.Client{Transport: transport}, nil
}
