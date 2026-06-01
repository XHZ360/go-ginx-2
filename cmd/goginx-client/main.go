package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/simp-frp/go-ginx-2/internal/clientjoin"
	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/daemon"
	"github.com/simp-frp/go-ginx-2/internal/deploypath"
)

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
		root, err := deploymentRoot()
		if err != nil {
			return config.LoadClient(configPath)
		}
		cfg, err := config.LoadClient(resolveExistingDeploymentPath(root, configPath))
		if err != nil {
			return config.Client{}, err
		}
		config.ResolveClientPaths(&cfg, root)
		return cfg, cfg.Validate()
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
	configPath := flags.String("config", defaultClientConfigPath(), "client config path to update")
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
	root, err := deploymentRoot()
	if err == nil {
		*statePath = deploypath.Resolve(root, *statePath)
		*configPath = deploypath.Resolve(root, *configPath)
		*caFile = deploypath.Resolve(root, *caFile)
	}
	if err := config.WriteClientCA(caPEM, *caFile); err != nil {
		return err
	}
	cfg.ServerCAFile = *caFile
	if err := config.SaveManagedClient(cfg, *statePath); err != nil {
		return err
	}
	return config.SaveManagedClient(cfg, *configPath)
}

func defaultClientStatePath() string {
	root, err := deploymentRoot()
	if err != nil {
		return config.DefaultClientStatePath
	}
	return deploypath.Resolve(root, config.DefaultClientStatePath)
}

func defaultClientCAFile() string {
	root, err := deploymentRoot()
	if err != nil {
		return config.DefaultClientCAFile
	}
	return deploypath.Resolve(root, config.DefaultClientCAFile)
}

func defaultClientConfigPath() string {
	root, err := deploymentRoot()
	if err != nil {
		return config.DefaultClientConfigPath
	}
	return deploypath.Resolve(root, config.DefaultClientConfigPath)
}

func deploymentRoot() (string, error) {
	return deploypath.Root(executablePath)
}

func resolveExistingDeploymentPath(root string, path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return deploypath.Resolve(root, path)
}
