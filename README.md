# go-ginx-2

`go-ginx-2` 是 Simp-Frp/go-ginx 设计的新实现目标。当前仓库已经完成里程碑一运行时和首个部署基线，重点覆盖控制面、反向代理、管理 API/UI、证书管理、SQLite 持久化和可复现部署包。

> 状态说明：这还不是完整生产平台。核心 TCP/UDP/HTTP/HTTPS 反向代理路径、QUIC 与 TCP+TLS 控制通道、无配置启动、客户端加入流程、管理后台、Linux `systemd` 部署模板和 Windows 原生服务命令已经实现并有本地测试覆盖；配额、限速、普通用户自助、备份恢复和更完整的运维能力仍在后续范围内。

## 目录

- [项目状态](#项目状态)
- [核心能力](#核心能力)
- [仓库结构](#仓库结构)
- [环境要求](#环境要求)
- [快速开始：Release 产物无配置运行](#快速开始release-产物无配置运行)
- [管理 CLI 示例](#管理-cli-示例)
- [运行时配置](#运行时配置)
- [托管 HTTPS 证书](#托管-https-证书)
- [管理 API 与前端](#管理-api-与前端)
- [源码开发与发布构建](#源码开发与发布构建)
- [Release 部署包部署](#release-部署包部署)
- [当前限制](#当前限制)
- [参考文档](#参考文档)

## 项目状态

| 维度 | 当前状态 |
| --- | --- |
| Go 模块 | `github.com/simp-frp/go-ginx-2` |
| Go 版本 | `go.mod` 使用 Go `1.25.0` |
| 运行时基线 | 服务端、客户端、管理 CLI、管理前端、部署包已具备 |
| 数据存储 | 默认 cgo-free SQLite，路径为 `data/go-ginx.db` |
| 部署目标 | Linux `systemd` 部署包和 Windows 发布包 |

## 核心能力

| 能力 | 说明 |
| --- | --- |
| 控制面 | 支持 QUIC 控制通道和 TCP+TLS 兜底控制通道；已实现客户端认证、TLS/CA 校验、代理快照同步、心跳和最新会话替换。 |
| 数据层 | 通过仓储接口使用 cgo-free SQLite；运行时状态默认保存在 `data/go-ginx.db`。 |
| TCP 代理 | TCP 反向代理，代理流可走 QUIC stream 或 TCP+TLS framed substream。 |
| UDP 代理 | UDP 反向代理，按外部源地址维护会话。 |
| HTTP 代理 | HTTP 反向代理，按 `Host` 路由。 |
| HTTPS 代理 | 要求服务端持有可用证书并终止 TLS，支持静态证书和 ACME DNS-01 托管证书。 |
| 证书管理 | 控制通道 TLS 材料可自动生成；HTTPS 托管证书支持 Cloudflare DNS-01、签发、续期、健康状态、退避重试和热加载。 |
| 管理能力 | `goginx-admin` 可创建管理员、用户、客户端、可重复查看的 join token、代理记录和部署包。 |
| 管理监听器 | 提供 8 小时 JWT 登录、会话引导、登出、GraphQL 管理操作、客户端注册和同源管理前端。 |
| 管理前端 | 默认使用部署根目录 `admin-ui/` 静态资源；`admin_frontend_dir` 可覆盖为自定义构建目录。 |
| 部署构建 | `build-deploy-bundle` 可生成 Linux `systemd` 部署包或带原生服务辅助脚本的 Windows 发布包。 |

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
openspec/          待迁移的 OpenSpec 历史资料
```

## 环境要求

### 部署环境

- 部署环境不需要安装 Go、Node.js、Corepack 或 pnpm，只需要使用 Release 后的部署包或二进制文件。
- Linux `systemd` 部署建议使用 Release 产物中的完整部署包，包内应包含 `bin/`、`config/`、`data/`、`logs/` 和 `systemd/`。
- Windows 或手动运行环境使用对应平台的 Release 压缩包即可。
- 下文 Linux 示例使用 `./bin/goginx-*`，Windows 可替换为 `.\bin\goginx-*.exe`。

### 开发与构建环境

- 只有源码开发、测试、发布构建环境需要 Go `1.25` 或与 `go.mod` 匹配的版本。
- 只有开发或重新构建 `admin-ui/` 时需要 Node.js；前端包管理通过 Corepack 启用 pnpm。
- 源码测试和发布构建建议禁用 cgo：

```powershell
$env:CGO_ENABLED="0"
```

## 快速开始：Release 产物无配置运行

以下命令假设已经下载并解压 Release 包，且当前目录是解压后的包根目录。默认路径不需要手写 `server.json` 或 `client.json`。

服务端首次启动会创建 `data/` 状态目录，并生成控制通道 TLS 材料和管理 JWT 签名密钥。

### 1. 确认客户端可访问的 join 地址

如果客户端不在服务端本机，必须先准备一个客户端可访问的域名或 IP，并在启动服务端和生成 join token 之前配置它。否则 token 可能写入 `127.0.0.1`、内网地址或其他本地兜底地址，远程客户端会因为找不到正确 server 而无法 join。

`GOGINX_JOIN_SERVICE_HOST` 只填写主机名或 IP，不要包含协议、端口或路径：

```bash
export GOGINX_JOIN_SERVICE_HOST="control.example.com"
```

这个变量需要出现在两个地方：

- 启动 `goginx-server` 的终端或服务环境。
- 执行 `goginx-admin create-client-join` 的终端。

服务端会用该地址生成默认控制通道和 enrollment 地址：

| 用途 | 默认生成结果 |
| --- | --- |
| 客户端 enrollment | `http://control.example.com:8081/api/client/enroll` |
| 控制通道 QUIC | `control.example.com:8443` |
| 控制通道 TCP+TLS | `control.example.com:9443` |

如果 enrollment 入口放在 HTTPS 反向代理之后，或端口不是默认值，请在生成 token 时显式传入 `-enrollment-url`、`-server-address` 和 `-server-tls-address`。

只在同一台机器上本地试用时，可以跳过本步骤。

### 2. 启动服务端

```bash
./bin/goginx-server
```

默认监听：

| 用途 | 地址 |
| --- | --- |
| 管理后台 | `127.0.0.1:8080` |
| 客户端 enrollment | `:8081`，仅服务 `/api/client/enroll` |
| 控制通道 QUIC | `:8443` |
| 控制通道 TCP+TLS | `:9443` |
| HTTP 入口 | `:80` |
| HTTPS 入口 | `:443` |

HTTP/HTTPS 入口使用标准低端口。在 Linux/Unix 上可能需要 root、`CAP_NET_BIND_SERVICE`、服务管理器授权，或通过环境变量/JSON 显式改用非特权端口。

服务端启动时会确认一个默认 join 服务域名或 IP，并在日志中显示：

- `join_service_host`
- 来源
- 默认控制通道地址
- 默认 enrollment URL

如果日志里看到 `join_service_source` 是 `local_interface` 或 `loopback_fallback`，或 `join_enrollment_url` 指向的不是客户端可访问地址，请先配置 `GOGINX_JOIN_SERVICE_HOST` 或 `join_service_host`，重启服务端，然后重新生成 join token。

### 3. 初始化第一个管理员

在另一个终端执行：

```bash
./bin/goginx-admin init-admin -id admin-1 -username admin -password "<password>"
```

### 4. 生成客户端一次性加入 token

可以在管理 UI 的 Clients 页面点击 `Create join token` 生成；也可以使用 CLI：

```bash
token="$(./bin/goginx-admin create-client-join -id client-1 -user admin-1 -name home)"
```

未显式填写地址时，管理 API、`goginx-admin create-client-join`、`goginx-admin client-join-command` 和 TUI 都会使用同一套默认 join 参数解析规则：

- 显式 `-server-config`
- 部署根 `config/server.json`
- `GOGINX_JOIN_SERVICE_HOST`
- `GOGINX_CLIENT_ENROLLMENT_LISTEN`
- managed 默认值
- 本地兜底

远程客户端场景必须配置 `join_service_host` 或 `GOGINX_JOIN_SERVICE_HOST`，不要把本地兜底地址当作公网默认值。已经生成过的旧 token 不会自动更新地址；修改 join 地址后需要重新生成 token。

客户端 join token 默认通过专用 enrollment listener 兑换，而不是 admin listener。旧的、指向 `admin_listen` 上 `/api/client/enroll` 的 token 不再可用；重新生成 token 后使用新的 enrollment URL。

也可以在单次创建 token 时显式指定外部可访问地址：

```bash
token="$(./bin/goginx-admin create-client-join \
  -id client-1 \
  -user admin-1 \
  -name home \
  -enrollment-url "https://join.example.com/api/client/enroll" \
  -server-address "control.example.com:8443" \
  -server-tls-address "control.example.com:9443" \
  -server-name "go-ginx-control.local"
)"
```

如果使用显式 server 配置文件生成 join token，可把配置路径交给 admin CLI：

```bash
token="$(./bin/goginx-admin create-client-join \
  -server-config config/server.json \
  -id client-1 \
  -user admin-1 \
  -name home
)"
```

### 5. 加入并启动客户端

在客户端主机解压对应平台的 Release 包，进入包根目录后执行：

```bash
./bin/goginx-client join "$token"
./bin/goginx-client
```

`join` 会写入：

- `data/client-state.json`
- `config/client.json`
- `data/certs/server-ca.crt`

后续客户端直接运行 `./bin/goginx-client` 即可读取托管状态；需要显式配置启动时也可以使用：

```bash
./bin/goginx-client -config config/client.json
```

默认 state/config/CA 路径按 `goginx-client` 二进制所在的部署根目录解析。如果二进制位于 `bin/`，部署根目录就是 `bin/` 的上一级，因此从 `bin/` 或其他目录启动都仍会使用同一个 `data/client-state.json` 和 `config/client.json`。

### 6. 打开管理后台

```text
http://127.0.0.1:8080
```

管理端点要求受保护传输。本机 loopback 可用于开发；跨机器部署时应放在 TLS 反向代理之后，或通过 `X-Forwarded-Proto: https` 进入服务。

## 管理 CLI 示例

以下命令可在 Release 包根目录执行。CLI 默认使用部署根目录下的 `data/go-ginx.db`；如果二进制位于 `bin/`，部署根目录就是 `bin/` 的上一级。也可用 `-db` 指定数据库路径。

### TUI 模式

本地首次配置或小规模维护可以使用 TUI：

```bash
./bin/goginx-admin tui
```

TUI 是面向服务器终端的本地运维入口，使用与其他 `goginx-admin` 子命令相同的默认 SQLite 路径，也支持 `-db <path>` 和 `-actor <id>`。

首版范围聚焦管理员设置、用户管理和客户端配置：

- 创建管理员、更新管理员密码、启用或禁用管理员。
- 创建用户。
- 选择用户快速创建客户端和 join token。
- 仅创建或轮换客户端凭据。

TUI 优先使用列表、角色选项、默认 join 参数和确认页。删除用户或客户端时要求输入资源 ID 强确认；客户端凭据只在当前结果页显示明文，join token 可在客户端菜单中查看，若不可用则查看时自动重置。

脚本和自动化仍应继续使用非交互式子命令。TUI 需要交互式终端；在重定向、CI 或不支持终端控制的环境中会拒绝启动，并提示改用非交互式命令。

### 用户与客户端

创建普通用户和客户端凭据：

```bash
./bin/goginx-admin create-user -id user-1 -username alice
./bin/goginx-admin create-client -id client-1 -user user-1 -name home -credential secret
```

### 代理配置

创建 TCP/UDP/HTTP/HTTPS 代理：

```bash
./bin/goginx-admin create-tcp-proxy -id tcp-1 -user user-1 -client client-1 -name ssh -port 10022 -target-host 127.0.0.1 -target-port 22
./bin/goginx-admin create-udp-proxy -id udp-1 -user user-1 -client client-1 -name dns -port 10053 -target-host 127.0.0.1 -target-port 53
./bin/goginx-admin create-http-proxy -id web-1 -user user-1 -client client-1 -name web -host app.example.com -target-host 127.0.0.1 -target-port 8080
./bin/goginx-admin create-https-proxy -id secure-1 -user user-1 -client client-1 -name secure -host secure.example.com -target-host 127.0.0.1 -target-port 8443
```

四类代理都可以用 `-bind-host` 指定实际监听地址：

- TCP/UDP 的 `-port` 是入口端口。
- HTTP/HTTPS 的 `-host` 是 HTTP Host 或 HTTPS SNI 域名。
- HTTP/HTTPS 的 `-port` 可指定入口监听端口，留空时使用 `http_entry_listen` 或 `https_entry_listen` 的默认端口。

服务端会在代理创建、更新、启用、禁用或删除后热协调 listener：需要的新监听会立即启动，不再被任何启用代理使用的自定义监听会关闭。

### HTTPS 静态证书终止

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

HTTPS 代理按 TLS ClientHello 中的 SNI 路由到对应代理后，会先尝试选择可用于该 SNI 的证书材料：

| 证书状态 | 运行行为 |
| --- | --- |
| 代理配置了完整的 `cert_file` 和 `key_file` | 服务端使用这对静态证书终止公网 TLS，并把解密后的 HTTP 请求转发到客户端本地 HTTP 目标。 |
| 没有静态证书，但存在健康的 active 托管证书 | `serving_status` 为 `usable` 或 `expiring_soon` 时执行 TLS 终止；`expiring_soon` 仍可服务并进入续期候选。 |
| 没有完整静态证书，也没有可服务 active 托管证书 | HTTPS 代理标记为 `needs_config`，匹配 SNI 的公网 TLS 连接会被拒绝或关闭。 |
| 显式配置了静态证书但无效 | 如果证书/私钥不完整、文件不可读、已过期、域名不匹配或 key 不匹配，该连接会失败；运行时不会自动降级为 passthrough。 |

HTTPS proxy 不再提供 SNI passthrough 回退。需要纯 TLS/SNI 透传的部署应迁移到后续独立的透传代理能力；当前 `https` 类型始终表示服务端终止公网 TLS 并把解密后的 HTTP 请求转发到客户端本地 HTTP 目标。

## 运行时配置

### 托管状态文件

服务端默认写入：

| 类型 | 路径 |
| --- | --- |
| SQLite 数据库 | `data/go-ginx.db` |
| 控制通道 CA | `data/certs/control-ca.crt` |
| 控制通道证书 | `data/certs/control.crt` |
| 控制通道私钥 | `data/certs/control.key` |
| 托管 HTTPS 证书 | `data/certs/managed/<host>/` |
| Cloudflare provider credential secrets | `data/secrets/provider-credentials/` |
| 管理 JWT 签名密钥 | `data/admin-jwt.key` |

客户端默认写入：

| 类型 | 路径 |
| --- | --- |
| 客户端托管状态 | `data/client-state.json` |
| 客户端显式配置 | `config/client.json` |
| 服务端 CA 信任文件 | `data/certs/server-ca.crt` |

运行日志默认写入：

| 类型 | 路径 |
| --- | --- |
| 服务端日志 | `logs/server.log` |
| 客户端日志 | `logs/client.log` |

server/client 默认启用应用内日志轮换，当前写入文件保持上述固定名称；达到 `log_max_size_mb` 后会归档为 `server-YYYYMMDD-HHMMSS.log` 或 `client-YYYYMMDD-HHMMSS.log`，并按 `log_retention_days` 和 `log_max_backups` 清理旧归档。错误日志不拆分为单独文件，而是在同一日志流中保留错误级别、错误类别和上下文；凭据、令牌、Cookie、私钥和请求体不应写入日志。

这些是应用生成的运行时状态，不需要手写。

### 环境变量覆盖

无配置启动仍可通过环境变量调整端口和路径：

| 分类 | 环境变量 |
| --- | --- |
| 管理监听器 | `GOGINX_ADMIN_LISTEN` |
| 管理 JWT | `GOGINX_ADMIN_JWT_SECRET_FILE` |
| 客户端加入 | `GOGINX_CLIENT_ENROLLMENT_LISTEN`、`GOGINX_JOIN_SERVICE_HOST` |
| 控制通道 | `GOGINX_CONTROL_QUIC_LISTEN`、`GOGINX_CONTROL_TLS_LISTEN`、`GOGINX_CONTROL_TLS_SERVER_NAME` |
| 控制通道 TLS 文件 | `GOGINX_CONTROL_TLS_CA_FILE`、`GOGINX_CONTROL_TLS_CERT_FILE`、`GOGINX_CONTROL_TLS_KEY_FILE` |
| 代理入口 | `GOGINX_TCP_ENTRY_HOST`、`GOGINX_HTTP_ENTRY_LISTEN`、`GOGINX_HTTPS_ENTRY_LISTEN` |
| 数据和证书目录 | `GOGINX_SQLITE_PATH`、`GOGINX_DATA_DIR`、`GOGINX_CERTIFICATE_DIR` |

### 显式服务端配置

高级或脚本化部署仍可使用 JSON 配置：

```json
{
  "admin_enabled": true,
  "admin_listen": "127.0.0.1:8080",
  "admin_frontend_dir": "",
  "admin_jwt_secret_file": "data/admin-jwt.key",
  "client_enrollment_listen": "0.0.0.0:8081",
  "control_quic_listen": "127.0.0.1:8443",
  "control_tls_listen": "127.0.0.1:9443",
  "control_tls_server_name": "go-ginx-control.local",
  "control_tls_ca_file": "data/certs/control-ca.crt",
  "control_tls_cert_file": "data/certs/control.crt",
  "control_tls_key_file": "data/certs/control.key",
  "join_service_host": "control.example.com",
  "tcp_entry_host": "0.0.0.0",
  "http_entry_listen": "0.0.0.0:80",
  "https_entry_listen": "0.0.0.0:443",
  "sqlite_path": "data/go-ginx.db",
  "data_dir": "data",
  "certificate_dir": "data/certs",
  "origin_ca_enabled": true,
  "origin_ca_secret_store_path": "data/secrets/provider-credentials",
  "origin_ca_default_request_type": "origin-ecc",
  "origin_ca_requested_validity": 5475,
  "origin_ca_rotation_window": 2592000000000000,
  "heartbeat_timeout": 45000000000,
  "log_max_size_mb": 50,
  "log_max_backups": 10,
  "log_retention_days": 7,
  "log_compress": true
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
  },
  "log_max_size_mb": 50,
  "log_max_backups": 10,
  "log_retention_days": 7,
  "log_compress": true
}
```

启动：

```bash
./bin/goginx-client -config client.json
```

`time.Duration` 字段在 JSON 中使用纳秒数。认证失败会立即退出；临时拨号失败或运行时故障会按 `reconnect` 退避重试。

Release 包包含 `config/client.example.json` 作为显式配置参考；`./bin/goginx-client join <token>` 会写入实际的 `config/client.json` 和客户端信任文件 `data/certs/server-ca.crt`。

如果跳过 join 而手写 `client.json`，需要把服务端的 `data/certs/control-ca.crt` 分发到客户端并保存为 `data/certs/server-ca.crt`。

## 托管 HTTPS 证书

### ACME DNS-01

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

托管证书文件保存在 `certificate_dir/managed/<host>/`。SQLite 只保存证书生命周期元数据和文件路径，不保存私钥字节或 Cloudflare token。

### Cloudflare Origin CA

Cloudflare Origin CA 是另一类托管证书 provider。它不使用 DNS-01 challenge；服务端本地生成私钥和 CSR，只把 CSR、hostnames、request type 和 requested validity 发送给 Cloudflare。私钥仍只写入 `certificate_dir/managed/<host>/`，Cloudflare API Token 明文只写入 SQLite 外的 `origin_ca_secret_store_path`。

托管默认启动会默认启用 Cloudflare Origin CA，并使用 `data/secrets/provider-credentials` 保存 Admin UI 写入的 credential secret。只有显式配置需要覆盖路径或关闭该能力时，才需要手写下面的配置项。

```json
{
  "origin_ca_enabled": true,
  "origin_ca_secret_store_path": "data/secrets/provider-credentials",
  "origin_ca_default_request_type": "origin-ecc",
  "origin_ca_requested_validity": 5475,
  "origin_ca_rotation_window": 2592000000000000
}
```

在 Admin UI 的 Certificates 页面创建、更新并验证 Cloudflare Origin CA API Token credential；token 字段是 write-only，查询响应、审计事件和 SQLite 只包含 credential metadata、token 指纹和 secret 引用。不要使用 Origin CA Service Key，配置中的 `origin_ca_service_key_path` 会被拒绝。

创建或轮换 Origin CA 证书时，HTTPS proxy 的 DNS 记录应在 Cloudflare 中保持 proxied，SSL/TLS mode 应使用 Full (strict) 或等价的严格 origin 校验路径。Origin CA 证书只适合 Cloudflare 到 origin 的 TLS 连接，公网浏览器直连 origin 不会按普通 WebPKI 信任该证书。

CLI 可以消费已经由 Admin UI/API 写入的 credential ID；如果未显式提供 credential，系统只会在 Cloudflare Origin CA 的可用 credential 中按 provider/status scoped 查询唯一默认项，多于一个可用 credential 时会要求显式选择：

```bash
./bin/goginx-admin issue-managed-certificate \
  -proxy secure-1 \
  -provider cloudflare_origin_ca \
  -credential cfcred_123 \
  -certificate-dir data/certs \
  -origin-ca-secret-store data/secrets/provider-credentials

./bin/goginx-admin sync-origin-ca-certificate \
  -proxy secure-1 \
  -certificate-dir data/certs \
  -origin-ca-secret-store data/secrets/provider-credentials
```

Origin CA 的调度窗口由 `origin_ca_rotation_window` 控制；ACME renewal window、Origin CA rotation window、`expiring_soon` 状态和失败 `next_attempt_at` 都来自统一生命周期调度规则。进入窗口后 daemon 会把证书纳入 provider-specific rotation 候选，并沿用失败退避和 active material 保留语义。撤销是高风险动作，不会在轮换后自动执行；只有显式提供 proxy ID、host 和 Cloudflare certificate ID 时才会调用 revoke。撤销当前 active 证书会让 Cloudflare 到 origin 的 Full (strict) 连接失败，通常应先轮换并确认新证书已部署。

```bash
./bin/goginx-admin revoke-origin-ca-certificate \
  -proxy secure-1 \
  -host secure.example.com \
  -cloudflare-certificate-id <cloudflare-origin-ca-certificate-id> \
  -certificate-dir data/certs \
  -origin-ca-secret-store data/secrets/provider-credentials
```

托管证书状态拆分为两类：`serving_status` 描述当前 active 证书材料是否能服务 TLS，取值包括 `usable`、`expiring_soon`、`expired`、`missing` 和 `invalid`；`operation_status` 描述最近一次签发或续期操作，取值包括 `idle`、`issue_failed` 和 `renewal_failed` 等。续期失败只会记录操作失败、失败次数和 `next_attempt_at` 退避时间；只要上一组 active 证书仍健康，新握手会继续使用它。续期成功后会更新 active 文件、证书 SHA-256 指纹、过期时间，并重置失败次数和退避状态。

Cloudflare Origin CA 还会展示 `provider_status`、`credential_id`、Cloudflare certificate ID、hostnames、request type、requested validity 和 `last_synced_at`。当 provider sync 确认 active Origin CA 证书已 revoked 或 missing_remote 时，HTTPS runtime 不会继续声明该托管证书可服务，并会把对应 HTTPS proxy 映射为需要证书配置。

## 管理 API 与前端

管理 API 保留在 `/api/admin/*` 命名空间：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/admin/login` | 管理员登录 |
| `GET` | `/api/admin/session` | 当前会话上下文 |
| `POST` | `/api/admin/logout` | 清除当前浏览器 Cookie |
| `POST` | `/api/admin/graphql` | GraphQL 管理操作 |

客户端加入接口仅由专用 enrollment listener 提供，不由 admin listener 提供：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/client/enroll` | 客户端 join token 兑换 |

登录成功后服务端签发 8 小时绝对有效期的管理员 JWT，并写入现有的 HttpOnly Cookie。`/api/admin/session` 验证 Cookie 后返回前端需要的管理员上下文和 CSRF token。

GraphQL query 只要求 JWT 有效，mutation 继续要求 `X-GoGinx-CSRF-Token` 与 JWT claim 匹配。短时间不操作不会触发 idle timeout。

只要 `admin_jwt_secret_file` 指向的签名密钥不变，服务端重启后未过期的管理员 JWT 仍可继续使用。`POST /api/admin/logout` 会清除当前浏览器 Cookie；纯无状态 JWT 不承诺服务端吊销外部保存的未过期 token。

首次从旧版本升级时，旧的进程内 session Cookie 无法迁移，管理员需要重新登录一次；之后请把 `data/admin-jwt.key` 与 SQLite、证书一起备份，删除或轮换该文件会让既有管理员 JWT 失效。

当前 GraphQL 管理范围包括仪表盘汇总、用户管理、客户端列表和详情、反向代理 CRUD 与生命周期操作、托管证书状态/签发/续期、最近审计列表。浏览器侧 legacy `/graphql` 路由和旧的服务端渲染管理页不再作为本阶段入口。

管理前端源码位于 `admin-ui/`：

```powershell
Set-Location admin-ui
corepack enable
pnpm install --frozen-lockfile
pnpm test
pnpm build
```

服务端默认使用部署根目录下的 `admin-ui/` 构建产物目录。若服务端二进制位于 `bin/`，部署根目录就是 `bin/` 的上一级；开发或自定义部署时，可将其他构建产物目录配置到 `admin_frontend_dir`。

## 源码开发与发布构建

本节只面向开发机、CI 或发布机。部署环境不需要执行这些命令，也不需要安装 Go。

### Docker 开发环境

可以使用仓库内置的 Docker Compose 配置在容器中运行后端、Admin UI、测试和构建：

```powershell
docker compose up --build
```

启动后访问 `http://localhost:5173`。初始化管理员、运行测试和清理 volume 的详细说明见 `docs/docker-development.md`。

### 本机开发命令

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

### Linux `systemd` 发布包

```powershell
Set-Location admin-ui
corepack enable
pnpm install --frozen-lockfile
pnpm build
Set-Location ..
$env:CGO_ENABLED="0"
go run ./cmd/goginx-admin build-deploy-bundle -output ./dist/linux-systemd-bundle -goos linux -goarch amd64 -install-root /opt/go-ginx
```

`build-deploy-bundle` 会把 `admin-ui/dist` 复制为发布包根目录下的 `admin-ui/`，并在 `config/` 下生成 `server.example.json` 和 `client.example.json` 作为显式配置参考。

将 `./dist/linux-systemd-bundle` 作为 Release 产物发布；目标服务器只需要拿到这个目录或其压缩包。

### Windows 发布包

```powershell
Set-Location admin-ui
corepack enable
pnpm install --frozen-lockfile
pnpm build
Set-Location ..
$env:CGO_ENABLED="0"
go run ./cmd/goginx-admin build-deploy-bundle -output ./dist/windows-amd64-bundle -goos windows -goarch amd64
```

`build-deploy-bundle` 会把 `admin-ui/dist` 复制为发布包根目录下的 `admin-ui/`，并在 `config/` 下生成 `server.example.json` 和 `client.example.json` 作为显式配置参考。

Windows 产物不包含 `systemd/`，但保留 `bin/`、`config/`、`data/`、`logs/`、`admin-ui/` 和 `scripts/` 目录。可以直接运行 `.\bin\goginx-server.exe`、`.\bin\goginx-client.exe` 和 `.\bin\goginx-admin.exe`，也可以使用内置 Windows Service 命令或 `scripts/` 下的 PowerShell 辅助脚本安装为原生 Windows 服务。

## Release 部署包部署

Linux `systemd` Release 包核心内容：

| 路径 | 内容 |
| --- | --- |
| `bin/` | `goginx-server`、`goginx-client`、`goginx-admin` |
| `admin-ui/` | 管理前端构建产物，默认由管理监听器同源服务 |
| `config/` | `server.example.json`、`client.example.json`、示例环境文件和可选凭据示例 |
| `data/` | SQLite、证书目录与 `admin-jwt.key` 管理 JWT 签名密钥 |
| `logs/` | 运行日志目录，默认包含 `server.log` 和 `client.log`，归档日志由应用内轮换机制管理 |
| `systemd/` | 渲染后的 `goginx-server.service` 和 `goginx-client.service` |

### 服务器部署流程

下面示例按远程客户端部署编写，请把 `control.example.com` 替换为客户端可访问的域名或 IP。这个值必须在 `goginx-server` 启动前写入服务环境文件，后续通过管理 UI 生成的 token 才会带上正确地址。

```bash
sudo mkdir -p /opt/go-ginx
sudo tar -xzf <release-bundle>.tar.gz -C /opt/go-ginx --strip-components=1
cd /opt/go-ginx
printf 'GOGINX_JOIN_SERVICE_HOST=control.example.com\n' | sudo tee config/goginx-server.env >/dev/null
sudo cp systemd/goginx-server.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now goginx-server
sudo ./bin/goginx-admin init-admin -id admin-1 -username admin -password "<password>"
# 也可以在管理 UI 的 Clients 页面点击 Create join token 生成。
token="$(sudo env GOGINX_JOIN_SERVICE_HOST=control.example.com ./bin/goginx-admin create-client-join -id client-1 -user admin-1 -name home)"
```

如果只做本机试用，可以不创建 `config/goginx-server.env`，但生成的 token 通常不能直接给远程客户端使用。

如果 enrollment 入口经过 HTTPS 反向代理，或控制通道端口不是默认值，请用显式地址生成 token：

```bash
token="$(sudo ./bin/goginx-admin create-client-join \
  -id client-1 \
  -user admin-1 \
  -name home \
  -enrollment-url "https://join.example.com/api/client/enroll" \
  -server-address "control.example.com:8443" \
  -server-tls-address "control.example.com:9443" \
  -server-name "go-ginx-control.local"
)"
```

### 客户端部署流程

```bash
sudo mkdir -p /opt/go-ginx
sudo tar -xzf <release-bundle>.tar.gz -C /opt/go-ginx --strip-components=1
cd /opt/go-ginx
sudo ./bin/goginx-client join "$token"
test -f data/client-state.json
test -f config/client.json
sudo cp systemd/goginx-client.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now goginx-client
```

`goginx-client` 服务默认读取部署根目录下的 `data/client-state.json`，同时 `./bin/goginx-client join <token>` 也会更新 `config/client.json` 供显式 `-config` 启动使用。

若先启动服务，会看到 `load client config: ... data/client-state.json ... cannot find the path specified`；处理方式是在客户端机器上先执行 join，然后再启动服务。若使用自定义路径，启动和 join 都需要显式传入对应 `-config`、`-state` 或 `-ca-file`。

如果不是 `systemd` 环境，也可以直接在 Release 包根目录运行 `./bin/goginx-server` 或 `./bin/goginx-client`，并由外部进程管理器负责守护进程生命周期。

### Windows 服务部署流程

Windows 发布包支持原生 Windows Service，不需要 WinSW 或 NSSM。以下示例假设发布包解压到 `C:\go-ginx`，并在管理员 PowerShell 中执行。

远程客户端部署时，推荐先写 `config/server.json` 并配置客户端可访问的 `join_service_host`。不要只依赖当前 PowerShell 中的临时环境变量，因为 Windows 服务运行环境不会稳定继承它们：

```powershell
cd C:\go-ginx
Copy-Item .\config\server.example.json .\config\server.json
# 编辑 config\server.json，把 join_service_host 设置为 control.example.com
```

安装并启动服务端服务：

```powershell
.\scripts\goginx-server-service.ps1 -Action install -Config config\server.json
.\scripts\goginx-server-service.ps1 -Action start
.\scripts\goginx-server-service.ps1 -Action status
```

也可以不用脚本，直接调用内置命令：

```powershell
.\bin\goginx-server.exe service install -config config\server.json
.\bin\goginx-server.exe service start
.\bin\goginx-server.exe service status
```

初始化管理员并生成客户端 join token 时，建议使用同一个 server 配置：

```powershell
.\bin\goginx-admin.exe init-admin -id admin-1 -username admin -password "<password>"
$token = .\bin\goginx-admin.exe create-client-join -server-config config\server.json -id client-1 -user admin-1 -name home
```

如果 enrollment 入口经过 HTTPS 反向代理，或控制通道端口不是默认值，请改用显式地址参数生成 token。

客户端机器上先执行 join，确认受管状态已写入，再安装服务：

```powershell
cd C:\go-ginx
.\bin\goginx-client.exe join "$token"
Test-Path .\data\client-state.json
.\scripts\goginx-client-service.ps1 -Action install
.\scripts\goginx-client-service.ps1 -Action start
.\scripts\goginx-client-service.ps1 -Action status
```

对应的内置命令是：

```powershell
.\bin\goginx-client.exe service install
.\bin\goginx-client.exe service start
.\bin\goginx-client.exe service status
```

常用服务操作：

```powershell
.\scripts\goginx-server-service.ps1 -Action restart
.\scripts\goginx-server-service.ps1 -Action stop
.\scripts\goginx-server-service.ps1 -Action uninstall
```

Windows 服务日志仍写入部署根目录下的 `logs/server.log` 和 `logs/client.log`，首版不注册 Windows Event Log source。首版安装命令也不提供自定义服务账户参数；如需自定义账户，可在服务安装后使用 Windows 原生服务管理工具调整。

升级 Windows 发布包时，先停止服务，替换 `bin/`、`admin-ui/`、`scripts/` 和示例配置等发布文件，保留 `data/`、已有 `config/server.json`、`config/client.json` 和证书/JWT 状态，然后再启动服务。回滚也按同样流程处理。

### 日志轮换与平台处理

默认日志轮换配置为单个当前文件 50 MiB、最多保留 10 个归档、归档保留 7 天并启用 gzip 压缩。Linux `systemd` 部署中，进程仍会写 stderr，journald 可以捕获服务日志；同时 `logs/server.log` 和 `logs/client.log` 由应用内轮换保护，通常不需要额外 `logrotate` 去重命名打开中的文件。Windows 部署默认依赖应用内轮换，因为外部 rename 型日志轮换工具通常不能稳定处理进程打开中的文件。Docker 或 Kubernetes 部署更推荐依赖 stdout/stderr 和容器运行时日志轮换，文件日志可作为显式部署选择或排障辅助。

如果磁盘占用异常增长，先检查 `log_max_size_mb`、`log_max_backups`、`log_retention_days`、`log_compress`，再确认服务账号有权限写入和删除部署根目录下的 `logs/` 归档文件。

### 8080 返回 404 的排查

Release 包的推荐启动方式是直接运行 `./bin/goginx-server`，或使用包内 `systemd/goginx-server.service`。这条路径会启用管理监听器，并从部署根目录下的 `admin-ui/` 提供 `/`、`/login`、`/dashboard` 等页面；该默认前端路径按二进制所在位置推导，不依赖启动时的当前工作目录。

如果访问 `8080` 返回 `404`，先检查实际访问到的是不是管理监听器：

```bash
curl -i http://127.0.0.1:8080/
curl -i http://127.0.0.1:8080/api/admin/session
```

正常情况下，第一个请求应返回管理前端 HTML，第二个请求应返回 JSON 会话状态。

常见原因和处理方式：

| 现象 | 处理 |
| --- | --- |
| 服务启动失败并提示 admin frontend 目录错误 | 确认 Release 根目录包含 `admin-ui/index.html`，或显式设置 `admin_frontend_dir`。 |
| 服务日志显示 `admin=disabled` | 说明不是按无配置 Release 路径启动，或显式配置关闭了 `admin_enabled`。 |
| 使用 `-config config/server.json` 后异常 | 确认 `admin_enabled` 为 `true`，并且控制通道证书文件已经存在。首次部署更推荐先使用无配置启动，让服务自动生成 `data/` 状态、控制通道 TLS 材料和 `data/admin-jwt.key`。 |
| 服务启动失败并指向 `admin_jwt_secret_file` | 检查 `data/admin-jwt.key` 是否存在、可由服务账号读取、内容未损坏且未被错误替换。恢复备份中的原文件可以保留未过期管理员 JWT；删除或轮换该文件会要求管理员重新登录。 |

更新管理前端时，重新构建 `admin-ui/dist`，把构建产物同步到 Release 根目录的 `admin-ui/`，然后重启 `goginx-server`。服务端不会热加载前端文件。

## 当前限制

- 尚未实现 forward proxy。
- 尚未实现配额、限速、普通用户自助、备份恢复、容量校验和高级告警。
- 原生安装器、包管理器分发和更完整的跨平台服务编排尚未实现。
- 通配域名/平台域名所有权校验尚未实现。
- 管理后台当前以管理员能力为主，普通用户自助和更完整的运维页面仍在后续范围内。

## 参考文档

- `docs/openspec-migration.md`：OpenSpec 历史规格向普通项目文档迁移的方法和完成条件。
- `docs/daemon-runtime.md`：守护进程运行和部署说明。
- `docs/engineering-quality-guardrails.md`：外部服务集成、生命周期状态和 Admin UI/API 的工程质量分层防线。
- `docs/milestone-one-e2e.md`：当前可执行验证路径。
- `docs/examples/admin-seed-sqlite.md`：SQLite 种子数据示例。
- `docs/admin-ui/README.md`：管理后台页面设计文档索引。
- `openspec/`：迁移期间保留的历史规格与变更资料，不再作为当前文档入口。
