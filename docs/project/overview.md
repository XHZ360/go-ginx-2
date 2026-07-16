# 项目概览

## 目标

`go-ginx-2` 是 Simp-Frp/go-ginx 设计的新实现。当前仓库完成里程碑一运行时和首个部署基线，重点覆盖：

- 控制面（QUIC 与 TCP+TLS）
- TCP/UDP/HTTP/HTTPS 反向代理（含路径路由与 HTTPS 访问激活）
- 管理 API/UI
- 证书管理
- SQLite 持久化
- 可复现部署包

## 产品形态

- **服务端**：资源管理、控制通道、入口 listener、运行时路由。
- **Provider 客户端**：连接本地 target，承接服务端打开的代理子流。
- **Consumer SDK**（可选）：应用侧主动访问用户已授权的代理。
- **Admin API/UI**：管理员资源管理、证书生命周期、审计与仪表盘。
- **数据层**：cgo-free SQLite（默认 `data/go-ginx.db`）。

当前系统是反向代理平台，不是任意目标正向代理。

## 技术栈约束

| 维度 | 约束 |
| --- | --- |
| Go 模块 | `github.com/simp-frp/go-ginx-2` |
| Go 版本 | `1.26.0`（见 `go.mod`） |
| CGO | 默认 `CGO_ENABLED=0` |
| 前端 | React、Vite、TypeScript、Ant Design、`pnpm@10.33.2` |
| 部署目标 | Linux `systemd` 部署包、Windows 发布包 |
| 文档语言 | 默认简体中文；代码与技术术语可保留英文 |

## 代码入口

- `cmd/`：server、client、admin
- `internal/`：控制通道、代理、证书、Admin、存储
- `admin-ui/`：管理前端源码
- `e2e/`：跨进程验证
- `sdk/`：consumer SDK
- `deploy/`：systemd 模板

## 明确边界

尚未实现：forward proxy、配额/限速、普通用户自助、备份恢复、完整指标/告警、原生安装器与包管理器分发。详见 [requirements/limits.md](../requirements/limits.md)。

## 相关入口

- 协作流程：[workflow.md](workflow.md)
- 文档规则：[documentation-workflow.md](documentation-workflow.md)
- 当前进展：[../worklog.md](../worklog.md)
