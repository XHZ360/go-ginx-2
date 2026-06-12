# 管理后台 UI 设计文档

本目录收录管理后台 V1 的逐页 UI 设计文档，覆盖登录、仪表盘、用户、客户端、代理、证书与审计页面。

## 文档列表

- `login.md`：登录页 UI 设计
- `dashboard.md`：仪表盘 UI 设计
- `users-list.md`：用户列表页 UI 设计
- `user-detail.md`：用户详情页 UI 设计
- `clients-list.md`：客户端列表页 UI 设计
- `client-detail.md`：客户端详情页 UI 设计
- `proxies-list.md`：代理列表页 UI 设计
- `proxy-detail.md`：代理详情页 UI 设计
- `certificates.md`：证书页 UI 设计
- `certificate-migration.md`：证书绑定迁移与兼容说明
- `audit.md`：审计页 UI 设计

## 统一约束

- 生产运行时由管理监听器同源托管前端资源；默认使用部署根目录下的 `admin-ui/` 构建产物，配置 `admin_frontend_dir` 后改用该目录，`/`、`/login` 以及各深链接页面由前端壳处理。
- `/api/admin/*` 始终保留为管理后台 API 命名空间，不与前端路由冲突。
- 所有受保护页面先通过 `GET /api/admin/session` 完成会话引导。
- 业务数据统一通过 `POST /api/admin/graphql` 获取或提交。
- 写操作必须带有效 CSRF Token。
- 页面状态统一区分加载、空数据、筛选无结果、资源不存在、接口失败、会话过期。
- 页面级轮询只影响当前页面，不触发全壳刷新。
