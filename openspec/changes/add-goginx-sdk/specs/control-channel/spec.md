## ADDED Requirements

### Requirement: Consumer control session role
系统 MUST 在控制通道认证后区分 provider 和 consumer 客户端会话。provider 会话 MUST 保持现有代理快照和服务端发起代理子流行为；consumer 会话 MUST NOT 替换同一用户下 provider client 的最新会话，也 MUST NOT 被用于承载 provider 代理子流。

#### Scenario: Provider session behavior remains unchanged
- **WHEN** provider client 使用有效凭据连接控制通道
- **THEN** 服务端按现有 provider 语义注册该 client 的最新会话
- **AND** 服务端向该连接发送按 client 作用域过滤的代理快照

#### Scenario: Consumer session does not replace provider
- **WHEN** 同一 user 下已有 provider client 在线，且另一个 consumer client 使用有效凭据连接控制通道
- **THEN** 服务端注册 consumer 会话
- **AND** provider client 的最新会话仍可被其 proxy 的桥接流选中

#### Scenario: Consumer is not selected as provider
- **WHEN** 服务端需要为某个 proxy 打开 provider 代理子流
- **THEN** 服务端 MUST 只选择该 proxy 所属 provider client 的活跃 provider 会话
- **AND** 服务端 MUST NOT 把 consumer 会话作为 provider 代理子流目标

### Requirement: Consumer proxy list delivery
服务端 MUST 为 consumer 控制通道提供按 user 作用域的代理列表消息。该列表 MUST 只包含 consumer 所属 user 名下当前已启用的 proxy，并且 MUST NOT 复用 provider 的按 client 作用域 `ProxySnapshot` 语义。

#### Scenario: Consumer receives enabled user proxies after authentication
- **WHEN** consumer client 使用有效凭据完成控制通道认证
- **THEN** 服务端发送 consumer proxy list response
- **AND** response 包含该 consumer 所属 user 名下所有已启用 proxy

#### Scenario: Disabled proxies are excluded
- **WHEN** consumer client 完成认证，且同一 user 下存在已禁用 proxy
- **THEN** consumer proxy list response MUST NOT 包含已禁用 proxy

#### Scenario: Other users proxies are excluded
- **WHEN** consumer client 完成认证，且系统中存在其他 user 的已启用 proxy
- **THEN** consumer proxy list response MUST NOT 包含其他 user 的 proxy

#### Scenario: Consumer can request refreshed proxy list
- **WHEN** 已认证 consumer 通过控制通道发送 proxy list request
- **THEN** 服务端返回当前 user 作用域的 proxy list response

### Requirement: Consumer initiated proxy stream bridge
consumer MUST 能够在已认证控制通道上打开多路复用数据流，请求连接某个已启用 proxy。服务端 MUST 校验 proxy 所属 user、启用状态和 provider 会话后，把该流桥接到 proxy 所属 provider client。

#### Scenario: Consumer stream bridges to provider target
- **WHEN** 已认证 consumer 打开数据流并发送包含有效 proxy ID 的 open-stream 请求
- **THEN** 服务端查找该 proxy
- **AND** 服务端校验该 proxy 属于 consumer 所属 user 且处于启用状态
- **AND** 服务端在 proxy 所属 provider client 的活跃 provider 会话上打开代理子流
- **AND** 服务端把 consumer 数据流与 provider 代理子流双向桥接

#### Scenario: Unknown proxy is rejected
- **WHEN** 已认证 consumer 请求打开不存在的 proxy ID
- **THEN** 服务端 MUST 拒绝或关闭该 consumer 数据流
- **AND** 服务端 MUST NOT 向任何 provider 会话打开代理子流

#### Scenario: Unauthorized proxy is rejected
- **WHEN** 已认证 consumer 请求打开其他 user 拥有的 proxy ID
- **THEN** 服务端 MUST 拒绝或关闭该 consumer 数据流
- **AND** 服务端 MUST NOT 向任何 provider 会话打开代理子流

#### Scenario: Disabled proxy is rejected
- **WHEN** 已认证 consumer 请求打开同 user 下已禁用的 proxy ID
- **THEN** 服务端 MUST 拒绝或关闭该 consumer 数据流
- **AND** 服务端 MUST NOT 向任何 provider 会话打开代理子流

#### Scenario: Offline provider is rejected
- **WHEN** 已认证 consumer 请求打开有效 proxy，但该 proxy 所属 provider client 没有活跃 provider 会话
- **THEN** 服务端 MUST 拒绝或关闭该 consumer 数据流
- **AND** 服务端 MUST NOT 把该流桥接到 consumer 自身或其他 client

### Requirement: Server-owned target injection for consumer streams
服务端 MUST 使用持久化 proxy 配置决定转发给 provider 的目标地址和流类型。consumer open-stream 请求中的 kind、host 或 port 不得覆盖服务端保存的 proxy target。

#### Scenario: Provider open stream uses proxy target
- **WHEN** consumer 请求打开某个有效 proxy
- **THEN** 服务端发送给 provider 的 open-stream 消息使用该 proxy 的 target host 和 target port
- **AND** provider 流类型由该 proxy 的类型派生

#### Scenario: Consumer supplied target is ignored
- **WHEN** consumer open-stream 请求携带与 proxy 配置不一致的目标地址或流类型
- **THEN** 服务端 MUST NOT 使用 consumer 提供的目标地址或流类型作为 provider 连接目标

### Requirement: Consumer stream accept support
控制通道多路复用实现 MUST 支持服务端从 consumer 控制连接接受由 consumer 主动打开的数据流。QUIC 和 TCP+TLS 回退路径均 MUST 支持该能力。

#### Scenario: QUIC consumer stream is accepted
- **WHEN** consumer 通过 QUIC 控制通道认证后打开新的数据流
- **THEN** 服务端从同一 QUIC 连接接受该数据流并按 consumer stream bridge 规则处理

#### Scenario: TCP TLS consumer stream is accepted after mux start
- **WHEN** consumer 通过 TCP+TLS 控制通道认证后打开新的 mux 数据流
- **THEN** 服务端在 mux read loop 启动后接受该数据流并按 consumer stream bridge 规则处理

### Requirement: Provider open timeout for bridged streams
服务端 MUST 为 consumer 桥接路径中打开 provider 子流设置有限超时。超时后服务端 MUST 释放 consumer 流处理资源并拒绝该次桥接。

#### Scenario: Provider open succeeds before timeout
- **WHEN** consumer 请求打开有效 proxy，且 provider 会话在超时前成功打开子流
- **THEN** 服务端继续桥接 consumer 流和 provider 子流

#### Scenario: Provider open timeout rejects stream
- **WHEN** consumer 请求打开有效 proxy，但 provider 会话未能在超时时间内打开子流
- **THEN** 服务端关闭或重置 consumer 数据流
- **AND** 服务端释放该次桥接占用的处理资源
