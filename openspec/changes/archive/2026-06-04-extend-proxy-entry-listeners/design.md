## Context

当前代理记录已有 `entry_host` 和 `entry_port`，但四种代理类型对这些字段的语义不一致：

- TCP/UDP 使用 `entry_port`，监听地址来自全局 `tcp_entry_host`。
- HTTP 使用全局 `http_entry_listen`，通过请求 `Host` 匹配代理记录的 `entry_host`。
- HTTPS 使用全局 `https_entry_listen`，通过 TLS ClientHello SNI 匹配代理记录的 `entry_host`。

这导致管理员无法对单个代理选择暴露网络或端口，也导致 HTTP/HTTPS 自定义端口只能通过全局配置实现。与此同时，当前运行时只在启动时创建 TCP/UDP listener，HTTP/HTTPS 也只有全局 listener，因此管理面配置变更不能可靠地即时反映到运行服务。

## Goals / Non-Goals

**Goals:**

- 为 TCP、UDP、HTTP、HTTPS 统一表达入口监听意图：监听地址、监听端口和路由域名。
- 管理面创建、更新、启用、禁用或删除代理后，运行时及时启动或关闭对应 listener 服务。
- 保持旧记录兼容：旧 HTTP/HTTPS 记录没有显式端口时继续使用全局默认端口，旧 TCP/UDP 记录没有显式监听地址时继续使用全局 `tcp_entry_host`。
- 让 UI 中的监听地址从后端提供选项选择，减少手动输入无效地址。
- 增强连接和 listener 生命周期日志，方便确认 HTTP/HTTPS proxy 是否正常提供服务。

**Non-Goals:**

- 不实现正向代理。
- 不实现多节点、负载均衡或远程分布式 listener 编排。
- 不实现完整日志查询、访问日志留存或告警系统。
- 不改变客户端控制通道协议中的本地目标转发语义。

## Decisions

### 1. 新增 `entryBindHost`，保留 `entryHost` 作为域名

代理领域模型新增 `EntryBindHost`，SQLite 新增 `entry_bind_host`。`EntryHost` 保持为 HTTP Host / HTTPS SNI 域名；TCP/UDP 可以不使用 `EntryHost`。

备选方案是把现有 `entry_host` 改为监听地址，再新增 `domain` 字段。这会破坏既有 HTTP/HTTPS 记录含义，也会让证书、Host/SNI 查询和 UI 展示迁移复杂化。因此选择新增监听地址字段。

### 2. 统一有效入口配置

运行时和管理服务使用一个归一化入口 key：

```text
network: tcp | udp
bind_host: 0.0.0.0 | 127.0.0.1 | :: | <local ip>
port: 1..65535
route_host: HTTP Host / HTTPS SNI，仅 HTTP/HTTPS 使用
```

默认规则：

- TCP/UDP：`entryBindHost` 为空时使用 `tcp_entry_host`；`entryPort` 必填。
- HTTP：`entryBindHost` 为空时使用 `http_entry_listen` 的 host；`entryPort` 为 0 时使用 `http_entry_listen` 的 port；`entryHost` 为路由域名且必填。
- HTTPS：`entryBindHost` 为空时使用 `https_entry_listen` 的 host；`entryPort` 为 0 时使用 `https_entry_listen` 的 port；`entryHost` 为 SNI 域名且必填。

### 3. 按有效监听地址分组运行 listener

daemon 持有 listener registry，而不是只持有单个 HTTP/HTTPS listener：

```text
tcpListeners   map[listenKey]*tcp.Listener
udpListeners   map[listenKey]*udp.Listener
httpServers    map[listenKey]*http.Server
httpsListeners map[listenKey]*https.Listener
```

同一个 HTTP/HTTPS `bind_host:port` 上的多个域名共享一个 server/listener，由请求 Host 或 SNI 在 listener 内查询对应代理。

### 4. 管理操作后执行 listener reconcile

admin command 服务在代理 create/update/enable/disable/delete 成功后通知运行时 listener reconciler。reconciler 重新计算所有启用代理的有效 listener key：

- 需要但尚未运行的 listener 立即启动。
- 正在运行但不再被任何启用代理使用的 listener 立即关闭。
- 仍被使用的 listener 保持运行，避免影响同地址上的其他代理。

