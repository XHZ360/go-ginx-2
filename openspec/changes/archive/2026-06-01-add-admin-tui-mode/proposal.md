## Why

当前 `goginx-admin` 已支持非交互式种子命令，但首次部署和本地运维仍要求操作者记住多个子命令、资源 ID 和参数组合。新增本地 TUI 模式可以把管理员初始化、用户与客户端配置整理成可发现的流程，降低部署初期和日常维护的出错概率。

## What Changes

- 为 `cmd/goginx-admin` 新增本地 TUI 入口，作为面向终端的交互式运维工具。
- TUI 聚焦本地 SQLite 管理，不引入远程登录、浏览器会话或 GraphQL 客户端模式。
- TUI 提供快速初始化管理员信息的流程，支持发现现有管理员并安全更新密码或启用状态。
- TUI 提供用户与客户端的快速配置流程，优先通过列表、选择器、默认值和确认步骤减少手动输入。
- TUI 提供用户和客户端的基础维护动作，包括启用、禁用和受保护删除。
- TUI 对关键字段执行强校验，并在提交前阻止无效配置进入持久化层。
- 保留现有非交互式 CLI 子命令行为，新增 TUI 不作为破坏性变更。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `admin-resource-management`: 增加本地 admin TUI 模式的启动、管理员初始化、用户/客户端配置、选项优先交互和强校验要求。

## Impact

- 影响 `cmd/goginx-admin` 的命令分发、帮助文案和测试。
- 可能新增 `internal/admintui` 或同等边界的 TUI 包，用于隔离交互状态、表单模型和终端渲染。
- 复用 `internal/admin` 的命令服务、`internal/adminquery` 的查询模型、`internal/store/sqlite` 的本地存储打开逻辑和现有部署根路径解析。
- 可能补充用户删除相关的受保护服务层行为，避免 TUI 直接触发 SQLite 级联删除。
- 可能引入 Go TUI 依赖，用于终端事件循环、列表、表单和样式。
- 更新 README 或运维文档，说明 `goginx-admin tui` 的定位、默认数据库路径和与非交互式子命令的关系。
