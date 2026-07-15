# 客户端接入与授权

## 角色

| 角色 | 职责 |
| --- | --- |
| Provider 客户端 | 连接本地 target，承接服务端打开的代理子流 |
| Consumer 客户端 / SDK | 主动打开用户已授权的固定代理流 |

同一 client 的最新有效会话优先；consumer 不替换同用户 provider 会话。

## 加入（Join）

1. 管理员创建一次性 join token（可查看/重置）。
2. 客户端调用专用 enrollment 地址 `POST /api/client/enroll` 兑换。
3. 兑换结果写入客户端受管状态与服务端 CA 信任文件。
4. 之后可无手写配置启动；token 过期、已消费、撤销或篡改后不可重用。

远程客户端必须使用可访问的 `join_service_host` / enrollment URL；本地兜底地址不得当作公网默认值。

## 认证与会话

- 控制通道：QUIC 优先，TCP+TLS 兜底。
- 先校验服务端证书链与 server name，再用 client ID + credential 认证。
- 不得提供跳过 TLS 校验的回退路径。
- Provider 接收代理快照并心跳；Consumer 接收已启用代理列表。

## 授权边界

- 代理必须属于正确用户且处于启用状态。
- Provider 不在线、代理禁用、未知代理或用户不匹配时拒绝桥接。
- Consumer SDK 只能选择已授权 proxy ID；本地 SOCKS5/CONNECT 目标地址不覆盖服务端 target。
- 日志与错误不得包含 credential、完整 token 或私钥。

## 验收口径

- 正确 token 可 join 并建立控制会话；错误凭据拒绝且不注册会话。
- Provider 收到自身代理快照；代理流量可达。
- Consumer/SDK 只能访问已授权启用代理。
- 临时控制面故障可按配置退避重连；认证拒绝立即停止重试。

## 相关文档

- 协议与 SDK：[../architecture/control-channel.md](../architecture/control-channel.md)
- 部署与 join 操作：[../operations/daemon-runtime.md](../operations/daemon-runtime.md)
- UI：客户端列表/详情见 [admin-ui/](admin-ui/README.md)
