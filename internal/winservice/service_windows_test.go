//go:build windows

package winservice

import (
	"context"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

func TestServiceHandlerCancelsRunnerOnStop(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	handler := serviceHandler{
		configPath: "config/server.json",
		runner: func(ctx context.Context, configPath string) error {
			if configPath != "config/server.json" {
				t.Errorf("unexpected config path %q", configPath)
			}
			close(started)
			<-ctx.Done()
			close(cancelled)
			return nil
		},
	}
	requests := make(chan svc.ChangeRequest)
	statuses := make(chan svc.Status, 4)
	done := make(chan uint32, 1)
	go func() {
		_, code := handler.Execute(nil, requests, statuses)
		done <- code
	}()
	expectStatus(t, statuses, svc.StartPending)
	expectStatus(t, statuses, svc.Running)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("runner did not start")
	}
	requests <- svc.ChangeRequest{Cmd: svc.Stop}
	expectStatus(t, statuses, svc.StopPending)
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("runner was not cancelled")
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("unexpected service exit code %d", code)
		}
	case <-time.After(time.Second):
		t.Fatal("handler did not exit")
	}
}

func expectStatus(t *testing.T, statuses <-chan svc.Status, state svc.State) {
	t.Helper()
	select {
	case status := <-statuses:
		if status.State != state {
			t.Fatalf("expected status %v, got %v", state, status.State)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for status %v", state)
	}
}
