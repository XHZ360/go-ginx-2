package tcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/simp-frp/go-ginx-2/internal/control"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/proxy/tunnel"
	"github.com/simp-frp/go-ginx-2/internal/session"
	"github.com/simp-frp/go-ginx-2/internal/stats"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

type Entry struct {
	Store               store.Store
	Sessions            *session.Manager
	ListenAddress       string
	EntryBindHost       string
	EntryPort           int
	IncludeDefaultEntry bool
	NewConnection       func() (string, error)
	Stats               stats.Recorder
}

type Listener struct {
	entry    Entry
	listener net.Listener
}

func Listen(entry Entry) (*Listener, error) {
	if entry.Store == nil {
		return nil, errors.New("store is required")
	}
	if entry.Sessions == nil {
		return nil, errors.New("session manager is required")
	}
	listener, err := net.Listen("tcp", entry.ListenAddress)
	if err != nil {
		return nil, err
	}
	return &Listener{entry: entry, listener: listener}, nil
}

func (listener *Listener) Addr() net.Addr {
	return listener.listener.Addr()
}

func (listener *Listener) Close() error {
	return listener.listener.Close()
}

func (listener *Listener) Serve(ctx context.Context) error {
	for {
		conn, err := listener.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		go listener.handleConn(ctx, conn)
	}
}

func (listener *Listener) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	entryPort := listener.entry.EntryPort
	if entryPort == 0 {
		entryPort = portFromAddr(listener.listener.Addr())
	}
	proxy, err := listener.entry.Store.Proxies().ByTCPEntry(ctx, listener.entry.EntryBindHost, entryPort, listener.entry.IncludeDefaultEntry)
	if err != nil || proxy.Status != domain.ProxyEnabled {
		return
	}
	failed := true
	var uploadBytes int64
	var downloadBytes int64
	if listener.entry.Stats != nil {
		listener.entry.Stats.RecordTCPStart(proxy.ID)
		defer func() {
			listener.entry.Stats.RecordTCPEnd(proxy.ID, uploadBytes, downloadBytes, failed)
		}()
	}
	latest, ok := listener.entry.Sessions.Latest(proxy.ClientID)
	if !ok || latest.StreamOpener == nil {
		return
	}
	stream, err := latest.StreamOpener.OpenStream(ctx)
	if err != nil {
		return
	}
	defer stream.Close()
	connectionID, err := listener.connectionID()
	if err != nil {
		return
	}
	if err := control.WriteMessage(stream, control.MessageOpenStream, control.OpenStream{ProxyID: proxy.ID, ConnectionID: connectionID, TargetHost: proxy.TargetHost, TargetPort: proxy.TargetPort}); err != nil {
		return
	}
	uploadBytes, downloadBytes = tunnel.CopyBidirectional(conn, stream)
	failed = false
}

func (listener *Listener) connectionID() (string, error) {
	if listener.entry.NewConnection != nil {
		return listener.entry.NewConnection()
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

func portFromAddr(addr net.Addr) int {
	_, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return 0
	}
	parsed, err := strconv.Atoi(port)
	if err != nil {
		return 0
	}
	return parsed
}

func Address(host string, port int) string {
	return net.JoinHostPort(host, fmt.Sprintf("%d", port))
}
