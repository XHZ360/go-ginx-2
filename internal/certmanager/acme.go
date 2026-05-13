package certmanager

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/acme"
)

const dnsChallengePrefix = "_acme-challenge."

type Issuer interface {
	Issue(ctx context.Context, request IssueRequest) (IssuedCertificate, error)
}

type DNSChallengeProvider interface {
	Present(ctx context.Context, fqdn string, value string) error
	CleanUp(ctx context.Context, fqdn string, value string) error
}

type IssueRequest struct {
	Host          string
	AccountEmail  string
	DirectoryURL  string
	TermsAccepted bool
	DNSProvider   DNSChallengeProvider
}

type IssuedCertificate struct {
	CertPEM  []byte
	KeyPEM   []byte
	NotAfter time.Time
}

type ACMEIssuer struct {
	AccountKey crypto.Signer
	HTTPClient *http.Client
}

func (issuer ACMEIssuer) Issue(ctx context.Context, request IssueRequest) (IssuedCertificate, error) {
	host := strings.ToLower(strings.TrimSpace(request.Host))
	if host == "" {
		return IssuedCertificate{}, errors.New("host is required")
	}
	if !request.TermsAccepted {
		return IssuedCertificate{}, errors.New("acme terms must be accepted")
	}
	if strings.TrimSpace(request.AccountEmail) == "" {
		return IssuedCertificate{}, errors.New("acme account email is required")
	}
	if request.DNSProvider == nil {
		return IssuedCertificate{}, errors.New("dns challenge provider is required")
	}
	accountKey, err := issuer.accountKey()
	if err != nil {
		return IssuedCertificate{}, err
	}
	client := &acme.Client{Key: accountKey, DirectoryURL: request.DirectoryURL, HTTPClient: issuer.HTTPClient}
	if _, err := client.Register(ctx, &acme.Account{Contact: []string{"mailto:" + strings.TrimSpace(request.AccountEmail)}}, acme.AcceptTOS); err != nil && !errors.Is(err, acme.ErrAccountAlreadyExists) {
		return IssuedCertificate{}, err
	}
	order, err := client.AuthorizeOrder(ctx, acme.DomainIDs(host))
	if err != nil {
		return IssuedCertificate{}, err
	}
	if err := issuer.completeDNSAuthorizations(ctx, client, request.DNSProvider, order); err != nil {
		return IssuedCertificate{}, err
	}
	readyOrder, err := client.WaitOrder(ctx, order.URI)
	if err != nil {
		return IssuedCertificate{}, err
	}
	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return IssuedCertificate{}, err
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: host}, DNSNames: []string{host}}, certKey)
	if err != nil {
		return IssuedCertificate{}, err
	}
	certDERs, _, err := client.CreateOrderCert(ctx, readyOrder.FinalizeURL, csrDER, true)
	if err != nil {
		return IssuedCertificate{}, err
	}
	certPEM, notAfter, err := encodeCertificateChain(certDERs)
	if err != nil {
		return IssuedCertificate{}, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(certKey)
	if err != nil {
		return IssuedCertificate{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return IssuedCertificate{CertPEM: certPEM, KeyPEM: keyPEM, NotAfter: notAfter}, nil
}

func (issuer ACMEIssuer) accountKey() (crypto.Signer, error) {
	if issuer.AccountKey != nil {
		return issuer.AccountKey, nil
	}
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

func (issuer ACMEIssuer) completeDNSAuthorizations(ctx context.Context, client *acme.Client, provider DNSChallengeProvider, order *acme.Order) error {
	for _, authzURL := range order.AuthzURLs {
		authorization, err := client.GetAuthorization(ctx, authzURL)
		if err != nil {
			return err
		}
		if authorization.Status == acme.StatusValid {
			continue
		}
		challenge := dnsChallenge(authorization.Challenges)
		if challenge == nil {
			return fmt.Errorf("dns-01 challenge not found for %s", authorization.Identifier.Value)
		}
		value, err := client.DNS01ChallengeRecord(challenge.Token)
		if err != nil {
			return err
		}
		fqdn := dnsChallengePrefix + strings.TrimSuffix(authorization.Identifier.Value, ".")
		if err := provider.Present(ctx, fqdn, value); err != nil {
			return err
		}
		accepted, err := client.Accept(ctx, challenge)
		if err != nil {
			return errors.Join(err, provider.CleanUp(ctx, fqdn, value))
		}
		if accepted.Status != acme.StatusValid {
			_, err = client.WaitAuthorization(ctx, authzURL)
		}
		cleanupErr := provider.CleanUp(ctx, fqdn, value)
		if err != nil {
			return errors.Join(err, cleanupErr)
		}
		if cleanupErr != nil {
			return cleanupErr
		}
	}
	return nil
}

func dnsChallenge(challenges []*acme.Challenge) *acme.Challenge {
	for _, challenge := range challenges {
		if challenge.Type == "dns-01" {
			return challenge
		}
	}
	return nil
}

func encodeCertificateChain(certDERs [][]byte) ([]byte, time.Time, error) {
	if len(certDERs) == 0 {
		return nil, time.Time{}, errors.New("acme certificate chain is empty")
	}
	leaf, err := x509.ParseCertificate(certDERs[0])
	if err != nil {
		return nil, time.Time{}, err
	}
	var buffer bytes.Buffer
	for _, certDER := range certDERs {
		if err := pem.Encode(&buffer, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
			return nil, time.Time{}, err
		}
	}
	return buffer.Bytes(), leaf.NotAfter, nil
}

type CloudflareDNSProvider struct {
	APIToken   string
	HTTPClient *http.Client
	BaseURL    string
}

func NewCloudflareDNSProviderFromEnv(tokenEnv string) (CloudflareDNSProvider, error) {
	if strings.TrimSpace(tokenEnv) == "" {
		return CloudflareDNSProvider{}, errors.New("cloudflare token environment variable is required")
	}
	token := strings.TrimSpace(os.Getenv(tokenEnv))
	if token == "" {
		return CloudflareDNSProvider{}, fmt.Errorf("cloudflare token environment variable %s is not set", tokenEnv)
	}
	return CloudflareDNSProvider{APIToken: token}, nil
}

func (provider CloudflareDNSProvider) Present(ctx context.Context, fqdn string, value string) error {
	zoneID, err := provider.zoneID(ctx, fqdn)
	if err != nil {
		return err
	}
	body := map[string]any{"type": "TXT", "name": fqdn, "content": value, "ttl": 120}
	_, err = provider.request(ctx, http.MethodPost, "/zones/"+url.PathEscape(zoneID)+"/dns_records", nil, body)
	return err
}

func (provider CloudflareDNSProvider) CleanUp(ctx context.Context, fqdn string, value string) error {
	zoneID, err := provider.zoneID(ctx, fqdn)
	if err != nil {
		return err
	}
	query := url.Values{"type": {"TXT"}, "name": {fqdn}, "content": {value}}
	data, err := provider.request(ctx, http.MethodGet, "/zones/"+url.PathEscape(zoneID)+"/dns_records", query, nil)
	if err != nil {
		return err
	}
	var list struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	for _, record := range list.Result {
		if record.ID == "" {
			continue
		}
		if _, err := provider.request(ctx, http.MethodDelete, "/zones/"+url.PathEscape(zoneID)+"/dns_records/"+url.PathEscape(record.ID), nil, nil); err != nil {
			return err
		}
	}
	return nil
}

func (provider CloudflareDNSProvider) zoneID(ctx context.Context, fqdn string) (string, error) {
	for _, zone := range candidateZones(fqdn) {
		data, err := provider.request(ctx, http.MethodGet, "/zones", url.Values{"name": {zone}}, nil)
		if err != nil {
			return "", err
		}
		var response struct {
			Result []struct {
				ID string `json:"id"`
			} `json:"result"`
		}
		if err := json.Unmarshal(data, &response); err != nil {
			return "", err
		}
		if len(response.Result) > 0 && response.Result[0].ID != "" {
			return response.Result[0].ID, nil
		}
	}
	return "", errors.New("cloudflare zone not found")
}

func candidateZones(fqdn string) []string {
	name := strings.TrimSuffix(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(fqdn)), dnsChallengePrefix), ".")
	parts := strings.Split(name, ".")
	zones := make([]string, 0, len(parts))
	for index := 0; index < len(parts)-1; index++ {
		zones = append(zones, strings.Join(parts[index:], "."))
	}
	return zones
}

func (provider CloudflareDNSProvider) request(ctx context.Context, method string, path string, query url.Values, body any) ([]byte, error) {
	if strings.TrimSpace(provider.APIToken) == "" {
		return nil, errors.New("cloudflare api token is required")
	}
	baseURL := provider.BaseURL
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.cloudflare.com/client/v4"
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	requestURL := strings.TrimRight(baseURL, "/") + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL, reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+provider.APIToken)
	request.Header.Set("Content-Type", "application/json")
	client := provider.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("cloudflare api request failed: status %d", response.StatusCode)
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
		return nil, errors.New("cloudflare api request failed")
	}
	return data, nil
}
