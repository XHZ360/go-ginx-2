## Why

Cloudflare Origin CA 接入后，托管证书生命周期已经同时覆盖 ACME DNS-01 与 Origin CA，但 provider 分支、字段校验、轮换窗口和查询加载路径仍分散在 service、store、admin query 与 controller 边界。现在需要把这些内部合同收敛，避免后续新增 provider 或调整生命周期规则时继续放大分支复杂度和数据库读取成本。

## What Changes

- 将托管证书生命周期按 provider 策略组织，使 ACME DNS-01 与 Cloudflare Origin CA 的签发、续期/轮换、同步、撤销和字段校验通过明确 provider contract 接入。
- 将 Origin CA 等 provider 专属字段校验下沉到证书领域模型或等价 domain 边界，服务层、controller 和管理 API 不再各自复制校验分支。
- 将 ACME renewal window、Origin CA rotation window、紧急过期窗口和失败退避候选时间收敛为单一调度来源，controller 与手动生命周期动作使用同一套窗口计算语义。
- 优化续期/轮换路径查询：生命周期调用链可以复用已加载的 managed certificate 行，避免按 proxy ID 重复查询和 credential 解析时全量 List 后内存过滤。
- 调整 certmanager service/store/admin query 调用合同，使管理查询与生命周期动作只加载目标证书、目标 credential 或必要的批量候选集合。
- 保持已有产品语义不变：轮换后不自动撤销 previous Cloudflare Origin CA 证书，撤销仍必须由管理员显式强确认。
- 顺手收敛局部重复实现，例如 Cloudflare HTTP request 封装、CSR/PEM 辅助逻辑、certificate status 映射和指纹格式化，但不为单纯去重引入跨层大抽象。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `certificate-management`: 明确托管证书 provider contract、provider 专属字段校验边界，以及轮换后不自动撤销 previous Origin CA 证书的保持语义。
- `certificate-lifecycle-health`: 明确生命周期窗口与候选时间必须来自单一调度来源，并支持复用已加载证书记录执行续期/轮换评估。
- `admin-resource-management`: 明确管理查询和生命周期动作的加载合同，避免证书列表、详情、credential 解析和手动动作通过全量扫描或重复 proxy 查询获取目标资源。

## Impact

- 后端 certmanager service、provider 实现、store 接口、daemon controller、admin query/command service 和 GraphQL resolver 会调整调用边界。
- SQLite schema 预计不需要引入新的产品字段；若实现发现缺少索引或唯一约束，可增加非破坏性迁移。
- 管理 API/UI 可见字段和操作语义保持兼容，但错误来源、校验错误和生命周期状态会通过统一 domain/provider 规则返回。
- 测试需要覆盖 provider contract、字段校验、窗口计算、候选加载、手动生命周期动作、credential 定点读取、secret-safe 输出和查询数量/路径回归。
