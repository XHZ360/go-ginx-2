# HTTP 路径路由与 HTTPS 访问激活

## 文档状态

本文描述已实现的 HTTP 路径路由与 HTTPS 访问激活行为。后续应将最终行为合并到反向代理运行时和 Admin UI 的当前事实文档，并删除不再需要的实施性描述。

## 目标

本次变更包含两项能力：

1. HTTP 和 HTTPS Proxy 支持根据请求路径选择不同上游，提供类似 nginx `location` 与 `proxy_pass` 的路径前缀转发能力。
2. HTTPS Proxy 支持可选的访问激活认证。管理员生成一次性激活链接和二维码，访问者确认激活后获得持久 Cookie；后续请求通过 Cookie 校验后才允许转发。

## 范围与边界

- 路径路由只适用于 HTTP 和终止 TLS 的 HTTPS Proxy。
- HTTPS 认证作用于整个 HTTPS Proxy，即同一 Domain 下的所有业务路径，不按单条路径分别配置。
- 激活链接必须使用受保护 HTTPS Proxy 的相同 Domain，才能为该 Domain 写入 Cookie。
- 激活端点使用系统保留路径，不进入用户配置的路径路由。
- 子路由可以选择不同的 go-ginx Client，但目标 Client 必须与父 Proxy 属于同一用户。
- 第一版不支持正则表达式、Header 匹配、Method 匹配、权重分流或负载均衡。
- 第一版访问凭据只支持整站统一撤销，不提供逐设备撤销。

## 核心模型

### Proxy

现有 `Proxy` 继续作为 HTTP/HTTPS 虚拟主机，负责：

- 监听地址与端口；
- HTTP Host 或 HTTPS SNI Domain；
- HTTPS 证书绑定；
- HTTPS 访问认证策略；
- 现有 `ClientID`、`TargetHost` 和 `TargetPort` 组成的 `/` 默认后端。

不应将同一 Domain 下的每个路径创建为独立 Proxy。HTTPS 必须在读取 HTTP Path 前根据 SNI 找到唯一 Proxy 和证书，多个同 Domain Proxy 会破坏当前证书选择和 Host 唯一性模型。

Proxy 需要增加以下认证状态：

- `AccessAuthEnabled`：是否要求访问激活；
- `AccessAuthVersion`：统一撤销代次，撤销全部访问或重新启用认证时递增。

### ProxyRoute

新增一对多子路由模型：

```text
ProxyRoute
  ID
  ProxyID
  ClientID
  PathPrefix
  StripPrefix
  UpstreamPathPrefix
  TargetHost
  TargetPort
  Status
  CreatedAt
  UpdatedAt
```

现有 Proxy 后端继续作为 `/` 默认路由。新增子路由只保存需要覆盖默认后端的路径，避免迁移后出现两个默认后端权威来源。

## 路径匹配与改写

### 匹配规则

- `PathPrefix` 必须以 `/` 开头。
- Query 和 Fragment 不参与路由匹配。
- 使用最长前缀优先。例如 `/api/v2` 优先于 `/api`。
- 前缀必须满足路径段边界：`/api` 匹配 `/api` 和 `/api/users`，但不匹配 `/apix`。
- 同一 Proxy 下规范化后的 `PathPrefix` 必须唯一。
- 系统保留 `/.well-known/goginx/`，用户路由不得配置该前缀。
- 请求包含非法转义、编码斜杠、编码反斜杠或 NUL 时应拒绝，避免匹配结果与上游解释不一致。
- 未匹配子路由时使用 Proxy 的 `/` 默认后端；没有可用默认后端时返回 `404 Not Found`。

### 改写规则

路径改写必须显式配置，不复刻 nginx 根据 `proxy_pass` 尾斜杠推断行为的隐式语义。

- `StripPrefix=false`：上游收到原始请求路径。
- `StripPrefix=true`：移除已匹配的 `PathPrefix`，并将剩余路径拼接到 `UpstreamPathPrefix`。
- `UpstreamPathPrefix` 默认为 `/`，必须以 `/` 开头。
- Query 原样保留。
- 改写后路径必须保持为绝对路径，不允许生成空路径或逃逸到非路径形式。

示例：

| PathPrefix | StripPrefix | UpstreamPathPrefix | 请求路径 | 上游路径 |
| --- | --- | --- | --- | --- |
| `/api` | `false` | `/` | `/api/users?id=1` | `/api/users?id=1` |
| `/api` | `true` | `/` | `/api/users?id=1` | `/users?id=1` |
| `/api` | `true` | `/v1` | `/api/users?id=1` | `/v1/users?id=1` |

## 运行时数据流

### HTTP

