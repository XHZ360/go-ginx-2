package udp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/control"
	"github.com/simp-frp/go-ginx-2/internal/domain"
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
	IdleTimeout         time.Duration
	NewSession          func() (string, error)
	Stats               stats.Recorder
}

type Listener struct {
	entry    Entry
	conn     net.PacketConn
	mu       sync.Mutex
	sessions map[string]*udpSession
}

type udpSession struct {
	proxyID    string
	remoteAddr net.Addr
	stream     netStream
	lastSeen   time.Time
	mu         sync.Mutex
}

type netStream interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
}

func Listen(entry Entry) (*Listener, error) {
	if entry.Store == nil {
		return nil, errors.New("store is required")
	}
	if entry.Sessions == nil {
		return nil, errors.New("session manager is required")
	}
	conn, err := net.ListenPacket("udp", entry.ListenAddress)
	if err != nil {
		return nil, err
	}
	return &Listener{entry: entry, conn: conn, sessions: make(map[string]*udpSession)}, nil
}

func (listener *Listener) Addr() net.Addr { return listener.conn.LocalAddr() }

func (listener *Listener) Close() error {
	listener.mu.Lock()
	for key, session := range listener.sessions {
		_ = session.stream.Close()
		delete(listener.sessions, key)
	}
	listener.mu.Unlock()
	return listener.conn.Close()
}

func (listener *Listener) Serve(ctx context.Context) error {
	go listener.cleanupLoop(ctx)
	buffer := make([]byte, 64*1024)
	for {
		n, remoteAddr, err := listener.conn.ReadFrom(buffer)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		payload := append([]byte(nil), buffer[:n]...)
		go listener.handlePacket(ctx, remoteAddr, payload)
	}
}

func (listener *Listener) handlePacket(ctx context.Context, remoteAddr net.Addr, payload []byte) {
	entryPort := listener.entry.EntryPort
	if entryPort == 0 {
		entryPort = portFromAddr(listener.conn.LocalAddr())
	}
	proxy, err := listener.entry.Store.Proxies().ByUDPEntry(ctx, listener.entry.EntryBindHost, entryPort, listener.entry.IncludeDefaultEntry)
	if err != nil || proxy.Status != domain.ProxyEnabled {
		return
	}
	session, err := listener.session(ctx, proxy, remoteAddr)
	if err != nil {
		listener.record(proxy.ID, int64(len(payload)), 0, true)
		return
	}
	session.mu.Lock()
	session.lastSeen = time.Now()
	err = control.WriteDatagramFrame(session.stream, payload)
	session.mu.Unlock()
	listener.record(proxy.ID, int64(len(payload)), 0, err != nil)
	if err != nil {
		listener.removeSession(proxy.ID, remoteAddr.String())
	}
}

func (listener *Listener) session(ctx context.Context, proxy domain.Proxy, remoteAddr net.Addr) (*udpSession, error) {
	key := sessionKey(proxy.ID, remoteAddr.String())
	listener.mu.Lock()
	existing, ok := listener.sessions[key]
	listener.mu.Unlock()
	if ok {
		return existing, nil
	}
	latest, ok := listener.entry.Sessions.Latest(proxy.ClientID)
	if !ok || latest.StreamOpener == nil {
		return nil, errors.New("client offline")
	}
	stream, err := latest.StreamOpener.OpenStream(ctx)
	if err != nil {
		return nil, err
	}
	connectionID, err := listener.sessionID()
	if err != nil {
		_ = stream.Close()
		return nil, err
	}
	if err := control.WriteMessage(stream, control.MessageOpenStream, control.OpenStream{Kind: "udp", ProxyID: proxy.ID, ConnectionID: connectionID, TargetHost: proxy.TargetHost, TargetPort: proxy.TargetPort}); err != nil {
		_ = stream.Close()
		return nil, err
	}
	session := &udpSession{proxyID: proxy.ID, remoteAddr: remoteAddr, stream: stream, lastSeen: time.Now()}
	listener.mu.Lock()
	if existing, ok := listener.sessions[key]; ok {
		listener.mu.Unlock()
		_ = stream.Close()
		return existing, nil
	}
	listener.sessions[key] = session
	listener.mu.Unlock()
	go listener.readResponses(key, session)
	return session, nil
}

func (listener *Listener) readResponses(key string, session *udpSession) {
	defer listener.removeKey(key)
	for {
		payload, err := control.ReadDatagramFrame(session.stream)
		if err != nil {
			return
		}
		if _, err := listener.conn.WriteTo(payload, session.remoteAddr); err != nil {
			listener.record(session.proxyID, 0, int64(len(payload)), true)
			return
		}
		listener.record(session.proxyID, 0, int64(len(payload)), false)
	}
}

func (listener *Listener) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(listener.idleTimeout() / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			listener.cleanupIdle()
		}
	}
}

func (listener *Listener) cleanupIdle() {
	deadline := time.Now().Add(-listener.idleTimeout())
	listener.mu.Lock()
	for key, session := range listener.sessions {
		if session.lastSeen.Before(deadline) {
			_ = session.stream.Close()
			delete(listener.sessions, key)
		}
	}
	listener.mu.Unlock()
}

func (listener *Listener) removeSession(proxyID string, remoteAddr string) {
	listener.removeKey(sessionKey(proxyID, remoteAddr))
}

func (listener *Listener) removeKey(key string) {
	listener.mu.Lock()
	session, ok := listener.sessions[key]
	if ok {
		_ = session.stream.Close()
		delete(listener.sessions, key)
	}
	listener.mu.Unlock()
}

func (listener *Listener) record(proxyID string, uploadBytes int64, downloadBytes int64, failed bool) {
	if listener.entry.Stats != nil {
		listener.entry.Stats.RecordUDP(proxyID, uploadBytes, downloadBytes, failed)
	}
}

func (listener *Listener) idleTimeout() time.Duration {
	if listener.entry.IdleTimeout > 0 {
		return listener.entry.IdleTimeout
	}
	return 30 * time.Second
}

func (listener *Listener) sessionID() (string, error) {
	if listener.entry.NewSession != nil {
		return listener.entry.NewSession()
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

func sessionKey(proxyID string, remoteAddr string) string { return proxyID + "|" + remoteAddr }

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
