# go-ginx-2

`go-ginx-2` 是 Simp-Frp/go-ginx 设计的新实现目标。当前仓库已经完成里程碑一运行时和首个部署基线，重点覆盖控制面、反向代理、管理 API/UI、证书管理、SQLite 持久化和可复现部署包。

> 状态说明：这还不是完整生产平台。核心 TCP/UDP/HTTP/HTTPS 反向代理路径、QUIC 与 TCP+TLS 控制通道、无配置启动、客户端加入流程、管理后台和 `systemd` 部署模板已经实现并有本地测试覆盖；配额、限速、普通用户自助、备份恢复和更完整的运维能力仍在后续范围内。

## 核心能力

- Go 模块：`github.com/simp-frp/go-ginx-2`，当前 `go.mod` 使用 Go `1.25.0`。
- 控制面：支持 QUIC 控制通道，以及 TCP+TLS 兜底控制通道；客户端认证、TLS/CA 校验、代理快照同步、心跳和最新会话替换已实现。
- 数据层：通过仓储接口使用 cgo-free SQLite，运行时状态默认保存在 `data/go-ginx.db`。
- 代理类型：
  - TCP 反向代理，代理流可走 QUIC stream 或 TCP+TLS framed substream。
  - UDP 反向代理，按外部源地址维护会话。
  - HTTP 反向代理，按 `Host` 路由。
  - HTTPS 反向代理，支持 SNI passthrough、静态证书终止和 ACME DNS-01 托管证书终止。
- 证书：控制通道 TLS 材料可自动生成；HTTPS 托管证书支持 Cloudflare DNS-01、签发、续期、状态查询和热加载。
- 管理能力：
  - `goginx-admin` 可创建管理员、用户、客户端、一次性 join token、代理记录和部署包。
  - 管理监听器提供登录、会话引导、登出、GraphQL 管理操作、客户端注册和同源管理前端。
  - 管理前端默认使用嵌入式静态资源，`admin_frontend_dir` 可覆盖为自定义构建目录。
- 部署：`build-deploy-bundle` 可生成包含服务端、客户端、管理 CLI、示例配置、目录结构和 `systemd` service 的部署包。

## 仓库结构

```text
cmd/
  goginx-server/   服务端守护进程入口
  goginx-client/   客户端守护进程入口与 join 命令
  goginx-admin/    管理 CLI
internal/
  admin*/          管理服务、查询模型、HTTP API 与会话
  config/          配置加载、无配置托管状态和 TLS 材料生成
  control/         QUIC 与 TCP+TLS 控制通道
  proxy/           TCP、UDP、HTTP、HTTPS 代理实现
  store/sqlite/    SQLite 仓储实现
admin-ui/          React/Vite 管理前端源码
deploy/systemd/    systemd 服务模板
docs/              运行、E2E、后台 UI 和示例文档
openspec/          OpenSpec 规格和历史变更
```

## 环境要求

- Go `1.25` 或与 `go.mod` 匹配的版本。
- Node.js 与 npm，仅在开发或构建 `admin-ui/` 时需要。
- 本地测试和构建建议禁用 cgo：

```powershell
$env:CGO_ENABLED="0"
```

## 快速开始：无配置运行

默认路径不需要手写 `server.json` 或 `client.json`。服务端首次启动会创建 `data/` 状态目录，并生成控制通道 TLS 材料。

1. 启动服务端：

```powershell
$env:CGO_ENABLED="0"
go run ./cmd/goginx-server
```

默认监听：

- 管理后台：`127.0.0.1:8080`
- 控制通道 QUIC：`:8443`
- 控制通道 TCP+TLS：`:9443`
- HTTP 入口：`:8081`
- HTTPS 入口：默认关闭，需通过环境变量或配置启用

2. 在另一个终端初始化第一个管理员：

```powershell
go run ./cmd/goginx-admin init-admin -id admin-1 -username admin -password "<password>"
```

3. 生成客户端一次性加入 token：

```powershell
$token = go run ./cmd/goginx-admin create-client-join -id client-1 -user admin-1 -name home
```

如果客户端不在本机，需要显式指定外部可访问地址，例如：

```powershell
$token = go run ./cmd/goginx-admin create-client-join `
  -id client-1 `
  -user admin-1 `
  -name home `
  -enrollment-url "https://admin.example.com/api/client/enroll" `
  -server-address "control.example.com:8443" `
  -server-tls-address "control.example.com:9443" `
  -server-name "go-ginx-control.local"
