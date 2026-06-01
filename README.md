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
  - `goginx-admin` 可创建管理员、用户、客户端、可重复查看的 join token、代理记录和部署包。
  - 管理监听器提供登录、会话引导、登出、GraphQL 管理操作、客户端注册和同源管理前端。
  - 管理前端默认使用部署根目录 `admin-ui/` 静态资源，`admin_frontend_dir` 可覆盖为自定义构建目录。
- 部署：`build-deploy-bundle` 可生成 Linux `systemd` 部署包或 Windows 发布包，包含服务端、客户端、管理 CLI、示例配置、目录结构和对应平台的运行时布局。

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

- 部署环境不需要安装 Go、Node.js 或 npm，只需要使用 Release 后的部署包或二进制文件。
- Linux `systemd` 部署建议使用 Release 产物中的完整部署包，包内应包含 `bin/`、`config/`、`data/`、`logs/` 和 `systemd/`。
- Windows 或手动运行环境使用对应平台的 Release 压缩包即可；下文 Linux 示例使用 `./bin/goginx-*`，Windows 可替换为 `.\bin\goginx-*.exe`。
- 只有源码开发、测试、发布构建环境需要 Go `1.25` 或与 `go.mod` 匹配的版本。
- 只有开发或重新构建 `admin-ui/` 时需要 Node.js 与 npm。
- 源码测试和发布构建建议禁用 cgo：

```powershell
$env:CGO_ENABLED="0"
```

## 快速开始：Release 产物无配置运行

以下命令假设已经下载并解压 Release 包，且当前目录是解压后的包根目录。默认路径不需要手写 `server.json` 或 `client.json`。服务端首次启动会创建 `data/` 状态目录，并生成控制通道 TLS 材料。

1. 启动服务端：

```bash
./bin/goginx-server
```

默认监听：

- 管理后台：`127.0.0.1:8080`
- 控制通道 QUIC：`:8443`
- 控制通道 TCP+TLS：`:9443`
- HTTP 入口：`:8081`
- HTTPS 入口：默认关闭，需通过环境变量或配置启用

2. 在另一个终端初始化第一个管理员：

```bash
./bin/goginx-admin init-admin -id admin-1 -username admin -password "<password>"
```

3. 生成客户端一次性加入 token：

可以在管理 UI 的 Clients 页面点击 `Create join token` 生成；也可以使用 CLI：

```bash
token="$(./bin/goginx-admin create-client-join -id client-1 -user admin-1 -name home)"
```

服务端启动时会确认一个默认 join 服务域名或 IP，并在日志中显示 `join_service_host`、来源和默认控制通道地址。未显式填写地址时，管理 API 生成的 join token 会使用该默认值；CLI 默认使用本机部署常用的 `127.0.0.1`，远程客户端场景应按下例覆盖。

如果客户端不在本机，需要显式指定外部可访问地址，例如：

```bash
token="$(./bin/goginx-admin create-client-join \
  -id client-1 \
  -user admin-1 \
  -name home \
  -enrollment-url "https://admin.example.com/api/client/enroll" \
  -server-address "control.example.com:8443" \
  -server-tls-address "control.example.com:9443" \
  -server-name "go-ginx-control.local"
)"
```

4. 在客户端主机解压对应平台的 Release 包，进入包根目录后加入并启动客户端：

```bash
./bin/goginx-client join "$token"
./bin/goginx-client
```

`join` 会写入：

- `data/client-state.json`
- `data/certs/server-ca.crt`

后续客户端直接运行 `./bin/goginx-client` 即可读取托管状态。

默认 state/CA 路径按 `goginx-client` 二进制所在的部署根目录解析；如果二进制位于 `bin/`，部署根目录就是 `bin/` 的上一级，因此从 `bin/` 或其他目录启动都仍会使用同一个 `data/client-state.json`。

5. 打开管理后台：

```text
http://127.0.0.1:8080
```

管理端点要求受保护传输：本机 loopback 可用于开发；跨机器部署时应放在 TLS 反向代理之后，或通过 `X-Forwarded-Proto: https` 进入服务。

## 管理 CLI 示例

以下命令可在 Release 包根目录执行。CLI 默认使用部署根目录下的 `data/go-ginx.db`，如果二进制位于 `bin/`，部署根目录就是 `bin/` 的上一级；也可用 `-db` 指定数据库路径。

本地首次配置或小规模维护可以使用 TUI 模式：

