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
	configPath := flag.String("config", "client.json", "client config path")
	flag.Parse()

	cfg, err := config.LoadClient(*configPath)
	if err != nil {
		log.Fatalf("load client config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := daemon.RunClient(ctx, cfg); err != nil {
		log.Fatalf("run client: %v", err)
	}
}
