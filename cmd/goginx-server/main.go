package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/daemon"
)

func main() {
	configPath := flag.String("config", "server.json", "server config path")
	flag.Parse()

	cfg, err := config.LoadServer(*configPath)
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
