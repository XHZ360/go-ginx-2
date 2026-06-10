## Why

当前托管 HTTPS 证书已经支持 ACME DNS-01 签发、续期、文件存储和热加载，但生命周期状态仍把“当前可服务材料”和“最近一次操作结果”混在一起。续期失败、证书过期、文件损坏或证书/key 不匹配时，管理端和运行时都缺少足够清晰的状态语义，容易出现旧证书仍有效却不再被使用，或证书实际不可用但状态仍显示可用的情况。

这次变更先补强证书管理的可靠性地基，让服务端能够准确判断证书是否可用于新握手，并让管理员看到可操作的证书健康状态和续期进度。

## What Changes

- 拆分托管证书的服务状态与操作结果：运行时基于已校验的 active material 决定是否终止 TLS，最近一次 issue/renew 失败不会自动使仍有效的 active 证书失效。
- 增加证书健康检查语义，覆盖文件缺失、证书过期、即将过期、域名不匹配、key 不匹配和无法读取等状态。
- 改造托管证书续期调度，使其记录下一次尝试时间、失败次数和最近检查时间，并在失败后退避重试。
- 保持成功续期的热加载行为，失败时继续保留和使用上一组仍有效证书，并在管理端暴露失败原因。
- **BREAKING**: HTTPS 代理不再支持 SNI passthrough；已启用 HTTPS 代理必须具备有效静态证书或托管 active 证书才能服务流量。
- 没有可用证书、证书缺失、过期或校验失败的 HTTPS 代理必须标记为证书失效/需要配置状态，并拒绝对应公网连接，而不是把加密字节流透传到客户端目标。
- 扩展管理查询和 GraphQL/UI 证书状态输出，展示服务可用性、操作状态、过期时间、下一次尝试时间和最近错误。
- 保持私钥边界：SQLite 仍只保存元数据和文件路径，不保存证书私钥或 DNS provider token。
- 不在本变更中实现完整证书上传、域名所有权验证、多 DNS provider、Cloudflare Origin CA 或控制通道 CA 轮换。

## Capabilities

### New Capabilities

- `certificate-lifecycle-health`: 定义 HTTPS 托管证书 active material 健康检查、操作结果、续期调度和管理端可见状态的生命周期契约。

### Modified Capabilities

- `certificate-management`: 明确托管证书续期失败不得禁用仍有效 active 证书，并要求证书健康状态与私钥边界保持一致。
- `acme-certificate-automation`: 扩展 ACME 续期调度、失败退避和状态输出要求。
- `reverse-proxy-runtime`: 移除 HTTPS SNI passthrough 基线，明确 HTTPS proxy 必须通过有效证书执行 TLS 终止；无证书或证书失效时代理不可服务。

## Impact

- 后端领域模型和 SQLite schema：托管证书元数据需要新增健康、调度和操作状态字段，或引入等价的派生状态与持久化调度字段。
- 证书存储与 resolver：需要在选择证书前校验 active material，并把校验结果反馈给生命周期状态。
- daemon 续期循环：从简单轮询改为带单飞、退避和可观测状态的证书控制器。
- Admin service、adminquery、GraphQL schema 和 Admin UI：需要展示更准确的证书服务状态、操作状态、失败原因和下一次尝试时间。
- 测试：需要覆盖续期失败保留旧证书、过期/损坏材料状态、无证书 HTTPS proxy 失效、passthrough 取消、调度退避、GraphQL secret-safe 输出和 HTTPS 运行时选择行为。
