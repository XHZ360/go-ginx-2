package winservice

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

type Runner func(context.Context, string) error

type Definition struct {
	DefaultName string
	DisplayName string
	Description string
}

type Options struct {
	Args            []string
	Definition      Definition
	Runner          Runner
	ValidateInstall func(configPath string) error
	ExecutablePath  func() (string, error)
	Manager         Manager
	Stdout          io.Writer
}

type Manager interface {
	Install(InstallSpec) error
	Uninstall(name string) error
	Start(name string) error
	Stop(name string) error
	Status(name string) (ServiceStatus, error)
}

type InstallSpec struct {
	Name        string
	DisplayName string
	Description string
	Executable  string
	Args        []string
	Startup     Startup
}

type ServiceStatus struct {
	Name  string
	State string
}

type Startup string

const (
	StartupAuto     Startup = "auto"
	StartupManual   Startup = "manual"
	StartupDisabled Startup = "disabled"
)

var ErrUnsupported = errors.New("native Windows service management is only supported on Windows")

func RunCommand(args []string, options Options) error {
	if len(args) == 0 {
		return errors.New("usage: service <install|uninstall|start|stop|restart|status|run> [flags]")
	}
	options = options.withDefaults()
	switch args[0] {
	case "install":
		return runInstall(args[1:], options)
	case "uninstall":
		return runNamedAction(args[1:], options, "uninstall", options.Manager.Uninstall)
	case "start":
		return runNamedAction(args[1:], options, "start", options.Manager.Start)
	case "stop":
		return runNamedAction(args[1:], options, "stop", options.Manager.Stop)
	case "restart":
		return runRestart(args[1:], options)
	case "status":
		return runStatus(args[1:], options)
	case "run":
		return runService(args[1:], options)
	default:
		return fmt.Errorf("unknown service command %q", args[0])
	}
}

func BuildRunArguments(configPath string) []string {
	args := []string{"service", "run"}
	if strings.TrimSpace(configPath) != "" {
		args = append(args, "-config", configPath)
	}
	return args
}

func (options Options) withDefaults() Options {
	if strings.TrimSpace(options.Definition.DefaultName) == "" {
		options.Definition.DefaultName = "go-ginx"
	}
	if strings.TrimSpace(options.Definition.DisplayName) == "" {
		options.Definition.DisplayName = options.Definition.DefaultName
	}
	if options.ExecutablePath == nil {
		options.ExecutablePath = os.Executable
	}
	if options.Manager == nil {
		options.Manager = defaultManager()
	}
	if options.Stdout == nil {
		options.Stdout = io.Discard
	}
	return options
}

func runInstall(args []string, options Options) error {
	flags := newFlagSet("service install")
	name := flags.String("name", options.Definition.DefaultName, "Windows service name")
	displayName := flags.String("display-name", options.Definition.DisplayName, "Windows service display name")
	description := flags.String("description", options.Definition.Description, "Windows service description")
	startupValue := flags.String("startup", string(StartupAuto), "service startup type: auto, manual, or disabled")
	configPath := flags.String("config", "", "config path to pass to service run")
	if err := flags.Parse(args); err != nil {
		return err
	}
	startup, err := parseStartup(*startupValue)
	if err != nil {
		return err
	}
	if options.ValidateInstall != nil {
		if err := options.ValidateInstall(*configPath); err != nil {
			return fmt.Errorf("validate service install: %w", err)
		}
	}
	executable, err := options.ExecutablePath()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	spec := InstallSpec{
		Name:        strings.TrimSpace(*name),
		DisplayName: strings.TrimSpace(*displayName),
		Description: strings.TrimSpace(*description),
		Executable:  executable,
		Args:        BuildRunArguments(*configPath),
		Startup:     startup,
	}
	if spec.Name == "" {
		return errors.New("service name is required")
	}
	return options.Manager.Install(spec)
}

func runNamedAction(args []string, options Options, command string, action func(string) error) error {
	flags := newFlagSet("service " + command)
	name := flags.String("name", options.Definition.DefaultName, "Windows service name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*name) == "" {
		return errors.New("service name is required")
	}
	return action(strings.TrimSpace(*name))
}

func runRestart(args []string, options Options) error {
	flags := newFlagSet("service restart")
	name := flags.String("name", options.Definition.DefaultName, "Windows service name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	serviceName := strings.TrimSpace(*name)
	if serviceName == "" {
		return errors.New("service name is required")
	}
	if err := options.Manager.Stop(serviceName); err != nil {
		return err
	}
	return options.Manager.Start(serviceName)
}

func runStatus(args []string, options Options) error {
	flags := newFlagSet("service status")
	name := flags.String("name", options.Definition.DefaultName, "Windows service name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	serviceName := strings.TrimSpace(*name)
	if serviceName == "" {
		return errors.New("service name is required")
	}
	status, err := options.Manager.Status(serviceName)
	if err != nil {
		return err
	}
	if status.Name == "" {
		status.Name = serviceName
	}
	_, err = fmt.Fprintf(options.Stdout, "%s: %s\n", status.Name, status.State)
	return err
}

func runService(args []string, options Options) error {
	flags := newFlagSet("service run")
	name := flags.String("name", options.Definition.DefaultName, "Windows service name")
	configPath := flags.String("config", "", "config path to load")
	if err := flags.Parse(args); err != nil {
		return err
	}
	serviceName := strings.TrimSpace(*name)
	if serviceName == "" {
		return errors.New("service name is required")
	}
	if options.Runner == nil {
		return errors.New("service runner is required")
	}
	return runPlatformService(serviceName, options.Runner, *configPath)
}

func parseStartup(value string) (Startup, error) {
	switch Startup(strings.ToLower(strings.TrimSpace(value))) {
	case StartupAuto:
		return StartupAuto, nil
	case StartupManual:
		return StartupManual, nil
	case StartupDisabled:
		return StartupDisabled, nil
	default:
		return "", fmt.Errorf("unsupported startup type %q", value)
	}
}

func newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}
