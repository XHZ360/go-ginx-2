package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/simp-frp/go-ginx-2/internal/clientjoin"
	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/daemon"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "join" {
		if err := runJoin(os.Args[2:]); err != nil {
			log.Fatalf("join client: %v", err)
		}
		return
	}

	configPath := flag.String("config", "", "client config path; when omitted, managed client state is used")
	flag.Parse()

	var cfg config.Client
	var err error
	if *configPath == "" {
		cfg, err = config.LoadManagedClient()
	} else {
		cfg, err = config.LoadClient(*configPath)
	}
	if err != nil {
		log.Fatalf("load client config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := daemon.RunClient(ctx, cfg); err != nil {
		log.Fatalf("run client: %v", err)
	}
}

func runJoin(args []string) error {
	flags := flag.NewFlagSet("join", flag.ContinueOnError)
	statePath := flags.String("state", config.DefaultClientStatePath, "managed client state path")
	caFile := flags.String("ca-file", config.DefaultClientCAFile, "managed server CA path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return flag.ErrHelp
	}
	cfg, caPEM, err := clientjoin.Join(context.Background(), flags.Arg(0), nil)
	if err != nil {
		return err
	}
	if err := config.WriteClientCA(caPEM, *caFile); err != nil {
		return err
	}
	cfg.ServerCAFile = *caFile
	return config.SaveManagedClient(cfg, *statePath)
}