```

4. 在客户端主机加入并启动客户端：

```powershell
go run ./cmd/goginx-client join $token
go run ./cmd/goginx-client
```

`join` 会写入：

- `data/client-state.json`
- `data/certs/server-ca.crt`

后续客户端直接运行 `goginx-client` 即可读取托管状态。

5. 打开管理后台：

```text
http://127.0.0.1:8080
```

管理端点要求受保护传输：本机 loopback 可用于开发；跨机器部署时应放在 TLS 反向代理之后，或通过 `X-Forwarded-Proto: https` 进入服务。

## 管理 CLI 示例

CLI 默认使用 `data/go-ginx.db`，也可用 `-db` 指定数据库路径。

创建普通用户和客户端凭据：

```powershell
go run ./cmd/goginx-admin create-user -id user-1 -username alice
go run ./cmd/goginx-admin create-client -id client-1 -user user-1 -name home -credential secret
```

创建 TCP/UDP/HTTP/HTTPS 代理：

```powershell
go run ./cmd/goginx-admin create-tcp-proxy -id tcp-1 -user user-1 -client client-1 -name ssh -port 10022 -target-host 127.0.0.1 -target-port 22
go run ./cmd/goginx-admin create-udp-proxy -id udp-1 -user user-1 -client client-1 -name dns -port 10053 -target-host 127.0.0.1 -target-port 53
go run ./cmd/goginx-admin create-http-proxy -id web-1 -user user-1 -client client-1 -name web -host app.example.com -target-host 127.0.0.1 -target-port 8080
go run ./cmd/goginx-admin create-https-proxy -id secure-1 -user user-1 -client client-1 -name secure -host secure.example.com -target-host 127.0.0.1 -target-port 8443
```

HTTPS 静态证书终止示例：

```powershell
go run ./cmd/goginx-admin create-https-proxy `
  -id secure-term-1 `
  -user user-1 `
  -client client-1 `
  -name secure-term `
  -host term.example.com `
  -target-host 127.0.0.1 `
  -target-port 8080 `
  -cert-file data/certs/term.crt `
  -key-file data/certs/term.key
```

## 运行时配置

### 托管状态文件

服务端默认写入：

- `data/go-ginx.db`
- `data/certs/control-ca.crt`
- `data/certs/control.crt`
- `data/certs/control.key`
- `data/certs/managed/<host>/`

客户端默认写入：

- `data/client-state.json`
- `data/certs/server-ca.crt`

这些是应用生成的运行时状态，不需要手写。

### 环境变量覆盖

无配置启动仍可通过环境变量调整端口和路径：

- `GOGINX_ADMIN_LISTEN`
- `GOGINX_CONTROL_QUIC_LISTEN`
- `GOGINX_CONTROL_TLS_LISTEN`
- `GOGINX_CONTROL_TLS_SERVER_NAME`
- `GOGINX_CONTROL_TLS_CA_FILE`
- `GOGINX_CONTROL_TLS_CERT_FILE`
- `GOGINX_CONTROL_TLS_KEY_FILE`
- `GOGINX_TCP_ENTRY_HOST`
- `GOGINX_HTTP_ENTRY_LISTEN`
- `GOGINX_HTTPS_ENTRY_LISTEN`
- `GOGINX_SQLITE_PATH`
- `GOGINX_DATA_DIR`
- `GOGINX_CERTIFICATE_DIR`

### 显式服务端配置

高级或脚本化部署仍可使用 JSON 配置：

```json
{
  "admin_enabled": true,
  "admin_listen": "127.0.0.1:8080",
  "admin_frontend_dir": "",
  "control_quic_listen": "127.0.0.1:8443",
  "control_tls_listen": "127.0.0.1:9443",
  "control_tls_server_name": "go-ginx-control.local",
  "control_tls_ca_file": "data/certs/control-ca.crt",
  "control_tls_cert_file": "data/certs/control.crt",
  "control_tls_key_file": "data/certs/control.key",
  "tcp_entry_host": "0.0.0.0",
  "http_entry_listen": "0.0.0.0:8081",
  "https_entry_listen": "0.0.0.0:8444",
  "sqlite_path": "data/go-ginx.db",
  "data_dir": "data",
  "certificate_dir": "data/certs",
  "heartbeat_timeout": 45000000000,
  "log_retention_days": 7
}
```

启动：

```powershell
go run ./cmd/goginx-server -config server.json
```

### 显式客户端配置

推荐使用 `goginx-client join <token>`。需要手写配置时可使用：

```json
{
  "server_address": "127.0.0.1:8443",
  "server_tls_address": "127.0.0.1:9443",
  "server_name": "go-ginx-control.local",
  "server_ca_file": "data/certs/server-ca.crt",
  "client_id": "client-1",
  "credential": "secret",
  "allowed_protocols": ["quic", "tcp_tls"],
  "reconnect": {
    "initial_delay": 1000000000,
    "max_delay": 30000000000
  }
}
```

启动：

```powershell
go run ./cmd/goginx-client -config client.json
```

`time.Duration` 字段在 JSON 中使用纳秒数。认证失败会立即退出；临时拨号失败或运行时故障会按 `reconnect` 退避重试。

