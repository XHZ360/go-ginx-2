## Why

join token 很长，终端复制时可能混入换行、空格或制表符。管理端继续原样输出 token，客户端 join 时忽略这些空白，可以让 SSH、PowerShell、日志转贴和手动复制场景更可靠，同时不改变终端展示格式。

## What Changes

- 管理 CLI/TUI 继续原样展示 join token，不主动插入换行。
- `goginx-client join <token>` 和 enrollment token 解码流程容忍 token 中的换行、空格、制表符等空白字符。
- 保持 token 的内容、签名/哈希校验、过期和单次消费语义不变。
- 移除此前用于终端展示自动换行的 helper 和测试覆盖。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `control-channel`: 客户端 join/enrollment token 解析必须容忍复制过程中引入的空白字符。

## Impact

- 影响 `internal/enrollment` token 解码，以及 `goginx-client join` 间接消费 token 的行为。
- 影响相关 Go 测试和 README 文档。
- 不引入新的外部依赖，不改变 token 格式或数据库 schema。
