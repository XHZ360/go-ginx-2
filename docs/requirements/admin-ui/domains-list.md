# Domain 列表页 UI 设计

## 1. 页面定位

Domain 列表是 Web 入口的主入口：先管理公网主机身份，再挂路径 Proxy 与证书。

## 2. 路由

- `/domains`

## 3. 页面目标

- 浏览 Domain 主机、所有者、状态、entry 数量、路径 Proxy 数量、证书绑定
- 创建 Domain（仅 host + owner）
- 启停 Domain
- 跳转 Domain 详情

## 4. 交互原则（友好 UX）

- 导航文案英文，安全提示可中文
- 创建成功后直接进入详情，引导添加 entry / 证书 / 路径
- 空状态说明“先 Domain，后路径 Proxy”
- 禁用 Domain 时明确会影响该主机的 HTTP/HTTPS listener
