package admintui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/simp-frp/go-ginx-2/internal/admin"
	"github.com/simp-frp/go-ginx-2/internal/adminquery"
	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/domain"
)

var ErrNonInteractiveTerminal = errors.New("tui requires an interactive terminal; use the non-interactive admin commands instead")

type Backend interface {
	Admins(context.Context) ([]adminquery.UserListItem, error)
	Users(context.Context) ([]adminquery.UserListItem, error)
	Clients(context.Context) ([]adminquery.ClientListItem, error)

	CreateUser(context.Context, admin.CreateUserInput) (domain.User, error)
	SetUserPassword(context.Context, string, string, string) error
	EnableUser(context.Context, string, string) error
	DisableUser(context.Context, string, string) error
	DeleteUser(context.Context, string, string) error

	CreateClientWithCredential(context.Context, admin.CreateClientInput) (admin.CreateClientResult, error)
	CreateClientJoin(context.Context, admin.CreateClientJoinInput) (admin.CreateClientJoinResult, error)
	ReviewClientJoinToken(context.Context, string, string) (admin.ReviewClientJoinTokenResult, error)
	EnableClient(context.Context, string, string) error
	DisableClient(context.Context, string, string) error
	RotateClientCredential(context.Context, admin.RotateClientCredentialInput) (admin.RotateClientCredentialResult, error)
	DeleteClient(context.Context, string, string) error

	JoinDefaults() config.JoinServiceDefaults
}

type LocalBackend struct {
	Commands          admin.CommandFacades
	Queries           adminquery.Service
	JoinDefaultsValue config.JoinServiceDefaults
}

func (b LocalBackend) Admins(ctx context.Context) ([]adminquery.UserListItem, error) {
	items, err := b.Users(ctx)
	if err != nil {
		return nil, err
	}
	admins := make([]adminquery.UserListItem, 0)
	for _, item := range items {
		if item.Role == domain.RoleAdmin {
			admins = append(admins, item)
		}
	}
	sort.Slice(admins, func(i, j int) bool {
		if admins[i].Username == admins[j].Username {
			return admins[i].ID < admins[j].ID
		}
		return admins[i].Username < admins[j].Username
	})
	return admins, nil
}

func (b LocalBackend) Users(ctx context.Context) ([]adminquery.UserListItem, error) {
	page, err := b.Queries.ListUsers(ctx, adminquery.UserListInput{Page: adminquery.PageInput{Page: 1, PageSize: 1000000}})
	if err != nil {
		return nil, err
	}
	return append([]adminquery.UserListItem(nil), page.Items...), nil
}

func (b LocalBackend) Clients(ctx context.Context) ([]adminquery.ClientListItem, error) {
	page, err := b.Queries.ListClients(ctx, adminquery.ClientListInput{Page: adminquery.PageInput{Page: 1, PageSize: 1000000}})
	if err != nil {
		return nil, err
	}
	return append([]adminquery.ClientListItem(nil), page.Items...), nil
}

func (b LocalBackend) CreateUser(ctx context.Context, input admin.CreateUserInput) (domain.User, error) {
	return b.Commands.CreateUser(ctx, input)
}

func (b LocalBackend) SetUserPassword(ctx context.Context, userID, password, actorID string) error {
	return b.Commands.SetUserPassword(ctx, userID, password, actorID)
}

func (b LocalBackend) EnableUser(ctx context.Context, userID, actorID string) error {
	return b.Commands.EnableUser(ctx, userID, actorID)
}

func (b LocalBackend) DisableUser(ctx context.Context, userID, actorID string) error {
	return b.Commands.DisableUser(ctx, userID, actorID)
}

func (b LocalBackend) DeleteUser(ctx context.Context, userID, actorID string) error {
	return b.Commands.DeleteUser(ctx, userID, actorID)
}

func (b LocalBackend) CreateClientWithCredential(ctx context.Context, input admin.CreateClientInput) (admin.CreateClientResult, error) {
	return b.Commands.CreateClientWithCredential(ctx, input)
}

func (b LocalBackend) CreateClientJoin(ctx context.Context, input admin.CreateClientJoinInput) (admin.CreateClientJoinResult, error) {
	return b.Commands.CreateClientJoin(ctx, input)
}

func (b LocalBackend) ReviewClientJoinToken(ctx context.Context, clientID, actorID string) (admin.ReviewClientJoinTokenResult, error) {
	return b.Commands.ReviewClientJoinToken(ctx, clientID, actorID)
}

func (b LocalBackend) EnableClient(ctx context.Context, clientID, actorID string) error {
	return b.Commands.EnableClient(ctx, clientID, actorID)
}

func (b LocalBackend) DisableClient(ctx context.Context, clientID, actorID string) error {
	return b.Commands.DisableClient(ctx, clientID, actorID)
}

func (b LocalBackend) RotateClientCredential(ctx context.Context, input admin.RotateClientCredentialInput) (admin.RotateClientCredentialResult, error) {
	return b.Commands.RotateClientCredential(ctx, input)
}

func (b LocalBackend) DeleteClient(ctx context.Context, clientID, actorID string) error {
	return b.Commands.DeleteClient(ctx, clientID, actorID)
}

func (b LocalBackend) JoinDefaults() config.JoinServiceDefaults {
	return b.JoinDefaultsValue
}

type RunOptions struct {
	Backend                   Backend
	ActorID                   string
	ClientJoinCommandTemplate string
	Input                     io.Reader
	Output                    io.Writer
	RequireTTY                bool
}

func Run(ctx context.Context, opts RunOptions) error {
	if opts.Backend == nil {
		return errors.New("backend is required")
	}
	if opts.RequireTTY && !interactiveTTY() {
		return ErrNonInteractiveTerminal
	}
	model := newModel(ctx, opts.Backend, opts.ActorID)
	if strings.TrimSpace(opts.ClientJoinCommandTemplate) != "" {
		model.clientJoinCommandTemplate = opts.ClientJoinCommandTemplate
	}
	programOptions := []tea.ProgramOption{tea.WithAltScreen()}
	if opts.Input != nil {
		programOptions = append(programOptions, tea.WithInput(opts.Input))
	}
	if opts.Output != nil {
		programOptions = append(programOptions, tea.WithOutput(opts.Output))
	}
	p := tea.NewProgram(model, programOptions...)
	_, err := p.Run()
	return err
}