如果新增 listener 启动失败，管理操作必须返回可消费错误，并避免留下“数据库已启用但 listener 未运行”的静默状态。实现上优先使用事务后置 reconcile 加补偿回滚，或在服务层先做准入与试绑定检查，再提交状态。

### 5. ListenerClaim 支持监听地址冲突

ListenerClaim 从 `network + port` 扩展为 `network + bind_host + port`。冲突规则：

- 同协议、同端口、同 bind host 冲突。
- wildcard host（`0.0.0.0`、`::`、空地址归一化后的 unspecified）与同协议同端口的任意具体地址冲突。
- IPv4 与 IPv6 按实际地址族分别判断，除非实现确认双栈监听会互相占用。

静态监听器、TCP/UDP 代理、HTTP/HTTPS 代理都进入 active claim 集合。

### 6. 路由查询携带监听上下文

HTTP listener 按 `domain + bind_host + port` 查询代理。HTTPS listener 按 `sni + bind_host + port` 查询代理。旧的 `ByHTTPHost` / `ByHTTPSHost` 可以保留兼容用途，但运行时应使用带监听上下文的新查询。

### 7. UI 通过后端选项选择监听地址

GraphQL 增加监听地址选项查询，例如 `proxyEntryOptions`。选项至少包括：

- 对应协议默认配置中的 host。
- `0.0.0.0`。
- `127.0.0.1`。
- 可发现的本机非回环 IPv4 地址。

UI 创建/编辑代理时：

- TCP/UDP 展示监听地址、入口端口、目标。
- HTTP/HTTPS 展示监听地址、入口端口、域名、目标。
- 监听地址使用选择器；域名和目标仍可手动输入。

### 8. 日志采用生命周期基线，不做完整访问日志

服务端记录：

- listener 启动和关闭，包含协议、监听地址、端口和引用代理数量。
- listener 启动失败和准入冲突。
- 客户端 session 认证成功、替换旧 session、正常断开、过期关闭。
- HTTP/HTTPS route miss、client offline、open stream failed 等关键失败。

客户端记录：

- 控制 session 建立。
- 正常关闭原因。
- 心跳或代理流循环失败后的重连原因。

不记录请求头、Cookie、令牌、私钥或请求体。

## Risks / Trade-offs

- **迁移旧唯一索引可能失败** -> 在迁移中先创建新列并回填默认值，再删除旧索引、创建复合索引；对冲突数据返回明确迁移错误。
- **管理操作和 listener reconcile 一致性复杂** -> 使用集中 listener manager 串行 reconcile，并让管理 mutation 在 reconcile 完成后返回。
- **wildcard 地址冲突规则容易误判** -> 把归一化与冲突判断放入 domain 层并用单元测试覆盖 IPv4、IPv6、空 host 和 wildcard。
- **HTTP/HTTPS 新 listener 增多会增加资源占用** -> 按监听地址分组复用 listener，并在无引用时及时关闭。
- **UI 选项可能遗漏容器/NAT 场景的地址** -> 后端提供默认选项和已发现地址；高级 JSON/CLI 仍可保留显式配置路径。

## Migration Plan

1. SQLite 新增 `entry_bind_host`，旧记录默认空值。
2. 更新服务层归一化逻辑，空 `entry_bind_host` 使用旧配置默认值，HTTP/HTTPS 空 `entry_port` 使用全局默认端口。
3. 删除旧的 TCP/UDP 仅端口唯一索引、HTTP/HTTPS 仅 host 唯一索引，创建新复合唯一索引。
4. 更新 daemon listener registry，并在启动时从所有启用代理构建 listener 集合。
5. 接入管理操作后的 reconcile。
6. 更新 UI、CLI、GraphQL 和文档。

回滚时，若新记录使用了非默认监听地址或 HTTP/HTTPS 自定义端口，旧版本无法完整表达这些配置；回滚前需要禁用或删除这些代理，或把它们迁回旧默认监听。

## Open Questions

- 是否允许 UI 中出现“自定义监听地址”自由输入作为高级选项，还是严格限制为后端发现的选项。
- IPv6 双栈监听在目标部署平台上的冲突规则是否需要按操作系统区分。
- 管理操作失败时是否必须补偿回滚数据库状态，还是允许返回失败并把代理置为 disabled / needs_config 状态。
