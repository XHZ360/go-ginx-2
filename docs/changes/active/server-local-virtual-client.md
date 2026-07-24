# Server 本机虚拟 Client

状态：`active`（feature 设计与计划，尚未实现）

架构前置条件：[Server Runtime Context 架构重整](../completed/server-runtime-context-architecture.md) 已完成。本 Change 不再承担全局架构重整，只定义本机虚拟 client 这一项业务能力。

## 1. 背景与目标

当前代理运行时假定每个代理都由远端 provider client 承接。该变更为 server 进程增加一个内置的虚拟/system client，使外部代理入口可以直接连接 server 所在机器上的本机网络服务。

虚拟 client 是 server 内部的逻辑实体，不启动独立 client 进程，不连接自身 control listener，也不改变现有 control wire protocol。它复用现有 `client_id`、proxy、session 和 stream 抽象，在 server 内注册一个 provider session，并由 server 进程执行本机拨号。

## 2. 明确约束

- 普通用户不能配置、创建、修改、启停、删除或使用本机代理。
- 本机目标必须命中管理员维护的白名单；白名单初始化至少包含 `127.0.0.1` 和 `::1`。
- 虚拟 client 出现在客户端管理列表，标记为系统 client。
- 系统 client 禁止删除、禁用、转移归属、凭据轮换和远程 enrollment。
- 不新增 client 类型字段，不扩展 control wire protocol。
- 首期优先实现 TCP；UDP、HTTP、HTTPS 复用同一抽象，按阶段交付。

## 3. 非目标

- 不把 server 改造成独立 client daemon。
- 不允许借助本机代理访问任意公网或未授权内网地址。
- 不通过 UI 隐藏代替后端授权。
- 不在首次实现中引入新的 proxy 类型字段。

## 4. 核心原则

1. **最小协议变更**：沿用现有 provider session 和 `StreamOpener`，不增加回环认证连接。
2. **默认拒绝**：目标地址、端口和解析结果未命中白名单时拒绝连接。
3. **多层授权**：admin API/UI、admin service 和 store/初始化边界都保护系统 client；UI 不是安全边界。
4. **系统身份不可变**：使用固定保留 ID（建议 `server-local`）判定系统 client，服务端不接受调用方修改系统属性。
5. **二次校验**：代理配置保存时校验一次，实际建立本机连接前再次校验，防止配置绕过和 TOCTOU。
6. **失败安全**：无效白名单或初始化失败不得注册可用虚拟 session；配置更新失败保留旧快照。
7. **幂等生命周期**：启动重复执行只产生一个系统 client 和一个虚拟 session；停止时注销 session；重启可恢复。
8. **可审计**：白名单及系统代理的管理操作记录操作者、摘要和结果，不记录凭据或不必要的敏感目标信息。

## 5. 目标架构

### 5.0 Feature 对架构上下文的使用

本 feature 使用架构 Change 定义的 `SystemClientFacade`、`LocalProxyFacade`、`LocalTargetPolicy`、`VirtualSession` 和 `LocalDialer`，不在 `daemon`、`control` 或 `adminapi` 中重新实现这些职责。

### 5.1 数据模型

- 复用现有 `domain.Client`、clients 表和 `domain.Proxy`。
- 增加服务端常量和识别函数，例如 `IsSystemClientID("server-local")`，不增加公开 client 类型字段。
- 系统 client 使用 provider `Kind` 以兼容现有代理快照和 listener 逻辑。
- 由于现有代码要求 `Client.UserID` 与 proxy 用户一致，需要在设计实现时选择保留系统用户或受保护的系统归属方案；该归属不得指向可被普通用户管理的身份。
- 白名单配置应持久化并支持地址/CIDR及可选端口范围；默认值为回环地址，规范化、去重后保存。

系统 client 的识别必须集中在 `systemclient` 上下文，禁止在多个包中散落保留 ID 字符串判断。对外可以复用现有 `domain.Client`，但所有系统对象的不可变属性由 facade 和初始化服务保护。

### 5.2 运行时

启动顺序：加载并校验白名单 → 幂等确保系统 client → 创建本地 `StreamOpener` → 注册常驻 virtual session → 启动/重建现有 proxy listeners。

本地 opener 建立内存双向管道：一端交给现有入口转发逻辑，另一端由 server `net.Dialer` 连接白名单目标。现有 TCP/UDP/HTTP/HTTPS listener 继续通过 `Sessions.Latest(proxy.ClientID)` 查找 provider session，无需修改 control wire protocol。

