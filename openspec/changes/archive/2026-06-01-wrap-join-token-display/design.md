## Context

join token 目前是一个带前缀的长字符串，CLI 和 TUI 都把它作为单行文本展示。很多终端在复制超长行时容易漏掉行尾、混入换行，或者在 SSH/PowerShell 中产生不可见空白。客户端解码入口 `internal/enrollment.DecodeToken` 目前只会修剪首尾空白，不能容忍 token 中间的换行或制表符。

## Goals / Non-Goals

**Goals:**

- 管理 CLI 和 TUI 继续原样展示 join token，不主动插入换行。
- 客户端 join 解析时忽略 token 中所有 Unicode 空白字符。
- 保持 token payload、hash 校验、过期和单次消费语义不变。
- 让命令替换和粘贴带空白 token 都可用。

**Non-Goals:**

- 不改变 join token 前缀、编码格式或 payload 字段。
- 不引入二维码、剪贴板、文件导出或新的交互依赖。
- 不让已使用或过期 token 可再次消费。

## Decisions

### 解析入口统一规范化 token

在 `internal/enrollment.DecodeToken` 中统一移除 token 中的空白字符，再执行前缀检查、base64 解码和 payload 校验。这样 `goginx-client join`、服务端兑换测试和任何后续共享解码路径都会得到一致行为。

替代方案是在 `cmd/goginx-client` 里预处理参数；这会让其他调用 `DecodeToken` 的路径继续脆弱。

### 展示层保持原样输出

管理端不主动插入换行，避免改变 `goginx-admin create-client-join` stdout 的单行 token 形态，也避免 TUI 结果页把 token 分割成多段导致终端选择行为不一致。

替代方案是在展示层按固定宽度硬换行；当前不采用，因为操作者明确要求移除终端自动换行展示。

### 保持 stdout 只输出 token

`goginx-admin create-client-join` 仍然只向 stdout 输出原始单行 token 内容。由于客户端会忽略空白，脚本中的命令替换和手工复制都能继续工作。

## Risks / Trade-offs

- [Risk] 移除所有空白可能掩盖复制过程中的格式问题。 -> Mitigation: token 字符集本身不需要空白，保留前缀、base64 解码和 hash 校验作为最终安全校验。
