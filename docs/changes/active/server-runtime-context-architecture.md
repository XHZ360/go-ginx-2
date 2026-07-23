# Server Runtime Context 架构重整

状态：`active`（架构变更）

## 1. 目标

以隔离上下文、清晰职责边界、降低认知与维护压力为目标，重整 server 的业务层、运行时层、通信层和持久化层。该 Change 不新增本机代理能力，先提供稳定的实现边界，供后续 feature 使用。

## 2. 上下文边界

```text
管理上下文：Admin API/UI/CLI → 业务 Facade
运行时上下文：Proxy Listener → Session Registry → StreamOpener
通信上下文：Remote Client ↔ Control Listener ↔ Session
持久化上下文：Domain Service → Repository → SQLite
```

- 管理上下文负责身份、权限、配置、审计和业务命令。
- 运行时上下文负责 listener、session、stream 生命周期和连接转发。
- 通信上下文负责 control wire protocol 和远端 client 握手。
- 持久化上下文只负责数据存取、事务和迁移，不承载管理员权限。

上下文之间通过接口、端口和不可变数据结构通信；禁止 API 层直接操作 session，禁止 runtime 层解析 JWT，禁止 control 层承担业务配置权限。

## 3. 外观与端口

业务入口按职责拆分，禁止创建上帝式 `ServerFacade`：

```go
type SystemClientFacade interface {
    Ensure(ctx context.Context) (domain.Client, error)
    Get(ctx context.Context) (domain.Client, error)
    IsSystemClient(clientID string) bool
}

type LocalProxyFacade interface {
    Create(ctx context.Context, actor Actor, input LocalProxyInput) (domain.Proxy, error)
    Update(ctx context.Context, actor Actor, input LocalProxyInput) (domain.Proxy, error)
    Delete(ctx context.Context, actor Actor, proxyID string) error
}

type LocalTargetPolicy interface {
    ValidateTarget(ctx context.Context, host string, port int) error
    Snapshot() AllowlistSnapshot
    Replace(ctx context.Context, input AllowlistInput) error
}
```

运行时端口包括 `VirtualSession`、`LocalDialer`、`SessionRegistry` 和 `ListenerReconciler`。Facade 负责业务编排；端口隐藏 session、socket 和数据库实现。

## 4. 依赖方向

```text
adminapi / CLI / UI adapter
            ↓
       business facades
            ↓
 domain policies + repository interfaces + runtime ports
            ↓
 store/sqlite, session, proxy listeners, net.Dialer
```

`daemon` 只负责装配和生命周期；`adminapi` 只负责认证和协议映射；`control` 只负责远端通信；`store` 只实现 repository。新业务不得继续堆叠到 `daemon/server.go`、`admin/service.go` 或 `control/transport.go`。

## 5. 实施顺序

1. 固定接口、错误模型、依赖方向和测试替身。
2. 将现有调用逐步迁移到 facade/port，保持行为不变。
3. 抽离 runtime session、listener reconcile 和持久化适配器。
4. 删除跨上下文直连和重复权限判断。
5. 通过普通 client、proxy、control 和 admin 回归测试后，标记架构 Change 完成。

## 6. 不变量

- 管理上下文不持有长生命周期网络连接。
- 运行时上下文不持有管理员 JWT 或用户密码。
- 所有外部连接都经由明确的 opener/dialer 端口。
- 所有业务 mutation 经过 facade；store 不提供安全策略绕过入口。
- 远端 client 和未来 system client 可以共享 session/stream 接口，但不共享错误的远端连接假设。

## 7. 验收

- 现有普通 client 和四类 proxy 行为无回归。
- facade 可独立进行业务测试，runtime 可使用 fake opener/dialer 测试。
- adminapi、daemon、control 之间不存在新的反向依赖。
- `go test ./...` 和现有 e2e 通过。