运行时端口定义如下：

- `VirtualSession`：实现现有 provider session 所需的 `StreamOpener`，只负责把代理流交给本地 opener。
- `LocalDialer`：执行目标解析、白名单二次校验、超时拨号和连接关闭，不负责 proxy 持久化。
- `SessionRegistry`：维护虚拟 session 的注册、替换和注销，不负责判断目标是否安全。
- `ListenerReconciler`：根据已持久化 proxy 启停外部入口，不直接创建 system client。

运行时只能依赖不可变的白名单快照；配置更新通过 policy 原子替换快照，不把管理请求对象直接传入连接协程。

连接建立前必须重新解析并匹配目标地址；首期建议仅允许 IP/CIDR，域名支持需另行定义 DNS 缓存和 rebinding 防护。应沿用现有超时、关闭、统计和并发限制语义。

白名单热更新采用原子快照：新连接使用新快照，已有连接按 graceful 策略继续；更新失败保留旧快照。是否立即 drain 现有连接需在实现阶段确定，默认不主动断开。

### 5.3 权限与管理面

- 系统 client 可在客户端列表查询中出现，并显示“系统 client”标记。
- 普通用户对系统 client 相关 proxy 的创建、修改、启停、删除和使用全部拒绝。
- 管理员可配置白名单和系统代理；所有 mutation 由后端确认管理员身份并写审计。
- 系统 client 的删除、禁用、凭据轮换、join/enrollment、归属修改返回明确的 forbidden/validation 错误。
- UI 禁用或隐藏危险操作，但 API/service/store 必须独立执行同等约束。

权限检查集中在业务 facade 的入口，并在关键 repository mutation 前保留系统对象保护。普通用户不得通过任意 proxy consumer、访问激活或未来新入口间接使用本机代理；“不可见”不作为授权条件。

管理上下文与运行时上下文之间通过持久化 proxy 和 `ListenerReconciler` 协作。管理请求完成不代表连接已成功建立；运行时失败通过状态、审计和日志反馈，不把网络错误伪装成配置成功。

## 5.4 目录与职责建议

```text
internal/
  systemclient/       系统 client 身份、ensure、保护规则
  localproxy/         本机代理 facade、白名单策略、local dialer、virtual session
  admin/               通用管理员业务；通过 facade 编排，不实现本机拨号
  adminapi/            认证与 GraphQL/HTTP 适配
  control/             远端 control wire protocol 和远端 session
  session/             session registry、生命周期和 stream opener 接口
  proxy/               TCP/UDP/HTTP/HTTPS 外部入口实现
  store/               repository 接口
  store/sqlite/        SQLite schema、migration 和 repository 实现
  daemon/              依赖装配、启动顺序、关闭顺序
```

目录不是强制的包拆分要求，但每个包必须遵守对应职责。新增代码优先进入职责所属上下文，避免继续向 `daemon/server.go`、`admin/service.go` 或 `control/transport.go` 堆叠本机代理特例。

## 5.5 关键不变量

- 所有 system client mutation 都经过 `SystemClientFacade` 或其受保护的内部初始化路径。
- 所有本机目标连接都经过 `LocalTargetPolicy` 和 `LocalDialer`，不存在裸 `net.Dial` 绕过入口。
- 管理上下文不得持有长生命周期连接；运行时上下文不得持有用户凭据或管理员 JWT。
- 远端 client 和 system client 共享 proxy/session 接口，但不共享错误地假设“必须存在远端 control 连接”。
- 任何 facade 返回成功后，持久化状态和 listener reconcile 状态必须满足既定一致性策略；部分失败必须可重试、可诊断。

## 6. 安全规则

- 默认白名单只包含 `127.0.0.1`、`::1`；不得把空白名单解释为允许全部。
- IP、CIDR、IPv4-mapped IPv6 和端口边界必须规范化测试。
- 默认拒绝域名；若未来支持域名，必须固定解析结果或使用受控解析缓存，防止 DNS rebinding。
- 任何重定向、代理协议升级或应用层转发都不能绕过白名单。
- 日志只记录 proxy/client ID、拒绝原因和必要的目标摘要，不记录 credential/token。

## 7. 数据迁移与兼容

