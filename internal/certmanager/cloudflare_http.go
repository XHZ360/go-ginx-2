package certmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type cloudflareAPIRequest struct {
	APIToken       string
	HTTPClient     *http.Client
	BaseURL        string
	Method         string
	Path           string
	Query          url.Values
	Body           any
	FailureMessage string
	NotFoundError  error
}

type CloudflareAPIError struct {
	FailureMessage string
	StatusCode     int
	Errors         []CloudflareAPIErrorDetail
}

type CloudflareAPIErrorDetail struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func doCloudflareAPIRequest(ctx context.Context, input cloudflareAPIRequest) ([]byte, error) {
	token := normalizeCloudflareAPIToken(input.APIToken)
	if token == "" {
		return nil, errors.New("cloudflare api token is required")
	}
	baseURL := strings.TrimRight(input.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.cloudflare.com/client/v4"
	}
	var reader io.Reader
	if input.Body != nil {
		encoded, err := json.Marshal(input.Body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	requestURL := baseURL + input.Path
	if len(input.Query) > 0 {
		requestURL += "?" + input.Query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, input.Method, requestURL, reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	httpClient := input.HTTPClient
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
	if response.StatusCode == http.StatusNotFound && input.NotFoundError != nil {
		return nil, input.NotFoundError
	}
	message := strings.TrimSpace(input.FailureMessage)
	if message == "" {
		message = "cloudflare api request failed"
	}
	var envelope struct {
		Success bool            `json:"success"`
		Errors  json.RawMessage `json:"errors"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, &CloudflareAPIError{FailureMessage: message, StatusCode: response.StatusCode}
		}
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, newCloudflareAPIError(message, response.StatusCode, envelope.Errors)
	}
	if !envelope.Success {
		return nil, newCloudflareAPIError(message, 0, envelope.Errors)
	}
	return data, nil
}

func normalizeCloudflareAPIToken(token string) string {
	token = strings.TrimSpace(token)
	fields := strings.Fields(token)
	if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
		return fields[1]
	}
	return token
}

func newCloudflareAPIError(message string, statusCode int, rawErrors json.RawMessage) error {
	var details []CloudflareAPIErrorDetail
	if len(rawErrors) > 0 {
		_ = json.Unmarshal(rawErrors, &details)
	}
	return &CloudflareAPIError{FailureMessage: message, StatusCode: statusCode, Errors: details}
}

func (err *CloudflareAPIError) Error() string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.FailureMessage)
	if message == "" {
		message = "cloudflare api request failed"
	}
	if err.StatusCode > 0 {
		message = fmt.Sprintf("%s: status %d", message, err.StatusCode)
	}
	codes := make([]string, 0, len(err.Errors))
	for _, detail := range err.Errors {
		if detail.Code != 0 {
			codes = append(codes, fmt.Sprintf("%d", detail.Code))
		}
	}
	if len(codes) > 0 {
		message += " (cloudflare error codes: " + strings.Join(codes, ",") + ")"
	}
	return message
}

func IsCloudflareAPIErrorCode(err error, code int) bool {
	var apiError *CloudflareAPIError
	if !errors.As(err, &apiError) {
		return false
	}
	for _, detail := range apiError.Errors {
		if detail.Code == code {
			return true
		}
	}
	return false
}
