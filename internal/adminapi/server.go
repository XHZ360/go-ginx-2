package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/parser"
	"github.com/graphql-go/graphql/language/source"
	"github.com/simp-frp/go-ginx-2/internal/admin"
	"github.com/simp-frp/go-ginx-2/internal/adminquery"
	"github.com/simp-frp/go-ginx-2/internal/domain"
)

const (
	adminAPIPrefix         = "/api/admin"
	adminSessionCookieName = "goginx_admin_session"
	adminSessionCookiePath = "/api/admin"
	adminCSRFHeader        = "X-GoGinx-CSRF-Token"
	defaultPollSeconds     = 5
)

type Server struct {
	query      adminquery.Service
	commands   admin.Service
	listener   net.Listener
	httpServer *http.Server
	schema     graphql.Schema
	creds      credentialSet
	sessions   *sessionManager
}

type Entry struct {
	ListenAddress            string
	AdminCredentialsFile     string
	Query                    adminquery.Service
	Commands                 admin.Service
	SessionIdleTimeout       time.Duration
	SessionAbsoluteLifetime  time.Duration
	Now                      func() time.Time
}

type credentialsFile struct {
	Administrators []administratorCredential `json:"administrators"`
}

type administratorCredential struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
}

type credentialSet map[string]string

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

