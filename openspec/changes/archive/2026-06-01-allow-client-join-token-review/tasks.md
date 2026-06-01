## 1. 存储与领域模型

- [x] 1.1 为 `domain.ClientEnrollment` 增加可选 token 明文字段，并更新校验语义保持旧记录兼容。
- [x] 1.2 为 SQLite `client_enrollments` 增加 `token` 列和迁移逻辑，新建记录写入 token。
- [x] 1.3 为 enrollment repository 增加按客户端查询最新未使用未过期 token 的能力。

## 2. 服务与交互

- [x] 2.1 在 `internal/admin` 中新增查看客户端 join token 服务方法，拒绝已使用、已过期和缺少明文的记录。
- [x] 2.2 保持客户端 redeem 单次消费语义不变。
- [x] 2.3 在 TUI 客户端操作菜单增加查看 join token 动作，并更新创建结果文案。

## 3. 测试与文档

- [x] 3.1 增加 SQLite 和 admin service 测试，覆盖 token 持久化、可重复查看、过期/已用拒绝和旧记录缺少明文拒绝。
- [x] 3.2 增加 TUI 模型测试，覆盖重复查看结果页和错误状态。
- [x] 3.3 更新 README/运维文档，说明 join token 可重复查看但只能消费一次。
- [x] 3.4 运行相关 Go 测试和 OpenSpec 校验。
- [x] 3.5 增加 TUI 结果页客户端 join 命令提示，并运行相关测试和 OpenSpec 校验。
- [x] 3.6 将 TUI 结果页提示调整为管理端 `client-join-command` 指令，并运行相关测试和 OpenSpec 校验。