var interactiveTTY = func() bool {
	return (isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())) &&
		(isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()))
}

type screenKind int

const (
	screenMenu screenKind = iota
	screenForm
	screenConfirm
	screenResult
)

type model struct {
	ctx                       context.Context
	backend                   Backend
	actorID                   string
	screen                    screenKind
	notice                    string
	loading                   bool
	width                     int
	height                    int
	menu                      menuState
	form                      formState
	confirm                   confirmState
	result                    resultState
	clientJoinCommandTemplate string
	quit                      bool
}

func newModel(ctx context.Context, backend Backend, actorID string) model {
	m := model{
		ctx:                       ctx,
		backend:                   backend,
		actorID:                   actorID,
		screen:                    screenMenu,
		menu:                      mainMenu(),
		clientJoinCommandTemplate: `.\bin\goginx-admin client-join-command -client {client}`,
	}
	return m
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		}
	}

	switch m.screen {
	case screenMenu:
		return m.updateMenu(msg)
	case screenForm:
		return m.updateForm(msg)
	case screenConfirm:
		return m.updateConfirm(msg)
	case screenResult:
		return m.updateResult(msg)
	default:
		return m, nil
	}
}

func (m model) View() string {
	if (m.width > 0 && m.width < 60) || (m.height > 0 && m.height < 12) {
		return "终端窗口过小，请至少调整到 60x12 后继续。"
	}
	if m.loading {
		return "Loading..."
	}
	var b strings.Builder
	if strings.TrimSpace(m.notice) != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(m.notice))
		b.WriteString("\n\n")
	}
	switch m.screen {
	case screenMenu:
		b.WriteString(m.menuView())
	case screenForm:
		b.WriteString(m.formView())
	case screenConfirm:
		b.WriteString(m.confirmView())
	case screenResult:
		b.WriteString(m.resultView())
	}
	return b.String()
}

func (m model) updateMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			return m.back()
		case tea.KeyUp, tea.KeyLeft:
			if len(m.menu.Items) > 0 {
				m.menu.Selected--
				if m.menu.Selected < 0 {
					m.menu.Selected = len(m.menu.Items) - 1
				}
			}
		case tea.KeyDown, tea.KeyRight, tea.KeyTab:
			if len(m.menu.Items) > 0 {
				m.menu.Selected++
				if m.menu.Selected >= len(m.menu.Items) {
					m.menu.Selected = 0
				}
			}
		case tea.KeyEnter:
			if len(m.menu.Items) > 0 {
				m.menu.Items[m.menu.Selected].Action(&m)
				if m.quit {
					return m, tea.Quit
				}
				return m, nil
			}
		}
	}
	return m, nil
}

func (m model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.form.updateInput(msg)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp:
			m.form.move(-1)
		case tea.KeyDown, tea.KeyTab:
			m.form.move(1)
		case tea.KeyShiftTab:
			m.form.move(-1)
		case tea.KeyLeft:
			m.form.adjustSelect(-1)
		case tea.KeyRight:
			m.form.adjustSelect(1)
		case tea.KeyEnter:
			if m.form.current().Kind == fieldSelect {
				if m.form.isLast() {
					return m.submitForm()
				}
				m.form.move(1)
			} else if m.form.isLast() {
				return m.submitForm()
			} else {
				m.form.move(1)
			}
		case tea.KeyEsc:
			if m.form.Cancel != nil {
				m.form.Cancel(&m)
			}
		}
	}
	return m, cmd
}

func (m model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			if m.confirm.RequireText != "" {
				if strings.TrimSpace(m.confirm.Typed) != m.confirm.RequireText {
					m.notice = fmt.Sprintf("请输入 %s 以确认删除", m.confirm.RequireText)
					return m, nil
				}
			}
			if m.confirm.Confirm != nil {
				if err := m.confirm.Confirm(&m); err != nil {
					m.notice = friendlyError(err)
				}
			}
		case tea.KeyBackspace, tea.KeyDelete:
			if len(m.confirm.Typed) > 0 {
				m.confirm.Typed = m.confirm.Typed[:len(m.confirm.Typed)-1]
			}
		case tea.KeyRunes:
			m.confirm.Typed += msg.String()
		case tea.KeyEsc:
			if m.confirm.Cancel != nil {
				m.confirm.Cancel(&m)
			}
		}
	}
	return m, nil
}

func (m model) updateResult(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyEnter, tea.KeyEsc:
			if m.result.Back != nil {
				m.result.Back(&m)
			}
		}
	}
	return m, nil
}

func (m model) back() (tea.Model, tea.Cmd) {
	if m.menu.Back != nil {
		m.menu.Back(&m)
		return m, nil
	}
	return m, tea.Quit
}

func (m model) submitForm() (tea.Model, tea.Cmd) {
	values := m.form.values()
	if errs := m.form.validate(values); len(errs) > 0 {
		m.form.Errors = errs
		return m, nil
	}
	if m.form.Submit != nil {
		if err := m.form.Submit(&m, values); err != nil {
			m.notice = friendlyError(err)
			if fields := validationFields(err); len(fields) > 0 {
				m.form.Errors = fields
			}
			return m, nil
		}
	}
	return m, nil
}

func (m *model) setMenu(menu menuState) {
	menu.Selected = 0
	m.menu = menu
	m.screen = screenMenu
}

func (m *model) openForm(form formState) {
	form.init()
	m.form = form
	m.notice = ""
	m.screen = screenForm
}

func (m *model) openConfirm(confirm confirmState) {
	confirm.Typed = ""
	m.confirm = confirm
	m.notice = ""
	m.screen = screenConfirm
}

func (m *model) openResult(result resultState) {
	m.result = result
	m.notice = ""
	m.screen = screenResult
}

func (m *model) setNotice(message string) {
	m.notice = strings.TrimSpace(message)
}

