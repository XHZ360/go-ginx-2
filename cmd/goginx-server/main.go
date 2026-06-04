package main

import (
	"context"
	"flag"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/daemon"
	"github.com/simp-frp/go-ginx-2/internal/deploypath"
)

var executablePath = os.Executable

func main() {
	closeLog := setupLogOutput("server.log")
	defer closeLog()
	configPath := flag.String("config", "", "server config path; when omitted, managed defaults are used")
	flag.Parse()

	cfg, err := loadServerConfig(*configPath)
	if err != nil {
		log.Fatalf("load server config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	runtime, err := daemon.StartServer(ctx, cfg)
	if err != nil {
		log.Fatalf("start server: %v", err)
	}
	defer func() { _ = runtime.Close() }()
	adminAddress := "disabled"
	if runtime.AdminServer != nil {
		adminAddress = runtime.AdminServer.Addr().String()
	}
	controlTLSAddress := "disabled"
	if runtime.ControlTLSListener != nil {
		controlTLSAddress = runtime.ControlTLSListener.Addr().String()
	}
	enrollmentAddress := "disabled"
	if runtime.EnrollmentServer != nil {
		enrollmentAddress = runtime.EnrollmentServer.Addr().String()
	}
	httpsAddress := "disabled"
	if runtime.HTTPSListener != nil {
		httpsAddress = runtime.HTTPSListener.Addr().String()
	}
	httpAddress := "disabled"
	if runtime.HTTPServer != nil {
		httpAddress = runtime.HTTPServer.Addr().String()
	}
	log.Printf("go-ginx server started: admin=%s enrollment=%s control_quic=%s control_tls=%s http=%s https=%s join_service_host=%s join_service_source=%s join_server_address=%s join_server_tls_address=%s join_enrollment_url=%s tcp_proxy_listeners=%d udp_proxy_listeners=%d http_proxy_listeners=%d https_proxy_listeners=%d", adminAddress, enrollmentAddress, runtime.ControlListener.Addr(), controlTLSAddress, httpAddress, httpsAddress, runtime.JoinService.Host, runtime.JoinService.Source, runtime.JoinService.ServerAddress, runtime.JoinService.ServerTLSAddress, runtime.JoinService.EnrollmentURL, runtime.TCPProxyListenerCount(), runtime.UDPProxyListenerCount(), runtime.HTTPProxyListenerCount(), runtime.HTTPSProxyListenerCount())
	<-ctx.Done()
}

func loadServerConfig(configPath string) (config.Server, error) {
	root, err := deploypath.Root(executablePath)
	if err != nil {
		if configPath == "" {
			return config.LoadManagedServer()
		}
		return config.LoadServer(configPath)
	}
	if configPath == "" {
		return config.LoadManagedServerAtRoot(root)
	}
	resolvedConfigPath := resolveExistingDeploymentPath(root, configPath)
	cfg, err := config.LoadServer(resolvedConfigPath)
	if err != nil {
		return config.Server{}, err
	}
	config.ResolveServerPaths(&cfg, root)
	return cfg, cfg.Validate()
}

func resolveExistingDeploymentPath(root string, path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return deploypath.Resolve(root, path)
}

func setupLogOutput(name string) func() {
	root, err := deploypath.Root(executablePath)
	if err != nil {
		root = "."
	}
	logDir := filepath.Join(root, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		log.Printf("create log directory %s: %v", logDir, err)
		return func() {}
	}
	file, err := os.OpenFile(filepath.Join(logDir, name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("open log file %s: %v", filepath.Join(logDir, name), err)
		return func() {}
	}
	log.SetOutput(io.MultiWriter(os.Stderr, file))
	return func() { _ = file.Close() }
}
