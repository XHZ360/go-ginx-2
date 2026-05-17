## Why

当前首个受支持部署模型依赖 `server.json`、`client.json`、管理员凭据 JSON、外部 admin UI 目录和预先准备的控制面 TLS 文件。这个模型可验证但不够部署友好：操作者在第一次启动前需要理解并拼装多个文件，容易把运行时状态、密钥材料和可选覆盖项混在一起。

本变更把默认部署路径调整为“无需额外配置文件即可启动和入网”：基础配置来自内置默认值，运行所需状态由程序生成或持久化，JSON/env 文件仅保留为高级覆盖和兼容路径。

## What Changes

- `goginx-server` 支持无 `-config` 启动，使用内置默认监听、路径、保留期和协议默认值，并在需要时创建运行目录、SQLite 数据库和控制面 TLS 材料。
- 部署包和 `systemd` 服务模板默认不再要求 `config/server.json` 或 `config/client.json`；示例配置保留为可选覆盖文档，而不是基础部署前置条件。
- 管理员登录凭据从独立 `admin_credentials_file` 迁移到 SQLite 中的管理员用户密码校验材料；首次部署提供明确的管理员初始化路径。
- 专用 admin UI 默认随服务端二进制或部署包交付，基础部署不再要求配置 `admin_frontend_dir` 才能获得浏览器管理面。
- 客户端支持通过一次性 join/enrollment 流程获得连接所需的服务端地址、信任材料、客户端 ID 和凭据；基础部署不再要求手写 `client.json`。
- 现有 JSON 配置、环境变量和文件路径覆盖能力保留为高级模式与兼容路径；本变更不移除已有可脚本化部署能力。
- 不把 SQLite、证书、托管证书文件或客户端本地状态视为“额外配置文件”；它们是应用生成或管理的运行时状态。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `deployment-operations`: 基础部署要求从“操作者创建 JSON 配置文件”改为“无额外配置文件即可启动、初始化并在受监督服务下运行”。
- `admin-resource-management`: 管理员认证基线从独立受保护凭据文件调整为 SQLite 管理员用户凭据，并增加首次管理员初始化要求。
- `control-channel`: 客户端连接基线增加一次性 join/enrollment 流程，使客户端可在不手写配置文件的情况下获得安全控制通道配置。
- `certificate-management`: 控制通道 TLS 边界增加首次启动自动生成和客户端信任分发/固定的要求，同时继续禁止跳过证书校验。

## Impact

- 影响命令入口：`cmd/goginx-server`、`cmd/goginx-client`、`cmd/goginx-admin`。
- 影响配置与启动：`internal/config`、`internal/daemon`、部署包生成和 `deploy/systemd` 模板。
- 影响管理员认证：`internal/adminapi`、`internal/admin`、SQLite 用户/密码数据模型与初始化 CLI。
- 影响控制通道入网体验：客户端本地状态、join token 生成/消费、服务端证书信任材料分发。
- 影响文档与验证：README、部署运行文档、外部进程 smoke 测试、配置加载和 bundle 测试。
