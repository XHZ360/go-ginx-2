## Context

当前部署路径以文件为中心：`goginx-server` 默认读取 `server.json`，`goginx-client` 默认读取 `client.json`，管理面依赖 `admin_credentials_file`，admin UI 需要 `admin_frontend_dir`，控制通道 TLS 证书和客户端 CA 文件也需要操作者提前准备。这个模型适合自动化验证，但第一次部署时要求操作者理解多个文件之间的关系。

本设计把“配置文件”降级为高级覆盖项，把基础部署变成程序管理的状态：

```text
基础部署输入
  ├─ server: goginx-server
  │    ├─ 内置默认配置
  │    ├─ data/ 自动创建
  │    ├─ SQLite 自动创建/迁移
  │    ├─ 控制面 CA/cert/key 自动创建
  │    └─ admin UI 默认内置交付
  │
  └─ client: goginx-client join <token>
       ├─ token 提供服务端地址和信任信息
       ├─ join 交换或解包客户端身份材料
       └─ 客户端写入本地受管状态
```

“无需额外配置文件”不等于“无状态”。SQLite、证书私钥、托管证书和客户端本地状态仍然存在，但它们由程序生成、迁移和管理，不要求操作者手写 JSON 或复制模板。

## Goals / Non-Goals

**Goals:**

- server 和 client 的基础部署路径不要求操作者创建 `server.json`、`client.json`、管理员凭据 JSON 或 admin UI 目录配置。
- 保留现有 JSON 配置加载能力，作为高级部署、测试和兼容覆盖路径。
- 控制面 TLS 继续保持证书校验，不引入 `InsecureSkipVerify` 或跳过身份校验的入网路径。
- 管理员认证使用 SQLite 中的管理员用户密码材料，消除独立凭据文件。
- 客户端通过 join/enrollment 流程生成本地受管状态，避免手写 client config。
- 部署包和 systemd 模板默认运行 configless 路径，同时仍记录可选覆盖方式。

**Non-Goals:**

- 不实现原生安装器、包管理器发布、签名工件或非 systemd supervisor。
- 不实现备份/恢复、容量验证、告警、RBAC 重设计、普通用户自助或配额管理。
- 不把 DNS 提供商令牌、ACME 账号配置、HTTPS 代理私钥等高级敏感材料迁入 SQLite。
- 不删除现有配置文件格式，也不把现有脚本化部署强制迁移到新格式。

## Decisions

### Decision: 配置加载分为默认模式和显式覆盖模式

`goginx-server` 和 `goginx-client` 不再把默认 `-config server.json/client.json` 当作必须读取的文件。未传 `-config` 时进入内置默认/受管状态模式；传入 `-config` 时继续使用严格 JSON 解码、未知字段拒绝和现有校验规则。

替代方案：启动时如果默认配置文件不存在就静默继续。拒绝，因为它会让“文件遗漏”和“故意 configless”难以区分，错误诊断会变差。

### Decision: 基础服务端状态由工作目录下的 `data/` 管理

服务端 configless 模式使用现有默认路径：`data/go-ginx.db`、`data/certs` 和后续需要的受管状态文件。启动前确保目录存在，再打开 SQLite 并执行迁移。这样部署包只需要稳定工作目录，不需要额外配置目录。

替代方案：使用系统级路径如 `/var/lib/go-ginx`。拒绝作为默认，因为当前 bundle 已围绕 install root 组织；系统路径适合作为显式配置覆盖。

### Decision: 控制面 TLS 使用生成的私有 CA 和稳定服务端名称

首次启动如果缺少控制面 TLS 材料，服务端生成本地私有 CA、控制面证书和私钥。证书包含一个由服务端状态派生的稳定 DNS 名称，例如 `goginx-control-<id>.internal`，客户端连接地址与 TLS 校验名称分离。join token 携带实际 server address、TLS server name 和 CA/pin 信息。

```text
client dials:      public.example.com:8443
TLS ServerName:    goginx-control-abcd.internal
trust anchor:      generated CA or pinned certificate from join
```

替代方案：生成只覆盖公网域名的证书。拒绝作为默认，因为服务端首次启动时通常不知道公网域名；可以作为显式覆盖或未来增强。

### Decision: admin UI 默认内置，外部目录作为覆盖

服务端二进制在构建时嵌入 admin UI 静态资源。`admin_frontend_dir` 保留为开发或定制覆盖；未配置时不再返回 404，而是服务内置前端。如果构建时没有嵌入资源，运行时应给出清晰错误或保持受限 API-only 模式，具体以实现证据决定。

