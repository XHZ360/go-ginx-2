## 1. 数据模型与持久化

- [x] 1.1 为 `domain.Proxy` 增加入口监听地址字段，并定义有效入口配置归一化辅助函数。
- [x] 1.2 为 SQLite `proxies` 表新增 `entry_bind_host` 迁移，旧记录默认保持空值以使用现有全局监听 fallback。
- [x] 1.3 替换旧的 TCP/UDP 仅端口唯一索引和 HTTP/HTTPS 仅域名唯一索引，新增按类型、监听地址、入口端口和域名适配的新唯一索引。
- [x] 1.4 更新 proxy repository 的扫描、创建、更新、列表和查询逻辑，使新字段完整读写。
- [x] 1.5 增加按监听上下文查询 HTTP/HTTPS 代理的 repository 方法，并保留必要的旧查询兼容路径。

## 2. 准入与服务层

- [x] 2.1 将 ListenerClaim 扩展为包含协议、监听地址和端口，并实现 wildcard 地址与具体地址的冲突规则。
- [x] 2.2 更新静态 listener claim 生成逻辑，使配置中的 admin、enrollment、control、默认 HTTP/HTTPS listener 都携带监听地址。
- [x] 2.3 更新代理 create/update/enable 校验，按类型校验监听地址、入口端口、HTTP Host / HTTPS SNI 域名和目标字段。
- [x] 2.4 更新代理准入检查，使 TCP、UDP、HTTP、HTTPS 已启用代理都参与活跃监听冲突检测。
- [x] 2.5 为入口冲突、无效监听地址和 listener 启动失败返回前端可消费的结构化错误。

## 3. 运行时 Listener 协调

- [x] 3.1 为 daemon 增加 listener registry，按协议和有效监听地址管理 TCP、UDP、HTTP、HTTPS listener。
- [x] 3.2 在服务端启动时从已启用代理集合构建并启动所需 listener，保持旧默认 HTTP/HTTPS 代理兼容。
- [x] 3.3 实现 listener reconcile：根据已启用代理重新计算所需 listener，启动新增 listener，关闭无人使用的 listener，保留共享 listener。
- [x] 3.4 将代理 create/update/enable/disable/delete 成功路径接入 listener reconcile，并确保管理操作返回前完成协调。
- [x] 3.5 处理 reconcile 失败的一致性策略，避免数据库显示启用但入口 listener 未运行的静默状态。
- [x] 3.6 更新 HTTP 和 HTTPS listener 路由，使其按当前 listener 地址、端口和 Host/SNI 匹配代理。

## 4. 管理 API、CLI 与 UI

- [x] 4.1 更新 GraphQL 代理配置类型、create/update mutation 和查询结果，暴露监听地址、入口端口、域名和证书文件字段。
- [x] 4.2 增加监听地址选项查询，返回默认监听地址、wildcard、loopback 和可发现的本机地址。
- [x] 4.3 更新 admin CLI proxy 创建参数说明和输入映射，使 `-host` / `-port` 对四种代理类型表达新入口语义。
- [x] 4.4 更新代理创建 UI，按类型展示监听地址选择器、入口端口、域名、目标和 HTTPS 证书字段。
- [x] 4.5 更新代理详情编辑 UI，按类型校验和提交入口字段，避免隐藏字段残留或清空必填字段。
- [x] 4.6 修复代理启用、禁用、更新和删除失败反馈，使冲突和监听启动失败显示在当前页面或表单中。
- [x] 4.7 更新代理列表和详情展示，将监听地址、入口端口、域名和目标分开展示。

## 5. 日志与诊断

- [x] 5.1 服务端记录代理 listener 启动、关闭、启动失败和引用代理数量。
- [x] 5.2 服务端记录客户端 session 认证成功、替换旧 session、正常断开和心跳过期关闭。
- [x] 5.3 客户端记录控制 session 正常关闭原因，并保留现有建立、失败和重连日志。
- [x] 5.4 HTTP/HTTPS 入口记录 route miss、client offline 和 open stream failed 等关键失败，避免记录敏感请求内容。
- [x] 5.5 更新服务端启动摘要，展示动态 TCP、UDP、HTTP 和 HTTPS proxy listener 数量。

## 6. 文档与验证

- [x] 6.1 更新 README 和 daemon runtime 文档，说明每代理监听地址、入口端口、域名和默认 fallback 语义。
- [x] 6.2 更新管理 UI 文档，说明监听地址选项、HTTP/HTTPS 端口、域名和 HTTPS 证书字段。
- [x] 6.3 增加 domain/store/admin 服务层单元测试，覆盖新字段、迁移、唯一索引和 ListenerClaim 冲突规则。
- [x] 6.4 增加 daemon/proxy 测试，覆盖 HTTP/HTTPS 多 listener、共享 listener 保留和禁用/删除后的关闭行为。
- [x] 6.5 增加 admin API/UI 测试，覆盖表单提交、字段校验、监听地址选项和错误反馈。
- [x] 6.6 增加外部进程或集成测试，验证 HTTP/HTTPS 自定义端口和 TCP/UDP 自定义监听地址无需服务端重启即可生效。
