package httpproxy

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net"
	nethttp "net/http"
	"strings"

	"github.com/simp-frp/go-ginx-2/internal/control"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/session"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

type Entry struct {
	Store         store.Store
	Sessions      *session.Manager
	ListenAddress string
	NewRequest    func() (string, error)
}

type Server struct {
	entry  Entry
	server *nethttp.Server
	ln     net.Listener
}

func Listen(entry Entry) (*Server, error) {
	if entry.Store == nil {
		return nil, errors.New("store is required")
	}
	if entry.Sessions == nil {
		return nil, errors.New("session manager is required")
	}
	ln, err := net.Listen("tcp", entry.ListenAddress)
	if err != nil {
		return nil, err
	}
	server := &Server{entry: entry, ln: ln}
	server.server = &nethttp.Server{Handler: server}
	return server, nil
}

func (server *Server) Addr() net.Addr {
	return server.ln.Addr()
}

func (server *Server) Serve(ctx context.Context) error {
	done := make(chan error, 1)
	go func() { done <- server.server.Serve(server.ln) }()
	select {
	case <-ctx.Done():
		_ = server.server.Close()
		return ctx.Err()
	case err := <-done:
		if errors.Is(err, nethttp.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (server *Server) Close() error {
	return server.server.Close()
}

func (server *Server) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	proxy, err := server.entry.Store.Proxies().ByHTTPHost(r.Context(), hostWithoutPort(r.Host))
	if err != nil || proxy.Status != domain.ProxyEnabled {
		nethttp.Error(w, "proxy not found", nethttp.StatusNotFound)
		return
	}
	latest, ok := server.entry.Sessions.Latest(proxy.ClientID)
	if !ok || latest.StreamOpener == nil {
		nethttp.Error(w, "client offline", nethttp.StatusServiceUnavailable)
		return
	}
	stream, err := latest.StreamOpener.OpenStream(r.Context())
	if err != nil {
		nethttp.Error(w, "open proxy stream failed", nethttp.StatusBadGateway)
		return
	}
	defer stream.Close()
	requestID, err := server.requestID()
	if err != nil {
		nethttp.Error(w, "request id failed", nethttp.StatusInternalServerError)
		return
	}
	if err := control.WriteMessage(stream, control.MessageOpenStream, control.OpenStream{Kind: "http", ProxyID: proxy.ID, ConnectionID: requestID, TargetHost: proxy.TargetHost, TargetPort: proxy.TargetPort}); err != nil {
		nethttp.Error(w, "write proxy stream failed", nethttp.StatusBadGateway)
		return
	}
	if err := r.Write(stream); err != nil {
		nethttp.Error(w, "write proxy request failed", nethttp.StatusBadGateway)
		return
	}
	response, err := nethttp.ReadResponse(bufio.NewReader(stream), r)
	if err != nil {
		nethttp.Error(w, "read proxy response failed", nethttp.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	copyHeader(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func (server *Server) requestID() (string, error) {
	if server.entry.NewRequest != nil {
		return server.entry.NewRequest()
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

func hostWithoutPort(host string) string {
	if strings.Contains(host, ":") {
		value, _, err := net.SplitHostPort(host)
		if err == nil {
			return value
		}
	}
	return host
}

func copyHeader(dst nethttp.Header, src nethttp.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
