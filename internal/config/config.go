package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/simp-frp/go-ginx-2/internal/domain"
)

type Server struct {
	AdminEnabled               bool          `json:"admin_enabled"`
	AdminListen                string        `json:"admin_listen"`
	AdminCredentialsFile       string        `json:"admin_credentials_file"`
	AdminFrontendDir           string        `json:"admin_frontend_dir"`
	AdminJWTSecretFile         string        `json:"admin_jwt_secret_file"`
	ClientEnrollmentListen     string        `json:"client_enrollment_listen"`
	ControlQUICListen          string        `json:"control_quic_listen"`
	ControlTLSListen           string        `json:"control_tls_listen"`
	ControlTLSServerName       string        `json:"control_tls_server_name"`
	ControlTLSCAFile           string        `json:"control_tls_ca_file"`
	ControlTLSCertFile         string        `json:"control_tls_cert_file"`
	ControlTLSKeyFile          string        `json:"control_tls_key_file"`
	JoinServiceHost            string        `json:"join_service_host"`
	TCPEntryHost               string        `json:"tcp_entry_host"`
	HTTPEntryListen            string        `json:"http_entry_listen"`
	HTTPSEntryListen           string        `json:"https_entry_listen"`
	SQLitePath                 string        `json:"sqlite_path"`
	DataDir                    string        `json:"data_dir"`
	CertificateDir             string        `json:"certificate_dir"`
	ACMEEnabled                bool          `json:"acme_enabled"`
	ACMEDirectoryURL           string        `json:"acme_directory_url"`
	ACMEAccountEmail           string        `json:"acme_account_email"`
	ACMETermsAccepted          bool          `json:"acme_terms_accepted"`
	ACMERenewalWindow          time.Duration `json:"acme_renewal_window"`
	ACMECloudflareTokenEnv     string        `json:"acme_cloudflare_token_env"`
	OriginCAEnabled            bool          `json:"origin_ca_enabled"`
	OriginCASecretStorePath    string        `json:"origin_ca_secret_store_path"`
	OriginCADefaultRequestType string        `json:"origin_ca_default_request_type"`
	OriginCARequestedValidity  int           `json:"origin_ca_requested_validity"`
	OriginCARotationWindow     time.Duration `json:"origin_ca_rotation_window"`
	OriginCAServiceKeyPath     string        `json:"origin_ca_service_key_path"`
	HeartbeatTimeout           time.Duration `json:"heartbeat_timeout"`
	LogMaxSizeMB               int           `json:"log_max_size_mb"`
	LogMaxBackups              int           `json:"log_max_backups"`
	LogRetentionDays           int           `json:"log_retention_days"`
	LogCompress                bool          `json:"log_compress"`
}

type JoinServiceDefaults struct {
	Host                     string
	Source                   string
	ServerAddress            string
	ServerTLSAddress         string
	EnrollmentURL            string
	LegacyAdminEnrollmentURL string
	ServerName               string
	ServerCAFile             string
}

type Client struct {
	ServerAddress    string            `json:"server_address"`
	ServerTLSAddress string            `json:"server_tls_address"`
	ServerName       string            `json:"server_name"`
	ServerCAFile     string            `json:"server_ca_file"`
	ClientID         string            `json:"client_id"`
	Credential       string            `json:"credential"`
	AllowedProtocols []domain.Protocol `json:"allowed_protocols"`
	Reconnect        Reconnect         `json:"reconnect"`
	LogMaxSizeMB     int               `json:"log_max_size_mb"`
	LogMaxBackups    int               `json:"log_max_backups"`
	LogRetentionDays int               `json:"log_retention_days"`
	LogCompress      bool              `json:"log_compress"`
}

type Reconnect struct {
	InitialDelay time.Duration `json:"initial_delay"`
	MaxDelay     time.Duration `json:"max_delay"`
}

type LogRotation struct {
	MaxSizeMB     int
	MaxBackups    int
	RetentionDays int
	Compress      bool
}

