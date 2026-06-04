## ADDED Requirements

### Requirement: Connection and listener lifecycle log baseline
系统 MUST 在当前本地日志能力范围内记录关键客户端连接生命周期和代理 listener 生命周期事件，同时避免记录敏感数据。

#### Scenario: Server logs client session lifecycle
- **WHEN** 客户端控制会话认证成功、替换旧会话、正常断开或因心跳超时过期
- **THEN** 服务端日志记录客户端 ID、会话 ID、协议和事件结果，不记录客户端凭据或令牌

#### Scenario: Client logs control session lifecycle
- **WHEN** 客户端控制会话建立、正常关闭、认证永久失败或因心跳/代理流错误进入重连
- **THEN** 客户端日志记录客户端 ID、协议、会话 ID 或错误摘要，不记录凭据或令牌

#### Scenario: Server logs proxy listener lifecycle
- **WHEN** 服务端启动或关闭 TCP、UDP、HTTP 或 HTTPS proxy listener
- **THEN** 服务端日志记录协议、监听地址、端口和相关代理数量，使操作者能够确认监听服务是否已按有效配置运行

#### Scenario: Server logs HTTP and HTTPS routing failures
- **WHEN** HTTP Host 或 HTTPS SNI 没有匹配已启用代理、匹配代理的客户端离线，或打开代理流失败
- **THEN** 服务端日志记录代理类型、监听地址、域名和错误类别，不记录请求头、Cookie、请求体、证书私钥或其他敏感数据
