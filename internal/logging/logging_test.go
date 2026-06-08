package logging

import (
	"bytes"
	"compress/gzip"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/config"
)

func TestRotatingFileRotatesBySize(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 8, 15, 30, 0, 0, time.UTC)
	writer, err := NewRotatingFile(RotatingFileOptions{
		Path: filepath.Join(dir, "server.log"),
		Rotation: config.LogRotation{
			MaxSizeMB:     1,
			MaxBackups:    10,
			RetentionDays: 7,
		},
		Now: nowFunc(now),
	})
	if err != nil {
		t.Fatalf("create rotating file: %v", err)
	}
	defer writer.Close()

	first := bytes.Repeat([]byte("a"), 700*1024)
	second := bytes.Repeat([]byte("b"), 700*1024)
	if _, err := writer.Write(first); err != nil {
		t.Fatalf("write first log chunk: %v", err)
	}
	if _, err := writer.Write(second); err != nil {
		t.Fatalf("write second log chunk: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close rotating file: %v", err)
	}

	current, err := os.ReadFile(filepath.Join(dir, "server.log"))
	if err != nil {
		t.Fatalf("read current log: %v", err)
	}
	if !bytes.Equal(current, second) {
		t.Fatalf("current log should contain second chunk after rotation")
	}
	archive, err := os.ReadFile(filepath.Join(dir, "server-20260608-153000.log"))
	if err != nil {
		t.Fatalf("read archive log: %v", err)
	}
	if !bytes.Equal(archive, first) {
		t.Fatalf("archive log should contain first chunk")
	}
}

func TestRotatingFileCompressesArchives(t *testing.T) {
	dir := t.TempDir()
	writer, err := NewRotatingFile(RotatingFileOptions{
		Path: filepath.Join(dir, "client.log"),
		Rotation: config.LogRotation{
			MaxSizeMB:     1,
			MaxBackups:    10,
			RetentionDays: 7,
			Compress:      true,
		},
		Now: nowFunc(time.Date(2026, 6, 8, 15, 31, 0, 0, time.UTC)),
	})
	if err != nil {
		t.Fatalf("create rotating file: %v", err)
	}
	defer writer.Close()

	first := bytes.Repeat([]byte("a"), 700*1024)
	second := bytes.Repeat([]byte("b"), 700*1024)
	if _, err := writer.Write(first); err != nil {
		t.Fatalf("write first log chunk: %v", err)
	}
	if _, err := writer.Write(second); err != nil {
		t.Fatalf("write second log chunk: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close rotating file: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "client-20260608-153100.log")); !os.IsNotExist(err) {
		t.Fatalf("expected uncompressed archive to be removed, got err=%v", err)
	}
	compressed, err := os.Open(filepath.Join(dir, "client-20260608-153100.log.gz"))
	if err != nil {
		t.Fatalf("open compressed archive: %v", err)
	}
	defer compressed.Close()
	gzipReader, err := gzip.NewReader(compressed)
	if err != nil {
		t.Fatalf("open gzip archive: %v", err)
	}
	content, err := io.ReadAll(gzipReader)
	if err != nil {
		t.Fatalf("read gzip archive: %v", err)
	}
	if err := gzipReader.Close(); err != nil {
		t.Fatalf("close gzip archive: %v", err)
	}
	if !bytes.Equal(content, first) {
		t.Fatalf("compressed archive should contain first chunk")
	}
}

func TestRotatingFileCleansExpiredAndExtraArchives(t *testing.T) {
	dir := t.TempDir()
	currentPath := filepath.Join(dir, "server.log")
	if err := os.WriteFile(currentPath, []byte("current"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 8, 15, 32, 0, 0, time.UTC)
	archives := []struct {
		name string
		age  time.Duration
	}{
		{name: "server-20260530-153200.log", age: 9 * 24 * time.Hour},
		{name: "server-20260605-153200.log", age: 3 * 24 * time.Hour},
		{name: "server-20260606-153200.log.gz", age: 2 * 24 * time.Hour},
		{name: "server-20260607-153200.log", age: 24 * time.Hour},
	}
	for _, archive := range archives {
		path := filepath.Join(dir, archive.name)
		if err := os.WriteFile(path, []byte(archive.name), 0o644); err != nil {
			t.Fatal(err)
		}
		modTime := now.Add(-archive.age)
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatal(err)
		}
	}

	writer, err := NewRotatingFile(RotatingFileOptions{
		Path: currentPath,
		Rotation: config.LogRotation{
			MaxSizeMB:     1,
			MaxBackups:    2,
			RetentionDays: 7,
		},
		Now: nowFunc(now),
	})
	if err != nil {
		t.Fatalf("create rotating file: %v", err)
	}
	defer writer.Close()

	for _, removed := range []string{"server-20260530-153200.log", "server-20260605-153200.log"} {
		if _, err := os.Stat(filepath.Join(dir, removed)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, err=%v", removed, err)
		}
	}
	for _, kept := range []string{"server.log", "server-20260606-153200.log.gz", "server-20260607-153200.log"} {
		if _, err := os.Stat(filepath.Join(dir, kept)); err != nil {
			t.Fatalf("expected %s to remain: %v", kept, err)
		}
	}
}

func TestSetupLoggerWritesStderrAndRotatingFile(t *testing.T) {
	root := t.TempDir()
	var stderr bytes.Buffer
	closeLog, err := SetupLogger(root, "client.log", config.LogRotation{
		MaxSizeMB:     1,
		MaxBackups:    10,
		RetentionDays: 7,
	}, &stderr)
	if err != nil {
		t.Fatalf("setup logger: %v", err)
	}
	defer log.SetOutput(os.Stderr)

	log.Print("hello stderr and file")
	closeLog()

	if !strings.Contains(stderr.String(), "hello stderr and file") {
		t.Fatalf("stderr did not receive log output: %q", stderr.String())
	}
	content, err := os.ReadFile(filepath.Join(root, "logs", "client.log"))
	if err != nil {
		t.Fatalf("read client log: %v", err)
	}
	if !strings.Contains(string(content), "hello stderr and file") {
		t.Fatalf("file did not receive log output: %q", string(content))
	}
}

func nowFunc(now time.Time) func() time.Time {
	return func() time.Time { return now }
}
