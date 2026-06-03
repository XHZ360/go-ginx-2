package admintui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/simp-frp/go-ginx-2/internal/admin"
	"github.com/simp-frp/go-ginx-2/internal/adminquery"
	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/domain"
)

func TestRunRejectsNonInteractiveTerminal(t *testing.T) {
	oldInteractiveTTY := interactiveTTY
	interactiveTTY = func() bool { return false }
	t.Cleanup(func() { interactiveTTY = oldInteractiveTTY })

	err := Run(context.Background(), RunOptions{Backend: &tuiFakeBackend{}, RequireTTY: true})
	if !errors.Is(err, ErrNonInteractiveTerminal) {
		t.Fatalf("expected ErrNonInteractiveTerminal, got %v", err)
	}
}

func TestMainMenuShowsTUIScope(t *testing.T) {
	m := newModel(context.Background(), &tuiFakeBackend{}, "actor-1")

	view := m.View()
	for _, want := range []string{"管理员设置", "用户管理", "客户端配置", "退出"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected main menu to contain %q:\n%s", want, view)
		}
	}
	for _, excluded := range []string{"证书", "审计", "配额", "远程登录"} {
		if strings.Contains(view, excluded) {
			t.Fatalf("main menu should not contain %q:\n%s", excluded, view)
		}
	}

	m.width = 50
	m.height = 10
	if got := m.View(); !strings.Contains(got, "终端窗口过小") {
		t.Fatalf("expected small terminal warning, got %q", got)
	}
}

func TestUserCreateFormValidatesRequiredFieldsAndDefaultRole(t *testing.T) {
	m := newModel(context.Background(), &tuiFakeBackend{}, "actor-1")
	m.openUserCreateForm()

	values := m.form.values()
	if values["role"] != string(domain.RoleUser) {
		t.Fatalf("expected default role user, got %q", values["role"])
	}
	updated, _ := m.submitForm()
	m = updated.(model)

	if m.screen != screenForm {
		t.Fatalf("expected to stay on form, got %v", m.screen)
	}
	if got := m.form.Errors["username"]; got == "" {
		t.Fatalf("expected username validation error, got %+v", m.form.Errors)
	}
}

func TestUserCreateConfirmsBeforeWriteAndSurfacesServiceError(t *testing.T) {
	backend := &tuiFakeBackend{
		createUserErr: contracterr.Validation("validation failed", map[string]string{"username": "already exists"}),
	}
	m := newModel(context.Background(), backend, "actor-1")
	m.openUserCreateForm()
	m.form.Fields[0].Input.SetValue("alice")

	updated, _ := m.submitForm()
	m = updated.(model)

	if m.screen != screenConfirm {
		t.Fatalf("expected confirmation before write, got %v", m.screen)
	}
	if backend.createUserCalls != 0 {
		t.Fatalf("expected no write before confirmation, got %d calls", backend.createUserCalls)
	}

	updated, _ = m.updateConfirm(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)

	if backend.createUserCalls != 1 {
		t.Fatalf("expected write after confirmation, got %d calls", backend.createUserCalls)
	}
	if !strings.Contains(m.notice, "validation failed") {
		t.Fatalf("expected service error notice, got %q", m.notice)
	}
}

