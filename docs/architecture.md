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

## 明确边界

当前系统是反向代理平台，不是任意目标正向代理。配额、限速、备份恢复、普通用户自助和完整指标/告警仍未实现，详见 `docs/limits.md` 和 `docs/admin-and-observability.md`。
