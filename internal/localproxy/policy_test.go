package localproxy

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type memoryAllowlistRepository struct {
	entries     []AllowlistEntry
	replaceErr  error
	replaceCall int
}

func (repo *memoryAllowlistRepository) List(context.Context) ([]AllowlistEntry, error) {
	return append([]AllowlistEntry(nil), repo.entries...), nil
}

func (repo *memoryAllowlistRepository) Replace(_ context.Context, entries []AllowlistEntry) error {
	repo.replaceCall++
	if repo.replaceErr != nil {
		return repo.replaceErr
	}
	repo.entries = append([]AllowlistEntry(nil), entries...)
	return nil
}

func TestPolicyNormalizesAndMatchesTargets(t *testing.T) {
	repo := &memoryAllowlistRepository{entries: []AllowlistEntry{
		{CIDR: "::ffff:127.0.0.1", PortStart: 8000, PortEnd: 9000},
		{CIDR: "2001:db8::/32"},
	}}
	policy, err := LoadPolicy(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	want := []AllowlistEntry{{CIDR: "127.0.0.1/32", PortStart: 8000, PortEnd: 9000}, {CIDR: "2001:db8::/32"}}
	if got := policy.Snapshot().Entries; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected normalized allowlist: %#v", got)
	}
	if err := policy.ValidateTarget(context.Background(), "127.0.0.1", 8080); err != nil {
		t.Fatalf("expected target allowed: %v", err)
	}
	for _, target := range []struct {
		host string
		port int
	}{{"localhost", 8080}, {"127.0.0.1", 7999}, {"10.0.0.1", 8080}, {"127.0.0.1", 0}} {
		if err := policy.ValidateTarget(context.Background(), target.host, target.port); !errors.Is(err, ErrTargetDenied) {
			t.Fatalf("expected %s:%d denied, got %v", target.host, target.port, err)
		}
	}
}

func TestPolicyReplacePublishesOnlyAfterPersistence(t *testing.T) {
	repo := &memoryAllowlistRepository{entries: DefaultAllowlist}
	policy, err := LoadPolicy(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	repo.replaceErr = errors.New("write failed")
	if err := policy.Replace(context.Background(), AllowlistInput{Entries: []AllowlistEntry{{CIDR: "10.0.0.0/8"}}}); err == nil {
		t.Fatal("expected replace failure")
	}
	if err := policy.ValidateTarget(context.Background(), "127.0.0.1", 80); err != nil {
		t.Fatalf("old snapshot should remain active: %v", err)
	}
	if err := policy.ValidateTarget(context.Background(), "10.0.0.1", 80); !errors.Is(err, ErrTargetDenied) {
		t.Fatalf("failed update must not become active: %v", err)
	}
}

func TestPolicyRejectsEmptyAndInvalidAllowlist(t *testing.T) {
	repo := &memoryAllowlistRepository{entries: DefaultAllowlist}
	policy, err := LoadPolicy(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []AllowlistInput{
		{},
		{Entries: []AllowlistEntry{{CIDR: "example.com"}}},
		{Entries: []AllowlistEntry{{CIDR: "127.0.0.1", PortStart: 9000, PortEnd: 8000}}},
	} {
		if err := policy.Replace(context.Background(), input); err == nil {
			t.Fatalf("expected invalid input rejected: %#v", input)
		}
	}
}