1. 根据 listener、端口和 Host 找到父 Proxy。
2. 根据请求 Path 选择最长匹配的启用子路由。
3. 按路由配置改写 Path。
4. 根据子路由的 `ClientID` 查找最新在线 Session；未匹配子路由时使用父 Proxy 的 Client。
5. 使用选中后端的 `TargetHost` 和 `TargetPort` 创建现有 `OpenStream`。
6. 转发请求、响应和 WebSocket Upgrade。

### HTTPS

1. 读取 ClientHello，并根据 listener、端口和 SNI 找到父 Proxy。
2. 根据父 Proxy 解析证书并完成 TLS 握手。
3. 读取 HTTP 请求。
4. 优先处理系统保留的激活端点。
5. 如果启用认证，校验访问 Cookie；校验失败时返回 `401 Unauthorized`，不得转发到上游。
6. 从转发请求中移除 go-ginx 认证 Cookie。
7. 根据请求 Path 选择子路由并改写路径。
8. 使用选中 Client 和目标创建现有 `OpenStream`，继续处理普通响应或 WebSocket Upgrade。

路径路由不产生新 listener，路由更新落库后应立即生效，不要求重启或 reconcile listener。

## 跨 Client 路由

每条 `ProxyRoute` 可以选择不同 Client。管理服务必须验证：

- Client 存在且类型允许提供反向代理服务；
- Client 与父 Proxy 属于同一用户；
- Target Host 和 Port 合法；
- Client 离线时只影响使用该 Client 的路由，返回 `503 Service Unavailable`。

当前 `OpenStream` 已携带动态 `TargetHost` 和 `TargetPort`，无需为路径路由修改开流协议。控制面需要补充 Client 的有效路由分配数量，使 Proxy Snapshot 和 heartbeat 中的 Active Proxy 统计包含通过子路由分配到该 Client 的服务。

第一版流量统计继续按父 Proxy 聚合，不按子路由拆分。

## HTTPS 访问激活

### 开启认证

管理端应提供原子操作“开启认证并生成激活链接”：

1. 验证 Proxy 是已配置可服务证书的 HTTPS Proxy；
2. 开启 `AccessAuthEnabled`；
3. 创建一次性激活 Token；
4. 在同一事务成功后返回激活 URL。

如果 Token 创建失败，不得留下已经开启认证但没有可用激活链接的状态。已经开启认证的 Proxy 可以继续生成新的激活链接。

关闭认证时应统一撤销所有未使用 Token 和已激活凭据。重新开启认证必须使用新的激活链接。

### 激活链接

激活链接使用 HTTPS Proxy 的相同 Domain：

```text
https://<proxy-domain>[:port]/.well-known/goginx/activate/<token>
```

- 标准 `443` 端口不显示端口号，非标准端口必须保留。
- 激活路径由系统保留，不参与用户路径路由。
- Token 默认 10 分钟有效，管理 API 可以允许受限范围内调整 TTL。
- Token 明文只在创建响应中返回一次；数据库只保存 Token hash。
- 完整 URL、Token 和 hash 不得进入日志、审计错误摘要或后续查询。
- Admin UI 使用返回的完整 URL 本地渲染二维码，不需要后端生成或保存二维码图片。

### 确认与兑换

为了避免邮件预览器、安全扫描器或浏览器预取提前消费链接，GET 不直接完成激活：

1. `GET` 激活链接验证 Token 基本状态并返回无外部资源的确认页；
2. 用户点击确认后，浏览器向同一 URL 发起 `POST`；
3. 服务端在单个数据库事务中原子标记 Token 已使用，并创建新的随机 Cookie secret；
4. 事务成功后设置 Cookie，并使用 `303 See Other` 跳转到安全的业务路径；
5. 同一 Token 的并发或重复 POST 只能有一个成功。

确认页不得加载第三方脚本、字体、图片或统计资源。响应至少设置：

```text
Cache-Control: no-store
Referrer-Policy: no-referrer
X-Content-Type-Options: nosniff
Content-Security-Policy: default-src 'none'; form-action 'self'; style-src 'unsafe-inline'
```

激活成功后的跳转目标默认为 `/`。如支持 `returnTo`，只能接受同 Domain 的绝对路径，禁止协议、Host、反斜杠和协议相对 URL，防止开放重定向。

### Cookie 与凭据

每次成功激活生成独立的 256-bit 随机 opaque secret：

- Cookie 保存 secret 明文；
- SQLite 只保存 secret 的 SHA-256 hash；
- 校验时同时绑定 Proxy ID 和 `AccessAuthVersion`；
- 高熵随机 secret 不需要 bcrypt，普通 SHA-256 hash 足以避免数据库泄露后直接获得 bearer secret；
- 原始 secret 不得进入 API、日志、审计、错误信息或测试快照。

Cookie 属性：

