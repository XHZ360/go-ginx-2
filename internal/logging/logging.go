package logging

import (
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/config"
)

type RotatingFileOptions struct {
	Path        string
	Rotation    config.LogRotation
	Now         func() time.Time
	Diagnostics io.Writer
}

type RotatingFile struct {
	mu          sync.Mutex
	path        string
	dir         string
	baseName    string
	extension   string
	maxBytes    int64
	maxBackups  int
	retention   time.Duration
	compress    bool
	now         func() time.Time
	diagnostics io.Writer
	file        *os.File
	size        int64
}

func SetupStandardLogger(root string, name string, rotation config.LogRotation) (func(), error) {
	return SetupLogger(root, name, rotation, os.Stderr)
}

func SetupLogger(root string, name string, rotation config.LogRotation, stderr io.Writer) (func(), error) {
	if stderr == nil {
		stderr = io.Discard
	}
	logDir := filepath.Join(root, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return func() {}, fmt.Errorf("create log directory %s: %w", logDir, err)
	}
	writer, err := NewRotatingFile(RotatingFileOptions{
		Path:        filepath.Join(logDir, name),
		Rotation:    rotation,
		Diagnostics: stderr,
	})
	if err != nil {
		return func() {}, err
	}
	log.SetOutput(io.MultiWriter(stderr, writer))
	return func() { _ = writer.Close() }, nil
}

