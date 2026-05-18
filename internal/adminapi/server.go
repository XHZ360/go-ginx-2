package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/gqlerrors"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/parser"
	"github.com/graphql-go/graphql/language/source"
	"github.com/simp-frp/go-ginx-2/internal/admin"
	"github.com/simp-frp/go-ginx-2/internal/adminquery"
	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/deploypath"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/enrollment"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

const (
	adminAPIPrefix          = "/api/admin"
	clientEnrollmentPrefix  = "/api/client"
	defaultBinaryDir        = "bin"
	defaultAdminFrontendDir = "admin-ui"
	adminSessionCookieName  = "goginx_admin_session"
	adminSessionCookiePath  = "/api/admin"
	adminCSRFHeader         = "X-GoGinx-CSRF-Token"
	defaultPollSeconds      = 5
)

var executablePath = os.Executable

type Server struct {
	query      adminquery.Service
	commands   admin.Service
	listener   net.Listener
	httpServer *http.Server
	schema     graphql.Schema
	creds      credentialVerifier
	sessions   *sessionManager
	frontend   *adminFrontend
	enrollment enrollment.Service
}

type Entry struct {
	ListenAddress           string
	AdminCredentialsFile    string
	AdminFrontendDir        string
	Credentials             credentialVerifier
	Enrollment              enrollment.Service
	Query                   adminquery.Service
	Commands                admin.Service
	SessionIdleTimeout      time.Duration
	SessionAbsoluteLifetime time.Duration
	Now                     func() time.Time
}

type adminFrontend struct {
	indexBytes []byte
	indexModAt time.Time
	fileSystem http.FileSystem
}

type credentialsFile struct {
	Administrators []administratorCredential `json:"administrators"`
}

type administratorCredential struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
}

type credentialSet map[string]string

type credentialVerifier interface {
	Verify(ctx context.Context, username string, password string) bool
}

type sqliteCredentialVerifier struct {
	Store store.Store
}

type contextKey string

const actorContextKey contextKey = "admin_actor"

type apiError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

