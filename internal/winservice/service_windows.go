//go:build windows

package winservice

import (
	"context"
	"fmt"
	"log"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type windowsManager struct{}

func defaultManager() Manager {
	return windowsManager{}
}

func runPlatformService(name string, runner Runner, configPath string) error {
	return svc.Run(name, serviceHandler{runner: runner, configPath: configPath})
}

func (windowsManager) Install(spec InstallSpec) error {
	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect service manager: %w", err)
	}
	defer manager.Disconnect()
	service, err := manager.CreateService(spec.Name, spec.Executable, mgr.Config{
		DisplayName: spec.DisplayName,
		Description: spec.Description,
		StartType:   windowsStartup(spec.Startup),
	}, spec.Args...)
	if err != nil {
		return fmt.Errorf("create service %s: %w", spec.Name, err)
	}
	return service.Close()
}

func (windowsManager) Uninstall(name string) error {
	service, closeService, err := openService(name)
	if err != nil {
		return err
	}
	defer closeService()
	if err := service.Delete(); err != nil {
		return fmt.Errorf("delete service %s: %w", name, err)
	}
	return nil
}

func (windowsManager) Start(name string) error {
	service, closeService, err := openService(name)
	if err != nil {
		return err
	}
	defer closeService()
	if err := service.Start(); err != nil {
		return fmt.Errorf("start service %s: %w", name, err)
	}
	return nil
}

func (windowsManager) Stop(name string) error {
	service, closeService, err := openService(name)
	if err != nil {
		return err
	}
	defer closeService()
	if _, err := service.Control(svc.Stop); err != nil {
		return fmt.Errorf("stop service %s: %w", name, err)
	}
	return nil
}

func (windowsManager) Status(name string) (ServiceStatus, error) {
	service, closeService, err := openService(name)
	if err != nil {
		return ServiceStatus{}, err
	}
	defer closeService()
	status, err := service.Query()
	if err != nil {
		return ServiceStatus{}, fmt.Errorf("query service %s: %w", name, err)
	}
	return ServiceStatus{Name: name, State: stateString(status.State)}, nil
}

func openService(name string) (*mgr.Service, func(), error) {
	manager, err := mgr.Connect()
	if err != nil {
		return nil, func() {}, fmt.Errorf("connect service manager: %w", err)
	}
	service, err := manager.OpenService(name)
	if err != nil {
		manager.Disconnect()
		return nil, func() {}, fmt.Errorf("open service %s: %w", name, err)
	}
	closeFn := func() {
		_ = service.Close()
		_ = manager.Disconnect()
	}
	return service, closeFn, nil
}

func windowsStartup(startup Startup) uint32 {
	switch startup {
	case StartupManual:
		return mgr.StartManual
	case StartupDisabled:
		return mgr.StartDisabled
	default:
		return mgr.StartAutomatic
	}
}

func stateString(state svc.State) string {
	switch state {
	case svc.Stopped:
		return "Stopped"
	case svc.StartPending:
		return "StartPending"
	case svc.StopPending:
		return "StopPending"
	case svc.Running:
		return "Running"
	case svc.ContinuePending:
		return "ContinuePending"
	case svc.PausePending:
		return "PausePending"
	case svc.Paused:
		return "Paused"
	default:
		return fmt.Sprintf("Unknown(%d)", state)
	}
}

type serviceHandler struct {
	runner     Runner
	configPath string
}

func (handler serviceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, statuses chan<- svc.Status) (bool, uint32) {
	statuses <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- handler.runner(ctx, handler.configPath)
	}()
	current := svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	statuses <- current
	for {
		select {
		case request, ok := <-requests:
			if !ok {
				cancel()
				err := <-done
				if err != nil {
					log.Printf("service stopped with error: %v", err)
					return false, 1
				}
				return false, 0
			}
			switch request.Cmd {
			case svc.Interrogate:
				statuses <- current
			case svc.Stop, svc.Shutdown:
				statuses <- svc.Status{State: svc.StopPending}
				cancel()
				err := <-done
				if err != nil {
					log.Printf("service stopped with error: %v", err)
					return false, 1
				}
				return false, 0
			default:
				statuses <- current
			}
		case err := <-done:
			if err != nil {
				log.Printf("service exited with error: %v", err)
				return false, 1
			}
			return false, 0
		}
	}
}
