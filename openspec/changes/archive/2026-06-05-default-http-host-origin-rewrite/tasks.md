## 1. 测试基线

- [x] 1.1 在 HTTP 反向代理测试中断言本地目标接收到的 `Host` 为 `targetHost:targetPort`
- [x] 1.2 在 HTTP 反向代理测试中覆盖可解析 `Origin` 被改写为 `http://targetHost:targetPort`
- [x] 1.3 在 HTTP 反向代理测试中覆盖缺失、`null` 或不可解析 `Origin` 不被新增或破坏
- [x] 1.4 在 HTTPS TLS 终止测试中断言本地 HTTP 目标接收到改写后的 `Host` 和可解析 `Origin`

## 2. 运行时实现

- [x] 2.1 在客户端 HTTP stream 出口提取 target authority，并同时设置 `Request.URL.Host` 和 `Request.Host`
- [x] 2.2 增加 Origin 改写辅助逻辑，只改写可解析的 HTTP/HTTPS origin
- [x] 2.3 确认 HTTPS SNI 透传路径仍走 `kind: "tcp"`，不引入请求头处理

## 3. 验证

- [x] 3.1 运行 `go test ./internal/control ./internal/proxy/http ./internal/proxy/https`
- [x] 3.2 运行受影响的 daemon 或 e2e HTTP/HTTPS 代理测试，确认端到端行为一致
- [x] 3.3 运行 `openspec status --change "default-http-host-origin-rewrite"` 并确认 artifacts apply-ready
