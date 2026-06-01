## 1. Token 解析

- [x] 1.1 在 `internal/enrollment` 中增加 token 空白规范化逻辑，移除首尾、行内、换行和制表等空白字符。
- [x] 1.2 增加 token 解码测试，覆盖换行、空格、制表符和被篡改 token 的拒绝语义。

## 2. Token 展示

- [x] 2.1 移除终端展示自动换行 helper。
- [x] 2.2 确认 `goginx-admin create-client-join` stdout 继续输出原始单行 token。
- [x] 2.3 确认 TUI 创建和查看 join token 结果页继续原样展示 token。

## 3. 测试与文档

- [x] 3.1 更新 jointoken 测试，移除展示换行 helper 覆盖。
- [x] 3.2 更新 OpenSpec 文档，移除终端自动换行展示要求。
- [x] 3.3 运行相关 Go 测试和 OpenSpec 校验。
