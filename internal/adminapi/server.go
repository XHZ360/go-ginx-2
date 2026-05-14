package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/simp-frp/go-ginx-2/internal/admin"
	"github.com/simp-frp/go-ginx-2/internal/adminquery"
	"github.com/simp-frp/go-ginx-2/internal/domain"
)

type Server struct {
	query      adminquery.Service
	commands   admin.Service
	listener   net.Listener
	httpServer *http.Server
	schema     graphql.Schema
	creds      credentialSet
}

type Entry struct {
	ListenAddress        string
	AdminCredentialsFile string
	Query                adminquery.Service
	Commands             admin.Service
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
	server := &Server{query: entry.Query, commands: entry.Commands, listener: listener, creds: creds}
	schema, err := server.buildSchema()
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	server.schema = schema
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", server.graphqlHandler)
	mux.HandleFunc("/", server.uiHandler)
	server.httpServer = &http.Server{Handler: server.authMiddleware(mux)}
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

func (server *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requestUsesProtectedTransport(r) {
			http.Error(w, "management endpoint requires protected transport", http.StatusUpgradeRequired)
			return
		}
		username, password, ok := r.BasicAuth()
		if !ok || !domain.CheckPasswordHash(password, server.creds[username]) {
			w.Header().Set("WWW-Authenticate", `Basic realm="go-ginx-admin"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorContextKey, username)))
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

func (server *Server) uiHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		server.handleFormAction(w, r)
		return
	}
	page := strings.Trim(r.URL.Path, "/")
	if strings.HasPrefix(page, "users/") {
		server.renderPage(w, r, "user-detail")
		return
	}
	if strings.HasPrefix(page, "clients/") {
		server.renderPage(w, r, "client-detail")
		return
	}
	if strings.HasPrefix(page, "proxies/") {
		server.renderPage(w, r, "proxy-detail")
		return
	}
	if page == "" {
		page = "dashboard"
	}
	server.renderPage(w, r, page)
}

func (server *Server) renderPage(w http.ResponseWriter, r *http.Request, page string) {
	data := uiPageData{Page: page, DashboardPollSeconds: 5}
	ctx := r.Context()
	switch page {
	case "dashboard":
		summary, err := server.query.DashboardSummary(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data.Dashboard = &summary
	case "users":
		users, err := server.query.ListUsers(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data.Users = users
	case "user-detail":
		userID := pathBase(r.URL.Path)
		user, err := server.query.UserDetail(ctx, userID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data.UserDetail = &user
	case "clients":
		clients, err := server.query.ListClients(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data.Clients = clients
	case "client-detail":
		clientID := pathBase(r.URL.Path)
		client, err := server.query.ClientDetail(ctx, clientID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data.ClientDetail = &client
	case "proxies":
		proxies, err := server.query.ListProxies(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		users, err := server.query.ListUsers(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		clients, err := server.query.ListClients(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data.Proxies = proxies
		data.Users = users
		data.Clients = clients
	case "proxy-detail":
		proxyID := pathBase(r.URL.Path)
		proxy, err := server.query.ProxyDetail(ctx, proxyID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data.ProxyDetail = &proxy
	case "certificates":
		certificates, err := server.query.ListManagedCertificates(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data.Certificates = certificates
	case "audit":
		events, err := server.query.ListRecentAuditEvents(ctx, 50)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data.AuditEvents = events
	default:
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = uiTemplate.Execute(w, data)
}

func (server *Server) handleFormAction(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	actor := actorFromContext(ctx)
	action := r.FormValue("action")
	redirectTo := r.FormValue("redirect")
	if redirectTo == "" {
		redirectTo = "/"
	}
	var err error
	switch action {
	case "create_user":
		_, err = server.commands.CreateUser(ctx, admin.CreateUserInput{Username: r.FormValue("username"), Password: r.FormValue("password"), Role: domain.Role(r.FormValue("role")), ActorID: actor})
	case "disable_user":
		err = server.commands.DisableUser(ctx, r.FormValue("id"), actor)
	case "set_user_password":
		err = server.commands.SetUserPassword(ctx, r.FormValue("id"), r.FormValue("password"), actor)
	case "create_proxy":
		err = server.handleCreateProxy(ctx, actor, r)
	case "update_proxy":
		err = server.handleUpdateProxy(ctx, actor, r)
	case "enable_proxy":
		err = server.commands.EnableProxy(ctx, r.FormValue("id"), actor)
	case "disable_proxy":
		err = server.commands.DisableProxy(ctx, r.FormValue("id"), actor)
	case "delete_proxy":
		err = server.commands.DeleteProxy(ctx, r.FormValue("id"), actor)
	case "issue_certificate":
		_, err = server.commands.IssueManagedCertificate(ctx, admin.CertificateInput{ProxyID: r.FormValue("proxy_id"), ActorID: actor})
	case "renew_certificate":
		_, err = server.commands.RenewManagedCertificate(ctx, admin.CertificateInput{ProxyID: r.FormValue("proxy_id"), ActorID: actor})
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

func (server *Server) handleCreateProxy(ctx context.Context, actor string, r *http.Request) error {
	entryPort, err := strconv.Atoi(defaultString(r.FormValue("entry_port"), "0"))
	if err != nil {
		return err
	}
	targetPort, err := strconv.Atoi(defaultString(r.FormValue("target_port"), "0"))
	if err != nil {
		return err
	}
	_, err = server.commands.CreateProxy(ctx, admin.CreateProxyInput{UserID: r.FormValue("user_id"), ClientID: r.FormValue("client_id"), Name: r.FormValue("name"), Type: domain.ProxyType(r.FormValue("type")), EntryHost: r.FormValue("entry_host"), EntryPort: entryPort, TargetHost: r.FormValue("target_host"), TargetPort: targetPort, CertFile: r.FormValue("cert_file"), KeyFile: r.FormValue("key_file"), Description: r.FormValue("description"), ActorID: actor})
	return err
}

func (server *Server) handleUpdateProxy(ctx context.Context, actor string, r *http.Request) error {
	entryPort, err := strconv.Atoi(defaultString(r.FormValue("entry_port"), "0"))
	if err != nil {
		return err
	}
	targetPort, err := strconv.Atoi(defaultString(r.FormValue("target_port"), "0"))
	if err != nil {
		return err
	}
	_, err = server.commands.UpdateProxy(ctx, admin.UpdateProxyInput{ID: r.FormValue("id"), Type: domain.ProxyType(r.FormValue("type")), Name: r.FormValue("name"), EntryHost: r.FormValue("entry_host"), EntryPort: entryPort, TargetHost: r.FormValue("target_host"), TargetPort: targetPort, CertFile: r.FormValue("cert_file"), KeyFile: r.FormValue("key_file"), Description: r.FormValue("description"), ActorID: actor})
	return err
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func pathBase(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

type graphqlRequest struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
}

func (server *Server) graphqlHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request graphqlRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result := graphql.Do(graphql.Params{Schema: server.schema, RequestString: request.Query, VariableValues: request.Variables, OperationName: request.OperationName, Context: r.Context()})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
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
		"id":            &graphql.Field{Type: graphql.String},
		"username":      &graphql.Field{Type: graphql.String},
		"role":          &graphql.Field{Type: graphql.String},
		"status":        &graphql.Field{Type: graphql.String},
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

type uiPageData struct {
	Page                 string
	Dashboard            *adminquery.DashboardSummary
	Users                []adminquery.UserListItem
	UserDetail           *adminquery.UserDetail
	Clients              []adminquery.ClientListItem
	ClientDetail         *adminquery.ClientDetail
	Proxies              []adminquery.ProxyListItem
	ProxyDetail          *adminquery.ProxyDetail
	Certificates         []adminquery.ManagedCertificateSummary
	AuditEvents          []adminquery.AuditListItem
	DashboardPollSeconds int
}

var uiTemplate = template.Must(template.New("admin").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><title>go-ginx admin</title>{{if or (eq .Page "dashboard") (eq .Page "clients") (eq .Page "client-detail") (eq .Page "proxies") (eq .Page "proxy-detail") (eq .Page "certificates")}}<meta http-equiv="refresh" content="{{.DashboardPollSeconds}}">{{end}}
<style>body{font-family:Arial,sans-serif;margin:24px}nav a{margin-right:12px}table{border-collapse:collapse;width:100%;margin-top:12px}th,td{border:1px solid #ccc;padding:6px;text-align:left}form{margin:8px 0}input,select{margin-right:8px}</style></head><body>
<nav><a href="/">Dashboard</a><a href="/users">Users</a><a href="/clients">Clients</a><a href="/proxies">Proxies</a><a href="/certificates">Certificates</a><a href="/audit">Audit</a></nav>
{{if eq .Page "dashboard"}}<h1>Dashboard</h1><table><tr><th>Online clients</th><th>Enabled proxies</th><th>Active TCP connections</th><th>Upload bytes</th><th>Download bytes</th><th>TCP errors</th><th>UDP errors</th><th>HTTP errors</th></tr><tr><td>{{.Dashboard.OnlineClientCount}}</td><td>{{.Dashboard.EnabledProxyCount}}</td><td>{{.Dashboard.ActiveTCPConnectionCount}}</td><td>{{.Dashboard.CumulativeUploadBytes}}</td><td>{{.Dashboard.CumulativeDownloadBytes}}</td><td>{{.Dashboard.CumulativeTCPErrorCount}}</td><td>{{.Dashboard.CumulativeUDPErrorCount}}</td><td>{{.Dashboard.CumulativeHTTPErrorCount}}</td></tr></table>{{end}}
{{if eq .Page "users"}}<h1>Users</h1><form method="post"><input type="hidden" name="action" value="create_user"><input type="hidden" name="redirect" value="/users"><input name="username" placeholder="username"><input name="password" placeholder="password" type="password"><select name="role"><option value="user">user</option><option value="admin">admin</option></select><button type="submit">Create user</button></form><table><tr><th>ID</th><th>Username</th><th>Role</th><th>Status</th><th>Clients</th><th>Proxies</th><th>Has password</th><th>Actions</th></tr>{{range .Users}}<tr><td><a href="/users/{{.ID}}">{{.ID}}</a></td><td>{{.Username}}</td><td>{{.Role}}</td><td>{{.Status}}</td><td>{{.ClientCount}}</td><td>{{.ProxyCount}}</td><td>{{.HasPasswordHash}}</td><td><form method="post" style="display:inline"><input type="hidden" name="action" value="disable_user"><input type="hidden" name="redirect" value="/users"><input type="hidden" name="id" value="{{.ID}}"><button type="submit">Disable</button></form><form method="post" style="display:inline"><input type="hidden" name="action" value="set_user_password"><input type="hidden" name="redirect" value="/users"><input type="hidden" name="id" value="{{.ID}}"><input name="password" placeholder="new password" type="password"><button type="submit">Set password</button></form></td></tr>{{end}}</table>{{end}}
{{if eq .Page "user-detail"}}<h1>User Detail</h1><p><strong>ID:</strong> {{.UserDetail.ID}}</p><p><strong>Username:</strong> {{.UserDetail.Username}}</p><p><strong>Role:</strong> {{.UserDetail.Role}}</p><p><strong>Status:</strong> {{.UserDetail.Status}}</p><p><strong>Clients:</strong> {{.UserDetail.ClientCount}}</p><p><strong>Proxies:</strong> {{.UserDetail.ProxyCount}}</p><p><strong>Upload Bytes:</strong> {{.UserDetail.UploadBytes}}</p><p><strong>Download Bytes:</strong> {{.UserDetail.DownloadBytes}}</p><p><a href="/users">Back to users</a></p>{{end}}
{{if eq .Page "clients"}}<h1>Clients</h1><table><tr><th>ID</th><th>User</th><th>Name</th><th>Status</th><th>Online</th><th>Protocol</th><th>Active proxies</th><th>Active streams</th><th>Error summary</th></tr>{{range .Clients}}<tr><td><a href="/clients/{{.ID}}">{{.ID}}</a></td><td>{{.UserID}}</td><td>{{.Name}}</td><td>{{.Status}}</td><td>{{.Runtime.Online}}</td><td>{{.Runtime.Protocol}}</td><td>{{.Runtime.ActiveProxies}}</td><td>{{.Runtime.ActiveStreams}}</td><td>{{.Runtime.ErrorSummary}}</td></tr>{{end}}</table>{{end}}
{{if eq .Page "client-detail"}}<h1>Client Detail</h1><p><strong>ID:</strong> {{.ClientDetail.ID}}</p><p><strong>User:</strong> {{.ClientDetail.UserID}}</p><p><strong>Name:</strong> {{.ClientDetail.Name}}</p><p><strong>Status:</strong> {{.ClientDetail.Status}}</p><p><strong>Online:</strong> {{.ClientDetail.Runtime.Online}}</p><p><strong>Protocol:</strong> {{.ClientDetail.Runtime.Protocol}}</p><p><strong>Active Proxies:</strong> {{.ClientDetail.Runtime.ActiveProxies}}</p><p><strong>Active Streams:</strong> {{.ClientDetail.Runtime.ActiveStreams}}</p><p><strong>Error Summary:</strong> {{.ClientDetail.Runtime.ErrorSummary}}</p><p><strong>Proxy IDs:</strong> {{range .ClientDetail.ProxyIDs}}{{.}} {{end}}</p><p><a href="/clients">Back to clients</a></p>{{end}}
{{if eq .Page "proxies"}}<h1>Proxies</h1><form method="post"><input type="hidden" name="action" value="create_proxy"><input type="hidden" name="redirect" value="/proxies"><input name="user_id" placeholder="user id"><input name="client_id" placeholder="client id"><input name="name" placeholder="name"><select name="type"><option value="tcp">tcp</option><option value="udp">udp</option><option value="http">http</option><option value="https">https</option></select><input name="entry_host" placeholder="entry host"><input name="entry_port" placeholder="entry port"><input name="target_host" placeholder="target host"><input name="target_port" placeholder="target port"><input name="cert_file" placeholder="cert file"><input name="key_file" placeholder="key file"><input name="description" placeholder="description"><button type="submit">Create proxy</button></form><table><tr><th>ID</th><th>Name</th><th>Type</th><th>Status</th><th>Runtime</th><th>Entry</th><th>Target</th><th>Actions</th></tr>{{range .Proxies}}<tr><td><a href="/proxies/{{.ID}}">{{.ID}}</a></td><td>{{.Name}}</td><td>{{.Type}}</td><td>{{.Status}}</td><td>{{.RuntimeStatus}}</td><td>{{if .EntryHost}}{{.EntryHost}}{{else}}{{.EntryPort}}{{end}}</td><td>{{.TargetHost}}:{{.TargetPort}}</td><td><details><summary>Edit</summary><form method="post"><input type="hidden" name="action" value="update_proxy"><input type="hidden" name="redirect" value="/proxies"><input type="hidden" name="id" value="{{.ID}}"><input type="hidden" name="type" value="{{.Type}}"><input name="name" value="{{.Name}}"><input name="entry_host" value="{{.EntryHost}}"><input name="entry_port" value="{{.EntryPort}}"><input name="target_host" value="{{.TargetHost}}"><input name="target_port" value="{{.TargetPort}}"><input name="cert_file" value="{{.CertFile}}"><input name="key_file" value="{{.KeyFile}}"><input name="description" value="{{.Description}}"><button type="submit">Save</button></form></details><form method="post" style="display:inline"><input type="hidden" name="action" value="enable_proxy"><input type="hidden" name="redirect" value="/proxies"><input type="hidden" name="id" value="{{.ID}}"><button type="submit">Enable</button></form><form method="post" style="display:inline"><input type="hidden" name="action" value="disable_proxy"><input type="hidden" name="redirect" value="/proxies"><input type="hidden" name="id" value="{{.ID}}"><button type="submit">Disable</button></form><form method="post" style="display:inline"><input type="hidden" name="action" value="delete_proxy"><input type="hidden" name="redirect" value="/proxies"><input type="hidden" name="id" value="{{.ID}}"><button type="submit">Delete</button></form></td></tr>{{end}}</table>{{end}}
{{if eq .Page "proxy-detail"}}<h1>Proxy Detail</h1><p><strong>ID:</strong> {{.ProxyDetail.ID}}</p><p><strong>Name:</strong> {{.ProxyDetail.Name}}</p><p><strong>Type:</strong> {{.ProxyDetail.Type}}</p><p><strong>Status:</strong> {{.ProxyDetail.Status}}</p><p><strong>Runtime:</strong> {{.ProxyDetail.RuntimeStatus}}</p><p><strong>Entry:</strong> {{if .ProxyDetail.EntryHost}}{{.ProxyDetail.EntryHost}}{{else}}{{.ProxyDetail.EntryPort}}{{end}}</p><p><strong>Target:</strong> {{.ProxyDetail.TargetHost}}:{{.ProxyDetail.TargetPort}}</p><p><strong>Upload Bytes:</strong> {{.ProxyDetail.UploadBytes}}</p><p><strong>Download Bytes:</strong> {{.ProxyDetail.DownloadBytes}}</p><p><strong>TCP Errors:</strong> {{.ProxyDetail.TCPErrorCount}}</p><p><strong>UDP Errors:</strong> {{.ProxyDetail.UDPErrorCount}}</p><p><strong>HTTP Errors:</strong> {{.ProxyDetail.HTTPErrorCount}}</p>{{if .ProxyDetail.Certificate}}<p><strong>Certificate Status:</strong> {{.ProxyDetail.Certificate.Status}}</p>{{end}}<p><a href="/proxies">Back to proxies</a></p>{{end}}
{{if eq .Page "certificates"}}<h1>Managed certificates</h1><table><tr><th>Proxy</th><th>Host</th><th>Status</th><th>Actions</th></tr>{{range .Certificates}}<tr><td>{{.ProxyID}}</td><td>{{.Host}}</td><td>{{.Status}}</td><td><form method="post" style="display:inline"><input type="hidden" name="action" value="issue_certificate"><input type="hidden" name="redirect" value="/certificates"><input type="hidden" name="proxy_id" value="{{.ProxyID}}"><button type="submit">Issue</button></form><form method="post" style="display:inline"><input type="hidden" name="action" value="renew_certificate"><input type="hidden" name="redirect" value="/certificates"><input type="hidden" name="proxy_id" value="{{.ProxyID}}"><button type="submit">Renew</button></form></td></tr>{{end}}</table>{{end}}
{{if eq .Page "audit"}}<h1>Recent audit events</h1><table><tr><th>When</th><th>Actor</th><th>Resource</th><th>Action</th><th>Result</th></tr>{{range .AuditEvents}}<tr><td>{{.CreatedAt.Format "2006-01-02 15:04:05Z07:00"}}</td><td>{{.ActorUserID}}</td><td>{{.ResourceType}} {{.ResourceID}}</td><td>{{.Action}}</td><td>{{.Result}}</td></tr>{{end}}</table>{{end}}
</body></html>`))