```text
Secure
HttpOnly
SameSite=Lax
Path=/
Domain 不设置
```

Cookie 名必须包含 Proxy 的稳定摘要，例如：

```text
__Host-goginx-access-<proxy-hash>
```

Cookie 不按端口隔离，因此不能在同一 Domain 的多个 HTTPS Proxy 上使用同一个固定名称。

服务端凭据长期有效，直到管理员统一撤销、关闭认证或删除 Proxy。浏览器可能限制持久 Cookie 的最大保存时间；Cookie 被浏览器清理后，用户需要重新使用新链接激活。

### 校验与统一撤销

普通 HTTPS 请求在路径路由前执行认证：

- 未启用认证时不读取或要求 go-ginx 访问 Cookie；
- Cookie 缺失、格式错误、hash 不存在或版本不匹配时返回 `401 Unauthorized`；
- 校验通过后允许进入路径匹配和转发；
- 转发前必须从请求 Cookie Header 中精确移除 go-ginx 认证 Cookie，其他业务 Cookie 保持不变；
- WebSocket Upgrade 同样必须先通过认证。

统一撤销操作应原子完成：

1. 递增 Proxy 的 `AccessAuthVersion`；
2. 撤销或删除该 Proxy 的所有 Access Credential；
3. 撤销所有未使用 Activation Token；
4. 记录不含 secret 的审计事件。

撤销完成后，所有已下发 Cookie 立即失效。

## 持久化设计

新增 `proxy_routes`：

```text
id
proxy_id
client_id
path_prefix
strip_prefix
upstream_path_prefix
target_host
target_port
status
created_at
updated_at
```

约束与索引：

- 外键指向 Proxy 和 Client；
- Proxy 删除时级联删除路由；
- `unique(proxy_id, path_prefix)`；
- 按 `proxy_id` 和状态建立查询索引。

新增 `proxy_activation_tokens`：

```text
id
proxy_id
auth_version
token_hash
expires_at
used_at
created_at
created_by
```

新增 `proxy_access_credentials`：

```text
id
proxy_id
auth_version
secret_hash
created_at
last_used_at
```

Token 兑换必须由 Repository 提供原子事务操作，不允许在 Service 中使用“先查询、再标记、再创建凭据”的非原子组合。

## Admin API

Proxy GraphQL 配置增加：

- `routes`；
- `accessAuthEnabled`；
- 可选的认证状态摘要，但不返回凭据数量以外的敏感信息。

新增 Mutation：

- `enableProxyAccessAuthAndCreateActivation`：原子开启认证并返回一次性 URL；
- `createProxyActivationLink`：为已开启认证的 Proxy 创建新链接；
- `revokeAllProxyAccess`：统一撤销全部 Token 和 Cookie 凭据；
- 关闭认证可以并入 `updateProxy`，但必须执行统一撤销语义。

激活 URL 只在创建 Mutation 的成功响应中出现一次。Mutation 继续受现有 Admin JWT、CSRF 和权限检查保护。

Proxy 与 Routes 的创建或全量更新必须在同一事务中提交。父 Proxy listener 字段变更失败时沿用现有 reconcile 回滚策略；仅修改路由或认证策略不需要 listener reconcile。

## Admin UI

HTTP/HTTPS Proxy 创建和编辑表单增加路由编辑器：

- Path Prefix；
- Client；
- Target Host；
- Target Port；
- 保留路径或剥离前缀；
- Upstream Path Prefix。

HTTPS Proxy 详情增加访问认证区域：

- 当前认证状态；
- 开启认证并生成链接；
- 为已开启认证的 Proxy 生成新链接；
- 统一撤销全部访问；
- 关闭认证。

激活链接对话框包含：

- 完整激活 URL；
- 复制按钮；
- Ant Design `QRCode`；
- Token 过期时间；
- “链接仅展示一次”的提示。

关闭对话框后必须清理前端内存中的 URL。URL 不得写入 React Query 长期缓存、表单草稿、`localStorage`、`sessionStorage` 或错误上报。

## 兼容性与迁移

- 现有 HTTP/HTTPS Proxy 无需数据迁移即可继续使用原 `ClientID`、`TargetHost` 和 `TargetPort` 作为 `/` 默认后端。
- 没有配置子路由时，行为与当前版本一致。
- 未开启 HTTPS 访问认证时，行为与当前版本一致。
- 路由配置不改变证书绑定关系；HTTPS 仍由父 Proxy 按 SNI 选择证书。
- 子路由不会创建额外 listener，也不会改变现有 listener admission 唯一性规则。
- 不修改 `OpenStream` 的目标字段；如需修正跨 Client 的 Active Proxy 统计，只扩展 Snapshot 的分配数量元数据。

## 安全要求

