# Domain 详情页 UI 设计

## 1. 页面定位

Domain 详情集中管理：

- 主机名
- HTTP/HTTPS entries（bind/port）
- 证书绑定（1:n，一证可服务多 Domain）
- 下属路径 Proxy 列表与创建入口

## 2. 路由

- `/domains/:id`

## 3. 区块

### Overview

- 状态、所有者、路径数量、更新时间

### Certificate

- 已绑定：展示 certificateId、serving、hostnames；支持解绑
- 未绑定：证书选择器按 Domain host 过滤，可绑定

### Listeners

- 表格：protocol / bind / port / status
- 添加 HTTP 或 HTTPS entry
- HTTPS entry 要求已绑定证书

### Path proxies

- 表格：path / name / client / target / status
- “Add path proxy” 弹窗：name、client、pathPrefix、rewrite、target
- 行链接到 Proxy 详情

## 4. 友好 UX

- 引导顺序：证书（若需 HTTPS）→ entry → 路径
- 路径冲突、证书不覆盖 host、跨用户 Domain 返回字段级错误
- 删除 Domain 前要求 disabled 且无 enabled proxy
