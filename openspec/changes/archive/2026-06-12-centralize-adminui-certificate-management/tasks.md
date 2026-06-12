## 1. 数据模型与迁移

- [x] 1.1 设计并实现独立 certificate resource 与 HTTPS proxy certificate binding 的存储模型
- [x] 1.2 增加幂等 SQLite 迁移，把旧 `managed_certificates.proxy_id` 语义迁移到显式绑定
- [x] 1.3 增加幂等 SQLite 迁移，把旧 HTTPS proxy `cert_file` / `key_file` 转换或适配为 file-backed certificate resource
- [x] 1.4 更新 domain/store repository 接口，支持按 certificate ID、host、binding、引用状态和 provider 条件查询
- [x] 1.5 保留旧数据读取兼容路径，并为新写入路径使用显式 certificate ID

## 2. 后端管理服务与运行时

- [x] 2.1 更新 admin command/query 服务，支持证书创建、删除、引用检查、绑定、解绑和绑定校验
- [x] 2.2 更新 ACME DNS-01 与 Cloudflare Origin CA 生命周期入口，使证书可先作为未绑定资源创建
- [x] 2.3 为 Origin CA 撤销、仍在使用且可服务证书删除和 active material 移除动作实现强确认校验，并允许无效、失效/过期或未使用证书无二次确认删除
- [x] 2.4 更新 HTTPS certificate resolver，优先按 proxy 绑定的 certificate ID 加载 active material
- [x] 2.5 为迁移期保留 host 查询和旧 proxy 文件路径 fallback，并确保 fallback 不成为新 AdminUI 主路径
- [x] 2.6 确保证书删除不会删除不属于受管证书目录或无法安全归属的外部文件

## 3. GraphQL 与 API 合同

- [x] 3.1 更新 Admin GraphQL schema，暴露 certificate ID、binding proxy、引用状态、deployment hints 和 secret-safe 摘要
- [x] 3.2 新增或调整证书创建、删除、绑定、解绑和生命周期 mutation
- [x] 3.3 调整 proxy create/update 输入，支持 certificate ID 选择并保留旧字段兼容策略
- [x] 3.4 更新 GraphQL generated types、前端 API wrapper 和错误映射
- [x] 3.5 为删除风险分级、证书不兼容、强确认失败和 provider-specific 错误返回可消费错误

## 4. AdminUI Certificates 页

- [x] 4.1 重构 Certificates 页，把证书创建、删除、签发、续期、轮换、同步、撤销和 credential 管理集中在该页面
- [x] 4.2 增加证书创建流程，支持从 proxy 表单带入 host/provider 上下文并在成功后返回
- [x] 4.3 展示证书 ID、provider、hostnames、绑定 proxy、有效期、状态维度、部署提示、引用状态和脱敏错误
- [x] 4.4 将 serving status、operation status 和 provider status 的筛选/展示分离，避免混合状态筛选
- [x] 4.5 按 provider、状态、引用关系和 active material 可用性控制动作按钮可用性
- [x] 4.6 为 Origin CA 撤销和仍在使用且可服务证书删除实现输入式强确认，并让无效、失效/过期或未使用证书走无二次确认的普通删除

## 5. AdminUI HTTPS Proxy 表单

- [x] 5.1 从 HTTPS proxy 创建/编辑主流程移除证书文件和私钥文件路径输入
- [x] 5.2 增加证书选择控件，只展示与当前 SNI 域名兼容且可绑定的证书
- [x] 5.3 增加所选证书摘要和不兼容/不可用原因提示
- [x] 5.4 增加创建证书跳转入口，并保存不含 secret material 的 proxy 表单草稿
- [x] 5.5 证书创建成功返回后恢复 proxy 表单草稿，并自动选中新证书
- [x] 5.6 草稿缺失、过期或解析失败时显示安全降级提示，并允许手动选择证书

## 6. 测试与验证

- [x] 6.1 增加 store migration 测试，覆盖旧托管证书和旧静态文件路径迁移
- [x] 6.2 增加 admin service 测试，覆盖证书创建、风险分级删除、绑定、解绑、低风险无确认删除和强确认失败
- [x] 6.3 增加 HTTPS resolver 测试，覆盖 certificate ID 绑定、hostnames 不匹配、旧 fallback 和 provider blocking 状态
- [x] 6.4 增加 GraphQL 测试，覆盖新 mutation、proxy certificate selection 和 secret-safe 响应
- [x] 6.5 增加 AdminUI 测试，覆盖 Certificates 页动作可用性、高风险强确认、低风险无二次确认删除、状态筛选维度和 provider hints
- [x] 6.6 增加 AdminUI 测试，覆盖从 HTTPS proxy 表单跳转创建证书、返回恢复草稿和自动选择新证书
- [x] 6.7 运行 `go test ./...`、`pnpm --dir admin-ui test` 和 `pnpm --dir admin-ui build`

## 7. 文档与迁移说明

- [x] 7.1 更新 AdminUI certificates 文档，说明证书页是证书增删和生命周期动作唯一入口
- [x] 7.2 更新 HTTPS proxy 文档，说明 proxy 只选择证书或跳转创建证书
- [x] 7.3 更新证书管理文档，说明显式绑定、一对一限制、迁移兼容和 Origin CA 部署提示
- [x] 7.4 记录旧 `certFile/keyFile` 和旧 `managed_certificates.proxy_id` 语义的迁移/兼容策略