## 托管 HTTPS 证书

启用 ACME DNS-01 时，服务端需要 Cloudflare API token 环境变量和额外配置：

```json
{
  "acme_enabled": true,
  "acme_directory_url": "https://acme-v02.api.letsencrypt.org/directory",
  "acme_account_email": "ops@example.com",
  "acme_terms_accepted": true,
  "acme_renewal_window": 2592000000000000,
  "acme_cloudflare_token_env": "CF_DNS_API_TOKEN"
}
```

证书管理命令：

```powershell
$env:CF_DNS_API_TOKEN="<cloudflare-token>"
go run ./cmd/goginx-admin issue-managed-certificate -proxy secure-1 -certificate-dir data/certs -acme-account-email ops@example.com -acme-terms-accepted
go run ./cmd/goginx-admin renew-managed-certificate -proxy secure-1 -certificate-dir data/certs -acme-account-email ops@example.com -acme-terms-accepted
go run ./cmd/goginx-admin managed-certificate-status -proxy secure-1 -certificate-dir data/certs -acme-account-email ops@example.com -acme-terms-accepted
```

托管证书文件保存在 `certificate_dir/managed/<host>/`。SQLite 只保存证书生命周期元数据和文件路径。

## 管理 API 与前端

管理 API 保留在 `/api/admin/*` 命名空间：

- `POST /api/admin/login`
- `GET /api/admin/session`
- `POST /api/admin/logout`
- `POST /api/admin/graphql`

客户端加入接口：

- `POST /api/client/enroll`

当前 GraphQL 管理范围包括仪表盘汇总、用户管理、客户端列表和详情、反向代理 CRUD 与生命周期操作、托管证书状态/签发/续期、最近审计列表。浏览器侧 legacy `/graphql` 路由和旧的服务端渲染管理页不再作为本阶段入口。

管理前端源码位于 `admin-ui/`：

```powershell
Set-Location admin-ui
npm ci
npm run test
npm run build
```

服务端默认使用 `internal/adminapi/embedded_admin/` 中的嵌入式前端资源。开发或自定义部署时，可将构建产物目录配置到 `admin_frontend_dir`。

## 构建与测试

完整验证：

```powershell
$env:CGO_ENABLED="0"
go test ./...
go build ./cmd/goginx-server ./cmd/goginx-client ./cmd/goginx-admin
```

重点包测试：

```powershell
$env:CGO_ENABLED="0"
go test ./internal/control
go test ./internal/daemon
go test ./internal/proxy/tcp
go test ./internal/proxy/udp
go test ./internal/proxy/http
go test ./internal/proxy/https
go test ./internal/admin
go test ./internal/adminapi
go test ./internal/certmanager
```

外部进程 smoke 测试：

```powershell
$env:CGO_ENABLED="0"
go test ./e2e -run "TestExternalProcessesProxy(TCP|UDP|HTTP|HTTPS)$" -count=1
```

## 部署包

生成 Linux `systemd` 部署包：

```powershell
$env:CGO_ENABLED="0"
go run ./cmd/goginx-admin build-deploy-bundle `
  -output ./.tmp/linux-systemd-bundle `
  -goos linux `
  -goarch amd64 `
  -install-root /opt/go-ginx
```

部署包核心内容：

- `bin/`：`goginx-server`、`goginx-client`、`goginx-admin`
- `config/`：示例配置和环境文件
- `data/`：SQLite 与证书目录
- `logs/`：日志目录
- `systemd/`：渲染后的 `goginx-server.service` 和 `goginx-client.service`

典型部署流程：

1. 将部署包复制到 `-install-root` 对应目录，例如 `/opt/go-ginx`。
2. 启动服务端并运行 `goginx-admin init-admin` 初始化管理员。
3. 使用 `goginx-admin create-client-join` 生成客户端 join token。
4. 在客户端执行 `goginx-client join <token>`。
5. 安装 `systemd/` 下的 service 到 `/etc/systemd/system/`。
6. 执行 `systemctl daemon-reload`。
7. 执行 `systemctl enable --now goginx-server goginx-client`。

## 当前限制

- 尚未实现 forward proxy。
- 尚未实现配额、限速、普通用户自助、备份恢复、容量校验和高级告警。
- 原生安装器和非 `systemd` 进程管理模板尚未实现。
- 通配域名/平台域名所有权校验尚未实现。
- 管理后台当前以管理员能力为主，普通用户自助和更完整的运维页面仍在后续范围内。

## 参考文档

- `docs/daemon-runtime.md`：守护进程运行和部署说明。
- `docs/milestone-one-e2e.md`：当前可执行验证路径。
- `docs/examples/admin-seed-sqlite.md`：SQLite 种子数据示例。
- `docs/admin-ui/README.md`：管理后台页面设计文档索引。
- `openspec/specs/`：当前规格说明。
