# 设计决策

影响长期方向的选择一事一文。日常进度与临时讨论不要放在这里。

## 何时写入

- 改变产品边界或安全模型
- 选定不可轻易回退的存储/协议/部署方案
- 明确否决某条可选路径并需后人遵守

## 命名

- `YYYYMMDD-short-title.md`，或稳定主题名（如 `sqlite-cgo-free.md`）

## 模板

```markdown
# 标题

## 状态

已采纳 | 已替代 | 草案

## 背景

## 决策

## 后果

## 相关文档
```

## 当前决策

| 文档 | 状态 | 说明 |
| --- | --- | --- |
| [domain-path-proxy-routing.md](domain-path-proxy-routing.md) | 已采纳，待实现 | Domain 独立管理，HTTP/HTTPS 共享 Domain + Path 到 Proxy 路由 |
| [server-runtime-context-boundaries.md](server-runtime-context-boundaries.md) | 已采纳，已实现 | 管理、运行时、通信和持久化上下文的依赖与端口边界 |

历史上下文见 [../changes/archive/milestone-one-continuation.md](../changes/archive/milestone-one-continuation.md) 与 [../references/](../references/)。