func DefaultServer() Server {
	logRotation := DefaultLogRotation()
	return Server{
		AdminEnabled:               false,
		AdminListen:                "127.0.0.1:8080",
		AdminCredentialsFile:       "",
		AdminFrontendDir:           "",
		AdminJWTSecretFile:         "data/admin-jwt.key",
		ClientEnrollmentListen:     ":8081",
		ControlQUICListen:          ":8443",
		ControlTLSListen:           ":9443",
		ControlTLSServerName:       "go-ginx-control.local",
		ControlTLSCAFile:           "data/certs/control-ca.crt",
		ControlTLSCertFile:         "data/certs/control.crt",
		ControlTLSKeyFile:          "data/certs/control.key",
		JoinServiceHost:            "",
		TCPEntryHost:               "0.0.0.0",
		HTTPEntryListen:            ":80",
		HTTPSEntryListen:           ":443",
		SQLitePath:                 "data/go-ginx.db",
		DataDir:                    "data",
		CertificateDir:             "data/certs",
		ACMEDirectoryURL:           "https://acme-v02.api.letsencrypt.org/directory",
		ACMERenewalWindow:          30 * 24 * time.Hour,
		ACMECloudflareTokenEnv:     "CF_DNS_API_TOKEN",
		OriginCAEnabled:            true,
		OriginCASecretStorePath:    "data/secrets/provider-credentials",
		OriginCADefaultRequestType: "origin-ecc",
		OriginCARequestedValidity:  5475,
		OriginCARotationWindow:     30 * 24 * time.Hour,
		HeartbeatTimeout:           45 * time.Second,
		LogMaxSizeMB:               logRotation.MaxSizeMB,
		LogMaxBackups:              logRotation.MaxBackups,
		LogRetentionDays:           logRotation.RetentionDays,
		LogCompress:                logRotation.Compress,
	}
}

func DefaultClient() Client {
	logRotation := DefaultLogRotation()
	return Client{
		AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC, domain.ProtocolTCPTLS},
		Reconnect: Reconnect{
			InitialDelay: time.Second,
			MaxDelay:     30 * time.Second,
		},
		LogMaxSizeMB:     logRotation.MaxSizeMB,
		LogMaxBackups:    logRotation.MaxBackups,
		LogRetentionDays: logRotation.RetentionDays,
		LogCompress:      logRotation.Compress,
	}
}

func DefaultLogRotation() LogRotation {
	return LogRotation{
		MaxSizeMB:     50,
		MaxBackups:    10,
		RetentionDays: 7,
		Compress:      true,
	}
}

func LoadServer(path string) (Server, error) {
	cfg := DefaultServer()
	if err := loadJSON(path, &cfg); err != nil {
		return Server{}, err
	}
	return cfg, cfg.Validate()
}

func LoadClient(path string) (Client, error) {
	cfg := DefaultClient()
	if err := loadJSON(path, &cfg); err != nil {
		return Client{}, err
	}
	return cfg, cfg.Validate()
}

func LoadDefaultServer() (Server, error) {
	cfg := DefaultServer()
	return cfg, cfg.Validate()
}