type menuState struct {
	Title     string
	Subtitle  string
	Items     []menuItem
	Selected  int
	Back      func(*model)
	EmptyText string
}

type menuItem struct {
	Label  string
	Desc   string
	Action func(*model)
}

func (m menuState) view() string {
	var b strings.Builder
	title := lipgloss.NewStyle().Bold(true).Render(m.Title)
	b.WriteString(title)
	b.WriteString("\n")
	if strings.TrimSpace(m.Subtitle) != "" {
		b.WriteString(m.Subtitle)
		b.WriteString("\n\n")
	} else {
		b.WriteString("\n")
	}
	if len(m.Items) == 0 {
		empty := m.EmptyText
		if strings.TrimSpace(empty) == "" {
			empty = "暂无可选项"
		}
		b.WriteString(empty)
		b.WriteString("\n")
		return b.String()
	}
	for i, item := range m.Items {
		prefix := "  "
		if i == m.Selected {
			prefix = "> "
		}
		b.WriteString(prefix)
		b.WriteString(item.Label)
		if strings.TrimSpace(item.Desc) != "" {
			b.WriteString(" - ")
			b.WriteString(item.Desc)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n↑↓ 选择  Enter 确认  Esc 返回\n")
	return b.String()
}

type resultState struct {
	Title string
	Body  string
	Back  func(*model)
}

func (r resultState) view() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(r.Title))
	b.WriteString("\n\n")
	b.WriteString(r.Body)
	b.WriteString("\n\nEnter 返回")
	return b.String()
}

type confirmState struct {
	Title       string
	Body        string
	RequireText string
	Typed       string
	Confirm     func(*model) error
	Cancel      func(*model)
}

func (c confirmState) view() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(c.Title))
	b.WriteString("\n\n")
	b.WriteString(c.Body)
	b.WriteString("\n")
	if strings.TrimSpace(c.RequireText) != "" {
		b.WriteString("输入 ")
		b.WriteString(c.RequireText)
		b.WriteString(" 以确认: ")
		b.WriteString(c.Typed)
		b.WriteString("\n")
		b.WriteString("Enter 确认  Esc 取消\n")
	} else {
		b.WriteString("Enter 确认  Esc 取消\n")
	}
	return b.String()
}

type fieldKind int

const (
	fieldText fieldKind = iota
	fieldPassword
	fieldSelect
)

type formState struct {
	Title      string
	Body       string
	Fields     []formField
	Selected   int
	Submit     func(*model, map[string]string) error
	Cancel     func(*model)
	Errors     map[string]string
	GeneralErr string
}

type formField struct {
	Kind     fieldKind
	Key      string
	Label    string
	Input    textinput.Model
	Options  []string
	Choice   int
	Help     string
	Required bool
}

func newTextField(key, label, value string, secret bool) formField {
	input := textinput.New()
	input.Placeholder = label
	input.SetValue(value)
	input.Width = 42
	input.CharLimit = 256
	if secret {
		input.EchoMode = textinput.EchoPassword
		input.EchoCharacter = '•'
	}
	kind := fieldText
	if secret {
		kind = fieldPassword
	}
	return formField{Kind: kind, Key: key, Label: label, Input: input, Required: true}
}

func newSelectField(key, label string, options []string, defaultValue string) formField {
	field := formField{Kind: fieldSelect, Key: key, Label: label, Options: append([]string(nil), options...), Required: true}
	for i, option := range field.Options {
		if option == defaultValue {
			field.Choice = i
			break
		}
	}
	return field
}

func optionalField(field formField) formField {
	field.Required = false
	return field
}

func (f *formState) init() {
	f.Selected = 0
	for i := range f.Fields {
		if f.Fields[i].Kind == fieldText || f.Fields[i].Kind == fieldPassword {
			f.Fields[i].Input.Focus()
		}
		if i != 0 {
			f.Fields[i].Input.Blur()
		}
	}
	f.focus(0)
	f.Errors = map[string]string{}
	f.GeneralErr = ""
}

func (f *formState) focus(index int) {
	if len(f.Fields) == 0 {
		return
	}
	if index < 0 {
		index = 0
	}
	if index >= len(f.Fields) {
		index = len(f.Fields) - 1
	}
	for i := range f.Fields {
		if f.Fields[i].Kind == fieldText || f.Fields[i].Kind == fieldPassword {
			if i == index {
				f.Fields[i].Input.Focus()
			} else {
				f.Fields[i].Input.Blur()
			}
		}
	}
	f.Selected = index
}

func (f *formState) move(delta int) {
	if len(f.Fields) == 0 {
		return
	}
	f.focus(f.Selected + delta)
}

func (f *formState) adjustSelect(delta int) {
	field := f.current()
	if field.Kind != fieldSelect || len(field.Options) == 0 {
		return
	}
	field.Choice = (field.Choice + delta) % len(field.Options)
	if field.Choice < 0 {
		field.Choice += len(field.Options)
	}
}

func (f *formState) current() *formField {
	if len(f.Fields) == 0 {
		return &formField{}
	}
	return &f.Fields[f.Selected]
}

func (f *formState) isLast() bool {
	return len(f.Fields) == 0 || f.Selected == len(f.Fields)-1
}

func (f *formState) updateInput(msg tea.Msg) tea.Cmd {
	if len(f.Fields) == 0 {
		return nil
	}
	field := &f.Fields[f.Selected]
	switch field.Kind {
	case fieldText, fieldPassword:
		updated, cmd := field.Input.Update(msg)
		field.Input = updated
		return cmd
	default:
		return nil
	}
}

func (f *formState) values() map[string]string {
	values := make(map[string]string, len(f.Fields))
	for _, field := range f.Fields {
		switch field.Kind {
		case fieldText, fieldPassword:
			values[field.Key] = strings.TrimSpace(field.Input.Value())
		case fieldSelect:
			if len(field.Options) > 0 {
				values[field.Key] = field.Options[field.Choice]
			}
		}
	}
	return values
}

