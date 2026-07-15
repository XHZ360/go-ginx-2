# 管理 CLI 示例

以下命令可在 Release 包根目录执行。CLI 默认使用部署根目录下的 `data/go-ginx.db`；如果二进制位于 `bin/`，部署根目录就是 `bin/` 的上一级。也可用 `-db` 指定数据库路径。

### TUI 模式

本地首次配置或小规模维护可以使用 TUI：

```bash
./bin/goginx-admin tui
```

TUI 是面向服务器终端的本地运维入口，使用与其他 `goginx-admin` 子命令相同的默认 SQLite 路径，也支持 `-db <path>` 和 `-actor <id>`。

首版范围聚焦管理员设置、用户管理和客户端配置：

- 创建管理员、更新管理员密码、启用或禁用管理员。
- 创建用户。
- 选择用户快速创建客户端和 join token。
- 仅创建或轮换客户端凭据。

TUI 优先使用列表、角色选项、默认 join 参数和确认页。删除用户或客户端时要求输入资源 ID 强确认；客户端凭据只在当前结果页显示明文，join token 可在客户端菜单中查看，若不可用则查看时自动重置。

脚本和自动化仍应继续使用非交互式子命令。TUI 需要交互式终端；在重定向、CI 或不支持终端控制的环境中会拒绝启动，并提示改用非交互式命令。

### 用户与客户端

创建普通用户和客户端凭据：

```bash
./bin/goginx-admin create-user -id user-1 -username alice
./bin/goginx-admin create-client -id client-1 -user user-1 -name home -credential secret
```

### 代理配置

创建 TCP/UDP/HTTP/HTTPS 代理：

```bash
./bin/goginx-admin create-tcp-proxy -id tcp-1 -user user-1 -client client-1 -name ssh -port 10022 -target-host 127.0.0.1 -target-port 22
./bin/goginx-admin create-udp-proxy -id udp-1 -user user-1 -client client-1 -name dns -port 10053 -target-host 127.0.0.1 -target-port 53
./bin/goginx-admin create-http-proxy -id web-1 -user user-1 -client client-1 -name web -host app.example.com -target-host 127.0.0.1 -target-port 8080
./bin/goginx-admin create-https-proxy -id secure-1 -user user-1 -client client-1 -name secure -host secure.example.com -target-host 127.0.0.1 -target-port 8443
```

四类代理都可以用 `-bind-host` 指定实际监听地址：

- TCP/UDP 的 `-port` 是入口端口。
- HTTP/HTTPS 的 `-host` 是 HTTP Host 或 HTTPS SNI 域名。
- HTTP/HTTPS 的 `-port` 可指定入口监听端口，留空时使用 `http_entry_listen` 或 `https_entry_listen` 的默认端口。

服务端会在代理创建、更新、启用、禁用或删除后热协调 listener：需要的新监听会立即启动，不再被任何启用代理使用的自定义监听会关闭。

### HTTPS 静态证书终止

```bash
./bin/goginx-admin create-https-proxy \
  -id secure-term-1 \
  -user user-1 \
  -client client-1 \
  -name secure-term \
  -host term.example.com \
  -target-host 127.0.0.1 \
  -target-port 8080 \
  -cert-file data/certs/term.crt \
  -key-file data/certs/term.key
```

HTTPS 代理按 TLS ClientHello 中的 SNI 路由到对应代理后，会先尝试选择可用于该 SNI 的证书材料：

| 证书状态 | 运行行为 |
| --- | --- |
| 代理配置了完整的 `cert_file` 和 `key_file` | 服务端使用这对静态证书终止公网 TLS，并把解密后的 HTTP 请求转发到客户端本地 HTTP 目标。 |
| 没有静态证书，但存在健康的 active 托管证书 | `serving_status` 为 `usable` 或 `expiring_soon` 时执行 TLS 终止；`expiring_soon` 仍可服务并进入续期候选。 |
| 没有完整静态证书，也没有可服务 active 托管证书 | HTTPS 代理标记为 `needs_config`，匹配 SNI 的公网 TLS 连接会被拒绝或关闭。 |
| 显式配置了静态证书但无效 | 如果证书/私钥不完整、文件不可读、已过期、域名不匹配或 key 不匹配，该连接会失败；运行时不会自动降级为 passthrough。 |

HTTPS proxy 不再提供 SNI passthrough 回退。需要纯 TLS/SNI 透传的部署应迁移到后续独立的透传代理能力；当前 `https` 类型始终表示服务端终止公网 TLS 并把解密后的 HTTP 请求转发到客户端本地 HTTP 目标。