```bash
./bin/goginx-admin tui
```

TUI 是面向服务器终端的本地运维入口，使用与其他 `goginx-admin` 子命令相同的默认 SQLite 路径，也支持 `-db <path>` 和 `-actor <id>`。首版范围聚焦管理员设置、用户管理和客户端配置：可创建管理员、更新管理员密码、启用或禁用管理员，创建用户，选择用户快速创建客户端和 join token，或仅创建/轮换客户端凭据。TUI 优先使用列表、角色选项、默认 join 参数和确认页，删除用户或客户端时要求输入资源 ID 强确认；客户端凭据只在当前结果页显示明文，join token 可在客户端菜单中查看，若不可用则查看时自动重置。

脚本和自动化仍应继续使用下方非交互式子命令。TUI 需要交互式终端；在重定向、CI 或不支持终端控制的环境中会拒绝启动，并提示改用非交互式命令。

创建普通用户和客户端凭据：

```bash
./bin/goginx-admin create-user -id user-1 -username alice
./bin/goginx-admin create-client -id client-1 -user user-1 -name home -credential secret
```

创建 TCP/UDP/HTTP/HTTPS 代理：

```bash
./bin/goginx-admin create-tcp-proxy -id tcp-1 -user user-1 -client client-1 -name ssh -port 10022 -target-host 127.0.0.1 -target-port 22
./bin/goginx-admin create-udp-proxy -id udp-1 -user user-1 -client client-1 -name dns -port 10053 -target-host 127.0.0.1 -target-port 53
./bin/goginx-admin create-http-proxy -id web-1 -user user-1 -client client-1 -name web -host app.example.com -target-host 127.0.0.1 -target-port 8080
./bin/goginx-admin create-https-proxy -id secure-1 -user user-1 -client client-1 -name secure -host secure.example.com -target-host 127.0.0.1 -target-port 8443
```

HTTPS 静态证书终止示例：

```bash
./bin/goginx-admin create-https-proxy \
  -id secure-term-1 \
  -user user-1 \
  -client client-1 \
  -name secure-term \
  -host term.example.com \
  -target-host 127.0.0.1 \
  -target-port 8080 \
  -cert-file data/certs/term.crt \
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
- `GOGINX_JOIN_SERVICE_HOST`
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
  "join_service_host": "control.example.com",
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

```bash
./bin/goginx-server -config server.json
```

### 显式客户端配置

推荐使用 `./bin/goginx-client join <token>`。需要手写配置时可使用：

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

```bash
./bin/goginx-client -config client.json
```

`time.Duration` 字段在 JSON 中使用纳秒数。认证失败会立即退出；临时拨号失败或运行时故障会按 `reconnect` 退避重试。

Release 包生成的 `config/client.json` 与 join 流程保持同一套文件名：客户端信任文件为 `data/certs/server-ca.crt`。该文件由 `./bin/goginx-client join <token>` 写入；如果跳过 join 而手写 `client.json`，需要把服务端的 `data/certs/control-ca.crt` 分发到客户端并保存为 `data/certs/server-ca.crt`。

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

```bash
export CF_DNS_API_TOKEN="<cloudflare-token>"
./bin/goginx-admin issue-managed-certificate -proxy secure-1 -certificate-dir data/certs -acme-account-email ops@example.com -acme-terms-accepted
./bin/goginx-admin renew-managed-certificate -proxy secure-1 -certificate-dir data/certs -acme-account-email ops@example.com -acme-terms-accepted
./bin/goginx-admin managed-certificate-status -proxy secure-1 -certificate-dir data/certs -acme-account-email ops@example.com -acme-terms-accepted
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

服务端默认使用部署根目录下的 `admin-ui/` 构建产物目录。若服务端二进制位于 `bin/`，部署根目录就是 `bin/` 的上一级；开发或自定义部署时，可将其他构建产物目录配置到 `admin_frontend_dir`。

## 源码开发与发布构建

本节只面向开发机、CI 或发布机。部署环境不需要执行这些命令，也不需要安装 Go。

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

生成 Linux `systemd` 发布包：

```powershell
Set-Location admin-ui
npm ci
npm run build
Set-Location ..
$env:CGO_ENABLED="0"
go run ./cmd/goginx-admin build-deploy-bundle -output ./dist/linux-systemd-bundle -goos linux -goarch amd64 -install-root /opt/go-ginx
```

