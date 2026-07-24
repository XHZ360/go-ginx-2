# 系统架构

## 范围

GoGinX 由服务端、provider 客户端、可选的 consumer SDK、Admin API/UI 和 SQLite 数据层组成。服务端负责资源管理、控制通道、入口 listener 和运行时路由；provider 客户端只连接本地目标；consumer 用于从应用侧主动访问用户已授权的代理。

## 主要数据流

1. Admin CLI 或 Admin UI 把用户、客户端和代理记录写入 SQLite。
2. 客户端通过 QUIC 或 TCP+TLS 控制通道认证。provider 接收自己的代理快照，consumer 接收按用户过滤的代理列表。
3. 服务端入口接收 TCP、UDP、HTTP 或 HTTPS 流量，并通过最新的 provider 会话打开代理子流。
4. consumer SDK 通过同一控制通道主动打开指定代理的流；服务端校验用户、代理状态和 provider 在线状态后桥接到固定 target。
5. 累计代理统计和管理审计写入 SQLite；活跃连接、会话和 listener 属于进程运行时状态。

## 代码入口

- `cmd/`：server、client、admin 命令入口。
- `internal/control/`：协议、认证、QUIC 和 TCP+TLS 多路复用。
- `internal/session/`：客户端会话和最新会话路由。
- `internal/proxy/`：TCP、UDP、HTTP、HTTPS 和双向隧道。
- `internal/certmanager/`：托管证书 provider、健康检查和调度。
- `internal/store/sqlite/`：cgo-free SQLite 仓储。
- `sdk/`：consumer SDK 和本地固定目标入口。

相关验证：`go test ./internal/control ./internal/session ./internal/proxy/... ./sdk -count=1`。

## 上下文边界

- 管理适配器通过 `internal/admin` 的按领域 facade 执行业务命令；`adminapi.Entry.Commands` 只接受这些 facade 的组合，不直接依赖 `admin.Service` 的实现字段。非命令的入口默认值作为独立装配配置通过 `adminapi.Entry.ProxyEntryDefaults` 传入。
- `internal/admin` 的业务命令由 `UserService`、`ClientService`、`DomainService`、`ProxyService`、`CertificateService` 和 `ProviderCredentialService` 六个 concrete service 实现；`Services` 只负责注入 Store、证书管理器、审计记录器和三类共享 policy，`Commands` 只组合六个 facade。
- Domain、Proxy、Certificate 之间的共享规则通过 `ProxyAdmissionPolicy`、`CertificateBindingPolicy` 和 `ProxyAccessPolicy` 注入；service 之间不访问对方的字段或私有方法。`createWebProxy` 属于 `ProxyService`，不形成 Domain 到 Proxy 的 facade 依赖。
- `internal/adminquery` 只能通过 `SessionSnapshotSource` 读取会话快照，不能操作 session 生命周期。
- `internal/control` 负责远端协议与连接生命周期。认证结果和连接关闭可直接同步 `ClientStatus`，这是唯一登记的直连 store 写入例外；其他业务 mutation 必须经业务 facade。
- `internal/session` 提供 `VirtualSession` 和 `SessionRegistry` 运行时端口；`internal/systemclient`、`internal/localproxy` 固定未来系统 client、本机代理、目标策略和拨号端口，不在此阶段实现业务能力。
- `internal/daemon` 只装配上述上下文及其生命周期，不承载管理业务规则。

## 明确边界

当前系统是反向代理平台，不是任意目标正向代理。HTTP/HTTPS 支持路径前缀路由；HTTPS 支持可选访问激活。配额、限速、备份恢复、普通用户自助和完整指标/告警仍未实现，详见 [../requirements/limits.md](../requirements/limits.md)、[../requirements/proxy-runtime.md](../requirements/proxy-runtime.md) 和 [admin-and-observability.md](admin-and-observability.md)。
