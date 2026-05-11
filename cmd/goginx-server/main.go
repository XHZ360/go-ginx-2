package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/simp-frp/go-ginx-2/internal/config"
)

func main() {
	configPath := flag.String("config", "server.json", "server config path")
	flag.Parse()

	cfg, err := config.LoadServer(*configPath)
	if err != nil {
		log.Fatalf("load server config: %v", err)
	}

	fmt.Printf("go-ginx server config loaded: admin=%s control_quic=%s db=%s\n", cfg.AdminListen, cfg.ControlQUICListen, cfg.SQLitePath)
}