替代方案：bundle 继续复制 `admin-ui/dist` 并自动写配置。拒绝作为主要路径，因为它仍然需要文件布局和配置字段保持一致。

### Decision: 管理员凭据迁移到 SQLite 管理员用户

管理面登录校验 `users` 表中 `role = admin`、状态启用且存在密码哈希的用户。首次部署通过本地 CLI 初始化管理员账号，例如 `goginx-admin init-admin`；不提供默认管理员密码，不开放无需认证的远程浏览器初始化。

替代方案：保留 `admin_credentials_file` 并自动生成一个。拒绝，因为这只是把手写文件变成生成文件，仍然保留两套身份来源。

### Decision: 客户端入网使用一次性 join token

管理员创建或轮换客户端时，可以生成一次性 join token。token 或 token 交换流程提供以下信息：

- 控制通道 QUIC/TCP+TLS 地址；
- TLS server name 和 CA/pin；
- client ID 和一次性 credential 或用于换取 credential 的 enrollment secret；
- 支持协议和重连默认值。

客户端执行 `goginx-client join <token>` 后写入本地受管状态；后续 `goginx-client` 无参数启动时读取该状态并运行。token 必须有过期或单次使用语义，避免长期可重放。

替代方案：把 `client.json` 内容 base64 后作为 token。拒绝作为最终设计，因为它没有解决凭据生命周期、单次使用和撤销语义；可以作为测试过渡但不应成为产品合同。

### Decision: ACME 和高级覆盖仍使用显式输入

ACME Cloudflare token、ACME account email、外部证书路径、自定义监听地址等仍可通过 JSON、CLI 参数或环境变量显式提供。configless 基线只覆盖“能部署、能初始化、能连接、能管理”的路径，不把所有高级生产策略隐藏在默认值里。

替代方案：把所有设置都塞进 SQLite 管理面。拒绝作为本变更范围，因为它会扩大到系统设置管理、RBAC、审计和热重载语义。

## Risks / Trade-offs

- [Risk] 自动生成控制面 TLS 材料可能让操作者误以为这是公网 HTTPS 证书。 → Mitigation: 文档和命名明确它只用于控制通道，代理 HTTPS 证书生命周期仍按现有证书管理合同处理。
- [Risk] SQLite 管理员登录会改变当前“管理员凭据独立于产品用户”的边界。 → Mitigation: 使用 `RoleAdmin` 明确浏览器管理员身份，普通用户不可登录管理面，客户端凭据仍完全独立。
- [Risk] join token 泄露会变成客户端入网风险。 → Mitigation: token 单次使用、短有效期、服务端记录审计事件，并支持轮换客户端凭据。
- [Risk] 内置 admin UI 增加构建流程耦合。 → Mitigation: 保留外部目录覆盖，测试 bundle 有/无前端资源两条路径。
- [Risk] configless 默认端口可能与宿主机冲突。 → Mitigation: 启动错误保持清晰；高级部署可用 JSON/flags 覆盖监听地址；admin 写路径继续做 ListenerClaim 冲突检查。

## Migration Plan

1. 添加 configless 启动路径，同时保留 `-config` 严格覆盖路径。
2. 添加服务端受管状态初始化：目录、SQLite、控制面 TLS 材料。
3. 将管理面认证接入 SQLite 管理员用户，并增加首次管理员初始化 CLI。
4. 嵌入或默认交付 admin UI，保留 `admin_frontend_dir` 覆盖。
5. 添加 join token 创建、消费和客户端本地状态读取路径。
6. 更新 bundle、systemd 模板、README、部署文档和 troubleshooting。
7. 扩展外部进程 smoke 测试，证明无 `server.json/client.json/admin_credentials_file/admin_frontend_dir` 的基础部署可启动并完成客户端连接。

回滚策略：保留现有 JSON 配置和凭据文件兼容路径，允许操作者继续使用旧 bundle 布局或显式 `-config` 启动。迁移到 SQLite 管理员认证后，应在发布说明中明确旧 `admin_credentials_file` 是兼容输入还是废弃输入；若实现选择废弃，必须提供迁移命令或明确错误信息。

## Open Questions

- join token 应由 `goginx-admin` CLI 生成，还是由 admin GraphQL mutation 生成并一次性展示？
- `admin_credentials_file` 在本变更中是保留为兼容 fallback，还是立即标记为废弃但仍可读？
- 客户端本地受管状态应使用 JSON 文件、SQLite，还是平台凭据存储？本变更只要求它不是操作者手写配置。
- 内置 admin UI 在开发构建缺失时应该失败启动、API-only 启动，还是回退外部目录？
