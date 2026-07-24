package localproxy

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/control"
	"github.com/simp-frp/go-ginx-2/internal/session"
)

type Dialer struct {
	Policy  LocalTargetPolicy
	Timeout time.Duration
	Network *net.Dialer
}

func (dialer Dialer) DialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	if dialer.Policy == nil {
		return nil, errors.New("local target policy is required")
	}
	host, rawPort, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		return nil, err
	}
	if err := dialer.Policy.ValidateTarget(ctx, host, port); err != nil {
		return nil, err
	}
	networkDialer := dialer.Network
	if networkDialer == nil {
		networkDialer = &net.Dialer{Timeout: dialer.Timeout}
	}
	return networkDialer.DialContext(ctx, network, address)
}

type Session struct {
	Dialer LocalDialer
}

func (virtual Session) OpenStream(ctx context.Context) (io.ReadWriteCloser, error) {
	if virtual.Dialer == nil {
		return nil, errors.New("local dialer is required")
	}
	entry, local := net.Pipe()
	go virtual.serve(ctx, local)
	return entry, nil
}

func (virtual Session) serve(ctx context.Context, stream net.Conn) {
	control.ServeProxyStream(ctx, stream, virtual.Dialer)
}

var _ session.VirtualSession = Session{}
var _ LocalDialer = Dialer{}
