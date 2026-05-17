package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/simp-frp/go-ginx-2/internal/clientjoin"
	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/daemon"
)

const defaultBinaryDir = "bin"

var executablePath = os.Executable

func main() {
	if len(os.Args) > 1 && os.Args[1] == "join" {
		if err := runJoin(os.Args[2:]); err != nil {
			log.Fatalf("join client: %v", err)
		}
		return
	}

	configPath := flag.String("config", "", "client config path; when omitted, managed client state is used")
	flag.Parse()

	cfg, err := loadClientConfig(*configPath)
	if err != nil {
		log.Fatalf("load client config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := daemon.RunClient(ctx, cfg); err != nil {
		log.Fatalf("run client: %v", err)
	}
}

func loadClientConfig(configPath string) (config.Client, error) {
	if configPath != "" {
		return config.LoadClient(configPath)
	}
	statePath := defaultClientStatePath()
	cfg, err := config.LoadClient(statePath)
	if err == nil {
		return cfg, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return config.Client{}, fmt.Errorf("managed client state %s is missing; run `goginx-client join <token>` before starting the client service, or pass `-config config/client.json` for explicit config: %w", statePath, err)
	}
	return config.Client{}, err
}

func runJoin(args []string) error {
	flags := flag.NewFlagSet("join", flag.ContinueOnError)
	statePath := flags.String("state", defaultClientStatePath(), "managed client state path")
	caFile := flags.String("ca-file", defaultClientCAFile(), "managed server CA path")
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

func defaultClientStatePath() string {
	root, err := deploymentRoot()
	if err != nil {
		return config.DefaultClientStatePath
	}
	return filepath.Join(root, config.DefaultClientStatePath)
}

func defaultClientCAFile() string {
	root, err := deploymentRoot()
	if err != nil {
		return config.DefaultClientCAFile
	}
	return filepath.Join(root, config.DefaultClientCAFile)
}

func deploymentRoot() (string, error) {
	executable, err := executablePath()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	absExecutable, err := filepath.Abs(executable)
	if err != nil {
		return "", fmt.Errorf("resolve absolute executable path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(absExecutable); err == nil {
		absExecutable = resolved
	}
	root := filepath.Dir(absExecutable)
	if filepath.Base(root) == defaultBinaryDir {
		root = filepath.Dir(root)
	}
	return root, nil
}
