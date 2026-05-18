package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/daemon"
	"github.com/simp-frp/go-ginx-2/internal/deploypath"
)

var executablePath = os.Executable

func main() {
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
	httpsAddress := "disabled"
	if runtime.HTTPSListener != nil {
		httpsAddress = runtime.HTTPSListener.Addr().String()
	}
	log.Printf("go-ginx server started: admin=%s control_quic=%s control_tls=%s http=%s https=%s tcp_entries=%d udp_entries=%d", adminAddress, runtime.ControlListener.Addr(), controlTLSAddress, runtime.HTTPServer.Addr(), httpsAddress, len(runtime.TCPListeners), len(runtime.UDPListeners))
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
