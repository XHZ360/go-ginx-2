## Why

当前客户端 join 会访问 token 中的 `enrollment_url`，而默认 `enrollment_url` 从 `admin_listen` 派生，导致客户端入网兑换流程和 admin-ui/admin API 共用 8080。生产部署为了安全通常会限制 admin-ui 外网访问，这会让远程客户端无法完成 join，或者迫使操作者把管理入口暴露到公网。

本变更将客户端 enrollment 兑换入口从管理入口拆分出来，使 admin-ui 可以继续仅内网访问，同时客户端 join 使用单独的外网可达端口。

## What Changes

- 新增客户端 enrollment 专用监听配置，默认监听所有地址的 `:8081`，仅服务 `/api/client/enroll`。
- join/enrollment 默认材料生成时，默认 `enrollment_url` 改为使用 enrollment 专用监听端口，而不是 `admin_listen` 端口。
- 新增环境变量覆盖 enrollment 监听地址，使 configless 部署可通过环境变量修改端口或绑定地址。
- 服务端运行时启动独立 enrollment listener；admin listener 继续服务 admin-ui、`/api/admin/*` 和管理会话，并移除 `/api/client/enroll` 路由。
- **BREAKING**：指向 admin listener 的旧 join token 或显式 `enrollment_url` 不再可兑换，操作者需要重新生成指向专用 enrollment listener 的 token。
- **BREAKING**：configless 默认端口分配调整为 `http_entry_listen: ":80"`、`https_entry_listen: ":443"`，显式配置的 `http_entry_listen`/`https_entry_listen` 不受影响。
- 文档更新默认端口、安全边界、反向代理建议和故障排查说明。

## Capabilities

### New Capabilities

暂无。

### Modified Capabilities

- `control-channel`：客户端 join/enrollment 材料的默认 `enrollment_url` 改为使用专用 enrollment listener。
- `deployment-operations`：服务端 configless 默认监听、环境覆盖、启动日志和运维文档需要包含 enrollment listener。
- `admin-resource-management`：静态 ListenerClaim 集合和管理入口边界需要包含独立 enrollment listener，避免端口冲突和误暴露 admin-ui。

## Impact

- 影响配置结构、默认配置、环境变量加载、配置校验和默认 join 参数解析。
- 影响服务端 daemon 启动/关闭生命周期，需要新增独立 HTTP listener 或复用可测试的轻量 server 封装。
- 影响 admin API/enrollment 路由组织：`/api/client/enroll` 只在专用 listener 上服务，admin listener 不再处理客户端 enrollment。
- 影响 admin CLI、TUI、Admin API 生成 join token 的默认值。
- 影响端口冲突检测、e2e/configless 测试、README 和 `docs/daemon-runtime.md`。