func Listen(entry Entry) (*Server, error) {
	if strings.TrimSpace(entry.AdminCredentialsFile) == "" {
		return nil, errors.New("admin credentials file is required")
	}
	listener, err := net.Listen("tcp", entry.ListenAddress)
	if err != nil {
		return nil, err
	}
	creds, err := loadCredentials(entry.AdminCredentialsFile)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	server := &Server{
		query:    entry.Query,
		commands: entry.Commands,
		listener: listener,
		creds:    creds,
		sessions: newSessionManager(entry.SessionIdleTimeout, entry.SessionAbsoluteLifetime, entry.Now),
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

func (server *Server) routeHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, adminAPIPrefix) {
			http.NotFound(w, r)
			return
		}
		if !requestUsesProtectedTransport(r) {
			writeErrorJSON(w, http.StatusUpgradeRequired, "PROTECTED_TRANSPORT_REQUIRED", "management endpoint requires protected transport", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
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
		writeErrorJSON(w, http.StatusBadRequest, "VALIDATION_FAILED", "validation failed", fields)
		return
	}
	if !domain.CheckPasswordHash(request.Password, server.creds[request.Username]) {
		writeErrorJSON(w, http.StatusUnauthorized, "UNAUTHENTICATED", "invalid administrator credentials", nil)
		return
	}
	if existingID, ok := server.sessionIDFromRequest(r); ok {
		server.sessions.Invalidate(existingID)
	}
	session, err := server.sessions.Create(request.Username)
	if err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	server.writeSessionCookie(w, r, session)
	writeBootstrapJSON(w, http.StatusOK, &session)
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
		writeErrorJSON(w, http.StatusUnauthorized, "UNAUTHENTICATED", "administrator session is required", nil)
		return
	}
	var request graphqlRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), nil)
		return
	}
	operationType, err := graphqlOperationType(request)
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), nil)
		return
	}
	if operationType == "mutation" && !server.hasValidCSRFToken(r, session) {
		writeErrorJSON(w, http.StatusForbidden, "INVALID_CSRF", "csrf token is invalid", nil)
		return
	}
	result := graphql.Do(graphql.Params{
		Schema:         server.schema,
		RequestString:  request.Query,
		VariableValues: request.Variables,
		OperationName:  request.OperationName,
		Context:        contextWithActor(r.Context(), session.Username),
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
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
	managedCertificateType := graphql.NewObject(graphql.ObjectConfig{Name: "ManagedCertificate", Fields: graphql.Fields{
		"proxyId":       &graphql.Field{Type: graphql.String},
		"certificateId": &graphql.Field{Type: graphql.String},
		"host":          &graphql.Field{Type: graphql.String},
		"status":        &graphql.Field{Type: graphql.String},
		"certFile":      &graphql.Field{Type: graphql.String},
		"keyFile":       &graphql.Field{Type: graphql.String},
		"lastError":     &graphql.Field{Type: graphql.String},
	}})
	auditType := graphql.NewObject(graphql.ObjectConfig{Name: "AuditEvent", Fields: graphql.Fields{
		"id":           &graphql.Field{Type: graphql.String},
		"actorUserId":  &graphql.Field{Type: graphql.String},
		"resourceType": &graphql.Field{Type: graphql.String},
		"resourceId":   &graphql.Field{Type: graphql.String},
		"action":       &graphql.Field{Type: graphql.String},
		"result":       &graphql.Field{Type: graphql.String},
		"createdAt":    &graphql.Field{Type: graphql.String, Resolve: timeResolve(func(event adminquery.AuditListItem) time.Time { return event.CreatedAt })},
	}})
	clientRuntimeType := graphql.NewObject(graphql.ObjectConfig{Name: "ClientRuntime", Fields: graphql.Fields{
		"online":        &graphql.Field{Type: graphql.Boolean},
		"protocol":      &graphql.Field{Type: graphql.String},
		"configVersion": &graphql.Field{Type: graphql.Int},
		"activeProxies": &graphql.Field{Type: graphql.Int},
		"activeStreams": &graphql.Field{Type: graphql.Int},
		"uploadBytes":   &graphql.Field{Type: graphql.Int},
		"downloadBytes": &graphql.Field{Type: graphql.Int},
		"errorSummary":  &graphql.Field{Type: graphql.String},
	}})
	clientType := graphql.NewObject(graphql.ObjectConfig{Name: "Client", Fields: graphql.Fields{
		"id":            &graphql.Field{Type: graphql.String},
		"userId":        &graphql.Field{Type: graphql.String},
		"name":          &graphql.Field{Type: graphql.String},
		"status":        &graphql.Field{Type: graphql.String},
		"version":       &graphql.Field{Type: graphql.Int},
		"runtime":       &graphql.Field{Type: clientRuntimeType},
		"proxyIds":      &graphql.Field{Type: graphql.NewList(graphql.String)},
		"lastOnlineAt":  &graphql.Field{Type: graphql.String, Resolve: nullableTimeResolveClientOnline()},
		"lastOfflineAt": &graphql.Field{Type: graphql.String, Resolve: nullableTimeResolveClientOffline()},
	}})
	userType := graphql.NewObject(graphql.ObjectConfig{Name: "User", Fields: graphql.Fields{
		"id":       &graphql.Field{Type: graphql.String},
		"username": &graphql.Field{Type: graphql.String},
		"role":     &graphql.Field{Type: graphql.String},
		"status":   &graphql.Field{Type: graphql.String},
		"clientCount":   &graphql.Field{Type: graphql.Int},
		"proxyCount":    &graphql.Field{Type: graphql.Int},
		"uploadBytes":   &graphql.Field{Type: graphql.Int},
		"downloadBytes": &graphql.Field{Type: graphql.Int},
		"hasPasswordHash": &graphql.Field{Type: graphql.Boolean, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			switch value := params.Source.(type) {
			case adminquery.UserListItem:
				return value.HasPasswordHash, nil
			case adminquery.UserDetail:
				return value.HasPasswordHash, nil
			case domain.User:
				return value.PasswordHash != "", nil
			default:
				return false, nil
			}
		}},
	}})
	proxyType := graphql.NewObject(graphql.ObjectConfig{Name: "Proxy", Fields: graphql.Fields{
		"id":                   &graphql.Field{Type: graphql.String},
		"userId":               &graphql.Field{Type: graphql.String},
		"clientId":             &graphql.Field{Type: graphql.String},
		"name":                 &graphql.Field{Type: graphql.String},
		"type":                 &graphql.Field{Type: graphql.String},
		"status":               &graphql.Field{Type: graphql.String},
		"entryHost":            &graphql.Field{Type: graphql.String},
		"entryPort":            &graphql.Field{Type: graphql.Int},
		"targetHost":           &graphql.Field{Type: graphql.String},
		"targetPort":           &graphql.Field{Type: graphql.Int},
		"description":          &graphql.Field{Type: graphql.String},
		"certFile":             &graphql.Field{Type: graphql.String},
		"keyFile":              &graphql.Field{Type: graphql.String},
		"runtimeStatus":        &graphql.Field{Type: graphql.String},
		"activeTCPConnections": &graphql.Field{Type: graphql.Int},
		"uploadBytes":          &graphql.Field{Type: graphql.Int},
		"downloadBytes":        &graphql.Field{Type: graphql.Int},
		"tcpErrorCount":        &graphql.Field{Type: graphql.Int},
		"udpErrorCount":        &graphql.Field{Type: graphql.Int},
		"httpErrorCount":       &graphql.Field{Type: graphql.Int},
		"certificate":          &graphql.Field{Type: managedCertificateType},
	}})
	dashboardType := graphql.NewObject(graphql.ObjectConfig{Name: "DashboardSummary", Fields: graphql.Fields{
		"onlineClientCount":        &graphql.Field{Type: graphql.Int},
		"enabledProxyCount":        &graphql.Field{Type: graphql.Int},
		"activeTCPConnectionCount": &graphql.Field{Type: graphql.Int},
		"cumulativeUploadBytes":    &graphql.Field{Type: graphql.Int},
		"cumulativeDownloadBytes":  &graphql.Field{Type: graphql.Int},
		"cumulativeTCPErrorCount":  &graphql.Field{Type: graphql.Int},
		"cumulativeUDPErrorCount":  &graphql.Field{Type: graphql.Int},
		"cumulativeHTTPErrorCount": &graphql.Field{Type: graphql.Int},
	}})
	query := graphql.NewObject(graphql.ObjectConfig{Name: "Query", Fields: graphql.Fields{
		"dashboardSummary": &graphql.Field{Type: dashboardType, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return server.query.DashboardSummary(params.Context)
		}},
		"users": &graphql.Field{Type: graphql.NewList(userType), Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return server.query.ListUsers(params.Context)
		}},
		"user": &graphql.Field{Type: userType, Args: graphql.FieldConfigArgument{"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return server.query.UserDetail(params.Context, params.Args["id"].(string))
		}},
		"clients": &graphql.Field{Type: graphql.NewList(clientType), Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return server.query.ListClients(params.Context)
		}},
		"client": &graphql.Field{Type: clientType, Args: graphql.FieldConfigArgument{"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return server.query.ClientDetail(params.Context, params.Args["id"].(string))
		}},
		"proxies": &graphql.Field{Type: graphql.NewList(proxyType), Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return server.query.ListProxies(params.Context)
		}},
		"proxy": &graphql.Field{Type: proxyType, Args: graphql.FieldConfigArgument{"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return server.query.ProxyDetail(params.Context, params.Args["id"].(string))
		}},
		"managedCertificates": &graphql.Field{Type: graphql.NewList(managedCertificateType), Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return server.query.ListManagedCertificates(params.Context)
		}},
		"auditEvents": &graphql.Field{Type: graphql.NewList(auditType), Args: graphql.FieldConfigArgument{"limit": &graphql.ArgumentConfig{Type: graphql.Int}}, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			limit, _ := params.Args["limit"].(int)
			return server.query.ListRecentAuditEvents(params.Context, limit)
		}},
	}})
	mutation := graphql.NewObject(graphql.ObjectConfig{Name: "Mutation", Fields: graphql.Fields{
		"createUser": &graphql.Field{Type: userType, Args: graphql.FieldConfigArgument{"username": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}, "password": &graphql.ArgumentConfig{Type: graphql.String}, "role": &graphql.ArgumentConfig{Type: graphql.String}}, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return server.commands.CreateUser(params.Context, admin.CreateUserInput{Username: params.Args["username"].(string), Password: stringArg(params.Args, "password"), Role: domain.Role(defaultString(stringArg(params.Args, "role"), string(domain.RoleUser))), ActorID: actorFromContext(params.Context)})
		}},
		"disableUser": &graphql.Field{Type: graphql.Boolean, Args: graphql.FieldConfigArgument{"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return true, server.commands.DisableUser(params.Context, params.Args["id"].(string), actorFromContext(params.Context))
		}},
		"setUserPassword": &graphql.Field{Type: graphql.Boolean, Args: graphql.FieldConfigArgument{"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}, "password": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return true, server.commands.SetUserPassword(params.Context, params.Args["id"].(string), params.Args["password"].(string), actorFromContext(params.Context))
		}},
		"createProxy": &graphql.Field{Type: proxyType, Args: proxyMutationArgs(true), Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return server.commands.CreateProxy(params.Context, createProxyInputFromArgs(params.Args, actorFromContext(params.Context)))
		}},
		"updateProxy": &graphql.Field{Type: proxyType, Args: proxyMutationArgs(false), Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return server.commands.UpdateProxy(params.Context, updateProxyInputFromArgs(params.Args, actorFromContext(params.Context)))
		}},
		"enableProxy": &graphql.Field{Type: graphql.Boolean, Args: graphql.FieldConfigArgument{"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return true, server.commands.EnableProxy(params.Context, params.Args["id"].(string), actorFromContext(params.Context))
		}},
		"disableProxy": &graphql.Field{Type: graphql.Boolean, Args: graphql.FieldConfigArgument{"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return true, server.commands.DisableProxy(params.Context, params.Args["id"].(string), actorFromContext(params.Context))
		}},
		"deleteProxy": &graphql.Field{Type: graphql.Boolean, Args: graphql.FieldConfigArgument{"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return true, server.commands.DeleteProxy(params.Context, params.Args["id"].(string), actorFromContext(params.Context))
		}},
		"issueManagedCertificate": &graphql.Field{Type: managedCertificateType, Args: graphql.FieldConfigArgument{"proxyId": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return server.commands.IssueManagedCertificate(params.Context, admin.CertificateInput{ProxyID: params.Args["proxyId"].(string), ActorID: actorFromContext(params.Context)})
		}},
		"renewManagedCertificate": &graphql.Field{Type: managedCertificateType, Args: graphql.FieldConfigArgument{"proxyId": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: func(params graphql.ResolveParams) (interface{}, error) {
			return server.commands.RenewManagedCertificate(params.Context, admin.CertificateInput{ProxyID: params.Args["proxyId"].(string), ActorID: actorFromContext(params.Context)})
		}},
	}})
	return graphql.NewSchema(graphql.SchemaConfig{Query: query, Mutation: mutation})
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func stringArg(args map[string]interface{}, key string) string {
	value, _ := args[key].(string)
	return value
}

