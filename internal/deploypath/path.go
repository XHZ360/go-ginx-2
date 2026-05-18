package deploypath

import (
	"fmt"
	"path/filepath"
	"strings"
)

const DefaultBinaryDir = "bin"

func Root(executablePath func() (string, error)) (string, error) {
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
	if filepath.Base(root) == DefaultBinaryDir {
		root = filepath.Dir(root)
	}
	return root, nil
}

func Resolve(root string, path string) string {
	if strings.TrimSpace(path) == "" || filepath.IsAbs(path) || path == ":memory:" || strings.HasPrefix(path, "file:") {
		return path
	}
	return filepath.Join(root, path)
}
