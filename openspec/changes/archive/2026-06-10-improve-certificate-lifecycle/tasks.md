## 1. 数据模型与迁移

- [x] 1.1 扩展 `domain.ManagedCertificate`，拆分或补充服务状态、操作状态、最近检查时间、最近尝试时间、下一次尝试时间、失败次数和证书指纹字段
- [x] 1.2 为 SQLite `managed_certificates` 添加对应迁移，保留旧记录兼容默认值，并确保私钥和 DNS token 仍不进入 SQLite
- [x] 1.3 更新 certificate repository 的 create/list/update 方法，支持健康、操作和调度字段的读写
- [x] 1.4 为 HTTPS proxy 定义证书失效/需要配置的运行时状态映射，覆盖无证书、证书过期、证书/key 不匹配和证书恢复
- [x] 1.5 增加 store 层测试，覆盖旧记录迁移、成功更新、失败更新、退避字段和 secret-safe 元数据持久化

## 2. Active Material 健康检查

- [x] 2.1 新增托管证书 active material 健康检查器，校验证书/私钥文件存在、可读、key 匹配、host 匹配和有效期
- [x] 2.2 定义健康检查结果到服务状态的映射，覆盖 usable、expiring_soon、expired、missing 和 invalid
- [x] 2.3 将 HTTPS proxy 无完整证书、静态证书失效或托管 active material 失效映射为证书失效/需要配置状态
- [x] 2.4 在健康检查错误中执行脱敏和长度限制，避免私钥内容、DNS token 或完整敏感响应进入日志/API/SQLite
- [x] 2.5 增加 HTTPS certificate 单元测试，覆盖健康、即将过期、过期、文件缺失、host mismatch、key mismatch 和无证书 proxy 失效

## 3. 签发与续期生命周期

- [x] 3.1 更新 `certmanager.Service`，让 issue/renew 成功时更新 active material、操作状态、健康状态和调度状态
- [x] 3.2 更新失败路径，确保 issue/renew 失败只记录操作失败和退避信息，不替换或禁用仍有效 active material
- [x] 3.3 将 daemon 续期循环重构为托管证书 controller，支持启动扫描、周期检查、`next_attempt_at` 跳过和同一 proxy/host 单飞
- [x] 3.4 实现续期失败退避策略，成功后重置失败次数和下一次尝试时间
- [x] 3.5 增加 certmanager/daemon 测试，覆盖续期失败保留旧证书、退避跳过、手动和自动续期并发、provider 凭据缺失

## 4. HTTPS 运行时证书选择

- [x] 4.1 更新 `CertificateResolver`，只返回通过健康检查的托管 active material，并保持静态证书无效时失败关闭
- [x] 4.2 确保最近续期失败但 active material 健康时，HTTPS TLS 终止继续使用旧有效证书
- [x] 4.3 取消 HTTPS SNI passthrough 路径；HTTPS proxy 无可服务证书时拒绝连接并记录证书缺失或证书失效错误
- [x] 4.4 在 listener reconcile、proxy 查询和运行时路由中暴露 HTTPS proxy 证书失效/需要配置状态
- [x] 4.5 增加 HTTPS runtime 测试，覆盖续期失败仍终止 TLS、托管证书损坏不被使用、无证书拒绝连接、旧 passthrough 行为取消

## 5. 管理 API 与前端状态

- [x] 5.1 扩展 adminquery 证书 summary，返回服务状态、操作状态、最近检查时间、最近尝试时间、下一次尝试时间、失败次数和指纹
- [x] 5.2 更新 GraphQL schema、resolver、前端 graphql 文档和生成类型，保持证书响应不泄露私钥或 DNS token
- [x] 5.3 更新证书列表和代理详情 UI，展示服务状态、操作状态、代理证书失效状态、退避/下一次尝试、失败次数和脱敏错误
- [x] 5.4 增加 admin API 和 UI 测试，覆盖 secret-safe 输出、状态筛选、失败续期但仍 serving、无证书 HTTPS proxy 失效的展示

## 6. 验证与文档

- [x] 6.1 更新 README 和 `docs/daemon-runtime.md`，说明 HTTPS passthrough 取消、HTTPS proxy 必须配置有效证书、新证书状态语义、续期退避、失败续期保留 active 证书和故障排查方法
- [x] 6.2 运行 Go 单元测试：`go test ./internal/proxy/https ./internal/certmanager ./internal/daemon ./internal/store/sqlite ./internal/admin ./internal/adminapi ./internal/adminquery`
- [x] 6.3 运行前端测试和类型生成验证，确保证书页面展示和 GraphQL 类型一致
- [x] 6.4 运行 OpenSpec 校验，确认 proposal、design、specs 和 tasks 均通过