func NewRotatingFile(options RotatingFileOptions) (*RotatingFile, error) {
	if strings.TrimSpace(options.Path) == "" {
		return nil, fmt.Errorf("log path is required")
	}
	if err := options.Rotation.Validate(); err != nil {
		return nil, err
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	if options.Diagnostics == nil {
		options.Diagnostics = io.Discard
	}
	writer := &RotatingFile{
		path:        options.Path,
		dir:         filepath.Dir(options.Path),
		baseName:    strings.TrimSuffix(filepath.Base(options.Path), filepath.Ext(options.Path)),
		extension:   filepath.Ext(options.Path),
		maxBytes:    int64(options.Rotation.MaxSizeMB) * 1024 * 1024,
		maxBackups:  options.Rotation.MaxBackups,
		retention:   time.Duration(options.Rotation.RetentionDays) * 24 * time.Hour,
		compress:    options.Rotation.Compress,
		now:         options.Now,
		diagnostics: options.Diagnostics,
	}
	if err := os.MkdirAll(writer.dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory %s: %w", writer.dir, err)
	}
	if err := writer.openCurrentLocked(); err != nil {
		return nil, err
	}
	if writer.size >= writer.maxBytes && writer.size > 0 {
		if err := writer.rotateLocked(); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	writer.cleanupArchivesLocked()
	return writer, nil
}

func (writer *RotatingFile) Write(content []byte) (int, error) {
	if len(content) == 0 {
		return 0, nil
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()

	if writer.file == nil {
		return 0, fmt.Errorf("log file is closed")
	}
	if writer.size > 0 && writer.size+int64(len(content)) > writer.maxBytes {
		if err := writer.rotateLocked(); err != nil {
			return 0, err
		}
	}
	n, err := writer.file.Write(content)
	writer.size += int64(n)
	return n, err
}

func (writer *RotatingFile) Close() error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.file == nil {
		return nil
	}
	err := writer.file.Close()
	writer.file = nil
	writer.size = 0
	return err
}

func (writer *RotatingFile) openCurrentLocked() error {
	file, err := os.OpenFile(writer.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", writer.path, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("stat log file %s: %w", writer.path, err)
	}
	writer.file = file
	writer.size = info.Size()
	return nil
}

func (writer *RotatingFile) rotateLocked() error {
	if writer.file != nil {
		if err := writer.file.Close(); err != nil {
			return fmt.Errorf("close log file before rotation: %w", err)
		}
		writer.file = nil
		writer.size = 0
	}

	archivePath := writer.nextArchivePathLocked()
	if err := os.Rename(writer.path, archivePath); err != nil {
		if openErr := writer.openCurrentLocked(); openErr != nil {
			return fmt.Errorf("rotate log file %s: %w; reopen failed: %v", writer.path, err, openErr)
		}
		return fmt.Errorf("rotate log file %s: %w", writer.path, err)
	}
	if err := writer.openCurrentLocked(); err != nil {
		return err
	}
	if writer.compress {
		if err := compressFile(archivePath); err != nil {
			writer.writeDiagnostic("compress rotated log %s: %v\n", archivePath, err)
		}
	}
	writer.cleanupArchivesLocked()
	return nil
}

func (writer *RotatingFile) nextArchivePathLocked() string {
	timestamp := writer.now().UTC().Format("20060102-150405")
	for index := 0; ; index++ {
		suffix := ""
		if index > 0 {
			suffix = fmt.Sprintf("-%03d", index)
		}
		candidate := filepath.Join(writer.dir, writer.baseName+"-"+timestamp+suffix+writer.extension)
		_, plainErr := os.Stat(candidate)
		_, compressedErr := os.Stat(candidate + ".gz")
		if os.IsNotExist(plainErr) && os.IsNotExist(compressedErr) {
			return candidate
		}
	}
}

func (writer *RotatingFile) cleanupArchivesLocked() {
	archives, err := writer.archivesLocked()
	if err != nil {
		writer.writeDiagnostic("list rotated logs in %s: %v\n", writer.dir, err)
		return
	}
	cutoff := writer.now().UTC().Add(-writer.retention)
	remaining := make([]archiveFile, 0, len(archives))
	for _, archive := range archives {
		if archive.modTime.Before(cutoff) {
			if err := os.Remove(archive.path); err != nil {
				writer.writeDiagnostic("remove expired rotated log %s: %v\n", archive.path, err)
			}
			continue
		}
		remaining = append(remaining, archive)
	}
	if writer.maxBackups == 0 || len(remaining) <= writer.maxBackups {
		return
	}
	sort.Slice(remaining, func(i, j int) bool {
		return remaining[i].modTime.Before(remaining[j].modTime)
	})
	for _, archive := range remaining[:len(remaining)-writer.maxBackups] {
		if err := os.Remove(archive.path); err != nil {
			writer.writeDiagnostic("remove extra rotated log %s: %v\n", archive.path, err)
		}
	}
}

func (writer *RotatingFile) archivesLocked() ([]archiveFile, error) {
	entries, err := os.ReadDir(writer.dir)
	if err != nil {
		return nil, err
	}
	prefix := writer.baseName + "-"
	suffix := writer.extension
	compressedSuffix := suffix + ".gz"
	var archives []archiveFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if !strings.HasSuffix(name, suffix) && !strings.HasSuffix(name, compressedSuffix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		archives = append(archives, archiveFile{path: filepath.Join(writer.dir, name), modTime: info.ModTime()})
	}
	return archives, nil
}

func (writer *RotatingFile) writeDiagnostic(format string, args ...any) {
	if writer.diagnostics == nil {
		return
	}
	_, _ = fmt.Fprintf(writer.diagnostics, "log rotation: "+format, args...)
}

type archiveFile struct {
	path    string
	modTime time.Time
}

func compressFile(path string) error {
	source, err := os.Open(path)
	if err != nil {
		return err
	}

	compressedPath := path + ".gz"
	dest, err := os.OpenFile(compressedPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		_ = source.Close()
		return err
	}
	gzipWriter := gzip.NewWriter(dest)
	_, copyErr := io.Copy(gzipWriter, source)
	closeGzipErr := gzipWriter.Close()
	closeDestErr := dest.Close()
	closeSourceErr := source.Close()
	if copyErr != nil {
		_ = os.Remove(compressedPath)
		return copyErr
	}
	if closeGzipErr != nil {
		_ = os.Remove(compressedPath)
		return closeGzipErr
	}
	if closeDestErr != nil {
		_ = os.Remove(compressedPath)
		return closeDestErr
	}
	if closeSourceErr != nil {
		_ = os.Remove(compressedPath)
		return closeSourceErr
	}
	return os.Remove(path)
}