`build-deploy-bundle` 会把 `admin-ui/dist` 复制为发布包根目录下的 `admin-ui/`。将 `./dist/linux-systemd-bundle` 作为 Release 产物发布；目标服务器只需要拿到这个目录或其压缩包。

生成 Windows 发布包：

```powershell
Set-Location admin-ui
npm ci
npm run build
Set-Location ..
$env:CGO_ENABLED="0"
go run ./cmd/goginx-admin build-deploy-bundle -output ./dist/windows-amd64-bundle -goos windows -goarch amd64
```

`build-deploy-bundle` 会把 `admin-ui/dist` 复制为发布包根目录下的 `admin-ui/`。Windows 产物不包含 `systemd/`，但保留 `bin/`、`config/`、`data/`、`logs/` 和 `admin-ui/` 目录，适合解压后直接运行 `.\bin\goginx-server.exe`、`.\bin\goginx-client.exe` 和 `.\bin\goginx-admin.exe`。

## Release 部署包部署

Linux `systemd` Release 包核心内容：

- `bin/`：`goginx-server`、`goginx-client`、`goginx-admin`
- `admin-ui/`：管理前端构建产物，默认由管理监听器同源服务
- `config/`：示例配置和环境文件
- `data/`：SQLite 与证书目录
- `logs/`：日志目录
- `systemd/`：渲染后的 `goginx-server.service` 和 `goginx-client.service`

服务器部署流程示例：

```bash
sudo mkdir -p /opt/go-ginx
sudo tar -xzf <release-bundle>.tar.gz -C /opt/go-ginx --strip-components=1
cd /opt/go-ginx
sudo cp systemd/goginx-server.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now goginx-server
sudo ./bin/goginx-admin init-admin -id admin-1 -username admin -password "<password>"
# 也可以在管理 UI 的 Clients 页面点击 Create join token 生成。
token="$(sudo ./bin/goginx-admin create-client-join -id client-1 -user admin-1 -name home)"
```

客户端部署流程示例：

```bash
sudo mkdir -p /opt/go-ginx
sudo tar -xzf <release-bundle>.tar.gz -C /opt/go-ginx --strip-components=1
cd /opt/go-ginx
sudo ./bin/goginx-client join "$token"
test -f data/client-state.json
sudo cp systemd/goginx-client.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now goginx-client
```

`goginx-client` 服务默认读取部署根目录下的 `data/client-state.json`，这个文件只会由 `./bin/goginx-client join <token>` 生成。若先启动服务，会看到 `load client config: ... data/client-state.json ... cannot find the path specified`；处理方式是在客户端机器上先执行 join，然后再启动服务。若使用自定义路径，启动和 join 都需要显式传入对应 `-config`、`-state` 或 `-ca-file`。

如果不是 `systemd` 环境，也可以直接在 Release 包根目录运行 `./bin/goginx-server` 或 `./bin/goginx-client`，并由外部进程管理器负责守护进程生命周期。

### 8080 返回 404 的排查

Release 包的推荐启动方式是直接运行 `./bin/goginx-server`，或使用包内 `systemd/goginx-server.service`。这条路径会启用管理监听器，并从部署根目录下的 `admin-ui/` 提供 `/`、`/login`、`/dashboard` 等页面；该默认前端路径按二进制所在位置推导，不依赖启动时的当前工作目录。

如果访问 `8080` 返回 `404`，先检查实际访问到的是不是管理监听器：

```bash
curl -i http://127.0.0.1:8080/
curl -i http://127.0.0.1:8080/api/admin/session
```

正常情况下，第一个请求应返回管理前端 HTML，第二个请求应返回 JSON 会话状态。若服务启动失败并提示 admin frontend 目录错误，请确认 Release 根目录包含 `admin-ui/index.html`，或显式设置 `admin_frontend_dir`。若服务日志显示 `admin=disabled`，说明不是按无配置 Release 路径启动，或显式配置关闭了 `admin_enabled`。若使用 `-config config/server.json`，请确认该配置中的 `admin_enabled` 为 `true`，并且控制通道证书文件已经存在；首次部署更推荐先使用无配置启动，让服务自动生成 `data/` 状态和控制通道 TLS 材料。

更新管理前端时，重新构建 `admin-ui/dist`，把构建产物同步到 Release 根目录的 `admin-ui/`，然后重启 `goginx-server`。服务端不会热加载前端文件。

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