func (f *formState) validate(values map[string]string) map[string]string {
	fields := make(map[string]string)
	for _, field := range f.Fields {
		if field.Required && strings.TrimSpace(values[field.Key]) == "" {
			fields[field.Key] = fmt.Sprintf("%s is required", field.Label)
			continue
		}
		if field.Input.Err != nil {
			fields[field.Key] = field.Input.Err.Error()
		}
	}
	return fields
}

func (f formState) view() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(f.Title))
	b.WriteString("\n")
	if strings.TrimSpace(f.Body) != "" {
		b.WriteString(f.Body)
		b.WriteString("\n\n")
	} else {
		b.WriteString("\n")
	}
	for i, field := range f.Fields {
		prefix := "  "
		if i == f.Selected {
			prefix = "> "
		}
		b.WriteString(prefix)
		b.WriteString(field.Label)
		b.WriteString(": ")
		switch field.Kind {
		case fieldText, fieldPassword:
			b.WriteString(field.Input.View())
		case fieldSelect:
			if len(field.Options) > 0 {
				b.WriteString("[")
				b.WriteString(field.Options[field.Choice])
				b.WriteString("]")
			}
		}
		if err := f.Errors[field.Key]; strings.TrimSpace(err) != "" {
			b.WriteString("  ")
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(err))
		}
		b.WriteString("\n")
		if strings.TrimSpace(field.Help) != "" {
			b.WriteString("    ")
			b.WriteString(field.Help)
			b.WriteString("\n")
		}
	}
	if strings.TrimSpace(f.GeneralErr) != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(f.GeneralErr))
		b.WriteString("\n")
	}
	b.WriteString("\nEnter 下一项/提交  Tab 下一项  Esc 返回\n")
	return b.String()
}

func (m model) menuView() string {
	return m.menu.view()
}

func (m model) formView() string {
	return m.form.view()
}

func (m model) confirmView() string {
	return m.confirm.view()
}

func (m model) resultView() string {
	return m.result.view()
}

func (m model) clientJoinCommand(clientID string) string {
	return strings.ReplaceAll(m.clientJoinCommandTemplate, "{client}", clientID)
}

func mainMenu() menuState {
	return menuState{
		Title:     "goginx-admin TUI",
		Subtitle:  "本地运维入口，优先使用选项和强校验。",
		EmptyText: "没有可用菜单项。",
		Items: []menuItem{
			{Label: "管理员设置", Desc: "初始化、改密、启用/禁用", Action: func(m *model) { m.openAdminMenu() }},
			{Label: "用户管理", Desc: "创建、启用/禁用、删除", Action: func(m *model) { m.openUserMenu() }},
			{Label: "客户端配置", Desc: "快速向导、凭据、启用/禁用、轮换、删除", Action: func(m *model) { m.openClientMenu() }},
			{Label: "退出", Desc: "关闭 TUI", Action: func(m *model) { m.quit = true }},
		},
	}
}

func (m *model) openAdminMenu() {
	m.setMenu(menuState{
		Title:    "管理员设置",
		Subtitle: "支持快速初始化管理员、更新密码和启用已存在管理员。",
		Back:     func(m *model) { m.setMenu(mainMenu()) },
		Items: []menuItem{
			{Label: "创建管理员", Desc: "首次初始化或新增管理员", Action: func(m *model) { m.openAdminCreateForm() }},
			{Label: "管理现有管理员", Desc: "从现有管理员列表选择", Action: func(m *model) { m.openAdminListMenu() }},
			{Label: "返回", Desc: "回到主菜单", Action: func(m *model) { m.setMenu(mainMenu()) }},
		},
	})
}

func (m *model) openUserMenu() {
	m.setMenu(menuState{
		Title:    "用户管理",
		Subtitle: "创建用户，或对现有用户进行启用、禁用和受保护删除。",
		Back:     func(m *model) { m.setMenu(mainMenu()) },
		Items: []menuItem{
			{Label: "创建用户", Desc: "通过角色选项创建新用户", Action: func(m *model) { m.openUserCreateForm() }},
			{Label: "管理现有用户", Desc: "从现有用户列表选择", Action: func(m *model) { m.openUserListMenu() }},
			{Label: "返回", Desc: "回到主菜单", Action: func(m *model) { m.setMenu(mainMenu()) }},
		},
	})
}

func (m *model) openClientMenu() {
	m.setMenu(menuState{
		Title:    "客户端配置",
		Subtitle: "优先使用用户选择和默认值完成客户端与加入令牌配置。",
		Back:     func(m *model) { m.setMenu(mainMenu()) },
		Items: []menuItem{
			{Label: "快速创建客户端", Desc: "选择用户、名称和 join 参数", Action: func(m *model) { m.openClientJoinForm() }},
			{Label: "仅创建客户端凭据", Desc: "生成或手动填写 credential", Action: func(m *model) { m.openClientCredentialForm() }},
			{Label: "查看 join token", Desc: "选择客户端查看未使用 token", Action: func(m *model) { m.openClientReviewJoinTokenListMenu() }},
			{Label: "管理现有客户端", Desc: "从现有客户端列表选择", Action: func(m *model) { m.openClientListMenu() }},
			{Label: "返回", Desc: "回到主菜单", Action: func(m *model) { m.setMenu(mainMenu()) }},
		},
	})
}

func (m *model) openAdminCreateForm() {
	m.openForm(formState{
		Title: "创建管理员",
		Body:  "用户名和密码需要通过强校验，角色固定为 admin。",
		Fields: []formField{
			newTextField("username", "用户名", "", false),
			newTextField("password", "密码", "", true),
			newTextField("confirm", "确认密码", "", true),
		},
		Submit: func(m *model, values map[string]string) error {
			if values["password"] != values["confirm"] {
				return contracterr.Validation("validation failed", map[string]string{"confirm": "confirmation does not match password"})
			}
			submitted := copyValues(values)
			m.openConfirm(confirmState{
				Title: "确认创建管理员",
				Body:  fmt.Sprintf("用户名: %s\n角色: %s", submitted["username"], domain.RoleAdmin),
				Confirm: func(m *model) error {
					user, err := m.backend.CreateUser(m.ctx, admin.CreateUserInput{
						Username: submitted["username"],
						Password: submitted["password"],
						Role:     domain.RoleAdmin,
						ActorID:  m.actorID,
					})
					if err != nil {
						return err
					}
					m.openResult(resultState{
						Title: "管理员已创建",
						Body:  fmt.Sprintf("用户名: %s\n用户 ID: %s\n状态: %s", user.Username, user.ID, user.Status),
						Back:  func(m *model) { m.openAdminMenu() },
					})
					return nil
				},
				Cancel: func(m *model) { m.openAdminCreateForm() },
			})
			return nil
		},
		Cancel: func(m *model) { m.openAdminMenu() },
	})
}