func proxyMutationArgs(create bool) graphql.FieldConfigArgument {
	args := graphql.FieldConfigArgument{
		"id":          &graphql.ArgumentConfig{Type: graphql.String},
		"type":        &graphql.ArgumentConfig{Type: graphql.String},
		"userId":      &graphql.ArgumentConfig{Type: graphql.String},
		"clientId":    &graphql.ArgumentConfig{Type: graphql.String},
		"name":        &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
		"entryHost":   &graphql.ArgumentConfig{Type: graphql.String},
		"entryPort":   &graphql.ArgumentConfig{Type: graphql.Int},
		"targetHost":  &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
		"targetPort":  &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)},
		"certFile":    &graphql.ArgumentConfig{Type: graphql.String},
		"keyFile":     &graphql.ArgumentConfig{Type: graphql.String},
		"description": &graphql.ArgumentConfig{Type: graphql.String},
	}
	if create {
		args["type"].Type = graphql.NewNonNull(graphql.String)
		args["userId"].Type = graphql.NewNonNull(graphql.String)
		args["clientId"].Type = graphql.NewNonNull(graphql.String)
	} else {
		args["id"].Type = graphql.NewNonNull(graphql.String)
	}
	return args
}

func createProxyInputFromArgs(args map[string]interface{}, actor string) admin.CreateProxyInput {
	entryPort, _ := args["entryPort"].(int)
	targetPort, _ := args["targetPort"].(int)
	return admin.CreateProxyInput{ID: stringArg(args, "id"), UserID: stringArg(args, "userId"), ClientID: stringArg(args, "clientId"), Name: args["name"].(string), Type: domain.ProxyType(args["type"].(string)), EntryHost: stringArg(args, "entryHost"), EntryPort: entryPort, TargetHost: args["targetHost"].(string), TargetPort: targetPort, CertFile: stringArg(args, "certFile"), KeyFile: stringArg(args, "keyFile"), Description: stringArg(args, "description"), ActorID: actor}
}

