## 1. Provider 与 Domain 边界

- [x] 1.1 抽取托管证书 provider strategy 接口，覆盖 issue、renew/rotate、sync、revoke、request defaulting 和 provider metadata validation
- [x] 1.2 将 ACME DNS-01 签发/续期流程迁入 ACME provider strategy，并保持现有 DNS challenge cleanup、active/previous material 和失败保留 active 语义
- [x] 1.3 将 Cloudflare Origin CA 签发、轮换、同步和强确认撤销流程迁入 Origin CA provider strategy，并保持不自动撤销 previous 证书
- [x] 1.4 将 Origin CA hostnames、request type、requested validity、credential ID、Cloudflare certificate ID 和 provider status 校验下沉到 domain/provider 边界
- [x] 1.5 保持历史托管证书记录宽读取、严写入，确保新成功结果和 provider sync 更新必须通过 provider metadata 校验

## 2. 生命周期调度收敛

- [x] 2.1 抽取统一 lifecycle scheduler，集中实现 provider-specific window、serving status、due 判断、max lookahead 和 next attempt backoff
- [x] 2.2 将 certmanager service 的 `servingStatus`、`renewalWindow` 和 `nextAttemptAt` 迁移为调用统一 scheduler
- [x] 2.3 将 admin query 的托管证书健康摘要窗口计算迁移为调用统一 scheduler
- [x] 2.4 将 daemon certificate controller 改为使用 scheduler 查询候选和判断 due，避免用最大固定窗口长期加载明显不可能到期的 provider 候选

## 3. Store 查询合同

- [x] 3.1 为 `CertificateRepository` 增加 provider-aware lifecycle candidate 查询或等价 scoped 查询，并在 SQLite 实现中使用 provider window/status/next_attempt 条件
- [x] 3.2 为 `CertificateRepository` 增加管理详情所需的目标证书 scoped 查询，避免详情路径通过全量证书列表过滤
- [x] 3.3 为 `ProviderCredentialRepository` 增加 provider/status 维度查询，用于默认 Origin CA credential 解析
- [x] 3.4 在 SQLite 层补齐必要索引或非破坏性迁移，覆盖 lifecycle candidate、proxy ID、provider type/status credential 查询路径

## 4. Service、Controller 与 Admin 调用链

- [x] 4.1 为 certmanager service 增加接受已加载 `domain.ManagedCertificate` 的续期/轮换、sync 和 revoke 内部入口
- [x] 4.2 调整 `Renew`、`SyncOriginCA`、`RevokeOriginCA` 等 proxy ID 外部入口，使其只加载一次目标证书后进入统一内部路径
- [x] 4.3 调整 daemon controller，使其从候选记录直接完成 provider 选择、操作锁 key 计算和生命周期调用，不在调用前重复 `ByProxyID`
- [x] 4.4 调整 Origin CA credential 默认解析，显式 ID 使用 `ByID`，默认候选使用 provider/status scoped 查询，不再 `List` 后内存过滤
- [x] 4.5 调整 admin query 的证书详情和代理详情证书摘要加载路径，列表保持批量加载且不引入 per-proxy certificate 查询
- [x] 4.6 顺手收敛被触碰路径中的重复 helper，包括 Cloudflare HTTP request、CSR/PEM 辅助、certificate status 映射和前端 fingerprint 格式化

## 5. 测试与回归

- [x] 5.1 增加 provider strategy 单元测试，覆盖 ACME 与 Origin CA 的成功、失败、unsupported provider、metadata validation 和不自动 revoke previous
- [x] 5.2 增加 lifecycle scheduler 单元测试，覆盖 ACME renewal window、Origin CA rotation window、expiring soon、max lookahead、失败退避和临近过期紧急重试
- [x] 5.3 增加 certmanager service 测试，断言已加载证书记录入口不在 provider 选择前重复 `ByProxyID`
- [x] 5.4 增加 daemon controller 测试，断言候选记录被直接复用，provider-specific due 判断正确，`ErrOperationBusy` 仍被忽略
- [x] 5.5 增加 store/SQLite 查询测试，覆盖 provider/status credential 查询、provider-aware lifecycle candidate 查询和 scoped certificate detail 查询
- [x] 5.6 增加 admin query/API 测试，覆盖证书列表无 per-proxy 查询、证书详情无全量证书列表过滤、credential 默认解析无全量 `List`、secret-safe 输出不回归

## 6. 验证与文档

- [x] 6.1 更新 README、docs/daemon-runtime.md 或相关开发文档中关于 Origin CA rotation window、credential 默认选择和不自动撤销 previous 的说明
- [x] 6.2 运行 `go test ./...`
- [x] 6.3 运行前端相关测试
- [x] 6.4 运行 `openspec validate converge-certificate-lifecycle-architecture --strict`
- [x] 6.5 运行 `openspec validate --all --strict`
