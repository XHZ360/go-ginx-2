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
	log.Printf("go-ginx server started: control_quic=%s http=%s tcp_entries=%d", runtime.ControlListener.Addr(), runtime.HTTPServer.Addr(), len(runtime.TCPListeners))
	<-ctx.Done()
}