type apiErrorResponse struct {
	Error apiError `json:"error"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type sessionBootstrapResponse struct {
	Authenticated      bool   `json:"authenticated"`
	Username           string `json:"username,omitempty"`
	CSRFToken          string `json:"csrfToken,omitempty"`
	PollIntervalSecond int    `json:"pollIntervalSeconds,omitempty"`
}

type graphqlRequest struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
}

type graphqlContractError struct {
	code    string
	message string
	fields  map[string]string
	err     error
}

type clientPayload struct {
	Client     adminquery.ClientDetail
	Credential string
	Token      string
}

type proxyPayload struct {
	Proxy  adminquery.ProxyDetail
	ID     string
	Status string
}

type userPayload struct {
	User   adminquery.UserDetail
	ID     string
	Status string
}

type certificatePayload struct {
	Certificate adminquery.ManagedCertificateSummary
	ProxyID     string
	Status      string
}

func Listen(entry Entry) (*Server, error) {
	frontend, err := loadAdminFrontend(entry.AdminFrontendDir)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", entry.ListenAddress)
	if err != nil {
		return nil, err
	}
	creds, err := credentialsForEntry(entry)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	server := &Server{
		query:      entry.Query,
		commands:   entry.Commands,
		listener:   listener,
		creds:      creds,
		sessions:   newSessionManager(entry.SessionIdleTimeout, entry.SessionAbsoluteLifetime, entry.Now),
		frontend:   frontend,
		enrollment: entry.Enrollment,
	}
	schema, err := server.buildSchema()
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	server.schema = schema
	mux := http.NewServeMux()
	mux.HandleFunc(adminAPIPrefix+"/login", server.loginHandler)
	mux.HandleFunc(adminAPIPrefix+"/session", server.sessionHandler)
	mux.HandleFunc(adminAPIPrefix+"/logout", server.logoutHandler)
	mux.HandleFunc(adminAPIPrefix+"/graphql", server.graphqlHandler)
	mux.HandleFunc(clientEnrollmentPrefix+"/enroll", server.enrollHandler)
	server.httpServer = &http.Server{Handler: server.routeHandler(mux)}
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

func (server *Server) Close() error { return server.httpServer.Close() }

func loadCredentials(path string) (credentialSet, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file credentialsFile
	if err := json.Unmarshal(content, &file); err != nil {
		return nil, err
	}
	creds := make(credentialSet, len(file.Administrators))
	for _, adminCredential := range file.Administrators {
		if strings.TrimSpace(adminCredential.Username) == "" || strings.TrimSpace(adminCredential.PasswordHash) == "" {
			return nil, errors.New("administrator username and password_hash are required")
		}
		creds[adminCredential.Username] = adminCredential.PasswordHash
	}
	if len(creds) == 0 {
		return nil, errors.New("at least one administrator credential is required")
	}
	return creds, nil
}

func credentialsForEntry(entry Entry) (credentialVerifier, error) {
	if entry.Credentials != nil {
		return entry.Credentials, nil
	}
	if strings.TrimSpace(entry.AdminCredentialsFile) != "" {
		return loadCredentials(entry.AdminCredentialsFile)
	}
	if entry.Query.Store != nil {
		return sqliteCredentialVerifier{Store: entry.Query.Store}, nil
	}
	return nil, errors.New("admin credentials source is required")
}

func (creds credentialSet) Verify(_ context.Context, username string, password string) bool {
	return domain.CheckPasswordHash(password, creds[username])
}

func (verifier sqliteCredentialVerifier) Verify(ctx context.Context, username string, password string) bool {
	if verifier.Store == nil {
		return false
	}
	user, err := verifier.Store.Users().ByUsername(ctx, username)
	if err != nil {
		return false
	}
	if user.Role != domain.RoleAdmin || user.Status != domain.UserEnabled {
		return false
	}
	return domain.CheckPasswordHash(password, user.PasswordHash)
}

func (server *Server) routeHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requestUsesProtectedTransport(r) {
			writeErrorJSON(w, http.StatusUpgradeRequired, "PROTECTED_TRANSPORT_REQUIRED", "management endpoint requires protected transport", nil)
			return
		}
		if !strings.HasPrefix(r.URL.Path, adminAPIPrefix) && !strings.HasPrefix(r.URL.Path, clientEnrollmentPrefix) {
			if server.serveFrontend(w, r) {
				return
			}
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loadAdminFrontend(dir string) (*adminFrontend, error) {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		defaultRoot, err := defaultAdminFrontendPath()
		if err != nil {
			return nil, err
		}
		trimmed = defaultRoot
	}
	absRoot, err := filepath.Abs(trimmed)
	if err != nil {
		return nil, fmt.Errorf("resolve admin frontend dir: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("stat admin frontend dir %q: %w", absRoot, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("admin frontend dir %q must be a directory", absRoot)
	}
	indexPath := filepath.Join(absRoot, "index.html")
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("read admin frontend index %q: %w", indexPath, err)
	}
	indexInfo, err := os.Stat(indexPath)
	if err != nil {
		return nil, fmt.Errorf("stat admin frontend index %q: %w", indexPath, err)
	}
	return &adminFrontend{indexBytes: indexBytes, indexModAt: indexInfo.ModTime(), fileSystem: http.Dir(absRoot)}, nil
}

func defaultAdminFrontendPath() (string, error) {
	deploymentRoot, err := deploypath.Root(executablePath)
	if err != nil {
		return "", err
	}
	return filepath.Join(deploymentRoot, defaultAdminFrontendDir), nil
}

func (server *Server) serveFrontend(w http.ResponseWriter, r *http.Request) bool {
	if server.frontend == nil {
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	cleanPath, ok := normalizeFrontendPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return true
	}
	if cleanPath != "" {
		if server.serveFrontendFile(w, r, cleanPath) {
			return true
		}
		if frontendPathLooksLikeAsset(cleanPath) {
			http.NotFound(w, r)
			return true
		}
	}
	server.serveFrontendIndex(w, r)
	return true
}

func normalizeFrontendPath(requestPath string) (string, bool) {
	if requestPath == "" {
		return "", true
	}
	cleaned := path.Clean("/" + requestPath)
	if cleaned == "/" {
		return "", true
	}
	trimmed := strings.TrimPrefix(cleaned, "/")
	if trimmed == "" || trimmed == "." || strings.HasPrefix(trimmed, "../") || trimmed == ".." {
		return "", false
	}
	return trimmed, true
}

func (server *Server) serveFrontendFile(w http.ResponseWriter, r *http.Request, relativePath string) bool {
	file, err := server.frontend.fileSystem.Open(relativePath)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return false
	}
	seeker, ok := file.(io.ReadSeeker)
	if !ok {
		return false
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), seeker)
	return true
}

func (server *Server) serveFrontendIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", server.frontend.indexModAt, bytes.NewReader(server.frontend.indexBytes))
}

func frontendPathLooksLikeAsset(relativePath string) bool {
	base := path.Base(relativePath)
	return strings.Contains(base, ".")
}

func requestUsesProtectedTransport(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func actorFromContext(ctx context.Context) string {
	actor, _ := ctx.Value(actorContextKey).(string)
	return actor
}

func contextWithActor(ctx context.Context, actor string) context.Context {
	return context.WithValue(ctx, actorContextKey, actor)
}

func (server *Server) loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorJSON(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	var request loginRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), nil)
		return
	}
	fields := make(map[string]string)
	if strings.TrimSpace(request.Username) == "" {
		fields["username"] = "username is required"
	}
	if strings.TrimSpace(request.Password) == "" {
		fields["password"] = "password is required"
	}
	if len(fields) > 0 {
		writeErrorJSON(w, http.StatusBadRequest, contracterr.CodeValidationFailed, "validation failed", fields)
		return
	}
	if !server.creds.Verify(r.Context(), request.Username, request.Password) {
		writeErrorJSON(w, http.StatusUnauthorized, contracterr.CodeUnauthenticated, "invalid administrator credentials", nil)
		return
	}
	if existingID, ok := server.sessionIDFromRequest(r); ok {
		server.sessions.Invalidate(existingID)
	}
	session, err := server.sessions.Create(request.Username)
	if err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, contracterr.CodeInternal, "internal server error", nil)
		return
	}
	server.writeSessionCookie(w, r, session)
	writeBootstrapJSON(w, http.StatusOK, &session)
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

func (server *Server) sessionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorJSON(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	session, ok := server.authenticateSession(w, r)
	if !ok {
		writeBootstrapJSON(w, http.StatusOK, nil)
		return
	}
	writeBootstrapJSON(w, http.StatusOK, &session)
}

func (server *Server) logoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorJSON(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	sessionID, ok := server.sessionIDFromRequest(r)
	if !ok {
		server.clearSessionCookie(w, r)
		writeBootstrapJSON(w, http.StatusOK, nil)
		return
	}
	session, sessionOK := server.sessions.Get(sessionID)
	if sessionOK {
		if !server.hasValidCSRFToken(r, session) {
			writeErrorJSON(w, http.StatusForbidden, "INVALID_CSRF", "csrf token is invalid", nil)
			return
		}
	}
	server.sessions.Invalidate(sessionID)
	server.clearSessionCookie(w, r)
	writeBootstrapJSON(w, http.StatusOK, nil)
}

func (server *Server) graphqlHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorJSON(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	session, ok := server.authenticateSession(w, r)
	if !ok {
		server.writeGraphQLError(w, http.StatusUnauthorized, newGraphQLContractError(contracterr.CodeUnauthenticated, "administrator session is required", nil, nil))
		return
	}
	var request graphqlRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeGraphQLError(w, http.StatusBadRequest, newGraphQLContractError(contracterr.CodeValidationFailed, "invalid graphql request", nil, err))
		return
	}
	operationType, err := graphqlOperationType(request)
	if err != nil {
		server.writeGraphQLError(w, http.StatusBadRequest, newGraphQLContractError(contracterr.CodeValidationFailed, "invalid graphql operation", nil, err))
		return
	}
	if operationType == "mutation" && !server.hasValidCSRFToken(r, session) {
		server.writeGraphQLError(w, http.StatusForbidden, newGraphQLContractError(contracterr.CodeForbidden, "csrf token is invalid", nil, nil))
		return
	}
	result := graphql.Do(graphql.Params{
		Schema:         server.schema,
		RequestString:  request.Query,
		VariableValues: request.Variables,
		OperationName:  request.OperationName,
		Context:        contextWithActor(r.Context(), session.Username),
	})
	result.Errors = sanitizeGraphQLErrors(result.Errors)
	writeJSON(w, http.StatusOK, result)
}

func (server *Server) sessionIDFromRequest(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(adminSessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", false
	}
	return cookie.Value, true
}

func (server *Server) authenticateSession(w http.ResponseWriter, r *http.Request) (administratorSession, bool) {
	sessionID, ok := server.sessionIDFromRequest(r)
	if !ok {
		return administratorSession{}, false
	}
	session, ok := server.sessions.Get(sessionID)
	if !ok {
		server.clearSessionCookie(w, r)
		return administratorSession{}, false
	}
	return session, true
}

func (server *Server) hasValidCSRFToken(r *http.Request, session administratorSession) bool {
	return subtleConstantTimeEquals(r.Header.Get(adminCSRFHeader), session.CSRFToken)
}

func (server *Server) writeSessionCookie(w http.ResponseWriter, r *http.Request, session administratorSession) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    session.ID,
		Path:     adminSessionCookiePath,
		HttpOnly: true,
		Secure:   requestHasTLSCookieContext(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (server *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    "",
		Path:     adminSessionCookiePath,
		HttpOnly: true,
		Secure:   requestHasTLSCookieContext(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func requestHasTLSCookieContext(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func writeBootstrapJSON(w http.ResponseWriter, status int, session *administratorSession) {
	response := sessionBootstrapResponse{Authenticated: false}
	if session != nil {
		response.Authenticated = true
		response.Username = session.Username
		response.CSRFToken = session.CSRFToken
		response.PollIntervalSecond = defaultPollSeconds
	}
	writeJSON(w, status, response)
}

func writeErrorJSON(w http.ResponseWriter, status int, code string, message string, fields map[string]string) {
	writeJSON(w, status, apiErrorResponse{Error: apiError{Code: code, Message: message, Fields: fields}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func subtleConstantTimeEquals(left string, right string) bool {
	if len(left) != len(right) {
		return false
	}
	var result byte
	for index := 0; index < len(left); index++ {
		result |= left[index] ^ right[index]
	}
	return result == 0
}

func graphqlOperationType(request graphqlRequest) (string, error) {
	document, err := parser.Parse(parser.ParseParams{Source: source.NewSource(&source.Source{Body: []byte(request.Query), Name: "GraphQL request"})})
	if err != nil {
		return "", err
	}
	operations := make([]*ast.OperationDefinition, 0)
	for _, definition := range document.Definitions {
		operation, ok := definition.(*ast.OperationDefinition)
		if ok {
			operations = append(operations, operation)
		}
	}
	if len(operations) == 0 {
		return "", errors.New("graphql document contains no operations")
	}
	if strings.TrimSpace(request.OperationName) != "" {
		for _, operation := range operations {
			if operation.Name != nil && operation.Name.Value == request.OperationName {
				return operation.Operation, nil
			}
		}
		return "", errors.New("graphql operationName was not found")
	}
	if len(operations) > 1 {
		return "", errors.New("graphql operationName is required when multiple operations are present")
	}
	return operations[0].Operation, nil
}

func (server *Server) buildSchema() (graphql.Schema, error) {
	pageInfoType := graphql.NewObject(graphql.ObjectConfig{Name: "AdminPageInfo", Fields: graphql.Fields{
		"page":       &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		"pageSize":   &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		"totalPages": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		"hasNext":    &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
		"hasPrev":    &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
	}})
	managedCertificateType := graphql.NewObject(graphql.ObjectConfig{Name: "AdminManagedCertificate", Fields: graphql.Fields{
		"proxyId":       &graphql.Field{Type: graphql.String},
		"certificateId": &graphql.Field{Type: graphql.String},
		"host":          &graphql.Field{Type: graphql.String},
		"status":        &graphql.Field{Type: graphql.String},
		"notAfter":      &graphql.Field{Type: graphql.String, Resolve: nullableCertificateTimeResolve(func(value adminquery.ManagedCertificateSummary) *time.Time { return value.NotAfter })},
		"lastIssuedAt":  &graphql.Field{Type: graphql.String, Resolve: nullableCertificateTimeResolve(func(value adminquery.ManagedCertificateSummary) *time.Time { return value.LastIssuedAt })},
		"lastRenewedAt": &graphql.Field{Type: graphql.String, Resolve: nullableCertificateTimeResolve(func(value adminquery.ManagedCertificateSummary) *time.Time { return value.LastRenewedAt })},
		"lastError":     &graphql.Field{Type: graphql.String},
	}})
	auditType := graphql.NewObject(graphql.ObjectConfig{Name: "AdminAuditEvent", Fields: graphql.Fields{
		"id":           &graphql.Field{Type: graphql.String},
		"actorType":    &graphql.Field{Type: graphql.String},
		"actorId":      &graphql.Field{Type: graphql.String},
		"resourceType": &graphql.Field{Type: graphql.String},
		"resourceId":   &graphql.Field{Type: graphql.String},
		"action":       &graphql.Field{Type: graphql.String},
		"result":       &graphql.Field{Type: graphql.String},
		"createdAt":    &graphql.Field{Type: graphql.String, Resolve: timeResolve(func(event adminquery.AuditListItem) time.Time { return event.CreatedAt })},
	}})
	clientRuntimeType := graphql.NewObject(graphql.ObjectConfig{Name: "AdminClientRuntime", Fields: graphql.Fields{
		"online":        &graphql.Field{Type: graphql.Boolean},
		"protocol":      &graphql.Field{Type: graphql.String},
		"connectedAt":   &graphql.Field{Type: graphql.String, Resolve: nullableClientRuntimeTimeResolve(func(value adminquery.ClientRuntime) *time.Time { return value.ConnectedAt })},
		"lastHeartbeat": &graphql.Field{Type: graphql.String, Resolve: nullableClientRuntimeTimeResolve(func(value adminquery.ClientRuntime) *time.Time { return value.LastHeartbeat })},
		"configVersion": &graphql.Field{Type: graphql.Int},
		"activeProxies": &graphql.Field{Type: graphql.Int},
		"activeStreams": &graphql.Field{Type: graphql.Int},
		"uploadBytes":   &graphql.Field{Type: graphql.Int},
		"downloadBytes": &graphql.Field{Type: graphql.Int},
		"errorSummary":  &graphql.Field{Type: graphql.String},
	}})
	proxySummaryType := graphql.NewObject(graphql.ObjectConfig{Name: "AdminProxySummary", Fields: graphql.Fields{
		"id":                   &graphql.Field{Type: graphql.String},
		"name":                 &graphql.Field{Type: graphql.String},
		"type":                 &graphql.Field{Type: graphql.String},
		"status":               &graphql.Field{Type: graphql.String},
		"runtimeStatus":        &graphql.Field{Type: graphql.String},
		"entryHost":            &graphql.Field{Type: graphql.String},
		"entryPort":            &graphql.Field{Type: graphql.Int},
		"targetHost":           &graphql.Field{Type: graphql.String},
		"targetPort":           &graphql.Field{Type: graphql.Int},
		"activeTCPConnections": &graphql.Field{Type: graphql.Int},
	}})
	clientType := graphql.NewObject(graphql.ObjectConfig{Name: "AdminClient", Fields: graphql.Fields{
		"id":            &graphql.Field{Type: graphql.String},
		"userId":        &graphql.Field{Type: graphql.String},
		"name":          &graphql.Field{Type: graphql.String},
		"status":        &graphql.Field{Type: graphql.String},
		"version":       &graphql.Field{Type: graphql.Int},
		"runtime":       &graphql.Field{Type: clientRuntimeType},
		"lastOnlineAt":  &graphql.Field{Type: graphql.String, Resolve: nullableTimeResolveClientOnline()},
		"lastOfflineAt": &graphql.Field{Type: graphql.String, Resolve: nullableTimeResolveClientOffline()},
		"createdAt":     &graphql.Field{Type: graphql.String, Resolve: clientCreatedAtResolve()},
		"updatedAt":     &graphql.Field{Type: graphql.String, Resolve: clientUpdatedAtResolve()},
	}})
	clientDetailType := graphql.NewObject(graphql.ObjectConfig{Name: "AdminClientDetail", Fields: graphql.Fields{
		"id":             &graphql.Field{Type: graphql.String},
		"userId":         &graphql.Field{Type: graphql.String},
		"name":           &graphql.Field{Type: graphql.String},
		"status":         &graphql.Field{Type: graphql.String},
		"version":        &graphql.Field{Type: graphql.Int},
		"runtime":        &graphql.Field{Type: clientRuntimeType},
		"lastOnlineAt":   &graphql.Field{Type: graphql.String, Resolve: clientDetailTimeResolve(func(value adminquery.ClientDetail) *time.Time { return value.LastOnlineAt })},
		"lastOfflineAt":  &graphql.Field{Type: graphql.String, Resolve: clientDetailTimeResolve(func(value adminquery.ClientDetail) *time.Time { return value.LastOfflineAt })},
		"managedProxies": &graphql.Field{Type: graphql.NewList(proxySummaryType)},
		"createdAt":      &graphql.Field{Type: graphql.String, Resolve: timeResolve(func(value adminquery.ClientDetail) time.Time { return value.CreatedAt })},
		"updatedAt":      &graphql.Field{Type: graphql.String, Resolve: timeResolve(func(value adminquery.ClientDetail) time.Time { return value.UpdatedAt })},
	}})
	userType := graphql.NewObject(graphql.ObjectConfig{Name: "AdminUser", Fields: graphql.Fields{
		"id":              &graphql.Field{Type: graphql.String, Resolve: userIDResolve()},
		"username":        &graphql.Field{Type: graphql.String},
		"role":            &graphql.Field{Type: graphql.String},
		"status":          &graphql.Field{Type: graphql.String},
		"clientCount":     &graphql.Field{Type: graphql.Int},
		"proxyCount":      &graphql.Field{Type: graphql.Int},
		"uploadBytes":     &graphql.Field{Type: graphql.Int},
		"downloadBytes":   &graphql.Field{Type: graphql.Int},
		"lastActivityAt":  &graphql.Field{Type: graphql.String, Resolve: nullableUserTimeResolve(func(value adminquery.UserListItem) *time.Time { return value.LastActivityAt })},
		"hasPasswordHash": &graphql.Field{Type: graphql.Boolean},
		"createdAt":       &graphql.Field{Type: graphql.String, Resolve: userCreatedAtResolve()},
		"updatedAt":       &graphql.Field{Type: graphql.String, Resolve: userUpdatedAtResolve()},
	}})
	proxyConfigType := graphql.NewObject(graphql.ObjectConfig{Name: "AdminProxyConfig", Fields: graphql.Fields{
		"entryHost":  &graphql.Field{Type: graphql.String},
		"entryPort":  &graphql.Field{Type: graphql.Int},
		"targetHost": &graphql.Field{Type: graphql.String},
		"targetPort": &graphql.Field{Type: graphql.Int},
	}})
	proxyType := graphql.NewObject(graphql.ObjectConfig{Name: "AdminProxy", Fields: graphql.Fields{
		"id":                   &graphql.Field{Type: graphql.String},
		"userId":               &graphql.Field{Type: graphql.String},
		"clientId":             &graphql.Field{Type: graphql.String},
		"name":                 &graphql.Field{Type: graphql.String},
		"type":                 &graphql.Field{Type: graphql.String},
		"status":               &graphql.Field{Type: graphql.String},
		"description":          &graphql.Field{Type: graphql.String},
		"runtimeStatus":        &graphql.Field{Type: graphql.String},
		"activeTCPConnections": &graphql.Field{Type: graphql.Int},
		"uploadBytes":          &graphql.Field{Type: graphql.Int},
		"downloadBytes":        &graphql.Field{Type: graphql.Int},
		"tcpErrorCount":        &graphql.Field{Type: graphql.Int},
		"udpErrorCount":        &graphql.Field{Type: graphql.Int},
		"httpErrorCount":       &graphql.Field{Type: graphql.Int},
		"config":               &graphql.Field{Type: proxyConfigType},
		"certificate":          &graphql.Field{Type: managedCertificateType},
		"createdAt":            &graphql.Field{Type: graphql.String, Resolve: proxyCreatedAtResolve()},
		"updatedAt":            &graphql.Field{Type: graphql.String, Resolve: proxyUpdatedAtResolve()},
	}})
	dashboardType := graphql.NewObject(graphql.ObjectConfig{Name: "AdminDashboard", Fields: graphql.Fields{
		"onlineClientCount":        &graphql.Field{Type: graphql.Int},
		"enabledProxyCount":        &graphql.Field{Type: graphql.Int},
		"activeTCPConnectionCount": &graphql.Field{Type: graphql.Int},
		"cumulativeUploadBytes":    &graphql.Field{Type: graphql.Int},
		"cumulativeDownloadBytes":  &graphql.Field{Type: graphql.Int},
		"cumulativeTCPErrorCount":  &graphql.Field{Type: graphql.Int},
		"cumulativeUDPErrorCount":  &graphql.Field{Type: graphql.Int},
		"cumulativeHTTPErrorCount": &graphql.Field{Type: graphql.Int},
	}})

	paginationInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminPaginationInput", Fields: graphql.InputObjectConfigFieldMap{
		"page":     &graphql.InputObjectFieldConfig{Type: graphql.Int},
		"pageSize": &graphql.InputObjectFieldConfig{Type: graphql.Int},
	}})
	sortInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminSortInput", Fields: graphql.InputObjectConfigFieldMap{
		"field":     &graphql.InputObjectFieldConfig{Type: graphql.String},
		"direction": &graphql.InputObjectFieldConfig{Type: graphql.String},
	}})
	userFilterInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminUserFilterInput", Fields: graphql.InputObjectConfigFieldMap{
		"query":  &graphql.InputObjectFieldConfig{Type: graphql.String},
		"role":   &graphql.InputObjectFieldConfig{Type: graphql.String},
		"status": &graphql.InputObjectFieldConfig{Type: graphql.String},
	}})
	clientFilterInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminClientFilterInput", Fields: graphql.InputObjectConfigFieldMap{
		"query":  &graphql.InputObjectFieldConfig{Type: graphql.String},
		"userId": &graphql.InputObjectFieldConfig{Type: graphql.String},
		"status": &graphql.InputObjectFieldConfig{Type: graphql.String},
		"online": &graphql.InputObjectFieldConfig{Type: graphql.Boolean},
	}})
	proxyFilterInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminProxyFilterInput", Fields: graphql.InputObjectConfigFieldMap{
		"query":    &graphql.InputObjectFieldConfig{Type: graphql.String},
		"userId":   &graphql.InputObjectFieldConfig{Type: graphql.String},
		"clientId": &graphql.InputObjectFieldConfig{Type: graphql.String},
		"type":     &graphql.InputObjectFieldConfig{Type: graphql.String},
		"status":   &graphql.InputObjectFieldConfig{Type: graphql.String},
	}})
	certificateFilterInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminCertificateFilterInput", Fields: graphql.InputObjectConfigFieldMap{
		"query":  &graphql.InputObjectFieldConfig{Type: graphql.String},
		"status": &graphql.InputObjectFieldConfig{Type: graphql.String},
	}})
	auditFilterInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminAuditFilterInput", Fields: graphql.InputObjectConfigFieldMap{
		"query":        &graphql.InputObjectFieldConfig{Type: graphql.String},
		"actorType":    &graphql.InputObjectFieldConfig{Type: graphql.String},
		"actorId":      &graphql.InputObjectFieldConfig{Type: graphql.String},
		"resourceType": &graphql.InputObjectFieldConfig{Type: graphql.String},
		"action":       &graphql.InputObjectFieldConfig{Type: graphql.String},
		"result":       &graphql.InputObjectFieldConfig{Type: graphql.String},
	}})
	usersInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminUsersInput", Fields: graphql.InputObjectConfigFieldMap{
		"page":   &graphql.InputObjectFieldConfig{Type: paginationInput},
		"filter": &graphql.InputObjectFieldConfig{Type: userFilterInput},
		"sort":   &graphql.InputObjectFieldConfig{Type: sortInput},
	}})
	clientsInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminClientsInput", Fields: graphql.InputObjectConfigFieldMap{
		"page":   &graphql.InputObjectFieldConfig{Type: paginationInput},
		"filter": &graphql.InputObjectFieldConfig{Type: clientFilterInput},
		"sort":   &graphql.InputObjectFieldConfig{Type: sortInput},
	}})
	proxiesInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminProxiesInput", Fields: graphql.InputObjectConfigFieldMap{
		"page":   &graphql.InputObjectFieldConfig{Type: paginationInput},
		"filter": &graphql.InputObjectFieldConfig{Type: proxyFilterInput},
		"sort":   &graphql.InputObjectFieldConfig{Type: sortInput},
	}})
	certificatesInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminCertificatesInput", Fields: graphql.InputObjectConfigFieldMap{
		"page":   &graphql.InputObjectFieldConfig{Type: paginationInput},
		"filter": &graphql.InputObjectFieldConfig{Type: certificateFilterInput},
		"sort":   &graphql.InputObjectFieldConfig{Type: sortInput},
	}})
	auditInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminAuditInput", Fields: graphql.InputObjectConfigFieldMap{
		"page":   &graphql.InputObjectFieldConfig{Type: paginationInput},
		"filter": &graphql.InputObjectFieldConfig{Type: auditFilterInput},
		"sort":   &graphql.InputObjectFieldConfig{Type: sortInput},
	}})
	proxyConfigInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminProxyConfigInput", Fields: graphql.InputObjectConfigFieldMap{
		"entryHost":  &graphql.InputObjectFieldConfig{Type: graphql.String},
		"entryPort":  &graphql.InputObjectFieldConfig{Type: graphql.Int},
		"targetHost": &graphql.InputObjectFieldConfig{Type: graphql.String},
		"targetPort": &graphql.InputObjectFieldConfig{Type: graphql.Int},
		"certFile":   &graphql.InputObjectFieldConfig{Type: graphql.String},
		"keyFile":    &graphql.InputObjectFieldConfig{Type: graphql.String},
	}})
	createUserInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminCreateUserInput", Fields: graphql.InputObjectConfigFieldMap{
		"username": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"password": &graphql.InputObjectFieldConfig{Type: graphql.String},
		"role":     &graphql.InputObjectFieldConfig{Type: graphql.String},
	}})
	userIDInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminUserIDInput", Fields: graphql.InputObjectConfigFieldMap{
		"id": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
	}})
	setUserPasswordInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminSetUserPasswordInput", Fields: graphql.InputObjectConfigFieldMap{
		"id":       &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"password": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
	}})
	createClientInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminCreateClientInput", Fields: graphql.InputObjectConfigFieldMap{
		"userId":     &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"name":       &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"credential": &graphql.InputObjectFieldConfig{Type: graphql.String},
	}})
	createClientJoinInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminCreateClientJoinInput", Fields: graphql.InputObjectConfigFieldMap{
		"id":               &graphql.InputObjectFieldConfig{Type: graphql.String},
		"userId":           &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"name":             &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"enrollmentUrl":    &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"serverAddress":    &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"serverTLSAddress": &graphql.InputObjectFieldConfig{Type: graphql.String},
		"serverName":       &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"serverCAFile":     &graphql.InputObjectFieldConfig{Type: graphql.String},
		"ttlSeconds":       &graphql.InputObjectFieldConfig{Type: graphql.Int},
	}})
	createProxyInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminCreateProxyInput", Fields: graphql.InputObjectConfigFieldMap{
		"userId":      &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"clientId":    &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"name":        &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"type":        &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"description": &graphql.InputObjectFieldConfig{Type: graphql.String},
		"config":      &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(proxyConfigInput)},
	}})
	updateProxyInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminUpdateProxyInput", Fields: graphql.InputObjectConfigFieldMap{
		"id":          &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"type":        &graphql.InputObjectFieldConfig{Type: graphql.String},
		"name":        &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"description": &graphql.InputObjectFieldConfig{Type: graphql.String},
		"config":      &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(proxyConfigInput)},
	}})
	certificateInput := graphql.NewInputObject(graphql.InputObjectConfig{Name: "AdminCertificateMutationInput", Fields: graphql.InputObjectConfigFieldMap{
		"proxyId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
	}})

	usersPageType := graphql.NewObject(graphql.ObjectConfig{Name: "AdminUsersPage", Fields: graphql.Fields{
		"items":      &graphql.Field{Type: graphql.NewList(userType)},
		"totalCount": &graphql.Field{Type: graphql.Int},
		"pageInfo":   &graphql.Field{Type: pageInfoType},
	}})
	clientsPageType := graphql.NewObject(graphql.ObjectConfig{Name: "AdminClientsPage", Fields: graphql.Fields{
		"items":      &graphql.Field{Type: graphql.NewList(clientType)},
		"totalCount": &graphql.Field{Type: graphql.Int},
		"pageInfo":   &graphql.Field{Type: pageInfoType},
	}})
	proxiesPageType := graphql.NewObject(graphql.ObjectConfig{Name: "AdminProxiesPage", Fields: graphql.Fields{
		"items":      &graphql.Field{Type: graphql.NewList(proxyType)},
		"totalCount": &graphql.Field{Type: graphql.Int},
		"pageInfo":   &graphql.Field{Type: pageInfoType},
	}})
	certificatesPageType := graphql.NewObject(graphql.ObjectConfig{Name: "AdminCertificatesPage", Fields: graphql.Fields{
		"items":      &graphql.Field{Type: graphql.NewList(managedCertificateType)},
		"totalCount": &graphql.Field{Type: graphql.Int},
		"pageInfo":   &graphql.Field{Type: pageInfoType},
	}})
	auditPageType := graphql.NewObject(graphql.ObjectConfig{Name: "AdminAuditPage", Fields: graphql.Fields{
		"items":      &graphql.Field{Type: graphql.NewList(auditType)},
		"totalCount": &graphql.Field{Type: graphql.Int},
		"pageInfo":   &graphql.Field{Type: pageInfoType},
	}})
	userPayloadType := graphql.NewObject(graphql.ObjectConfig{Name: "AdminUserPayload", Fields: graphql.Fields{
		"user": &graphql.Field{Type: userType},
		"userId": &graphql.Field{Type: graphql.String, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return extractUserPayload(params.Source).ID, nil
		}},
		"status": &graphql.Field{Type: graphql.String, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return extractUserPayload(params.Source).Status, nil
		}},
	}})
	clientPayloadType := graphql.NewObject(graphql.ObjectConfig{Name: "AdminClientPayload", Fields: graphql.Fields{
		"client": &graphql.Field{Type: clientDetailType, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return extractClientPayload(params.Source).Client, nil
		}},
		"clientId": &graphql.Field{Type: graphql.String, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return extractClientPayload(params.Source).Client.ID, nil
		}},
		"credential": &graphql.Field{Type: graphql.String, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return extractClientPayload(params.Source).Credential, nil
		}},
		"token": &graphql.Field{Type: graphql.String, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return extractClientPayload(params.Source).Token, nil
		}},
	}})
	proxyPayloadType := graphql.NewObject(graphql.ObjectConfig{Name: "AdminProxyPayload", Fields: graphql.Fields{
		"proxy": &graphql.Field{Type: proxyType, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return extractProxyPayload(params.Source).Proxy, nil
		}},
		"proxyId": &graphql.Field{Type: graphql.String, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return extractProxyPayload(params.Source).ID, nil
		}},
		"status": &graphql.Field{Type: graphql.String, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return extractProxyPayload(params.Source).Status, nil
		}},
	}})
	certificatePayloadType := graphql.NewObject(graphql.ObjectConfig{Name: "AdminCertificatePayload", Fields: graphql.Fields{
		"certificate": &graphql.Field{Type: managedCertificateType, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return extractCertificatePayload(params.Source).Certificate, nil
		}},
		"proxyId": &graphql.Field{Type: graphql.String, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return extractCertificatePayload(params.Source).ProxyID, nil
		}},
		"status": &graphql.Field{Type: graphql.String, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return extractCertificatePayload(params.Source).Status, nil
		}},
	}})

	query := graphql.NewObject(graphql.ObjectConfig{Name: "Query", Fields: graphql.Fields{
		"dashboard": &graphql.Field{Type: dashboardType, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			return server.query.DashboardSummary(params.Context)
		})},
		"users": &graphql.Field{Type: usersPageType, Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: usersInput}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			return server.query.ListUsers(params.Context, userListInputFromArgs(params.Args))
		})},
		"user": &graphql.Field{Type: userType, Args: graphql.FieldConfigArgument{"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			return server.query.UserDetail(params.Context, params.Args["id"].(string))
		})},
		"clients": &graphql.Field{Type: clientsPageType, Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: clientsInput}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			return server.query.ListClients(params.Context, clientListInputFromArgs(params.Args))
		})},
		"client": &graphql.Field{Type: clientDetailType, Args: graphql.FieldConfigArgument{"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			return server.query.ClientDetail(params.Context, params.Args["id"].(string))
		})},
		"proxies": &graphql.Field{Type: proxiesPageType, Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: proxiesInput}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			return server.query.ListProxies(params.Context, proxyListInputFromArgs(params.Args))
		})},
		"proxy": &graphql.Field{Type: proxyType, Args: graphql.FieldConfigArgument{"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			return server.query.ProxyDetail(params.Context, params.Args["id"].(string))
		})},
		"certificates": &graphql.Field{Type: certificatesPageType, Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: certificatesInput}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			return server.query.ListManagedCertificates(params.Context, certificateListInputFromArgs(params.Args))
		})},
		"audit": &graphql.Field{Type: auditPageType, Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: auditInput}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			return server.query.ListRecentAuditEvents(params.Context, auditListInputFromArgs(params.Args))
		})},
	}})
	mutation := graphql.NewObject(graphql.ObjectConfig{Name: "Mutation", Fields: graphql.Fields{
		"createUser": &graphql.Field{Type: userPayloadType, Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(createUserInput)}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			input := mapArg(params.Args, "input")
			created, err := server.commands.CreateUser(params.Context, admin.CreateUserInput{Username: stringValue(input, "username"), Password: stringValue(input, "password"), Role: domain.Role(defaultString(stringValue(input, "role"), string(domain.RoleUser))), ActorID: actorFromContext(params.Context)})
			if err != nil {
				return nil, err
			}
			detail, err := server.query.UserDetail(params.Context, created.ID)
			if err != nil {
				return nil, err
			}
			return userPayload{User: detail, ID: created.ID, Status: string(detail.Status)}, nil
		})},
		"disableUser": &graphql.Field{Type: userPayloadType, Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(userIDInput)}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			input := mapArg(params.Args, "input")
			userID := stringValue(input, "id")
			if err := server.commands.DisableUser(params.Context, userID, actorFromContext(params.Context)); err != nil {
				return nil, err
			}
			detail, err := server.query.UserDetail(params.Context, userID)
			if err != nil {
				return nil, err
			}
			return userPayload{User: detail, ID: userID, Status: string(detail.Status)}, nil
		})},
		"setUserPassword": &graphql.Field{Type: userPayloadType, Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(setUserPasswordInput)}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			input := mapArg(params.Args, "input")
			userID := stringValue(input, "id")
			if err := server.commands.SetUserPassword(params.Context, userID, stringValue(input, "password"), actorFromContext(params.Context)); err != nil {
				return nil, err
			}
			detail, err := server.query.UserDetail(params.Context, userID)
			if err != nil {
				return nil, err
			}
			return userPayload{User: detail, ID: userID, Status: string(detail.Status)}, nil
		})},
		"createClient": &graphql.Field{Type: clientPayloadType, Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(createClientInput)}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			input := mapArg(params.Args, "input")
			created, err := server.commands.CreateClientWithCredential(params.Context, admin.CreateClientInput{UserID: stringValue(input, "userId"), Name: stringValue(input, "name"), Credential: stringValue(input, "credential"), ActorID: actorFromContext(params.Context)})
			if err != nil {
				return nil, err
			}
			detail, err := server.query.ClientDetail(params.Context, created.Client.ID)
			if err != nil {
				return nil, err
			}
			return clientPayload{Client: detail, Credential: created.Credential}, nil
		})},
		"createClientJoin": &graphql.Field{Type: clientPayloadType, Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(createClientJoinInput)}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			created, err := server.commands.CreateClientJoin(params.Context, createClientJoinInputFromArgs(mapArg(params.Args, "input"), actorFromContext(params.Context)))
			if err != nil {
				return nil, err
			}
			detail, err := server.query.ClientDetail(params.Context, created.Client.ID)
			if err != nil {
				return nil, err
			}
			return clientPayload{Client: detail, Token: created.Token}, nil
		})},
		"enableClient": &graphql.Field{Type: clientPayloadType, Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(userIDInput)}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			clientID := stringValue(mapArg(params.Args, "input"), "id")
			if err := server.commands.EnableClient(params.Context, clientID, actorFromContext(params.Context)); err != nil {
				return nil, err
			}
			detail, err := server.query.ClientDetail(params.Context, clientID)
			if err != nil {
				return nil, err
			}
			return clientPayload{Client: detail}, nil
		})},
		"disableClient": &graphql.Field{Type: clientPayloadType, Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(userIDInput)}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			clientID := stringValue(mapArg(params.Args, "input"), "id")
			if err := server.commands.DisableClient(params.Context, clientID, actorFromContext(params.Context)); err != nil {
				return nil, err
			}
			detail, err := server.query.ClientDetail(params.Context, clientID)
			if err != nil {
				return nil, err
			}
			return clientPayload{Client: detail}, nil
		})},
		"rotateClientCredential": &graphql.Field{Type: clientPayloadType, Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(userIDInput)}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			clientID := stringValue(mapArg(params.Args, "input"), "id")
			rotated, err := server.commands.RotateClientCredential(params.Context, admin.RotateClientCredentialInput{ClientID: clientID, ActorID: actorFromContext(params.Context)})
			if err != nil {
				return nil, err
			}
			detail, err := server.query.ClientDetail(params.Context, clientID)
			if err != nil {
				return nil, err
			}
			return clientPayload{Client: detail, Credential: rotated.Credential}, nil
		})},
		"createProxy": &graphql.Field{Type: proxyPayloadType, Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(createProxyInput)}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			created, err := server.commands.CreateProxy(params.Context, createProxyInputFromArgs(mapArg(params.Args, "input"), actorFromContext(params.Context)))
			if err != nil {
				return nil, err
			}
			detail, err := server.query.ProxyDetail(params.Context, created.ID)
			if err != nil {
				return nil, err
			}
			return proxyPayload{Proxy: detail, ID: created.ID, Status: string(detail.Status)}, nil
		})},
		"updateProxy": &graphql.Field{Type: proxyPayloadType, Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(updateProxyInput)}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			updated, err := server.commands.UpdateProxy(params.Context, updateProxyInputFromArgs(mapArg(params.Args, "input"), actorFromContext(params.Context)))
			if err != nil {
				return nil, err
			}
			detail, err := server.query.ProxyDetail(params.Context, updated.ID)
			if err != nil {
				return nil, err
			}
			return proxyPayload{Proxy: detail, ID: updated.ID, Status: string(detail.Status)}, nil
		})},
		"enableProxy": &graphql.Field{Type: proxyPayloadType, Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(userIDInput)}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			proxyID := stringValue(mapArg(params.Args, "input"), "id")
			if err := server.commands.EnableProxy(params.Context, proxyID, actorFromContext(params.Context)); err != nil {
				return nil, err
			}
			detail, err := server.query.ProxyDetail(params.Context, proxyID)
			if err != nil {
				return nil, err
			}
			return proxyPayload{Proxy: detail, ID: proxyID, Status: string(detail.Status)}, nil
		})},
		"disableProxy": &graphql.Field{Type: proxyPayloadType, Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(userIDInput)}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			proxyID := stringValue(mapArg(params.Args, "input"), "id")
			if err := server.commands.DisableProxy(params.Context, proxyID, actorFromContext(params.Context)); err != nil {
				return nil, err
			}
			detail, err := server.query.ProxyDetail(params.Context, proxyID)
			if err != nil {
				return nil, err
			}
			return proxyPayload{Proxy: detail, ID: proxyID, Status: string(detail.Status)}, nil
		})},
		"deleteProxy": &graphql.Field{Type: proxyPayloadType, Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(userIDInput)}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			proxyID := stringValue(mapArg(params.Args, "input"), "id")
			if err := server.commands.DeleteProxy(params.Context, proxyID, actorFromContext(params.Context)); err != nil {
				return nil, err
			}
			return proxyPayload{ID: proxyID, Status: string(domain.ProxyDisabled)}, nil
		})},
		"issueManagedCertificate": &graphql.Field{Type: certificatePayloadType, Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(certificateInput)}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			proxyID := stringValue(mapArg(params.Args, "input"), "proxyId")
			certificate, err := server.commands.IssueManagedCertificate(params.Context, admin.CertificateInput{ProxyID: proxyID, ActorID: actorFromContext(params.Context)})
			if err != nil {
				return nil, err
			}
			page, err := server.query.ListManagedCertificates(params.Context, adminquery.CertificateListInput{Page: adminquery.PageInput{Page: 1, PageSize: 1000}})
			if err != nil {
				return nil, err
			}
			return certificatePayload{Certificate: findCertificate(page.Items, certificate.ProxyID), ProxyID: proxyID, Status: string(certificate.Status)}, nil
		})},
		"renewManagedCertificate": &graphql.Field{Type: certificatePayloadType, Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(certificateInput)}}, Resolve: server.wrapResolve(func(params graphql.ResolveParams) (interface{}, error) {
			proxyID := stringValue(mapArg(params.Args, "input"), "proxyId")
			certificate, err := server.commands.RenewManagedCertificate(params.Context, admin.CertificateInput{ProxyID: proxyID, ActorID: actorFromContext(params.Context)})
			if err != nil {
				return nil, err
			}
			page, err := server.query.ListManagedCertificates(params.Context, adminquery.CertificateListInput{Page: adminquery.PageInput{Page: 1, PageSize: 1000}})
			if err != nil {
				return nil, err
			}
			return certificatePayload{Certificate: findCertificate(page.Items, certificate.ProxyID), ProxyID: proxyID, Status: string(certificate.Status)}, nil
		})},
	}})
	return graphql.NewSchema(graphql.SchemaConfig{Query: query, Mutation: mutation})
}

func (server *Server) wrapResolve(next graphql.FieldResolveFn) graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		value, err := next(params)
		if err != nil {
			return value, translateGraphQLError(err)
		}
		return value, nil
	}
}

func (server *Server) writeGraphQLError(w http.ResponseWriter, status int, err error) {
	formatted := formatGraphQLError(err)
	writeJSON(w, status, graphql.Result{Errors: []gqlerrors.FormattedError{formatted}})
}

func newGraphQLContractError(code string, message string, fields map[string]string, err error) error {
	return graphqlContractError{code: code, message: message, fields: fields, err: err}
}

func (err graphqlContractError) Error() string {
	if strings.TrimSpace(err.message) != "" {
		return err.message
	}
	if err.err != nil {
		return err.err.Error()
	}
	return contracterr.CodeInternal
}

func (err graphqlContractError) Unwrap() error { return err.err }

func (err graphqlContractError) Extensions() map[string]interface{} {
	extensions := map[string]interface{}{"code": err.code}
	if len(err.fields) > 0 {
		extensions["fields"] = err.fields
	}
	return extensions
}

func translateGraphQLError(err error) error {
	var contractError *contracterr.Error
	if errors.As(err, &contractError) {
		message := contractError.Message
		if strings.TrimSpace(message) == "" {
			message = safeMessage(contractError.Code)
		}
		return newGraphQLContractError(contractError.Code, message, contractError.Fields, err)
	}
	if errors.Is(err, domain.ErrEntryConflict) {
		return newGraphQLContractError(contracterr.CodeEntryConflict, "requested listener conflicts with an active listener", nil, err)
	}
	if errors.Is(err, store.ErrNotFound) {
		return newGraphQLContractError(contracterr.CodeNotFound, "resource was not found", nil, err)
	}
	if errors.Is(err, store.ErrAlreadyExists) || errors.Is(err, store.ErrConflict) {
		return newGraphQLContractError(contracterr.CodeConflict, "resource conflict", nil, err)
	}
	return newGraphQLContractError(contracterr.CodeInternal, safeMessage(contracterr.CodeInternal), nil, err)
}

func sanitizeGraphQLErrors(errorsIn []gqlerrors.FormattedError) []gqlerrors.FormattedError {
	if len(errorsIn) == 0 {
		return nil
	}
	result := make([]gqlerrors.FormattedError, 0, len(errorsIn))
	for _, current := range errorsIn {
		if code, ok := current.Extensions["code"].(string); ok && code != "" {
			result = append(result, current)
			continue
		}
		current.Extensions = map[string]interface{}{"code": contracterr.CodeValidationFailed}
		if strings.TrimSpace(current.Message) == "" {
			current.Message = "invalid graphql request"
		}
		result = append(result, current)
	}
	return result
}

func formatGraphQLError(err error) gqlerrors.FormattedError {
	if contractError, ok := err.(graphqlContractError); ok {
		formatted := gqlerrors.FormattedError{Message: contractError.Error(), Extensions: contractError.Extensions()}
		return formatted
	}
	formatted := gqlerrors.FormatError(err)
	if formatted.Extensions == nil {
		formatted.Extensions = map[string]interface{}{}
	}
	if _, ok := formatted.Extensions["code"]; !ok {
		formatted.Extensions["code"] = contracterr.CodeInternal
		formatted.Message = safeMessage(contracterr.CodeInternal)
	}
	return formatted
}

func safeMessage(code string) string {
	switch code {
	case contracterr.CodeUnauthenticated:
		return "authentication is required"
	case contracterr.CodeForbidden:
		return "request is forbidden"
	case contracterr.CodeValidationFailed:
		return "validation failed"
	case contracterr.CodeNotFound:
		return "resource was not found"
	case contracterr.CodeConflict:
		return "resource conflict"
	case contracterr.CodeUnsupported:
		return "operation is not supported"
	case contracterr.CodeEntryConflict:
		return "requested listener conflicts with an active listener"
	default:
		return "internal server error"
	}
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func createProxyInputFromArgs(args map[string]interface{}, actor string) admin.CreateProxyInput {
	config := mapValue(args, "config")
	return admin.CreateProxyInput{UserID: stringValue(args, "userId"), ClientID: stringValue(args, "clientId"), Name: stringValue(args, "name"), Type: domain.ProxyType(stringValue(args, "type")), EntryHost: stringValue(config, "entryHost"), EntryPort: intValue(config, "entryPort"), TargetHost: stringValue(config, "targetHost"), TargetPort: intValue(config, "targetPort"), CertFile: stringValue(config, "certFile"), KeyFile: stringValue(config, "keyFile"), Description: stringValue(args, "description"), ActorID: actor}
}

func createClientJoinInputFromArgs(args map[string]interface{}, actor string) admin.CreateClientJoinInput {
	input := admin.CreateClientJoinInput{ID: stringValue(args, "id"), UserID: stringValue(args, "userId"), Name: stringValue(args, "name"), ActorID: actor, EnrollmentURL: stringValue(args, "enrollmentUrl"), ServerAddress: stringValue(args, "serverAddress"), ServerTLSAddress: stringValue(args, "serverTLSAddress"), ServerName: stringValue(args, "serverName"), ServerCAFile: stringValue(args, "serverCAFile")}
	if strings.TrimSpace(input.ServerCAFile) == "" {
		input.ServerCAFile = config.DefaultServer().ControlTLSCAFile
	}
	input.ServerCAFile = deploymentRelativePath(input.ServerCAFile)
	if ttlSeconds := intValue(args, "ttlSeconds"); ttlSeconds > 0 {
		input.TTL = time.Duration(ttlSeconds) * time.Second
	}
	return input
}

func deploymentRelativePath(path string) string {
	root, err := deploypath.Root(executablePath)
	if err != nil {
		return path
	}
	return deploypath.Resolve(root, path)
}

func updateProxyInputFromArgs(args map[string]interface{}, actor string) admin.UpdateProxyInput {
	config := mapValue(args, "config")
	return admin.UpdateProxyInput{ID: stringValue(args, "id"), Type: domain.ProxyType(stringValue(args, "type")), Name: stringValue(args, "name"), EntryHost: stringValue(config, "entryHost"), EntryPort: intValue(config, "entryPort"), TargetHost: stringValue(config, "targetHost"), TargetPort: intValue(config, "targetPort"), CertFile: stringValue(config, "certFile"), KeyFile: stringValue(config, "keyFile"), Description: stringValue(args, "description"), ActorID: actor}
}

func userListInputFromArgs(args map[string]interface{}) adminquery.UserListInput {
	input := mapArg(args, "input")
	return adminquery.UserListInput{Page: pageInput(mapValue(input, "page")), Filter: adminquery.UserFilter{Query: stringValue(mapValue(input, "filter"), "query"), Role: stringValue(mapValue(input, "filter"), "role"), Status: stringValue(mapValue(input, "filter"), "status")}, Sort: sortInput(mapValue(input, "sort"))}
}

func clientListInputFromArgs(args map[string]interface{}) adminquery.ClientListInput {
	input := mapArg(args, "input")
	filter := mapValue(input, "filter")
	return adminquery.ClientListInput{Page: pageInput(mapValue(input, "page")), Filter: adminquery.ClientFilter{Query: stringValue(filter, "query"), UserID: stringValue(filter, "userId"), Status: stringValue(filter, "status"), Online: boolPointer(filter, "online")}, Sort: sortInput(mapValue(input, "sort"))}
}

func proxyListInputFromArgs(args map[string]interface{}) adminquery.ProxyListInput {
	input := mapArg(args, "input")
	filter := mapValue(input, "filter")
	return adminquery.ProxyListInput{Page: pageInput(mapValue(input, "page")), Filter: adminquery.ProxyFilter{Query: stringValue(filter, "query"), UserID: stringValue(filter, "userId"), ClientID: stringValue(filter, "clientId"), Type: stringValue(filter, "type"), Status: stringValue(filter, "status")}, Sort: sortInput(mapValue(input, "sort"))}
}

func certificateListInputFromArgs(args map[string]interface{}) adminquery.CertificateListInput {
	input := mapArg(args, "input")
	filter := mapValue(input, "filter")
	return adminquery.CertificateListInput{Page: pageInput(mapValue(input, "page")), Filter: adminquery.CertificateFilter{Query: stringValue(filter, "query"), Status: stringValue(filter, "status")}, Sort: sortInput(mapValue(input, "sort"))}
}

func auditListInputFromArgs(args map[string]interface{}) adminquery.AuditListInput {
	input := mapArg(args, "input")
	filter := mapValue(input, "filter")
	return adminquery.AuditListInput{Page: pageInput(mapValue(input, "page")), Filter: adminquery.AuditFilter{Query: stringValue(filter, "query"), ActorType: stringValue(filter, "actorType"), ActorID: stringValue(filter, "actorId"), ResourceType: stringValue(filter, "resourceType"), Action: stringValue(filter, "action"), Result: stringValue(filter, "result")}, Sort: sortInput(mapValue(input, "sort"))}
}

func pageInput(value map[string]interface{}) adminquery.PageInput {
	return adminquery.PageInput{Page: intValue(value, "page"), PageSize: intValue(value, "pageSize")}
}

func sortInput(value map[string]interface{}) adminquery.SortInput {
	return adminquery.SortInput{Field: stringValue(value, "field"), Direction: stringValue(value, "direction")}
}

func mapArg(args map[string]interface{}, key string) map[string]interface{} {
	return mapValue(args, key)
}

func mapValue(values map[string]interface{}, key string) map[string]interface{} {
	if values == nil {
		return nil
	}
	value, _ := values[key].(map[string]interface{})
	return value
}

func stringValue(values map[string]interface{}, key string) string {
	if values == nil {
		return ""
	}
	value, _ := values[key].(string)
	return value
}

func intValue(values map[string]interface{}, key string) int {
	if values == nil {
		return 0
	}
	value, _ := values[key].(int)
	return value
}

func boolPointer(values map[string]interface{}, key string) *bool {
	if values == nil {
		return nil
	}
	value, ok := values[key].(bool)
	if !ok {
		return nil
	}
	copy := value
	return &copy
}

func extractUserPayload(source interface{}) userPayload {
	value, _ := source.(userPayload)
	return value
}

func extractClientPayload(source interface{}) clientPayload {
	value, _ := source.(clientPayload)
	return value
}

func extractProxyPayload(source interface{}) proxyPayload {
	value, _ := source.(proxyPayload)
	return value
}

func extractCertificatePayload(source interface{}) certificatePayload {
	value, _ := source.(certificatePayload)
	return value
}

func findCertificate(items []adminquery.ManagedCertificateSummary, proxyID string) adminquery.ManagedCertificateSummary {
	for _, item := range items {
		if item.ProxyID == proxyID {
			return item
		}
	}
	return adminquery.ManagedCertificateSummary{ProxyID: proxyID}
}

func timeResolve[T any](selector func(T) time.Time) graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		if value, ok := params.Source.(T); ok {
			return selector(value).Format(time.RFC3339), nil
		}
		return "", nil
	}
}

func nullableCertificateTimeResolve(selector func(adminquery.ManagedCertificateSummary) *time.Time) graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		if value, ok := params.Source.(adminquery.ManagedCertificateSummary); ok {
			if when := selector(value); when != nil {
				return when.Format(time.RFC3339), nil
			}
		}
		return nil, nil
	}
}

func nullableClientRuntimeTimeResolve(selector func(adminquery.ClientRuntime) *time.Time) graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		if value, ok := params.Source.(adminquery.ClientRuntime); ok {
			if when := selector(value); when != nil {
				return when.Format(time.RFC3339), nil
			}
		}
		return nil, nil
	}
}

func nullableTimeResolveClientOnline() graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		if client, ok := params.Source.(adminquery.ClientListItem); ok && client.LastOnlineAt != nil {
			return client.LastOnlineAt.Format(time.RFC3339), nil
		}
		return nil, nil
	}
}

func nullableTimeResolveClientOffline() graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		if client, ok := params.Source.(adminquery.ClientListItem); ok && client.LastOfflineAt != nil {
			return client.LastOfflineAt.Format(time.RFC3339), nil
		}
		return nil, nil
	}
}

func clientDetailTimeResolve(selector func(adminquery.ClientDetail) *time.Time) graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		if client, ok := params.Source.(adminquery.ClientDetail); ok {
			if when := selector(client); when != nil {
				return when.Format(time.RFC3339), nil
			}
		}
		return nil, nil
	}
}

func nullableUserTimeResolve(selector func(adminquery.UserListItem) *time.Time) graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		if value, ok := params.Source.(adminquery.UserListItem); ok {
			if when := selector(value); when != nil {
				return when.Format(time.RFC3339), nil
			}
		}
		if value, ok := params.Source.(adminquery.UserDetail); ok {
			if when := selector(value.UserListItem); when != nil {
				return when.Format(time.RFC3339), nil
			}
		}
		return nil, nil
	}
}

func userIDResolve() graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		switch value := params.Source.(type) {
		case adminquery.UserListItem:
			return value.ID, nil
		case adminquery.UserDetail:
			return value.ID, nil
		default:
			return "", nil
		}
	}
}

func userCreatedAtResolve() graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		switch value := params.Source.(type) {
		case adminquery.UserListItem:
			return value.CreatedAt.Format(time.RFC3339), nil
		case adminquery.UserDetail:
			return value.CreatedAt.Format(time.RFC3339), nil
		default:
			return "", nil
		}
	}
}

func userUpdatedAtResolve() graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		switch value := params.Source.(type) {
		case adminquery.UserListItem:
			return value.UpdatedAt.Format(time.RFC3339), nil
		case adminquery.UserDetail:
			return value.UpdatedAt.Format(time.RFC3339), nil
		default:
			return "", nil
		}
	}
}

func clientCreatedAtResolve() graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		if value, ok := params.Source.(adminquery.ClientListItem); ok {
			return value.CreatedAt.Format(time.RFC3339), nil
		}
		return "", nil
	}
}

func clientUpdatedAtResolve() graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		if value, ok := params.Source.(adminquery.ClientListItem); ok {
			return value.UpdatedAt.Format(time.RFC3339), nil
		}
		return "", nil
	}
}

func proxyCreatedAtResolve() graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		switch value := params.Source.(type) {
		case adminquery.ProxyListItem:
			return value.CreatedAt.Format(time.RFC3339), nil
		case adminquery.ProxyDetail:
			return value.CreatedAt.Format(time.RFC3339), nil
		default:
			return "", nil
		}
	}
}

func proxyUpdatedAtResolve() graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		switch value := params.Source.(type) {
		case adminquery.ProxyListItem:
			return value.UpdatedAt.Format(time.RFC3339), nil
		case adminquery.ProxyDetail:
			return value.UpdatedAt.Format(time.RFC3339), nil
		default:
			return "", nil
		}
	}
}
