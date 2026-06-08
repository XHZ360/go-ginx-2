package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/daemon"
	"github.com/simp-frp/go-ginx-2/internal/deploypath"
	"github.com/simp-frp/go-ginx-2/internal/logging"
	"github.com/simp-frp/go-ginx-2/internal/winservice"
)

var executablePath = os.Executable
var runWindowsServiceCommand = winservice.RunCommand

func main() {
	if len(os.Args) > 1 && os.Args[1] == "service" {
		if err := runServiceCommand(os.Args[2:]); err != nil {
			log.Fatalf("service: %v", err)
		}
		return
	}
	configPath := flag.String("config", "", "server config path; when omitted, managed defaults are used")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := runServer(ctx, *configPath); err != nil {
		log.Fatalf("%v", err)
	}
}

func runServer(ctx context.Context, configPath string) error {
	cfg, err := loadServerConfig(configPath)
	if err != nil {
		return fmt.Errorf("load server config: %w", err)
	}
	closeLog := setupLogOutput("server.log", cfg.LogRotation())
	defer closeLog()
	runtime, err := daemon.StartServer(ctx, cfg)
	if err != nil {
		return fmt.Errorf("start server: %w", err)
	}
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
	return runtime.Close()
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

func setupLogOutput(name string, rotation config.LogRotation) func() {
	root, err := deploypath.Root(executablePath)
	if err != nil {
		root = "."
	}
	closeLog, err := logging.SetupStandardLogger(root, name, rotation)
	if err != nil {
		log.Printf("setup log output: %v", err)
		return func() {}
	}
	return closeLog
}

func runServiceCommand(args []string) error {
	return runWindowsServiceCommand(args, winservice.Options{
		Args: args,
		Definition: winservice.Definition{
			DefaultName: "goginx-server",
			DisplayName: "go-ginx server",
			Description: "go-ginx server daemon",
		},
		Runner:          runServer,
		ValidateInstall: validateServerServiceInstall,
		ExecutablePath:  executablePath,
		Stdout:          os.Stdout,
	})
}

func validateServerServiceInstall(configPath string) error {
	if configPath == "" {
		return nil
	}
	_, err := loadServerConfig(configPath)
	return err
}
