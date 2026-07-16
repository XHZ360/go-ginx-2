# go-ginx-2

`go-ginx-2` 是 Simp-Frp/go-ginx 设计的新实现目标。当前仓库已经完成里程碑一运行时和首个部署基线，重点覆盖控制面、反向代理、管理 API/UI、证书管理、SQLite 持久化和可复现部署包。

> 状态说明：这还不是完整生产平台。核心 TCP/UDP/HTTP/HTTPS 反向代理路径、QUIC 与 TCP+TLS 控制通道、无配置启动、客户端加入流程、管理后台、Linux `systemd` 部署模板和 Windows 原生服务命令已经实现并有本地测试覆盖；配额、限速、普通用户自助、备份恢复和更完整的运维能力仍在后续范围内。

## 目录

- [项目状态](#项目状态)
- [核心能力](#核心能力)
- [仓库结构](#仓库结构)
- [环境要求](#环境要求)
- [快速开始：Release 产物无配置运行](#快速开始release-产物无配置运行)
- [更多操作文档](#更多操作文档)
- [当前限制](#当前限制)
- [参考文档](#参考文档)

## 项目状态

| 维度 | 当前状态 |
| --- | --- |
| Go 模块 | `github.com/simp-frp/go-ginx-2` |
| Go 版本 | `go.mod` 使用 Go `1.26.0` |
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
docs/              文档入口地图；按 project/requirements/architecture/operations 等归档
```

## 环境要求

### 部署环境

- 部署环境不需要安装 Go、Node.js、Corepack 或 pnpm，只需要使用 Release 后的部署包或二进制文件。
- Linux `systemd` 部署建议使用 Release 产物中的完整部署包，包内应包含 `bin/`、`config/`、`data/`、`logs/` 和 `systemd/`。
- Windows 或手动运行环境使用对应平台的 Release 压缩包即可。
- 下文 Linux 示例使用 `./bin/goginx-*`，Windows 可替换为 `.\bin\goginx-*.exe`。

### 开发与构建环境

- 只有源码开发、测试、发布构建环境需要 Go `1.26` 或与 `go.mod` 匹配的版本。
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

## 更多操作文档

详细步骤已按主题归档，避免与 `docs/` 双份维护：

| 主题 | 文档 |
| --- | --- |
| 管理 CLI / TUI / 代理种子 | [`docs/operations/admin-cli.md`](docs/operations/admin-cli.md) |
| 运行时路径、环境变量、显式配置 | [`docs/operations/runtime-configuration.md`](docs/operations/runtime-configuration.md) |
| 托管证书签发/续期/Origin CA | [`docs/operations/certificate-operations.md`](docs/operations/certificate-operations.md) |
| 管理 API、会话与前端构建 | [`docs/operations/admin-api.md`](docs/operations/admin-api.md) |
| Docker 与本机开发构建 | [`docs/operations/docker-development.md`](docs/operations/docker-development.md)、下方简要命令 |
| Release 部署（Linux/Windows） | [`docs/operations/deploy-release.md`](docs/operations/deploy-release.md)、[`docs/operations/daemon-runtime.md`](docs/operations/daemon-runtime.md) |
| E2E 验证 | [`docs/operations/milestone-one-e2e.md`](docs/operations/milestone-one-e2e.md) |

### 本机开发简要命令

```powershell
$env:CGO_ENABLED="0"
go test ./...
go build ./cmd/goginx-server ./cmd/goginx-client ./cmd/goginx-admin
```

```powershell
docker compose up --build
```

Admin UI 开发见 `docs/operations/docker-development.md` 与 `admin-ui/`。

## 当前限制

- 尚未实现 forward proxy。
- 尚未实现配额、限速、普通用户自助、备份恢复、容量校验和高级告警。
- 原生安装器、包管理器分发和更完整的跨平台服务编排尚未实现。
- 通配域名/平台域名所有权校验尚未实现。
- 管理后台当前以管理员能力为主，普通用户自助和更完整的运维页面仍在后续范围内。

详见 [`docs/requirements/limits.md`](docs/requirements/limits.md)。

## 参考文档

完整入口地图见 [`docs/README.md`](docs/README.md)。

- [`docs/project/overview.md`](docs/project/overview.md)：项目目标、产品形态与技术栈约束。
- [`docs/requirements/proxy-runtime.md`](docs/requirements/proxy-runtime.md)：反向代理产品需求。
- [`docs/requirements/client-access.md`](docs/requirements/client-access.md)：客户端接入与授权。
- [`docs/requirements/certificate-lifecycle.md`](docs/requirements/certificate-lifecycle.md)：证书生命周期需求。
- [`docs/architecture/system-architecture.md`](docs/architecture/system-architecture.md)：系统组成与数据流。
- [`docs/architecture/reverse-proxy.md`](docs/architecture/reverse-proxy.md)：反向代理运行时。
- [`docs/operations/daemon-runtime.md`](docs/operations/daemon-runtime.md)：守护进程与部署。
- [`docs/worklog.md`](docs/worklog.md)：当前进展与下一步。
