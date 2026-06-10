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

func doCloudflareAPIRequest(ctx context.Context, input cloudflareAPIRequest) ([]byte, error) {
	token := strings.TrimSpace(input.APIToken)
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
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: status %d", message, response.StatusCode)
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
		return nil, errors.New(message)
	}
	return data, nil
}
