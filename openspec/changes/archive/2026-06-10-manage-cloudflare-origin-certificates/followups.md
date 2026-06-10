## Deferred Review Follow-ups

本文件记录 `review01.txt` 中已评估但未在本次 change 内继续实现的建议，用于归档后追踪。

### 不建议在当前设计下实现

- 自动撤销轮换后的 previous Cloudflare Origin CA 证书。
  - 原因：当前 OpenSpec 明确要求 “Rotation does not auto-revoke previous certificate”，自动撤销可能破坏仍在使用的 Cloudflare 到 origin TLS 路径。
  - 当前处理：保留手动强确认 revoke，并已强化 active revoke 的 UI 告警。

### 建议另开 change 处理

- 生命周期架构收敛。
  - 范围：provider 策略化、Origin CA 字段校验下沉到 domain、rotation/renewal window 单一来源。
  - 价值：降低后续新增 provider 或修改生命周期规则时的分支复杂度。
  - 风险：会牵动 service、store、admin query 和 controller 边界，不适合塞进本次 review 修正。

- 续期/轮换路径查询优化。
  - 范围：减少 `ByProxyID` 重复查询，避免 credential 解析时全量 `List` 后内存过滤，允许 controller 透传已加载的 managed certificate 行。
  - 价值：证书数量增加后降低 DB 读放大，并让 lifecycle 调用链更清晰。
  - 风险：需要调整 certmanager service API 和调用方合同。

### 低优先级维护清理

- 重复实现抽取。
  - 候选：Cloudflare HTTP request 封装、CSR/PEM 辅助逻辑、`certificateStatusFromServing`、前端 `formatFingerprint`。
  - 建议：后续遇到相关代码时顺手收敛，避免为了去重引入过早抽象。

- controller 动态 sleep。
  - 当前已将默认扫描间隔从 1 秒调整为 1 分钟，解决主要空转问题。
  - 动态按下一张证书到期时间休眠仍可作为规模化优化，但当前不是 correctness blocker。
