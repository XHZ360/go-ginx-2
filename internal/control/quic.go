package control

import (
	"time"

	"github.com/quic-go/quic-go"
)

const DefaultMaxIncomingStreams int64 = 1024

func DefaultQUICConfig() *quic.Config {
	return &quic.Config{
		MaxIncomingStreams: DefaultMaxIncomingStreams,
		KeepAlivePeriod:    15 * time.Second,
		MaxIdleTimeout:     45 * time.Second,
	}
}
