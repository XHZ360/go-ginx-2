package enrollmentapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/enrollment"
)

const ClientEnrollmentPath = "/api/client/enroll"

type Server struct {
	listener   net.Listener
	httpServer *http.Server
	enrollment enrollment.Service
}

type Entry struct {
	ListenAddress string
	Enrollment    enrollment.Service
}

type apiError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

type apiErrorResponse struct {
	Error apiError `json:"error"`
}

func Listen(entry Entry) (*Server, error) {
	listener, err := net.Listen("tcp", entry.ListenAddress)
	if err != nil {
		return nil, err
	}
	server := &Server{listener: listener, enrollment: entry.Enrollment}
	mux := http.NewServeMux()
	mux.HandleFunc(ClientEnrollmentPath, server.enrollHandler)
	server.httpServer = &http.Server{Handler: mux}
	return server, nil
}

func (server *Server) Addr() net.Addr { return server.listener.Addr() }

func (server *Server) Serve(ctx context.Context) error {
	done := make(chan error, 1)
	go func() { done <- server.httpServer.Serve(server.listener) }()
	select {
	case <-ctx.Done():
		_ = server.httpServer.Close()
		return ctx.Err()
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (server *Server) Close() error {
	if server == nil || server.httpServer == nil {
		return nil
	}
	return server.httpServer.Close()
}

func (server *Server) enrollHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorJSON(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	var request enrollment.RedeemRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), nil)
		return
	}
	response, err := server.enrollment.Redeem(r.Context(), request.Token)
	if err != nil {
		writeErrorJSON(w, enrollment.HTTPStatusForError(err), contracterr.CodeUnauthenticated, "invalid or expired enrollment token", nil)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeErrorJSON(w http.ResponseWriter, status int, code string, message string, fields map[string]string) {
	writeJSON(w, status, apiErrorResponse{Error: apiError{Code: code, Message: message, Fields: fields}})
}
