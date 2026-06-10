package certmanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type SecretStore interface {
	Write(ctx context.Context, credentialID string, material string) (string, error)
	Read(ctx context.Context, secretRef string) (string, error)
	Delete(ctx context.Context, secretRef string) error
}

type FileSecretStore struct {
	Dir string
}

func (store FileSecretStore) Write(ctx context.Context, credentialID string, material string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(material) == "" {
		return "", errors.New("secret material is required")
	}
	name, err := safeSecretName(credentialID)
	if err != nil {
		return "", err
	}
	dir, err := store.dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	temp, err := os.CreateTemp(dir, name+"-*.tmp")
	if err != nil {
		return "", err
	}
	tempName := temp.Name()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempName)
		return "", err
	}
	if _, err := temp.WriteString(material); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempName)
		return "", err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempName)
		return "", err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return "", err
	}
	secretRef := name + ".secret"
	target := filepath.Join(dir, secretRef)
	if err := os.Rename(tempName, target); err != nil {
		_ = os.Remove(tempName)
		return "", err
	}
	syncDirectoryBestEffort(dir)
	return secretRef, nil
}

func (store FileSecretStore) Read(ctx context.Context, secretRef string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	path, err := store.resolve(secretRef)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errors.New("secret material is missing")
		}
		return "", err
	}
	material := strings.TrimSpace(string(content))
	if material == "" {
		return "", errors.New("secret material is empty")
	}
	return material, nil
}

func (store FileSecretStore) Delete(ctx context.Context, secretRef string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := store.resolve(secretRef)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func TokenFingerprint(material string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(material)))
	return hex.EncodeToString(sum[:])
}

func RejectOriginCAServiceKey(material string) error {
	if strings.HasPrefix(strings.TrimSpace(material), "v1.0-") {
		return errors.New("origin ca service keys are deprecated; use a cloudflare api token")
	}
	return nil
}

func (store FileSecretStore) resolve(secretRef string) (string, error) {
	if strings.TrimSpace(secretRef) == "" {
		return "", errors.New("secret ref is required")
	}
	if filepath.Base(secretRef) != secretRef || strings.Contains(secretRef, "..") {
		return "", errors.New("secret ref is invalid")
	}
	dir, err := store.dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, secretRef), nil
}

func (store FileSecretStore) dir() (string, error) {
	if strings.TrimSpace(store.Dir) == "" {
		return "", errors.New("secret store path is required")
	}
	return filepath.Abs(store.Dir)
}

func syncDirectoryBestEffort(dir string) {
	file, err := os.Open(dir)
	if err != nil {
		return
	}
	defer file.Close()
	_ = file.Sync()
}

func safeSecretName(credentialID string) (string, error) {
	credentialID = strings.TrimSpace(credentialID)
	if credentialID == "" {
		return "", errors.New("credential id is required")
	}
	var builder strings.Builder
	for _, r := range credentialID {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			builder.WriteRune(r)
			continue
		}
		return "", fmt.Errorf("credential id contains unsupported character %q", r)
	}
	return builder.String(), nil
}
