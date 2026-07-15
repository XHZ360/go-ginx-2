# 运行时配置

### 托管状态文件

服务端默认写入：

| 类型 | 路径 |
| --- | --- |
| SQLite 数据库 | `data/go-ginx.db` |
| 控制通道 CA | `data/certs/control-ca.crt` |
| 控制通道证书 | `data/certs/control.crt` |
| 控制通道私钥 | `data/certs/control.key` |
| 托管 HTTPS 证书 | `data/certs/managed/<host>/` |
| Cloudflare provider credential secrets | `data/secrets/provider-credentials/` |
| 管理 JWT 签名密钥 | `data/admin-jwt.key` |

客户端默认写入：

| 类型 | 路径 |
| --- | --- |
| 客户端托管状态 | `data/client-state.json` |
| 客户端显式配置 | `config/client.json` |
| 服务端 CA 信任文件 | `data/certs/server-ca.crt` |

运行日志默认写入：

| 类型 | 路径 |
| --- | --- |
| 服务端日志 | `logs/server.log` |
| 客户端日志 | `logs/client.log` |

server/client 默认启用应用内日志轮换，当前写入文件保持上述固定名称；达到 `log_max_size_mb` 后会归档为 `server-YYYYMMDD-HHMMSS.log` 或 `client-YYYYMMDD-HHMMSS.log`，并按 `log_retention_days` 和 `log_max_backups` 清理旧归档。错误日志不拆分为单独文件，而是在同一日志流中保留错误级别、错误类别和上下文；凭据、令牌、Cookie、私钥和请求体不应写入日志。

这些是应用生成的运行时状态，不需要手写。

### 环境变量覆盖

无配置启动仍可通过环境变量调整端口和路径：

| 分类 | 环境变量 |
| --- | --- |
| 管理监听器 | `GOGINX_ADMIN_LISTEN` |
| 管理 JWT | `GOGINX_ADMIN_JWT_SECRET_FILE` |
| 客户端加入 | `GOGINX_CLIENT_ENROLLMENT_LISTEN`、`GOGINX_JOIN_SERVICE_HOST` |
| 控制通道 | `GOGINX_CONTROL_QUIC_LISTEN`、`GOGINX_CONTROL_TLS_LISTEN`、`GOGINX_CONTROL_TLS_SERVER_NAME` |
| 控制通道 TLS 文件 | `GOGINX_CONTROL_TLS_CA_FILE`、`GOGINX_CONTROL_TLS_CERT_FILE`、`GOGINX_CONTROL_TLS_KEY_FILE` |
| 代理入口 | `GOGINX_TCP_ENTRY_HOST`、`GOGINX_HTTP_ENTRY_LISTEN`、`GOGINX_HTTPS_ENTRY_LISTEN` |
| 数据和证书目录 | `GOGINX_SQLITE_PATH`、`GOGINX_DATA_DIR`、`GOGINX_CERTIFICATE_DIR` |

### 显式服务端配置

高级或脚本化部署仍可使用 JSON 配置：

```json
{
  "admin_enabled": true,
  "admin_listen": "127.0.0.1:8080",
  "admin_frontend_dir": "",
  "admin_jwt_secret_file": "data/admin-jwt.key",
  "client_enrollment_listen": "0.0.0.0:8081",
  "control_quic_listen": "127.0.0.1:8443",
  "control_tls_listen": "127.0.0.1:9443",
  "control_tls_server_name": "go-ginx-control.local",
  "control_tls_ca_file": "data/certs/control-ca.crt",
  "control_tls_cert_file": "data/certs/control.crt",
  "control_tls_key_file": "data/certs/control.key",
  "join_service_host": "control.example.com",
  "tcp_entry_host": "0.0.0.0",
  "http_entry_listen": "0.0.0.0:80",
  "https_entry_listen": "0.0.0.0:443",
  "sqlite_path": "data/go-ginx.db",
  "data_dir": "data",
  "certificate_dir": "data/certs",
  "origin_ca_enabled": true,
  "origin_ca_secret_store_path": "data/secrets/provider-credentials",
  "origin_ca_default_request_type": "origin-ecc",
  "origin_ca_requested_validity": 5475,
  "origin_ca_rotation_window": 2592000000000000,
  "heartbeat_timeout": 45000000000,
  "log_max_size_mb": 50,
  "log_max_backups": 10,
  "log_retention_days": 7,
  "log_compress": true
}
```

启动：

```bash
./bin/goginx-server -config server.json
```

### 显式客户端配置

推荐使用 `./bin/goginx-client join <token>`。需要手写配置时可使用：

```json
{
  "server_address": "127.0.0.1:8443",
  "server_tls_address": "127.0.0.1:9443",
  "server_name": "go-ginx-control.local",
  "server_ca_file": "data/certs/server-ca.crt",
  "client_id": "client-1",
  "credential": "secret",
  "allowed_protocols": ["quic", "tcp_tls"],
  "reconnect": {
    "initial_delay": 1000000000,
    "max_delay": 30000000000
  },
  "log_max_size_mb": 50,
  "log_max_backups": 10,
  "log_retention_days": 7,
  "log_compress": true
}
```

启动：

```bash
./bin/goginx-client -config client.json
```

`time.Duration` 字段在 JSON 中使用纳秒数。认证失败会立即退出；临时拨号失败或运行时故障会按 `reconnect` 退避重试。

Release 包包含 `config/client.example.json` 作为显式配置参考；`./bin/goginx-client join <token>` 会写入实际的 `config/client.json` 和客户端信任文件 `data/certs/server-ca.crt`。

如果跳过 join 而手写 `client.json`，需要把服务端的 `data/certs/control-ca.crt` 分发到客户端并保存为 `data/certs/server-ca.crt`。