func (cfg Server) Validate() error {
	if err := requireAddress("admin_listen", cfg.AdminListen); err != nil {
		return err
	}
	if err := requireAddress("client_enrollment_listen", cfg.ClientEnrollmentListen); err != nil {
		return err
	}
	if err := requireAddress("control_quic_listen", cfg.ControlQUICListen); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.ControlTLSListen) != "" {
		if err := requireAddress("control_tls_listen", cfg.ControlTLSListen); err != nil {
			return err
		}
	}
	if strings.TrimSpace(cfg.ControlTLSCertFile) == "" {
		return errors.New("control_tls_cert_file is required")
	}
	if strings.TrimSpace(cfg.ControlTLSKeyFile) == "" {
		return errors.New("control_tls_key_file is required")
	}
	if err := validateOptionalServiceHost("join_service_host", cfg.JoinServiceHost); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.TCPEntryHost) == "" {
		return errors.New("tcp_entry_host is required")
	}
	if err := requireAddress("http_entry_listen", cfg.HTTPEntryListen); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.HTTPSEntryListen) != "" {
		if err := requireAddress("https_entry_listen", cfg.HTTPSEntryListen); err != nil {
			return err
		}
	}
	if strings.TrimSpace(cfg.SQLitePath) == "" {
		return errors.New("sqlite_path is required")
	}
	if strings.TrimSpace(cfg.DataDir) == "" {
		return errors.New("data_dir is required")
	}
	if strings.TrimSpace(cfg.CertificateDir) == "" {
		return errors.New("certificate_dir is required")
	}
	if (cfg.AdminEnabled || strings.TrimSpace(cfg.AdminCredentialsFile) != "") && strings.TrimSpace(cfg.AdminJWTSecretFile) == "" {
		return errors.New("admin_jwt_secret_file is required when admin is enabled")
	}
	if cfg.ACMEEnabled {
		if strings.TrimSpace(cfg.ACMEDirectoryURL) == "" {
			return errors.New("acme_directory_url is required when acme is enabled")
		}
		if strings.TrimSpace(cfg.ACMEAccountEmail) == "" {
			return errors.New("acme_account_email is required when acme is enabled")
		}
		if !cfg.ACMETermsAccepted {
			return errors.New("acme_terms_accepted is required when acme is enabled")
		}
		if cfg.ACMERenewalWindow <= 0 {
			return errors.New("acme_renewal_window must be positive when acme is enabled")
		}
		if strings.TrimSpace(cfg.ACMECloudflareTokenEnv) == "" {
			return errors.New("acme_cloudflare_token_env is required when acme is enabled")
		}
	}
	if strings.TrimSpace(cfg.OriginCAServiceKeyPath) != "" {
		return errors.New("origin_ca_service_key_path is not supported; use an admin-managed cloudflare api token credential")
	}
	if cfg.OriginCAEnabled {
		if strings.TrimSpace(cfg.OriginCASecretStorePath) == "" {
			return errors.New("origin_ca_secret_store_path is required when origin ca is enabled")
		}
		switch strings.TrimSpace(cfg.OriginCADefaultRequestType) {
		case "", "origin-ecc", "origin-rsa":
		default:
			return errors.New("origin_ca_default_request_type must be origin-ecc or origin-rsa")
		}
		if cfg.OriginCARequestedValidity <= 0 {
			return errors.New("origin_ca_requested_validity must be positive when origin ca is enabled")
		}
		if cfg.OriginCARotationWindow <= 0 {
			return errors.New("origin_ca_rotation_window must be positive when origin ca is enabled")
		}
	}
	if cfg.HeartbeatTimeout <= 0 {
		return errors.New("heartbeat_timeout must be positive")
	}
	if err := cfg.LogRotation().Validate(); err != nil {
		return err
	}
	return nil
}

func ConfirmJoinServiceDefaults(cfg Server) (JoinServiceDefaults, error) {
	if err := cfg.Validate(); err != nil {
		return JoinServiceDefaults{}, err
	}
	host, source, err := confirmedJoinServiceHost(cfg)
	if err != nil {
		return JoinServiceDefaults{}, err
	}
	quicPort, err := addressPort("control_quic_listen", cfg.ControlQUICListen)
	if err != nil {
		return JoinServiceDefaults{}, err
	}
	tlsAddress := ""
	if strings.TrimSpace(cfg.ControlTLSListen) != "" {
		tlsPort, err := addressPort("control_tls_listen", cfg.ControlTLSListen)
		if err != nil {
			return JoinServiceDefaults{}, err
		}
		tlsAddress = net.JoinHostPort(host, tlsPort)
	}
	enrollmentPort, err := addressPort("client_enrollment_listen", cfg.ClientEnrollmentListen)
	if err != nil {
		return JoinServiceDefaults{}, err
	}
	adminPort, err := addressPort("admin_listen", cfg.AdminListen)
	if err != nil {
		return JoinServiceDefaults{}, err
	}
	return JoinServiceDefaults{
		Host:                     host,
		Source:                   source,
		ServerAddress:            net.JoinHostPort(host, quicPort),
		ServerTLSAddress:         tlsAddress,
		EnrollmentURL:            "http://" + net.JoinHostPort(host, enrollmentPort) + "/api/client/enroll",
		LegacyAdminEnrollmentURL: "http://" + net.JoinHostPort(host, adminPort) + "/api/client/enroll",
		ServerName:               cfg.ControlTLSServerName,
		ServerCAFile:             cfg.ControlTLSCAFile,
	}, nil
}