func (m *model) openAdminListMenu() {
	admins, err := m.backend.Admins(m.ctx)
	if err != nil {
		m.setNotice(err.Error())
		m.openAdminMenu()
		return
	}
	items := make([]menuItem, 0, len(admins)+1)
	if len(admins) == 0 {
		items = append(items, menuItem{Label: "尚未创建管理员", Desc: "按回车创建首个管理员", Action: func(m *model) { m.openAdminCreateForm() }})
	} else {
		for _, adminItem := range admins {
			item := adminItem
			items = append(items, menuItem{
				Label:  fmt.Sprintf("%s (%s)", item.Username, item.ID),
				Desc:   fmt.Sprintf("角色: %s  状态: %s", item.Role, item.Status),
				Action: func(m *model) { m.openAdminActionMenu(item) },
			})
		}
	}
	items = append(items, menuItem{Label: "返回", Desc: "回到管理员菜单", Action: func(m *model) { m.openAdminMenu() }})
	m.setMenu(menuState{
		Title:    "现有管理员",
		Subtitle: "选择一个管理员继续操作。",
		Back:     func(m *model) { m.openAdminMenu() },
		Items:    items,
	})
}

func (m *model) openAdminActionMenu(item adminquery.UserListItem) {
	items := []menuItem{
		{Label: "修改密码", Desc: "更新该管理员的登录密码", Action: func(m *model) { m.openSetPasswordForm(item) }},
	}
	if item.Status == domain.UserDisabled {
		items = append(items, menuItem{Label: "启用", Desc: "恢复该管理员登录", Action: func(m *model) { m.openUserToggleConfirm(item, true) }})
	} else {
		items = append(items, menuItem{Label: "禁用", Desc: "暂停该管理员登录", Action: func(m *model) { m.openUserToggleConfirm(item, false) }})
	}
	items = append(items, menuItem{Label: "返回", Desc: "回到管理员列表", Action: func(m *model) { m.openAdminListMenu() }})
	m.setMenu(menuState{
		Title:    fmt.Sprintf("管理员: %s", item.Username),
		Subtitle: fmt.Sprintf("用户 ID: %s  状态: %s", item.ID, item.Status),
		Back:     func(m *model) { m.openAdminListMenu() },
		Items:    items,
	})
}

func (m *model) openSetPasswordForm(item adminquery.UserListItem) {
	m.openForm(formState{
		Title: fmt.Sprintf("更新管理员密码 - %s", item.Username),
		Body:  fmt.Sprintf("用户 ID: %s", item.ID),
		Fields: []formField{
			newTextField("password", "新密码", "", true),
			newTextField("confirm", "确认密码", "", true),
		},
		Submit: func(m *model, values map[string]string) error {
			if values["password"] != values["confirm"] {
				return contracterr.Validation("validation failed", map[string]string{"confirm": "confirmation does not match password"})
			}
			submitted := copyValues(values)
			m.openConfirm(confirmState{
				Title: "确认更新管理员密码",
				Body:  fmt.Sprintf("用户名: %s\n用户 ID: %s", item.Username, item.ID),
				Confirm: func(m *model) error {
					if err := m.backend.SetUserPassword(m.ctx, item.ID, submitted["password"], m.actorID); err != nil {
						return err
					}
					m.openResult(resultState{
						Title: "管理员密码已更新",
						Body:  fmt.Sprintf("用户名: %s\n用户 ID: %s", item.Username, item.ID),
						Back:  func(m *model) { m.openAdminListMenu() },
					})
					return nil
				},
				Cancel: func(m *model) { m.openSetPasswordForm(item) },
			})
			return nil
		},
		Cancel: func(m *model) { m.openAdminActionMenu(item) },
	})
}

func (m *model) openUserToggleConfirm(item adminquery.UserListItem, enable bool) {
	verb := "启用"
	if !enable {
		verb = "禁用"
	}
	m.openConfirm(confirmState{
		Title: fmt.Sprintf("%s 用户", verb),
		Body:  fmt.Sprintf("确认%s用户 %s (%s)？", verb, item.Username, item.ID),
		Confirm: func(m *model) error {
			var err error
			if enable {
				err = m.backend.EnableUser(m.ctx, item.ID, m.actorID)
			} else {
				err = m.backend.DisableUser(m.ctx, item.ID, m.actorID)
			}
			if err != nil {
				return err
			}
			m.openResult(resultState{
				Title: fmt.Sprintf("用户已%s", verb),
				Body:  fmt.Sprintf("用户名: %s\n用户 ID: %s", item.Username, item.ID),
				Back:  func(m *model) { m.openUserListMenu() },
			})
			return nil
		},
		Cancel: func(m *model) { m.openUserActionMenu(item) },
	})
}

func (m *model) openUserCreateForm() {
	m.openForm(formState{
		Title: "创建用户",
		Body:  "角色默认选中 user，可显式改为 admin。",
		Fields: []formField{
			newTextField("username", "用户名", "", false),
			newSelectField("role", "角色", []string{string(domain.RoleUser), string(domain.RoleAdmin)}, string(domain.RoleUser)),
		},
		Submit: func(m *model, values map[string]string) error {
			submitted := copyValues(values)
			m.openConfirm(confirmState{
				Title: "确认创建用户",
				Body:  fmt.Sprintf("用户名: %s\n角色: %s", submitted["username"], submitted["role"]),
				Confirm: func(m *model) error {
					user, err := m.backend.CreateUser(m.ctx, admin.CreateUserInput{
						Username: submitted["username"],
						Role:     domain.Role(submitted["role"]),
						ActorID:  m.actorID,
					})
					if err != nil {
						return err
					}
					m.openResult(resultState{
						Title: "用户已创建",
						Body:  fmt.Sprintf("用户名: %s\n用户 ID: %s\n角色: %s", user.Username, user.ID, user.Role),
						Back:  func(m *model) { m.openUserMenu() },
					})
					return nil
				},
				Cancel: func(m *model) { m.openUserCreateForm() },
			})
			return nil
		},
		Cancel: func(m *model) { m.openUserMenu() },
	})
}

