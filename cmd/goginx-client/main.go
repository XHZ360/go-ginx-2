package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/simp-frp/go-ginx-2/internal/config"
)

func main() {
	configPath := flag.String("config", "client.json", "client config path")
	flag.Parse()

	cfg, err := config.LoadClient(*configPath)
	if err != nil {
		log.Fatalf("load client config: %v", err)
	}

	fmt.Printf("go-ginx client config loaded: client=%s server=%s protocols=%v\n", cfg.ClientID, cfg.ServerAddress, cfg.AllowedProtocols)
}