func updateProxyInputFromArgs(args map[string]interface{}, actor string) admin.UpdateProxyInput {
	entryPort, _ := args["entryPort"].(int)
	targetPort, _ := args["targetPort"].(int)
	return admin.UpdateProxyInput{ID: args["id"].(string), Type: domain.ProxyType(stringArg(args, "type")), Name: args["name"].(string), EntryHost: stringArg(args, "entryHost"), EntryPort: entryPort, TargetHost: args["targetHost"].(string), TargetPort: targetPort, CertFile: stringArg(args, "certFile"), KeyFile: stringArg(args, "keyFile"), Description: stringArg(args, "description"), ActorID: actor}
}

func timeResolve[T any](selector func(T) time.Time) graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		if value, ok := params.Source.(T); ok {
			return selector(value).Format(time.RFC3339), nil
		}
		return "", nil
	}
}

func nullableTimeResolveClientOnline() graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		if client, ok := params.Source.(adminquery.ClientListItem); ok && client.LastOnlineAt != nil {
			return client.LastOnlineAt.Format(time.RFC3339), nil
		}
		if client, ok := params.Source.(adminquery.ClientDetail); ok && client.LastOnlineAt != nil {
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
		if client, ok := params.Source.(adminquery.ClientDetail); ok && client.LastOfflineAt != nil {
			return client.LastOfflineAt.Format(time.RFC3339), nil
		}
		return nil, nil
	}
}