func (m *model) openUserListMenu() {
	users, err := m.backend.Users(m.ctx)
	if err != nil {
		m.setNotice(err.Error())
		m.openUserMenu()
		return
	}
	items := make([]menuItem, 0, len(users)+1)
	if len(users) == 0 {
		items = append(items, menuItem{Label: "尚无用户", Desc: "按回车创建首个用户", Action: func(m *model) { m.openUserCreateForm() }})
	} else {
		for _, userItem := range users {
			item := userItem
			items = append(items, menuItem{
				Label:  fmt.Sprintf("%s (%s)", item.Username, item.ID),
				Desc:   fmt.Sprintf("角色: %s  状态: %s", item.Role, item.Status),
				Action: func(m *model) { m.openUserActionMenu(item) },
			})
		}
	}
	items = append(items, menuItem{Label: "返回", Desc: "回到用户菜单", Action: func(m *model) { m.openUserMenu() }})
	m.setMenu(menuState{
		Title:    "现有用户",
		Subtitle: "选择一个用户继续操作。",
		Back:     func(m *model) { m.openUserMenu() },
		Items:    items,
	})
}

func (m *model) openUserActionMenu(item adminquery.UserListItem) {
	items := []menuItem{
		{Label: "启用/禁用", Desc: "切换该用户的登录状态", Action: func(m *model) { m.openUserToggleConfirm(item, item.Status == domain.UserDisabled) }},
		{Label: "删除", Desc: "受保护删除，需要强确认", Action: func(m *model) { m.openUserDeleteConfirm(item) }},
	}
	items = append(items, menuItem{Label: "返回", Desc: "回到用户列表", Action: func(m *model) { m.openUserListMenu() }})
	m.setMenu(menuState{
		Title:    fmt.Sprintf("用户: %s", item.Username),
		Subtitle: fmt.Sprintf("用户 ID: %s  角色: %s  状态: %s", item.ID, item.Role, item.Status),
		Back:     func(m *model) { m.openUserListMenu() },
		Items:    items,
	})
}

func (m *model) openUserDeleteConfirm(item adminquery.UserListItem) {
	m.openConfirm(confirmState{
		Title:       "删除用户",
		Body:        fmt.Sprintf("此操作会删除用户 %s (%s)。如要确认，请输入用户 ID。", item.Username, item.ID),
		RequireText: item.ID,
		Confirm: func(m *model) error {
			if err := m.backend.DeleteUser(m.ctx, item.ID, m.actorID); err != nil {
				return err
			}
			m.openResult(resultState{
				Title: "用户已删除",
				Body:  fmt.Sprintf("用户名: %s\n用户 ID: %s", item.Username, item.ID),
				Back:  func(m *model) { m.openUserListMenu() },
			})
			return nil
		},
		Cancel: func(m *model) { m.openUserActionMenu(item) },
	})
}

func (m *model) openClientJoinForm() {
	defaults := m.backend.JoinDefaults()
	users, err := m.backend.Users(m.ctx)
	if err != nil {
		m.setNotice(err.Error())
		m.openClientMenu()
		return
	}
	if len(users) == 0 {
		m.setNotice("当前没有可选用户，请先创建用户。")
		m.openUserMenu()
		return
	}
	userOptions := make([]string, 0, len(users))
	for _, user := range users {
		userOptions = append(userOptions, fmt.Sprintf("%s (%s)", user.Username, user.ID))
	}
	m.openForm(formState{
		Title: "快速创建客户端",
		Body:  "选择用户、填写客户端名，再确认 join 参数。",
		Fields: []formField{
			newSelectField("user", "所属用户", userOptions, userOptions[0]),
			newTextField("name", "客户端名称", "", false),
			newTextField("enrollment_url", "Enrollment URL", defaults.EnrollmentURL, false),
			newTextField("server_address", "控制通道地址", defaults.ServerAddress, false),
			newTextField("server_tls_address", "TLS 通道地址", defaults.ServerTLSAddress, false),
			newTextField("server_name", "Server Name", defaults.ServerName, false),
			newTextField("server_ca_file", "CA 文件", defaults.ServerCAFile, false),
			newTextField("ttl", "TTL", time.Hour.String(), false),
		},
		Submit: func(m *model, values map[string]string) error {
			userID, err := parseUserID(values["user"])
			if err != nil {
				return err
			}
			ttl, err := time.ParseDuration(values["ttl"])
			if err != nil {
				return contracterr.Validation("validation failed", map[string]string{"ttl": "invalid duration"})
			}
			if _, err := os.Stat(values["server_ca_file"]); err != nil {
				if os.IsNotExist(err) {
					return contracterr.Validation("validation failed", map[string]string{"server_ca_file": "CA file was not found"})
				}
				return err
			}
			submitted := copyValues(values)
			m.openConfirm(confirmState{
				Title: "确认快速创建客户端",
				Body: fmt.Sprintf(
					"所属用户: %s\n客户端名称: %s\nEnrollment URL: %s\n控制通道地址: %s\nTLS 通道地址: %s\nServer Name: %s\nCA 文件: %s\nTTL: %s",
					submitted["user"],
					submitted["name"],
					submitted["enrollment_url"],
					submitted["server_address"],
					submitted["server_tls_address"],
					submitted["server_name"],
					submitted["server_ca_file"],
					submitted["ttl"],
				),
				Confirm: func(m *model) error {
					result, err := m.backend.CreateClientJoin(m.ctx, admin.CreateClientJoinInput{
						UserID:           userID,
						Name:             submitted["name"],
						EnrollmentURL:    submitted["enrollment_url"],
						ServerAddress:    submitted["server_address"],
						ServerTLSAddress: submitted["server_tls_address"],
						ServerName:       submitted["server_name"],
						ServerCAFile:     submitted["server_ca_file"],
						TTL:              ttl,
						ActorID:          m.actorID,
					})
					if err != nil {
						return err
					}
					m.openResult(resultState{
						Title: "客户端和 join token 已创建",
						Body:  fmt.Sprintf("客户端 ID: %s\nToken: %s\n\n使用以下指令获取客户端 join 指令:\n%s\n\n该 token 可在未使用期间从客户端菜单重复查看；如果不可用，查看时会自动重置 join token。token 仍只能被客户端消费一次。", result.Client.ID, result.Token, m.clientJoinCommand(result.Client.ID)),
						Back:  func(m *model) { m.openClientMenu() },
					})
					return nil
				},
				Cancel: func(m *model) { m.openClientJoinForm() },
			})
			return nil
		},
		Cancel: func(m *model) { m.openClientMenu() },
	})
}

