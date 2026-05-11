package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
)

type Server struct {
	AdminListen        string        `json:"admin_listen"`
	ControlQUICListen  string        `json:"control_quic_listen"`
	ControlTLSListen   string        `json:"control_tls_listen"`
	ControlTLSCertFile string        `json:"control_tls_cert_file"`
	ControlTLSKeyFile  string        `json:"control_tls_key_file"`
	TCPEntryHost       string        `json:"tcp_entry_host"`
	HTTPEntryListen    string        `json:"http_entry_listen"`
	SQLitePath         string        `json:"sqlite_path"`
	DataDir            string        `json:"data_dir"`
	CertificateDir     string        `json:"certificate_dir"`
	HeartbeatTimeout   time.Duration `json:"heartbeat_timeout"`
	LogRetentionDays   int           `json:"log_retention_days"`
}

type Client struct {
	ServerAddress    string            `json:"server_address"`
	ServerName       string            `json:"server_name"`
	ServerCAFile     string            `json:"server_ca_file"`
	ClientID         string            `json:"client_id"`
	Credential       string            `json:"credential"`
	AllowedProtocols []domain.Protocol `json:"allowed_protocols"`
	Reconnect        Reconnect         `json:"reconnect"`
}

type Reconnect struct {
	InitialDelay time.Duration `json:"initial_delay"`
	MaxDelay     time.Duration `json:"max_delay"`
}

func DefaultServer() Server {
	return Server{
		AdminListen:        "127.0.0.1:8080",
		ControlQUICListen:  ":8443",
		ControlTLSListen:   ":9443",
		ControlTLSCertFile: "data/certs/control.crt",
		ControlTLSKeyFile:  "data/certs/control.key",
		TCPEntryHost:       "0.0.0.0",
		HTTPEntryListen:    ":8081",
		SQLitePath:         "data/go-ginx.db",
		DataDir:            "data",
		CertificateDir:     "data/certs",
		HeartbeatTimeout:   45 * time.Second,
		LogRetentionDays:   7,
	}
}

func DefaultClient() Client {
	return Client{
		AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC, domain.ProtocolTCPTLS},
		Reconnect: Reconnect{
			InitialDelay: time.Second,
			MaxDelay:     30 * time.Second,
		},
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

func (cfg Server) Validate() error {
	if err := requireAddress("admin_listen", cfg.AdminListen); err != nil {
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
	if strings.TrimSpace(cfg.TCPEntryHost) == "" {
		return errors.New("tcp_entry_host is required")
	}
	if err := requireAddress("http_entry_listen", cfg.HTTPEntryListen); err != nil {
		return err
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
	if cfg.HeartbeatTimeout <= 0 {
		return errors.New("heartbeat_timeout must be positive")
	}
	if cfg.LogRetentionDays <= 0 {
		return errors.New("log_retention_days must be positive")
	}
	return nil
}

func (cfg Client) Validate() error {
	if err := requireAddress("server_address", cfg.ServerAddress); err != nil {
		return err
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
	return nil
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
