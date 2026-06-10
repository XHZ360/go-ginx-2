package certmanager

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFileSecretStoreRoundTripAndPathSafety(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := FileSecretStore{Dir: dir}

	ref, err := store.Write(ctx, "cred_1-token", "  cf-token-secret  \n")
	if err != nil {
		t.Fatalf("write secret: %v", err)
	}
	if ref != "cred_1-token.secret" {
		t.Fatalf("unexpected secret ref %q", ref)
	}
	if strings.Contains(ref, "cf-token-secret") {
		t.Fatalf("secret ref leaked token material: %q", ref)
	}

	loaded, err := store.Read(ctx, ref)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	if loaded != "cf-token-secret" {
		t.Fatalf("unexpected secret material %q", loaded)
	}

	secretInfo, err := os.Stat(filepath.Join(dir, ref))
	if err != nil {
		t.Fatalf("stat secret file: %v", err)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat secret dir: %v", err)
	}
	if runtime.GOOS != "windows" {
		if secretInfo.Mode().Perm() != 0o600 {
			t.Fatalf("unexpected secret file mode %v", secretInfo.Mode().Perm())
		}
		if dirInfo.Mode().Perm() != 0o700 {
			t.Fatalf("unexpected secret dir mode %v", dirInfo.Mode().Perm())
		}
	}

	if _, err := store.Write(ctx, "../bad", "token"); err == nil {
		t.Fatal("expected unsafe credential id to be rejected")
	}
	if _, err := store.Read(ctx, "../cred_1-token.secret"); err == nil {
		t.Fatal("expected unsafe secret ref to be rejected")
	}
	if err := store.Delete(ctx, ref); err != nil {
		t.Fatalf("delete secret: %v", err)
	}
	if _, err := store.Read(ctx, ref); err == nil {
		t.Fatal("expected deleted secret to be missing")
	}
}

func TestOriginCASecretSafetyHelpers(t *testing.T) {
	first := TokenFingerprint(" cf-token-secret \n")
	second := TokenFingerprint("cf-token-secret")
	other := TokenFingerprint("other-token")
	if first == "" || first != second || first == other {
		t.Fatalf("unexpected token fingerprints first=%q second=%q other=%q", first, second, other)
	}
	if err := RejectOriginCAServiceKey("v1.0-legacy-service-key"); err == nil {
		t.Fatal("expected legacy service key to be rejected")
	}
	if err := RejectOriginCAServiceKey("cf-api-token"); err != nil {
		t.Fatalf("expected API token to be accepted: %v", err)
	}
}