func (m *model) openClientCredentialForm() {
	users, err := m.backend.Users(m.ctx)
	if err != nil {
		m.setNotice(err.Error())
		m.openClientMenu()
		return
	}
	if len(users) == 0 {
		m.setNotice("当前没有可选用户，请先创建用户。")
		m.openUserMenu()
		return
	}
	userOptions := make([]string, 0, len(users))
	for _, user := range users {
		userOptions = append(userOptions, fmt.Sprintf("%s (%s)", user.Username, user.ID))
	}
	m.openForm(formState{
		Title: "仅创建客户端凭据",
		Body:  "credential 留空时由服务端生成。",
		Fields: []formField{
			newSelectField("user", "所属用户", userOptions, userOptions[0]),
			newTextField("name", "客户端名称", "", false),
			optionalField(newTextField("credential", "Credential", "", true)),
		},
		Submit: func(m *model, values map[string]string) error {
			userID, err := parseUserID(values["user"])
			if err != nil {
				return err
			}
			submitted := copyValues(values)
			credentialMode := "自动生成"
			if strings.TrimSpace(submitted["credential"]) != "" {
				credentialMode = "使用手动输入"
			}
			m.openConfirm(confirmState{
				Title: "确认创建客户端凭据",
				Body:  fmt.Sprintf("所属用户: %s\n客户端名称: %s\nCredential: %s", submitted["user"], submitted["name"], credentialMode),
				Confirm: func(m *model) error {
					result, err := m.backend.CreateClientWithCredential(m.ctx, admin.CreateClientInput{
						UserID:     userID,
						Name:       submitted["name"],
						Credential: submitted["credential"],
						ActorID:    m.actorID,
					})
					if err != nil {
						return err
					}
					m.openResult(resultState{
						Title: "客户端已创建",
						Body:  fmt.Sprintf("客户端 ID: %s\nCredential: %s\n\n该 credential 只在当前结果页明文展示。离开后需要重新创建或轮换。", result.Client.ID, result.Credential),
						Back:  func(m *model) { m.openClientMenu() },
					})
					return nil
				},
				Cancel: func(m *model) { m.openClientCredentialForm() },
			})
			return nil
		},
		Cancel: func(m *model) { m.openClientMenu() },
	})
}

func (m *model) openClientListMenu() {
	clients, err := m.backend.Clients(m.ctx)
	if err != nil {
		m.setNotice(err.Error())
		m.openClientMenu()
		return
	}
	items := make([]menuItem, 0, len(clients)+1)
	if len(clients) == 0 {
		items = append(items, menuItem{Label: "尚无客户端", Desc: "按回车创建第一个客户端", Action: func(m *model) { m.openClientJoinForm() }})
	} else {
		for _, clientItem := range clients {
			item := clientItem
			items = append(items, menuItem{
				Label:  fmt.Sprintf("%s (%s)", item.Name, item.ID),
				Desc:   fmt.Sprintf("用户: %s  状态: %s", item.UserID, item.Status),
				Action: func(m *model) { m.openClientActionMenu(item) },
			})
		}
	}
	items = append(items, menuItem{Label: "返回", Desc: "回到客户端菜单", Action: func(m *model) { m.openClientMenu() }})
	m.setMenu(menuState{
		Title:    "现有客户端",
		Subtitle: "选择一个客户端继续操作。",
		Back:     func(m *model) { m.openClientMenu() },
		Items:    items,
	})
}

func (m *model) openClientReviewJoinTokenListMenu() {
	clients, err := m.backend.Clients(m.ctx)
	if err != nil {
		m.setNotice(err.Error())
		m.openClientMenu()
		return
	}
	items := make([]menuItem, 0, len(clients)+1)
	if len(clients) == 0 {
		items = append(items, menuItem{Label: "尚无客户端", Desc: "按回车快速创建客户端", Action: func(m *model) { m.openClientJoinForm() }})
	} else {
		for _, clientItem := range clients {
			item := clientItem
			items = append(items, menuItem{
				Label:  fmt.Sprintf("%s (%s)", item.Name, item.ID),
				Desc:   fmt.Sprintf("用户: %s  状态: %s", item.UserID, item.Status),
				Action: func(m *model) { m.openClientJoinTokenResult(item) },
			})
		}
	}
	items = append(items, menuItem{Label: "返回", Desc: "回到客户端菜单", Action: func(m *model) { m.openClientMenu() }})
	m.setMenu(menuState{
		Title:    "查看 join token",
		Subtitle: "选择客户端查看 join token；不可用时会自动重置。",
		Back:     func(m *model) { m.openClientMenu() },
		Items:    items,
	})
}