- 新增配置采用向后兼容默认值；旧 server 配置不启用额外本机目标。
- 启动 migration/ensure 逻辑幂等创建系统 client，不改写既有普通 client/proxy。
- 若系统归属、白名单或初始化失败，server 不注册虚拟 session，并返回可诊断启动错误。
- 删除/回滚实现时应保留系统 client 和配置数据，先停止使用，再迁移或人工清理。

## 8. 分阶段实施

### P0：设计与数据基础

- 固定系统 client ID、系统对象保护函数和权限矩阵。
- 定义白名单 schema、默认值、规范化和错误码。
- 明确系统归属用户方案及 migration/ensure 行为。
- 冻结上下文边界、facade 接口和依赖方向；禁止在 P1 后临时把管理逻辑塞入 transport 或 daemon。

### P1：后端最小链路

- 实现 virtual session/local opener。
- 接入 server runtime 生命周期。
- 打通 TCP 本机 echo/真实本机服务。
- 增加连接超时、白名单二次校验和失败关闭。
- 先以 facade/port 形式接入，不让 Admin API 直接调用 runtime 实现。

### P2：管理 API/UI

- 增加管理员白名单查询/更新。
- 系统 client 列表标记及危险操作禁用。
- 系统代理 mutation 的后端权限和审计。

### P3：扩展与强化

- UDP、HTTP、HTTPS 本机承接。
- IPv6/CIDR、端口范围、并发/限流、热更新和 graceful drain。
- E2E、运维说明和回滚演练。

## 9. 验收条件

- server 启动后客户端列表存在且仅存在一个系统 client，重复启动不重复创建。
- 普通用户所有系统 client/proxy mutation 和使用路径均被拒绝。
- 管理员可以维护白名单；默认回环地址可用，非白名单目标不可用。
- 外部 TCP 代理可通过系统 client 连接 server 本机 echo 服务；目标不可达时连接按现有错误语义关闭。
- 删除、禁用、凭据轮换和 enrollment 系统 client 均失败且产生可审计结果。
- server 停止、重启后 session 和系统 client 状态恢复，不影响普通 client 路径。

## 10. 验证计划

- 单元：系统 client 识别、不可变属性、CIDR/IP/端口匹配、权限矩阵、白名单原子更新与回滚。
- 集成：VirtualSession 本机 TCP echo、无 session、白名单拒绝、拨号失败、并发和超时。
- API/UI：普通用户越权拒绝、管理员更新和审计、列表系统标记。
- 范围验证：`go test ./internal/<相关包>`；跨模块后执行 `go test ./...`；涉及 UI 时执行 `pnpm test` 与 `pnpm build`；跨进程场景执行 `go test ./e2e -count=1`。

## 11. 待决策与风险

- 系统 client 的 `UserID` 采用保留系统用户还是数据库允许的特殊归属；实现前需检查现有外键约束。
- 白名单是否支持端口范围；安全上建议支持并默认最小权限。
- 白名单收紧时已有连接是否 drain；默认只影响新连接。
- 是否允许绑定非 loopback 私有地址；需进行 SSRF/内网横向访问评审。
- 普通用户“不能使用”需覆盖哪些入口（公开代理入口、SDK、访问激活）；实现时按所有入口统一拒绝，不依赖单一 UI。

### 架构风险控制

- **Facade 膨胀**：按 system client、本机代理、目标策略拆分接口；禁止 `ServerFacade` 聚合所有业务。
- **保护规则绕过**：store 保持最小 CRUD，所有外部 mutation 只能通过业务 facade；初始化使用受限内部端口。
- **上下文反向依赖**：运行时不得导入 adminapi；control 不得导入 UI/配置适配器；通过 interface 注入 reconciler、policy 和 registry。
- **隐式状态传播**：连接协程只接收不可变 snapshot 和明确 target，不读取可变 admin request 或全局配置。
- **迁移期间双模型**：旧远端 provider 路径保持不变，system client 只在固定 ID 命中时进入 local runtime；完成后再清理兼容分支。

## 12. 当前状态

本文档只记录目标设计和实施计划，当前尚未修改运行时代码、数据库 schema、Admin API 或 UI。实现完成后需同步 `docs/requirements/client-access.md`、`docs/architecture/reverse-proxy.md`、`docs/architecture/control-channel.md` 和相关运维文档，再将本 Change 移至 `completed/`。