func (cfg Client) Validate() error {
	if err := requireAddress("server_address", cfg.ServerAddress); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.ServerTLSAddress) != "" {
		if err := requireAddress("server_tls_address", cfg.ServerTLSAddress); err != nil {
			return err
		}
	}
	if strings.TrimSpace(cfg.ServerName) == "" {
		return errors.New("server_name is required")
	}
	if strings.TrimSpace(cfg.ServerCAFile) == "" {
		return errors.New("server_ca_file is required")
	}
	if strings.TrimSpace(cfg.ClientID) == "" {
		return errors.New("client_id is required")
	}
	if strings.TrimSpace(cfg.Credential) == "" {
		return errors.New("credential is required")
	}
	if len(cfg.AllowedProtocols) == 0 {
		return errors.New("allowed_protocols is required")
	}
	for _, protocol := range cfg.AllowedProtocols {
		if !protocol.Valid() {
			return fmt.Errorf("unsupported protocol %q", protocol)
		}
	}
	if cfg.Reconnect.InitialDelay <= 0 {
		return errors.New("reconnect.initial_delay must be positive")
	}
	if cfg.Reconnect.MaxDelay < cfg.Reconnect.InitialDelay {
		return errors.New("reconnect.max_delay must be greater than or equal to reconnect.initial_delay")
	}
	if err := cfg.LogRotation().Validate(); err != nil {
		return err
	}
	return nil
}

func (cfg Server) LogRotation() LogRotation {
	return LogRotation{
		MaxSizeMB:     cfg.LogMaxSizeMB,
		MaxBackups:    cfg.LogMaxBackups,
		RetentionDays: cfg.LogRetentionDays,
		Compress:      cfg.LogCompress,
	}
}

func (cfg Server) WithLogRotationDefaults() Server {
	rotation := cfg.LogRotation().WithDefaults()
	cfg.LogMaxSizeMB = rotation.MaxSizeMB
	cfg.LogMaxBackups = rotation.MaxBackups
	cfg.LogRetentionDays = rotation.RetentionDays
	cfg.LogCompress = rotation.Compress
	return cfg
}

func (cfg Client) LogRotation() LogRotation {
	return LogRotation{
		MaxSizeMB:     cfg.LogMaxSizeMB,
		MaxBackups:    cfg.LogMaxBackups,
		RetentionDays: cfg.LogRetentionDays,
		Compress:      cfg.LogCompress,
	}
}

func (cfg Client) WithLogRotationDefaults() Client {
	rotation := cfg.LogRotation().WithDefaults()
	cfg.LogMaxSizeMB = rotation.MaxSizeMB
	cfg.LogMaxBackups = rotation.MaxBackups
	cfg.LogRetentionDays = rotation.RetentionDays
	cfg.LogCompress = rotation.Compress
	return cfg
}

func (rotation LogRotation) WithDefaults() LogRotation {
	defaults := DefaultLogRotation()
	if rotation.MaxSizeMB == 0 && rotation.MaxBackups == 0 && rotation.RetentionDays == 0 && !rotation.Compress {
		return defaults
	}
	if rotation.MaxSizeMB == 0 {
		rotation.MaxSizeMB = defaults.MaxSizeMB
	}
	if rotation.RetentionDays == 0 {
		rotation.RetentionDays = defaults.RetentionDays
	}
	return rotation
}

func (rotation LogRotation) Validate() error {
	if rotation.MaxSizeMB <= 0 {
		return errors.New("log_max_size_mb must be positive")
	}
	if rotation.MaxBackups < 0 {
		return errors.New("log_max_backups must be zero or positive")
	}
	if rotation.RetentionDays <= 0 {
		return errors.New("log_retention_days must be positive")
	}
	return nil
}

func (cfg Server) RuntimeListenerClaims(includeAdmin bool) ([]domain.ListenerClaim, error) {
	claims := make([]domain.ListenerClaim, 0, 6)
	claims = append(claims, listenerClaimFromAddress("control_quic_listen", "control_quic", domain.ListenerNetworkUDP, cfg.ControlQUICListen)...)
	claims = append(claims, listenerClaimFromAddress("control_tls_listen", "control_tls", domain.ListenerNetworkTCP, cfg.ControlTLSListen)...)
	claims = append(claims, listenerClaimFromAddress("client_enrollment_listen", "client_enrollment", domain.ListenerNetworkTCP, cfg.ClientEnrollmentListen)...)
	if includeAdmin {
		claims = append(claims, listenerClaimFromAddress("admin_listen", "admin", domain.ListenerNetworkTCP, cfg.AdminListen)...)
	}
	claims = append(claims, listenerClaimFromAddress("http_entry_listen", domain.ListenerProtocolHTTP, domain.ListenerNetworkTCP, cfg.HTTPEntryListen)...)
	claims = append(claims, listenerClaimFromAddress("https_entry_listen", domain.ListenerProtocolHTTPS, domain.ListenerNetworkTCP, cfg.HTTPSEntryListen)...)
	for index := range claims {
		if claims[index].Port == 0 {
			return nil, fmt.Errorf("%s port is invalid", claims[index].Source)
		}
	}
	return claims, nil
}