func (m *model) openClientActionMenu(item adminquery.ClientListItem) {
	items := []menuItem{
		{Label: "启用/禁用", Desc: "切换客户端状态", Action: func(m *model) { m.openClientToggleConfirm(item, item.Status == domain.ClientDisabled) }},
		{Label: "查看 join token", Desc: "查看 token，不可用时自动重置", Action: func(m *model) { m.openClientJoinTokenResult(item) }},
		{Label: "轮换凭据", Desc: "生成新的客户端 credential", Action: func(m *model) { m.openClientRotateConfirm(item) }},
		{Label: "删除", Desc: "受保护删除，需要强确认", Action: func(m *model) { m.openClientDeleteConfirm(item) }},
	}
	items = append(items, menuItem{Label: "返回", Desc: "回到客户端列表", Action: func(m *model) { m.openClientListMenu() }})
	m.setMenu(menuState{
		Title:    fmt.Sprintf("客户端: %s", item.Name),
		Subtitle: fmt.Sprintf("客户端 ID: %s  用户: %s  状态: %s", item.ID, item.UserID, item.Status),
		Back:     func(m *model) { m.openClientListMenu() },
		Items:    items,
	})
}

func (m *model) openClientJoinTokenResult(item adminquery.ClientListItem) {
	result, err := m.backend.ReviewClientJoinToken(m.ctx, item.ID, m.actorID)
	if err != nil {
		m.setNotice(friendlyError(err))
		m.openClientActionMenu(item)
		return
	}
	m.openResult(resultState{
		Title: "join token",
		Body: fmt.Sprintf(
			"客户端 ID: %s\n过期时间: %s\nToken: %s\n\n使用以下指令获取客户端 join 指令:\n%s\n\n该 token 可在未使用期间重复查看；如果不可用，查看时会自动重置 join token。token 仍只能被客户端消费一次。",
			result.Client.ID,
			result.ExpiresAt.Format(time.RFC3339),
			result.Token,
			m.clientJoinCommand(result.Client.ID),
		),
		Back: func(m *model) { m.openClientActionMenu(item) },
	})
}

func (m *model) openClientToggleConfirm(item adminquery.ClientListItem, enable bool) {
	verb := "启用"
	if !enable {
		verb = "禁用"
	}
	m.openConfirm(confirmState{
		Title: fmt.Sprintf("%s 客户端", verb),
		Body:  fmt.Sprintf("确认%s客户端 %s (%s)？", verb, item.Name, item.ID),
		Confirm: func(m *model) error {
			var err error
			if enable {
				err = m.backend.EnableClient(m.ctx, item.ID, m.actorID)
			} else {
				err = m.backend.DisableClient(m.ctx, item.ID, m.actorID)
			}
			if err != nil {
				return err
			}
			m.openResult(resultState{
				Title: fmt.Sprintf("客户端已%s", verb),
				Body:  fmt.Sprintf("客户端 ID: %s\n用户: %s", item.ID, item.UserID),
				Back:  func(m *model) { m.openClientListMenu() },
			})
			return nil
		},
		Cancel: func(m *model) { m.openClientActionMenu(item) },
	})
}

func (m *model) openClientRotateConfirm(item adminquery.ClientListItem) {
	m.openConfirm(confirmState{
		Title: "轮换客户端凭据",
		Body:  fmt.Sprintf("确认轮换客户端 %s (%s) 的凭据？", item.Name, item.ID),
		Confirm: func(m *model) error {
			result, err := m.backend.RotateClientCredential(m.ctx, admin.RotateClientCredentialInput{ClientID: item.ID, ActorID: m.actorID})
			if err != nil {
				return err
			}
			m.openResult(resultState{
				Title: "客户端凭据已轮换",
				Body:  fmt.Sprintf("客户端 ID: %s\nCredential: %s\n\n该 credential 只在当前结果页明文展示。离开后需要重新创建或轮换。", result.Client.ID, result.Credential),
				Back:  func(m *model) { m.openClientListMenu() },
			})
			return nil
		},
		Cancel: func(m *model) { m.openClientActionMenu(item) },
	})
}

func (m *model) openClientDeleteConfirm(item adminquery.ClientListItem) {
	m.openConfirm(confirmState{
		Title:       "删除客户端",
		Body:        fmt.Sprintf("此操作会删除客户端 %s (%s)。如要确认，请输入客户端 ID。", item.Name, item.ID),
		RequireText: item.ID,
		Confirm: func(m *model) error {
			if err := m.backend.DeleteClient(m.ctx, item.ID, m.actorID); err != nil {
				return err
			}
			m.openResult(resultState{
				Title: "客户端已删除",
				Body:  fmt.Sprintf("客户端 ID: %s\n用户: %s", item.ID, item.UserID),
				Back:  func(m *model) { m.openClientListMenu() },
			})
			return nil
		},
		Cancel: func(m *model) { m.openClientActionMenu(item) },
	})
}

func (m *model) openClientJoinDefaultsFromInput(values map[string]string) admin.CreateClientJoinInput {
	ttl, _ := time.ParseDuration(values["ttl"])
	return admin.CreateClientJoinInput{
		UserID:           values["user"],
		Name:             values["name"],
		EnrollmentURL:    values["enrollment_url"],
		ServerAddress:    values["server_address"],
		ServerTLSAddress: values["server_tls_address"],
		ServerName:       values["server_name"],
		ServerCAFile:     values["server_ca_file"],
		TTL:              ttl,
		ActorID:          m.actorID,
	}
}

func copyValues(values map[string]string) map[string]string {
	copied := make(map[string]string, len(values))
	maps.Copy(copied, values)
	return copied
}

func parseUserID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", contracterr.Validation("validation failed", map[string]string{"user": "user selection is required"})
	}
	start := strings.LastIndex(value, "(")
	if start >= 0 && strings.HasSuffix(value, ")") {
		return strings.TrimSpace(value[start+1 : len(value)-1]), nil
	}
	return value, nil
}

func validationFields(err error) map[string]string {
	var contractError *contracterr.Error
	if errors.As(err, &contractError) && contractError.Code == contracterr.CodeValidationFailed {
		return contractError.Fields
	}
	return nil
}

func friendlyError(err error) string {
	if err == nil {
		return ""
	}
	var contractError *contracterr.Error
	if errors.As(err, &contractError) {
		if strings.TrimSpace(contractError.Message) != "" {
			return contractError.Message
		}
	}
	return err.Error()
}
