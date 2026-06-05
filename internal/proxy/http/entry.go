package httpproxy

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net"
	nethttp "net/http"
	"strings"

	"github.com/simp-frp/go-ginx-2/internal/control"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/proxy/tunnel"
	"github.com/simp-frp/go-ginx-2/internal/session"
	"github.com/simp-frp/go-ginx-2/internal/stats"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

type Entry struct {
	Store                store.Store
	Sessions             *session.Manager
	ListenAddress        string
	EntryBindHost        string
	EntryPort            int
	IncludeDefaultRoutes bool
	NewRequest           func() (string, error)
	Stats                stats.Recorder
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

func (server *Server) SetEntryPort(port int) {
	server.entry.EntryPort = port
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
	proxy, err := server.entry.Store.Proxies().ByHTTPRoute(r.Context(), server.entry.EntryBindHost, server.entry.EntryPort, hostWithoutPort(r.Host), server.entry.IncludeDefaultRoutes)
	if err != nil || proxy.Status != domain.ProxyEnabled {
		log.Printf("http proxy route failed: bind_host=%s port=%d host=%s category=route_miss", displayBindHost(server.entry.EntryBindHost), server.entry.EntryPort, hostWithoutPort(r.Host))
		nethttp.Error(w, "proxy not found", nethttp.StatusNotFound)
		return
	}
	statusCode := nethttp.StatusBadGateway
	failed := true
	var uploadBytes int64
	var downloadBytes int64
	if server.entry.Stats != nil {
		defer func() {
			server.entry.Stats.RecordHTTP(proxy.ID, statusCode, uploadBytes, downloadBytes, failed)
		}()
	}
	latest, ok := server.entry.Sessions.Latest(proxy.ClientID)
	if !ok || latest.StreamOpener == nil {
		log.Printf("http proxy route failed: bind_host=%s port=%d host=%s proxy_id=%s category=client_offline", displayBindHost(server.entry.EntryBindHost), server.entry.EntryPort, hostWithoutPort(r.Host), proxy.ID)
		statusCode = nethttp.StatusServiceUnavailable
		nethttp.Error(w, "client offline", nethttp.StatusServiceUnavailable)
		return
	}
	stream, err := latest.StreamOpener.OpenStream(r.Context())
	if err != nil {
		log.Printf("http proxy route failed: bind_host=%s port=%d host=%s proxy_id=%s category=open_stream_failed error=%v", displayBindHost(server.entry.EntryBindHost), server.entry.EntryPort, hostWithoutPort(r.Host), proxy.ID, err)
		nethttp.Error(w, "open proxy stream failed", nethttp.StatusBadGateway)
		return
	}
	defer control.CloseStream(stream)
	requestID, err := server.requestID()
	if err != nil {
		nethttp.Error(w, "request id failed", nethttp.StatusInternalServerError)
		return
	}
	if err := control.WriteMessage(stream, control.MessageOpenStream, control.OpenStream{Kind: "http", ProxyID: proxy.ID, ConnectionID: requestID, TargetHost: proxy.TargetHost, TargetPort: proxy.TargetPort}); err != nil {
		nethttp.Error(w, "write proxy stream failed", nethttp.StatusBadGateway)
		return
	}
	if r.Body != nil {
		r.Body = &countingReadCloser{ReadCloser: r.Body, count: &uploadBytes}
	}
	if err := r.Write(stream); err != nil {
		nethttp.Error(w, "write proxy request failed", nethttp.StatusBadGateway)
		return
	}
	responseReader := bufio.NewReader(stream)
	response, err := nethttp.ReadResponse(responseReader, r)
	if err != nil {
		nethttp.Error(w, "read proxy response failed", nethttp.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	statusCode = response.StatusCode
	if tunnel.IsWebSocketUpgrade(r.Header) && response.StatusCode == nethttp.StatusSwitchingProtocols {
		failed = false
		conn, rw, err := nethttp.NewResponseController(w).Hijack()
		if err != nil {
			return
		}
		if err := response.Write(rw); err != nil {
			_ = conn.Close()
			return
		}
		if err := rw.Flush(); err != nil {
			_ = conn.Close()
			return
		}
		publicSide := tunnel.WithBufferedReader(conn, rw.Reader)
		streamSide := tunnel.WithBufferedReader(stream, responseReader)
		tunnelUploadBytes, tunnelDownloadBytes := tunnel.CopyBidirectional(publicSide, streamSide)
		uploadBytes += tunnelUploadBytes
		downloadBytes += tunnelDownloadBytes
		return
	}
	copyHeader(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	bytes, err := io.Copy(w, response.Body)
	downloadBytes = bytes
	failed = err != nil || response.StatusCode >= 500
}

func displayBindHost(host string) string {
	host = domain.NormalizeBindHost(host)
	if host == "" {
		return "*"
	}
	return host
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

type countingReadCloser struct {
	io.ReadCloser
	count *int64
}

func (reader *countingReadCloser) Read(p []byte) (int, error) {
	n, err := reader.ReadCloser.Read(p)
	*reader.count += int64(n)
	return n, err
}
