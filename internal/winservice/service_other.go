//go:build !windows

package winservice

import "fmt"

type unsupportedManager struct{}

func defaultManager() Manager {
	return unsupportedManager{}
}

func runPlatformService(name string, _ Runner, _ string) error {
	return fmt.Errorf("%w: %s", ErrUnsupported, name)
}

func (unsupportedManager) Install(InstallSpec) error {
	return ErrUnsupported
}

func (unsupportedManager) Uninstall(string) error {
	return ErrUnsupported
}

func (unsupportedManager) Start(string) error {
	return ErrUnsupported
}

func (unsupportedManager) Stop(string) error {
	return ErrUnsupported
}

func (unsupportedManager) Status(string) (ServiceStatus, error) {
	return ServiceStatus{}, ErrUnsupported
}
