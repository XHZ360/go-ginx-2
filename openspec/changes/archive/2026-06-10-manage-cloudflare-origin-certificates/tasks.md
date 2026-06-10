## 1. 数据模型与配置

- [x] 1.1 为托管证书领域模型新增 provider 类型、provider 名称、credential ID、provider 状态、Cloudflare certificate ID、hostnames、request type、requested validity、last_synced_at 等非敏感字段
- [x] 1.2 新增 provider credential 领域模型和 SQLite metadata 表，保存 credential ID、名称、provider 类型、作用域、token 指纹、secret 引用、验证状态和脱敏错误
- [x] 1.3 新增 SQLite 外 secret store，用于保存 Admin UI 写入的 Cloudflare API Token material，并确保文件权限、日志和 API 响应保持 secret-safe
- [x] 1.4 为 SQLite `managed_certificates` 增加对应迁移，迁移旧 ACME 记录为 `provider_type=acme_dns01`、`provider_name=cloudflare`，并保持私钥和 token 明文不入库
- [x] 1.5 扩展服务端配置，增加 Cloudflare Origin CA 启用开关、secret store 路径、request type 默认值、requested validity 和 rotation window
- [x] 1.6 更新配置加载和校验，拒绝 Origin CA Service Key 路径，并对缺失、禁用或验证失败的 credential 记录可消费的 provider 配置错误

## 2. Origin CA Provider

- [x] 2.1 新增 Cloudflare Origin CA provider 接口和实现，通过 credential ID 从 secret store 读取 token，覆盖 create、get/list、revoke 和脱敏错误处理
- [x] 2.2 实现本地私钥和 CSR 生成，确保 Cloudflare API 请求只包含 CSR、hostnames、request type 和 requested validity
- [x] 2.3 实现 Origin CA 签发流程：调用 Cloudflare create、校验证书/key/host/有效期、写入 active material、记录 Cloudflare certificate ID 和 fingerprint
- [x] 2.4 实现 Origin CA provider sync，记录 active 远端状态、last_synced_at、provider_status 和脱敏错误
- [x] 2.5 实现强确认 revoke，要求 proxy ID、host 和 Cloudflare certificate ID 匹配，并区分 active 与 previous 证书撤销结果

## 3. 托管证书生命周期

- [x] 3.1 将现有 certmanager 调整为 provider-aware lifecycle service，支持 ACME DNS-01 与 Cloudflare Origin CA 两类 provider
- [x] 3.2 将 Origin CA rotation 接入通用生命周期操作，成功时复用 active/previous 文件替换、热加载和回滚语义
- [x] 3.3 扩展 daemon 证书 controller，根据 provider 类型分别使用 ACME renewal window 和 Origin CA rotation window，并保留退避和单飞
- [x] 3.4 在签发、轮换、同步和撤销失败时保留仍有效 active material，并记录 operation_status、failure_count、next_attempt_at 和 last_error
- [x] 3.5 当 provider sync 确认 active Origin CA 证书已撤销或远端不可用时，将证书和对应 HTTPS proxy 标记为需要配置或不可服务

## 4. 管理 API、CLI 与 UI Surface

- [x] 4.1 扩展 admin query/API，支持 Cloudflare Origin CA credential 列表、详情、创建、更新、验证、禁用和删除，token 字段只允许写入不允许读取
- [x] 4.2 扩展 admin query 证书 summary，返回 provider 类型、credential ID、provider 状态、Cloudflare certificate ID、hostnames、request type、requested validity、last_synced_at 和 deployment hints
- [x] 4.3 扩展 GraphQL mutation/input，支持选择 Cloudflare Origin CA provider 和 credential，并提供 issue、rotate、sync 和强确认 revoke 动作
- [x] 4.4 更新 admin service 审计事件，记录 credential 维护以及 Origin CA 签发、轮换、同步和撤销动作，同时保持 secret-safe 输出
- [x] 4.5 更新 admin CLI 证书命令，支持 Origin CA provider、credential 参数和 sync/revoke 子命令或等价命令
- [x] 4.6 更新管理 UI 证书页和设置/凭据入口，展示 Origin CA credential 状态、Cloudflare 部署提示、过期/轮换提示和危险撤销确认

## 5. HTTPS Runtime 与状态映射

- [x] 5.1 扩展证书 resolver 或健康视图，使已确认 provider-side revoked/missing 的 Origin CA active material 不被声明为可服务
- [x] 5.2 确认 HTTPS runtime 对 Origin CA 托管证书不增加专用 TLS 分支，仍通过通用 active material 完成 TLS 终止
- [x] 5.3 清理运行时规格和文档中的残留 HTTPS passthrough 表述，保持无证书或证书失效时 fail closed
- [x] 5.4 更新代理列表和详情状态映射，使 Origin CA provider 不可用时 HTTPS proxy 展示为 `needs_config` 或等价证书不可服务状态

## 6. 测试与文档

- [x] 6.1 增加 provider credential 和 secret store 测试，覆盖 Admin UI/API 创建、更新、验证、禁用、删除、write-only token 和 secret-safe 输出
- [x] 6.2 增加 Cloudflare Origin CA provider 单元测试，覆盖 credential token 加载、Service Key 拒绝、create/list/get/revoke 请求和响应脱敏
- [x] 6.3 增加证书生命周期测试，覆盖 Origin CA 签发成功、签发失败不替换 active、轮换成功保留 previous、轮换失败保留 active、sync revoked 标记不可服务
- [x] 6.4 增加 SQLite 迁移和 admin query/API 测试，覆盖 provider metadata、credential metadata、secret-safe 输出、强确认 revoke 和审计事件
- [x] 6.5 增加 HTTPS runtime 测试，覆盖 Origin CA managed certificate 可终止 TLS、provider-side revoked 不被使用、无可服务证书拒绝连接
- [x] 6.6 更新 README、docs/daemon-runtime.md 和证书故障排查文档，说明 Admin UI 维护 API Token credential、Full (strict)、proxied DNS、直连限制、rotation window 和撤销风险
- [x] 6.7 运行 `go test ./...`、前端相关测试和 `openspec validate --all --strict`
