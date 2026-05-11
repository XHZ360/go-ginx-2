package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/simp-frp/go-ginx-2/internal/admin"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: goginx-admin <create-user|create-client|create-tcp-proxy|create-http-proxy> [flags]")
	}
	command := args[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	dbPath := flags.String("db", "data/go-ginx.db", "SQLite database path")
	actorID := flags.String("actor", "system", "audit actor user ID")

	switch command {
	case "create-user":
		id := flags.String("id", "", "user ID")
		username := flags.String("username", "", "username")
		role := flags.String("role", string(domain.RoleUser), "role: admin or user")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		service, closeStore, err := openService(*dbPath)
		if err != nil {
			return err
		}
		defer closeStore()
		user, err := service.CreateUser(context.Background(), admin.CreateUserInput{ID: *id, Username: *username, Role: domain.Role(*role), ActorID: *actorID})
		if err != nil {
			return err
		}
		fmt.Println(user.ID)
		return nil
	case "create-client":
		id := flags.String("id", "", "client ID")
		userID := flags.String("user", "", "owner user ID")
		name := flags.String("name", "", "client display name")
		credential := flags.String("credential", "", "client credential")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		service, closeStore, err := openService(*dbPath)
		if err != nil {
			return err
		}
		defer closeStore()
		client, err := service.CreateClient(context.Background(), admin.CreateClientInput{ID: *id, UserID: *userID, Name: *name, Credential: *credential, ActorID: *actorID})
		if err != nil {
			return err
		}
		fmt.Println(client.ID)
		return nil
	case "create-tcp-proxy":
		return createProxy(flags, args[1:], domain.ProxyTCP)
	case "create-http-proxy":
		return createProxy(flags, args[1:], domain.ProxyHTTP)
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func createProxy(flags *flag.FlagSet, args []string, proxyType domain.ProxyType) error {
	id := flags.String("id", "", "proxy ID")
	userID := flags.String("user", "", "owner user ID")
	clientID := flags.String("client", "", "client ID")
	name := flags.String("name", "", "proxy name")
	entryHost := flags.String("host", "", "HTTP entry host")
	entryPort := flags.Int("port", 0, "TCP entry port")
	targetHost := flags.String("target-host", "", "local target host")
	targetPort := flags.Int("target-port", 0, "local target port")
	description := flags.String("description", "", "proxy description")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dbPath := flags.Lookup("db").Value.String()
	actorID := flags.Lookup("actor").Value.String()
	service, closeStore, err := openService(dbPath)
	if err != nil {
		return err
	}
	defer closeStore()
	proxy, err := service.CreateProxy(context.Background(), admin.CreateProxyInput{ID: *id, UserID: *userID, ClientID: *clientID, Name: *name, Type: proxyType, EntryHost: *entryHost, EntryPort: *entryPort, TargetHost: *targetHost, TargetPort: *targetPort, Description: *description, ActorID: actorID})
	if err != nil {
		return err
	}
	fmt.Println(proxy.ID)
	return nil
}

func openService(dbPath string) (admin.Service, func(), error) {
	db, err := sqlite.Open(dbPath)
	if err != nil {
		return admin.Service{}, nil, err
	}
	return admin.Service{Store: db}, func() { _ = db.Close() }, nil
}