- Token 和 Cookie secret 使用 `crypto/rand` 生成，至少 256-bit 熵。
- 数据库只保存 hash，不保存明文 Token、完整激活 URL或 Cookie secret。
- Token 消费必须原子且只能成功一次。
- 认证 Cookie 不得转发到用户上游。
- 激活端点必须在认证和用户路径路由之前处理。
- 激活页面和响应禁止缓存、禁止向第三方发送 Referer。
- 日志只记录 Proxy ID、错误类别和必要的脱敏元数据，不记录完整请求 Path。
- GraphQL、审计和 UI 错误不得包含 Token、hash、Cookie 或完整激活 URL。
- 删除 Proxy、关闭认证或统一撤销时必须使全部历史凭据失效。
- 认证失败应 fail closed；存储不可用时不得绕过认证继续转发。

## 错误语义

- 路由不存在：`404 Not Found`。
- 路由 Client 离线：`503 Service Unavailable`。
- 上游开流或响应失败：`502 Bad Gateway`。
- 未激活或 Cookie 无效：`401 Unauthorized`。
- Token 无效：返回不暴露存在性的通用失败页面。
- Token 过期或已使用：返回通用失效页面，不区分具体原因给匿名访问者。
- 管理端输入非法：`validation`。
- 路由重复、跨用户 Client 或认证状态冲突：`conflict`。
- Token 或凭据持久化失败：`persistence_failed`，不得返回假成功。

## 验证计划

### Domain 与 Store

- Path Prefix 规范化、唯一性和保留路径校验；
- 最长前缀与路径段边界；
- 保留路径、剥离前缀和目标前缀拼接；
- 跨用户 Client 被拒绝；
- Proxy、Routes 和认证策略事务回滚；
- Token hash 和 Credential hash 唯一；
- 数据库中不存在 Token、完整 URL 或 Cookie secret 明文；
- Token 过期、重放和并发消费只有一次成功；
- 统一撤销使全部历史凭据失效。

### HTTP Runtime

- 同一 Host 的 `/`、`/api` 和 `/api/v2` 转发到不同后端；
- 子路由转发到不同 Client；
- Query、Body 和业务 Cookie 保留；
- 路径保留与改写；
- WebSocket 使用匹配路由；
- 子路由 Client 离线只影响对应路径；
- 路由更新无需重启 listener 即生效。

### HTTPS Runtime

- 同一 SNI 和证书下按路径转发到不同后端；
- 未开启认证时保持当前行为；
- 未激活请求返回 `401`；
- GET 只显示确认页，POST 才消费 Token；
- 激活成功设置正确 Cookie 属性并跳转；
- Token 过期、重放和并发兑换；
- 有效 Cookie 允许普通请求和 WebSocket；
- Cookie 不可跨 Proxy 使用；
- 认证 Cookie 不到达上游；
- 统一撤销、关闭认证和删除 Proxy 后 Cookie 立即失效；
- 服务重启后未撤销凭据仍有效；
- Store 校验失败时 fail closed。

### Admin API 与 UI

- Routes 和认证字段的 GraphQL 输入输出；
- 开启认证与创建 Token 的原子性；
- 激活 URL 只返回一次；
- Mutation 要求 Admin JWT 和 CSRF；
- API、审计和错误响应不泄露敏感材料；
- 路由编辑器的增删、重复校验和跨 Client 选择；
- 二维码内容与激活 URL 完全一致；
- 关闭激活对话框后清理 URL；
- 统一撤销和关闭认证的确认交互。

### E2E

- HTTP 两个路径转发到两个真实 Client/Origin；
- HTTPS 两个路径共用同一 Domain 和证书；
- 未激活拒绝访问；
- 激活确认、Cookie Jar、业务请求和 Token 重放完整链路；
- 服务端重启后 Cookie 继续有效；
- 统一撤销后原 Cookie 失效；
- Origin 明确断言未收到 go-ginx 认证 Cookie。

最小完整验证命令：

```powershell
$env:CGO_ENABLED="0"
go test ./...
go test ./e2e -count=1

Set-Location admin-ui
pnpm graphql:refresh
pnpm test
pnpm build
```

## 已知限制

- 当前 HTTPS Entry 每条 TLS 连接只处理一个普通 HTTP 请求，不支持完整 Keep-Alive 和 HTTP/2；本变更不会顺带重构该连接模型。
- 第一版统计按父 Proxy 聚合，无法直接查看单条路径的流量。
- 统一撤销会让所有设备同时失效，无法只移除单个设备。
- 浏览器可能限制长期 Cookie 的最大保存时间，Cookie 被清理后需要重新激活。
- Cookie 不按端口隔离，同 Domain 多 Proxy 依赖 Proxy 专属 Cookie 名避免覆盖。