func (cfg Server) ProxyEntryDefaults() (domain.ProxyEntryDefaults, error) {
	httpHost, httpPort, err := domain.ParseListenAddress(cfg.HTTPEntryListen)
	if err != nil {
		return domain.ProxyEntryDefaults{}, fmt.Errorf("http_entry_listen must be host:port: %w", err)
	}
	defaults := domain.ProxyEntryDefaults{
		TCPBindHost:  domain.NormalizeBindHost(cfg.TCPEntryHost),
		HTTPBindHost: httpHost,
		HTTPPort:     httpPort,
	}
	if strings.TrimSpace(cfg.HTTPSEntryListen) != "" {
		httpsHost, httpsPort, err := domain.ParseListenAddress(cfg.HTTPSEntryListen)
		if err != nil {
			return domain.ProxyEntryDefaults{}, fmt.Errorf("https_entry_listen must be host:port: %w", err)
		}
		defaults.HTTPSBindHost = httpsHost
		defaults.HTTPSPort = httpsPort
	}
	return defaults, nil
}

func loadJSON(path string, target any) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func requireAddress(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	if _, _, err := net.SplitHostPort(value); err != nil {
		return fmt.Errorf("%s must be host:port: %w", name, err)
	}
	return nil
}

func validateOptionalServiceHost(name string, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if strings.Contains(value, "://") || strings.Contains(value, "/") {
		return fmt.Errorf("%s must be a domain name or IP address without scheme or path", name)
	}
	if strings.Contains(value, ":") && net.ParseIP(value) == nil {
		return fmt.Errorf("%s must not include a port", name)
	}
	if ip := net.ParseIP(value); ip != nil {
		return nil
	}
	labels := strings.Split(value, ".")
	for _, label := range labels {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return fmt.Errorf("%s must be a valid domain name or IP address", name)
		}
		for _, r := range label {
			if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-') {
				return fmt.Errorf("%s must be a valid domain name or IP address", name)
			}
		}
	}
	return nil
}

func confirmedJoinServiceHost(cfg Server) (string, string, error) {
	if host := strings.TrimSpace(cfg.JoinServiceHost); host != "" {
		if err := validateOptionalServiceHost("join_service_host", host); err != nil {
			return "", "", err
		}
		return trimIPv6Brackets(host), "join_service_host", nil
	}
	for _, candidate := range []struct {
		source  string
		address string
	}{
		{source: "control_quic_listen", address: cfg.ControlQUICListen},
		{source: "control_tls_listen", address: cfg.ControlTLSListen},
	} {
		host, _, err := net.SplitHostPort(strings.TrimSpace(candidate.address))
		if err != nil || isUnspecifiedHost(host) {
			continue
		}
		return trimIPv6Brackets(host), candidate.source, nil
	}
	if host := firstNonLoopbackIP(); host != "" {
		return host, "local_interface", nil
	}
	return "127.0.0.1", "loopback_fallback", nil
}

func isUnspecifiedHost(host string) bool {
	host = strings.TrimSpace(trimIPv6Brackets(host))
	if host == "" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsUnspecified()
}

func firstNonLoopbackIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch value := addr.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
				continue
			}
			if ipv4 := ip.To4(); ipv4 != nil {
				return ipv4.String()
			}
		}
	}
	return ""
}

func addressPort(name string, value string) (string, error) {
	_, port, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("%s must be host:port: %w", name, err)
	}
	if port == "" {
		return "", fmt.Errorf("%s port is invalid", name)
	}
	return port, nil
}

func trimIPv6Brackets(host string) string {
	return strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(host), "["), "]")
}

func listenerClaimFromAddress(sourceName string, protocol string, network string, value string) []domain.ListenerClaim {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	host, portText, err := net.SplitHostPort(value)
	if err != nil {
		return []domain.ListenerClaim{{Protocol: protocol, Network: network, Source: sourceName}}
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return []domain.ListenerClaim{{Protocol: protocol, Network: network, Source: sourceName}}
	}
	return []domain.ListenerClaim{{Protocol: protocol, Network: network, BindHost: domain.NormalizeBindHost(host), Port: port, Source: sourceName, ResourceID: sourceName}}
}
