package daemon

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

const managedCertificateControllerInterval = time.Minute

type managedCertificateController struct {
	db                     store.Store
	service                certmanager.Service
	renewalWindow          time.Duration
	originCARotationWindow time.Duration
	interval               time.Duration
	now                    func() time.Time

	mu      sync.Mutex
	running map[string]struct{}
}

func newManagedCertificateController(db store.Store, service certmanager.Service, renewalWindow time.Duration, originCARotationWindow time.Duration) *managedCertificateController {
	return &managedCertificateController{db: db, service: service, renewalWindow: renewalWindow, originCARotationWindow: originCARotationWindow, interval: managedCertificateControllerInterval, running: make(map[string]struct{})}
}

func (controller *managedCertificateController) Run(ctx context.Context) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("managed certificate controller stopped after panic: error=%v", recovered)
		}
	}()
	controller.RenewDue(ctx)
	interval := controller.interval
	if interval <= 0 {
		interval = managedCertificateControllerInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			controller.RenewDue(ctx)
		}
	}
}

func (controller *managedCertificateController) RenewDue(ctx context.Context) {
	if controller == nil || controller.db == nil || (controller.renewalWindow <= 0 && controller.originCARotationWindow <= 0) {
		return
	}
	now := controller.nowTime()
	window := controller.renewalWindow
	if controller.originCARotationWindow > window {
		window = controller.originCARotationWindow
	}
	certificates, err := controller.db.Certificates().ListRenewable(ctx, now.Add(window), now)
	if err != nil {
		return
	}
	for _, certificate := range certificates {
		if !controller.certificateDue(certificate, now) {
			continue
		}
		controller.renewOne(ctx, certificate)
	}
}

func (controller *managedCertificateController) certificateDue(certificate domain.ManagedCertificate, now time.Time) bool {
	if certificate.NotAfter == nil {
		return false
	}
	window := controller.renewalWindow
	if certificate.ProviderType == domain.CertificateProviderCloudflareOriginCA {
		window = controller.originCARotationWindow
	}
	if window <= 0 {
		return false
	}
	return !certificate.NotAfter.After(now.Add(window))
}

func (controller *managedCertificateController) renewOne(ctx context.Context, certificate domain.ManagedCertificate) {
	key := certificateOperationKey(certificate.ProxyID, certificate.Host)
	if !controller.start(key) {
		return
	}
	defer controller.finish(key)
	_, err := controller.service.Renew(ctx, certificate.ProxyID)
	if errors.Is(err, certmanager.ErrOperationBusy) {
		return
	}
}

func (controller *managedCertificateController) start(key string) bool {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if _, ok := controller.running[key]; ok {
		return false
	}
	controller.running[key] = struct{}{}
	return true
}

func (controller *managedCertificateController) finish(key string) {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	delete(controller.running, key)
}

func (controller *managedCertificateController) nowTime() time.Time {
	if controller.now != nil {
		return controller.now().UTC()
	}
	return time.Now().UTC()
}

func certificateOperationKey(proxyID string, host string) string {
	return strings.TrimSpace(proxyID) + "\x00" + strings.ToLower(strings.TrimSpace(host))
}

type failedDNSProvider struct {
	err error
}

func (provider failedDNSProvider) Present(context.Context, string, string) error {
	if provider.err != nil {
		return provider.err
	}
	return errors.New("dns challenge provider is unavailable")
}

func (provider failedDNSProvider) CleanUp(context.Context, string, string) error {
	return nil
}

type failedCertificateIssuer struct {
	err error
}

func (issuer failedCertificateIssuer) Issue(context.Context, certmanager.IssueRequest) (certmanager.IssuedCertificate, error) {
	if issuer.err != nil {
		return certmanager.IssuedCertificate{}, issuer.err
	}
	return certmanager.IssuedCertificate{}, errors.New("certificate issuer is unavailable")
}
