## ADDED Requirements

### Requirement: Whitespace-tolerant join token parsing
系统 MUST 在客户端 join/enrollment 解析时容忍复制过程中引入的空白字符，同时保持 token 校验语义不变。

#### Scenario: Client accepts wrapped join token
- **WHEN** 操作者把包含换行的 join token 传给 `goginx-client join`
- **THEN** 客户端移除 token 中的空白字符后继续解码和兑换 token

#### Scenario: Client accepts token with incidental whitespace
- **WHEN** join token 中包含首尾空格、行内空格、制表符或回车换行
- **THEN** 客户端在执行前缀、payload、hash、过期和单次消费校验前忽略这些空白字符

#### Scenario: Token security checks remain enforced
- **WHEN** 移除空白后的 join token 已使用、过期、被篡改或 hash 不匹配
- **THEN** 客户端 join 仍被拒绝
