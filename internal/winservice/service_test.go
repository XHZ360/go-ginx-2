package winservice

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestRunCommandInstallBuildsSpec(t *testing.T) {
	manager := &fakeManager{}
	var validated string
	err := RunCommand([]string{"install", "-name", "custom-service", "-display-name", "Custom Service", "-description", "Custom description", "-startup", "manual", "-config", "config/server.json"}, Options{
		Definition: Definition{
			DefaultName: "goginx-server",
			DisplayName: "go-ginx server",
			Description: "go-ginx server daemon",
		},
		ValidateInstall: func(configPath string) error {
			validated = configPath
			return nil
		},
		ExecutablePath: func() (string, error) {
			return `C:\go-ginx\bin\goginx-server.exe`, nil
		},
		Manager: manager,
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if validated != "config/server.json" {
		t.Fatalf("expected config validation, got %q", validated)
	}
	want := InstallSpec{
		Name:        "custom-service",
		DisplayName: "Custom Service",
		Description: "Custom description",
		Executable:  `C:\go-ginx\bin\goginx-server.exe`,
		Args:        []string{"service", "run", "-config", "config/server.json"},
		Startup:     StartupManual,
	}
	if !reflect.DeepEqual(manager.installed, want) {
		t.Fatalf("unexpected install spec:\nwant=%+v\n got=%+v", want, manager.installed)
	}
}

func TestRunCommandStatusWritesState(t *testing.T) {
	manager := &fakeManager{status: ServiceStatus{Name: "goginx-server", State: "Running"}}
	var output bytes.Buffer
	err := RunCommand([]string{"status"}, Options{
		Definition: Definition{DefaultName: "goginx-server"},
		Manager:    manager,
		Stdout:     &output,
	})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if got := output.String(); got != "goginx-server: Running\n" {
		t.Fatalf("unexpected status output %q", got)
	}
	if manager.statusName != "goginx-server" {
		t.Fatalf("expected default service name, got %q", manager.statusName)
	}
}

func TestRunCommandRestartStopsThenStarts(t *testing.T) {
	manager := &fakeManager{}
	if err := RunCommand([]string{"restart", "-name", "custom"}, Options{Manager: manager}); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if !reflect.DeepEqual(manager.actions, []string{"stop:custom", "start:custom"}) {
		t.Fatalf("unexpected actions: %+v", manager.actions)
	}
}

func TestRunCommandInstallPropagatesValidationError(t *testing.T) {
	wantErr := errors.New("missing client state")
	err := RunCommand([]string{"install"}, Options{
		ValidateInstall: func(string) error { return wantErr },
		Manager:         &fakeManager{},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestBuildRunArgumentsOmitsEmptyConfig(t *testing.T) {
	if got := BuildRunArguments(""); !reflect.DeepEqual(got, []string{"service", "run"}) {
		t.Fatalf("unexpected run args: %+v", got)
	}
}

func TestRunCommandRunRequiresRunner(t *testing.T) {
	err := RunCommand([]string{"run"}, Options{})
	if err == nil || err.Error() != "service runner is required" {
		t.Fatalf("expected missing runner error, got %v", err)
	}
}

type fakeManager struct {
	installed  InstallSpec
	status     ServiceStatus
	statusName string
	actions    []string
}

func (manager *fakeManager) Install(spec InstallSpec) error {
	manager.installed = spec
	return nil
}

func (manager *fakeManager) Uninstall(name string) error {
	manager.actions = append(manager.actions, "uninstall:"+name)
	return nil
}

func (manager *fakeManager) Start(name string) error {
	manager.actions = append(manager.actions, "start:"+name)
	return nil
}

func (manager *fakeManager) Stop(name string) error {
	manager.actions = append(manager.actions, "stop:"+name)
	return nil
}

func (manager *fakeManager) Status(name string) (ServiceStatus, error) {
	manager.statusName = name
	return manager.status, nil
}

func TestRunnerTypeAcceptsContext(t *testing.T) {
	var runner Runner = func(context.Context, string) error { return nil }
	if runner == nil {
		t.Fatal("expected runner")
	}
}
