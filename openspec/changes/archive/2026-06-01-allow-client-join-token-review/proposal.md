## Why

当前客户端 join token 只在创建结果中展示一次。操作者一旦离开结果页或终端输出丢失，就只能重新创建客户端或重新生成 join 材料。这不符合本地运维“简单、可恢复”的目标，尤其是在服务器终端配置后还需要把 token 转交到另一台客户端主机的场景。

## What Changes

- 保存新创建的客户端 join token 明文，使授权管理员可以在 token 未使用且未过期时重复查看。
- 保持 join token 的客户端消费语义为单次使用；重复查看不等于重复消费。
- 为本地 TUI 提供查看当前客户端可用 join token 的动作。
- 增加管理端 `client-join-command` 子命令，用普通终端输出客户端可直接执行的 join 指令，TUI 仅展示该短指令以降低复制长 token 的风险。
- 更新提示文案，不再把 join token 描述成离开结果页后不可再次获得。
- 继续避免在日志和审计事件中写入完整 join token。

## Capabilities

### Modified Capabilities

- `admin-resource-management`: 管理员创建客户端 join token 后，可以在未使用且未过期期间重复查看该 token。
- `control-channel`: join/enrollment 材料仍保持单次消费和过期语义，但允许管理员侧重复展示未消费材料。

## Impact

- 影响 SQLite `client_enrollments` 表，新增保存 token 明文的列；旧记录没有明文 token 时不可回看。
- 影响 `cmd/goginx-admin`、`internal/admin`、`internal/store`、`internal/store/sqlite` 和 `internal/admintui`。
- 影响 README 和相关测试。
- 不改变 `goginx-client join <token>` 的消费行为。
