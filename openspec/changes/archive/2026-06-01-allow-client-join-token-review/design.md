## Context

join token 当前由 `internal/admin.Service.CreateClientJoin` 生成并返回，服务层随后只保存 `secret_hash` 和 `token_hash`。`internal/enrollment.Service.Redeem` 使用 token payload 与数据库中的 hash 做单次消费校验。这个模型能防止后续读取明文，但也导致管理员离开创建结果后无法再查看 token。

## Decisions

### Store token text for active admin review

新增 `domain.ClientEnrollment.Token` 字段和 SQLite `client_enrollments.token` 列。新建 join token 时同时保存明文 token、hash 和过期时间。服务端兑换 token 仍使用现有 hash 校验和 `used_at` 标记，不依赖明文列完成认证。

旧数据库迁移通过 `alter table ... add column token text not null default ''` 完成。已有 enrollment 记录没有明文 token，无法回看；这比尝试从 hash 还原更明确。

### Review only active unused tokens

新增管理员服务能力按客户端 ID 查找最新的可查看 join token。可查看条件是：

- token 明文存在；
- `used_at` 为空；
- `expires_at` 晚于当前时间。

用过、过期或旧格式缺少明文的 token 不返回。管理员需要重新生成 join token。

### Keep redemption single-use

重复查看仅发生在管理员侧，不改变 `/api/client/enroll` 的兑换语义。客户端仍只能成功 redeem 一次，重复 redeem 仍返回冲突。

### Provide a terminal command for the client join command

TUI 结果页继续展示 token，同时额外展示 `goginx-admin client-join-command -client <id>` 管理端指令。该指令在普通终端中读取最新可查看 token，并输出客户端可直接执行的 `goginx-client join <token>`。这样 TUI 不需要承载完整客户端命令中的长 token，操作者可在普通终端中复制完整单行命令。

### Audit review without leaking token

查看 token 记录审计事件时只写资源类型、资源 ID 和动作，不写 token 明文。普通日志和错误信息仍不得包含完整 token。

## Risks / Trade-offs

- 明文 token 进入 SQLite 会增加本地数据库泄露时的风险。该风险通过只返回未使用未过期 token、短 TTL、管理员权限边界和不写日志来控制。
- 如果操作者需要更强安全语义，可以缩短 TTL 或在 token 被查看后尽快完成客户端 join。
- 已有 token 因历史记录没有明文字段，无法追溯展示；这是安全 hash 设计的自然限制。
