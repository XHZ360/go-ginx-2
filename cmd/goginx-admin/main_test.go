package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestRunCreatesResources(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "admin.db")

	if err := run([]string{"create-user", "-db", dbPath, "-id", "user-1", "-username", "alice"}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := run([]string{"create-client", "-db", dbPath, "-id", "client-1", "-user", "user-1", "-name", "home", "-credential", "secret"}); err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := run([]string{"create-tcp-proxy", "-db", dbPath, "-id", "proxy-1", "-user", "user-1", "-client", "client-1", "-name", "ssh", "-port", "10022", "-target-host", "127.0.0.1", "-target-port", "22"}); err != nil {
		t.Fatalf("create tcp proxy: %v", err)
	}

	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	found, err := db.Proxies().ByTCPEntryPort(context.Background(), 10022)
	if err != nil {
		t.Fatalf("lookup tcp proxy: %v", err)
	}
	if found.ID != "proxy-1" {
		t.Fatalf("unexpected proxy: %+v", found)
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	if err := run([]string{"unknown"}); err == nil {
		t.Fatal("expected unknown command error")
	}
}
