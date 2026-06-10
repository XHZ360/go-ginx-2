## Why

Cloudflare Origin CA 证书将成为当前阶段 HTTPS proxy 的主要证书来源，但现有托管证书能力仍以 ACME DNS-01 为中心，Origin CA 只被标记为未来高级模式缺口。现在需要把 Origin CA 提升为一等托管证书 provider，让签发、轮换、同步、状态和安全边界都有明确合同。

## What Changes

- 新增 Cloudflare Origin CA 托管证书能力，支持为 HTTPS proxy 主机通过 Cloudflare Origin CA API 签发 origin 证书。
- 本地生成私钥和 CSR，只向 Cloudflare 发送 CSR，私钥继续只落受管证书目录并保持在 SQLite、管理 API 和普通日志之外。
- 管理员可以在 Admin UI 中维护 Cloudflare API Token credential；SQLite 只保存 credential metadata 和 secret 引用，token 明文不得进入 SQLite 或管理查询响应。
- 将托管证书来源显式区分为 ACME DNS-01 与 Cloudflare Origin CA，并复用现有 active material 健康检查、热加载、previous 回滚和失败保留 active 证书语义。
- 增加 Origin CA 轮换、远端同步和可见状态字段，包括 Cloudflare 证书 ID、hostnames、request type、requested validity、最近同步时间和 provider 错误摘要。
- 管理面增加 Origin CA 的签发、轮换、同步和强确认撤销入口；默认不自动撤销仍可能被 Cloudflare 使用的旧证书。
- 明确运行安全边界：Cloudflare Origin CA 证书仅适合作为 Cloudflare 到 origin 的 TLS 终止证书，直连公网浏览器信任不作为本阶段目标；管理员需要使用 Cloudflare proxied DNS 与 Full (strict) 等价配置。
- 清理残留 HTTPS passthrough 规格表述，HTTPS proxy 继续要求有效证书并执行 TLS 终止。

## Capabilities

### New Capabilities

- `cloudflare-origin-certificates`: 定义 Cloudflare Origin CA provider 的凭据、签发、存储、轮换、同步、撤销和管理端可见状态。

### Modified Capabilities

- `certificate-management`: 将 Cloudflare Origin CA 从未来高级模式缺口调整为受支持的托管 HTTPS 证书来源，并保持私钥和 provider 凭据边界。
- `certificate-lifecycle-health`: 扩展托管证书生命周期元数据，使状态和调度对 provider 可见，并支持 Origin CA 轮换/同步语义。
- `admin-resource-management`: 扩展证书管理 API/UI 合同，使管理员可以选择 Origin CA provider 并执行 Origin CA 生命周期动作。
- `reverse-proxy-runtime`: 明确 HTTPS runtime 对 Origin CA active material 的处理与其他托管证书一致，并移除残留 passthrough 场景表述。

## Impact

- 后端领域模型和 SQLite schema：托管证书需要明确 provider 类型，并保存 Cloudflare Origin CA 的非敏感元数据；新增 provider credential metadata 与 secret 引用；私钥、Cloudflare API token 明文和完整敏感响应不得进入 SQLite。
- 证书管理服务：需要抽象 ACME 与 Origin CA provider 操作，新增 Cloudflare Origin CA issuer、CSR/key 生成、credential 读取、轮换、同步和撤销保护。
- HTTPS runtime：继续基于 active material 健康检查选择证书，无需引入 Origin CA 专用 TLS 逻辑。
- Admin GraphQL/API/UI：证书列表和详情需要展示 provider、Origin CA 元数据和动作状态，mutation 需要支持 credential 维护、签发、轮换、同步和强确认撤销。
- 配置与部署文档：需要说明 Cloudflare API token credential 维护、推荐权限、Full (strict)、proxied DNS、直连 origin 信任限制和过期提醒策略。
- 测试：需要覆盖 Admin UI/API token 创建/更新/验证/禁用、Origin CA 签发成功、凭据缺失、API 失败保留 active 证书、轮换成功热加载、同步远端状态、强确认撤销、secret-safe 输出以及 HTTPS runtime 使用 Origin CA 托管证书。
