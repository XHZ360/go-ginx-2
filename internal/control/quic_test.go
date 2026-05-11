package control

import "testing"

func TestDefaultQUICConfigUsesMilestoneOneLimits(t *testing.T) {
	cfg := DefaultQUICConfig()

	if cfg.MaxIncomingStreams != DefaultMaxIncomingStreams {
		t.Fatalf("expected %d streams, got %d", DefaultMaxIncomingStreams, cfg.MaxIncomingStreams)
	}
	if cfg.KeepAlivePeriod <= 0 {
		t.Fatal("expected positive keepalive")
	}
	if cfg.MaxIdleTimeout <= cfg.KeepAlivePeriod {
		t.Fatal("expected idle timeout greater than keepalive")
	}
}