func TestDeleteUserRequiresTypedIDConfirmation(t *testing.T) {
	backend := &tuiFakeBackend{}
	m := newModel(context.Background(), backend, "actor-1")
	item := adminquery.UserListItem{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}
	m.openUserDeleteConfirm(item)

	updated, _ := m.updateConfirm(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if backend.deleteUserCalls != 0 {
		t.Fatalf("delete should not run without typed ID")
	}
	if !strings.Contains(m.notice, "user-1") {
		t.Fatalf("expected strong confirmation notice, got %q", m.notice)
	}

	m.confirm.Typed = "user-1"
	updated, _ = m.updateConfirm(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if backend.deleteUserCalls != 1 {
		t.Fatalf("expected delete after typed ID, got %d calls", backend.deleteUserCalls)
	}
	if m.screen != screenResult || !strings.Contains(m.result.Body, "user-1") {
		t.Fatalf("expected delete result, screen=%v body=%q", m.screen, m.result.Body)
	}
}

func TestClientCredentialFlowDisplaysCredentialOnce(t *testing.T) {
	backend := &tuiFakeBackend{
		users: []adminquery.UserListItem{{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}},
		createClientResult: admin.CreateClientResult{
			Client:     domain.Client{ID: "client-1", UserID: "user-1", Name: "home"},
			Credential: "generated-secret",
		},
	}
	m := newModel(context.Background(), backend, "actor-1")
	m.openClientCredentialForm()
	m.form.Fields[1].Input.SetValue("home")

	updated, _ := m.submitForm()
	m = updated.(model)
	if m.screen != screenConfirm {
		t.Fatalf("expected confirmation before client write, got %v", m.screen)
	}

	updated, _ = m.updateConfirm(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if backend.createClientCalls != 1 {
		t.Fatalf("expected client create call, got %d", backend.createClientCalls)
	}
	if !strings.Contains(m.result.Body, "generated-secret") || !strings.Contains(m.result.Body, "只在当前结果页明文展示") {
		t.Fatalf("expected one-time credential result, got %q", m.result.Body)
	}
}

func TestClientJoinFlowValidatesCAFileAndDisplaysTokenOnce(t *testing.T) {
	missingCA := filepath.Join(t.TempDir(), "missing-ca.crt")
	backend := &tuiFakeBackend{
		users: []adminquery.UserListItem{{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}},
		joinDefaults: config.JoinServiceDefaults{
			EnrollmentURL:    "http://127.0.0.1:8080/api/client/enroll",
			ServerAddress:    "127.0.0.1:8443",
			ServerTLSAddress: "127.0.0.1:9443",
			ServerName:       "go-ginx-control.test",
			ServerCAFile:     missingCA,
		},
		createJoinResult: admin.CreateClientJoinResult{
			Client: domain.Client{ID: "client-join-1", UserID: "user-1", Name: "home"},
			Token:  "join-token",
		},
	}
	m := newModel(context.Background(), backend, "actor-1")
	m.openClientJoinForm()
	if m.form.Fields[2].Input.Value() != "http://127.0.0.1:8080/api/client/enroll" || m.form.Fields[3].Input.Value() != "127.0.0.1:8443" || m.form.Fields[4].Input.Value() != "127.0.0.1:9443" {
		t.Fatalf("expected join defaults in form, got enrollment=%q server=%q tls=%q", m.form.Fields[2].Input.Value(), m.form.Fields[3].Input.Value(), m.form.Fields[4].Input.Value())
	}
	m.form.Fields[1].Input.SetValue("home")
	m.form.Fields[2].Input.SetValue("http://edited.example.com:8080/api/client/enroll")
	m.form.Fields[3].Input.SetValue("edited.example.com:8443")
	m.form.Fields[4].Input.SetValue("edited.example.com:9443")

	updated, _ := m.submitForm()
	m = updated.(model)
	if m.screen != screenForm {
		t.Fatalf("expected missing CA to keep form open, got %v", m.screen)
	}
	if got := m.form.Errors["server_ca_file"]; got == "" {
		t.Fatalf("expected CA file validation error, got %+v", m.form.Errors)
	}

	caFile := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(caFile, []byte("ca-pem"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.form.Fields[6].Input.SetValue(caFile)
	updated, _ = m.submitForm()
	m = updated.(model)
	if m.screen != screenConfirm {
		t.Fatalf("expected confirmation after valid join form, got %v", m.screen)
	}

	updated, _ = m.updateConfirm(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if backend.createJoinCalls != 1 {
		t.Fatalf("expected join create call, got %d", backend.createJoinCalls)
	}
	if backend.createJoinInput.EnrollmentURL != "http://edited.example.com:8080/api/client/enroll" || backend.createJoinInput.ServerAddress != "edited.example.com:8443" || backend.createJoinInput.ServerTLSAddress != "edited.example.com:9443" {
		t.Fatalf("expected edited join defaults to be submitted, got %+v", backend.createJoinInput)
	}
	if !strings.Contains(m.result.Body, "join-token") || !strings.Contains(m.result.Body, `.\bin\goginx-admin client-join-command -client client-join-1`) || !strings.Contains(m.result.Body, "重复查看") {
		t.Fatalf("expected reviewable token result, got %q", m.result.Body)
	}
}

func TestClientJoinTokenCanBeReviewedFromClientAction(t *testing.T) {
	backend := &tuiFakeBackend{
		clients: []adminquery.ClientListItem{{ID: "client-1", UserID: "user-1", Name: "home", Status: domain.ClientOffline}},
		reviewJoinResult: admin.ReviewClientJoinTokenResult{
			Client:    domain.Client{ID: "client-1", UserID: "user-1", Name: "home"},
			Token:     "join-token",
			ExpiresAt: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		},
	}
	m := newModel(context.Background(), backend, "actor-1")
	m.openClientActionMenu(backend.clients[0])
	m.menu.Items[1].Action(&m)

	if backend.reviewJoinCalls != 1 {
		t.Fatalf("expected review call, got %d", backend.reviewJoinCalls)
	}
	if m.screen != screenResult || !strings.Contains(m.result.Body, "join-token") || !strings.Contains(m.result.Body, `.\bin\goginx-admin client-join-command -client client-1`) || !strings.Contains(m.result.Body, "只能被客户端消费一次") {
		t.Fatalf("expected reviewable join token result, screen=%v body=%q", m.screen, m.result.Body)
	}
}

type tuiFakeBackend struct {
	admins  []adminquery.UserListItem
	users   []adminquery.UserListItem
	clients []adminquery.ClientListItem

	joinDefaults config.JoinServiceDefaults

	createUserCalls int
	createUserErr   error

	deleteUserCalls int
	deleteUserErr   error

	createClientCalls  int
	createClientResult admin.CreateClientResult
	createClientErr    error

	createJoinCalls  int
	createJoinInput  admin.CreateClientJoinInput
	createJoinResult admin.CreateClientJoinResult
	createJoinErr    error

	reviewJoinCalls  int
	reviewJoinResult admin.ReviewClientJoinTokenResult
	reviewJoinErr    error
}

func (b *tuiFakeBackend) Admins(context.Context) ([]adminquery.UserListItem, error) {
	return append([]adminquery.UserListItem(nil), b.admins...), nil
}

func (b *tuiFakeBackend) Users(context.Context) ([]adminquery.UserListItem, error) {
	return append([]adminquery.UserListItem(nil), b.users...), nil
}

func (b *tuiFakeBackend) Clients(context.Context) ([]adminquery.ClientListItem, error) {
	return append([]adminquery.ClientListItem(nil), b.clients...), nil
}

func (b *tuiFakeBackend) CreateUser(_ context.Context, input admin.CreateUserInput) (domain.User, error) {
	b.createUserCalls++
	if b.createUserErr != nil {
		return domain.User{}, b.createUserErr
	}
	role := input.Role
	if role == "" {
		role = domain.RoleUser
	}
	return domain.User{ID: "created-user", Username: input.Username, Role: role, Status: domain.UserEnabled}, nil
}

func (b *tuiFakeBackend) SetUserPassword(context.Context, string, string, string) error {
	return nil
}

func (b *tuiFakeBackend) EnableUser(context.Context, string, string) error {
	return nil
}

func (b *tuiFakeBackend) DisableUser(context.Context, string, string) error {
	return nil
}

func (b *tuiFakeBackend) DeleteUser(context.Context, string, string) error {
	b.deleteUserCalls++
	return b.deleteUserErr
}

func (b *tuiFakeBackend) CreateClientWithCredential(_ context.Context, input admin.CreateClientInput) (admin.CreateClientResult, error) {
	b.createClientCalls++
	if b.createClientErr != nil {
		return admin.CreateClientResult{}, b.createClientErr
	}
	if b.createClientResult.Credential != "" {
		return b.createClientResult, nil
	}
	credential := input.Credential
	if credential == "" {
		credential = "generated-secret"
	}
	return admin.CreateClientResult{Client: domain.Client{ID: "created-client", UserID: input.UserID, Name: input.Name}, Credential: credential}, nil
}

func (b *tuiFakeBackend) CreateClientJoin(_ context.Context, input admin.CreateClientJoinInput) (admin.CreateClientJoinResult, error) {
	b.createJoinCalls++
	b.createJoinInput = input
	if b.createJoinErr != nil {
		return admin.CreateClientJoinResult{}, b.createJoinErr
	}
	if b.createJoinResult.Token != "" {
		return b.createJoinResult, nil
	}
	return admin.CreateClientJoinResult{Client: domain.Client{ID: "created-client"}, Token: "join-token"}, nil
}

func (b *tuiFakeBackend) ReviewClientJoinToken(context.Context, string, string) (admin.ReviewClientJoinTokenResult, error) {
	b.reviewJoinCalls++
	if b.reviewJoinErr != nil {
		return admin.ReviewClientJoinTokenResult{}, b.reviewJoinErr
	}
	if b.reviewJoinResult.Token != "" {
		return b.reviewJoinResult, nil
	}
	return admin.ReviewClientJoinTokenResult{Client: domain.Client{ID: "client-1"}, Token: "join-token", ExpiresAt: time.Now().UTC().Add(time.Hour)}, nil
}

func (b *tuiFakeBackend) EnableClient(context.Context, string, string) error {
	return nil
}

func (b *tuiFakeBackend) DisableClient(context.Context, string, string) error {
	return nil
}

func (b *tuiFakeBackend) RotateClientCredential(context.Context, admin.RotateClientCredentialInput) (admin.RotateClientCredentialResult, error) {
	return admin.RotateClientCredentialResult{Client: domain.Client{ID: "client-1"}, Credential: "rotated-secret"}, nil
}

func (b *tuiFakeBackend) DeleteClient(context.Context, string, string) error {
	return nil
}

func (b *tuiFakeBackend) JoinDefaults() config.JoinServiceDefaults {
	if b.joinDefaults.EnrollmentURL != "" {
		return b.joinDefaults
	}
	return config.JoinServiceDefaults{
		EnrollmentURL:    "http://127.0.0.1:8080/api/client/enroll",
		ServerAddress:    "127.0.0.1:8443",
		ServerTLSAddress: "127.0.0.1:9443",
		ServerName:       "go-ginx-control.test",
		ServerCAFile:     filepath.Join(os.TempDir(), "ca.crt"),
	}
}

var _ Backend = (*tuiFakeBackend)(nil)

func TestParseUserID(t *testing.T) {
	got, err := parseUserID("alice (user-1)")
	if err != nil {
		t.Fatal(err)
	}
	if got != "user-1" {
		t.Fatalf("unexpected user id %q", got)
	}
}

func TestTTLValidation(t *testing.T) {
	backend := &tuiFakeBackend{
		users: []adminquery.UserListItem{{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}},
		joinDefaults: config.JoinServiceDefaults{
			EnrollmentURL:    "http://127.0.0.1:8080/api/client/enroll",
			ServerAddress:    "127.0.0.1:8443",
			ServerTLSAddress: "127.0.0.1:9443",
			ServerName:       "go-ginx-control.test",
			ServerCAFile:     filepath.Join(os.TempDir(), "ca.crt"),
		},
	}
	m := newModel(context.Background(), backend, "actor-1")
	m.openClientJoinForm()
	m.form.Fields[1].Input.SetValue("home")
	m.form.Fields[7].Input.SetValue("not-a-duration")

	updated, _ := m.submitForm()
	m = updated.(model)
	if got := m.form.Errors["ttl"]; got != "invalid duration" {
		t.Fatalf("expected ttl validation error, got %q", got)
	}
	if backend.createJoinCalls != 0 {
		t.Fatalf("expected no join create call, got %d", backend.createJoinCalls)
	}
	_ = time.Hour
}
