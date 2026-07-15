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

当前尚无独立决策文件；历史上下文见 [../changes/archive/milestone-one-continuation.md](../changes/archive/milestone-one-continuation.md) 与 [../references/](../references/)。
