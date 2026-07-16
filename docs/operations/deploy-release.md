# Release 部署包部署

Linux `systemd` Release 包核心内容：

| 路径 | 内容 |
| --- | --- |
| `bin/` | `goginx-server`、`goginx-client`、`goginx-admin` |
| `admin-ui/` | 管理前端构建产物，默认由管理监听器同源服务 |
| `config/` | `server.example.json`、`client.example.json`、示例环境文件和可选凭据示例 |
| `data/` | SQLite、证书目录与 `admin-jwt.key` 管理 JWT 签名密钥 |
| `logs/` | 运行日志目录，默认包含 `server.log` 和 `client.log`，归档日志由应用内轮换机制管理 |
| `systemd/` | 渲染后的 `goginx-server.service` 和 `goginx-client.service` |

### 服务器部署流程

下面示例按远程客户端部署编写，请把 `control.example.com` 替换为客户端可访问的域名或 IP。这个值必须在 `goginx-server` 启动前写入服务环境文件，后续通过管理 UI 生成的 token 才会带上正确地址。

```bash
sudo mkdir -p /opt/go-ginx
sudo tar -xzf <release-bundle>.tar.gz -C /opt/go-ginx --strip-components=1
cd /opt/go-ginx
# 若需要 ACME、Origin CA 或自定义监听配置，先编辑 config/server.json；
# systemd 的无参数 server 启动会自动读取该文件。
printf 'GOGINX_JOIN_SERVICE_HOST=control.example.com\n' | sudo tee config/goginx-server.env >/dev/null
sudo cp systemd/goginx-server.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now goginx-server
sudo ./bin/goginx-admin init-admin -id admin-1 -username admin -password "<password>"
# 也可以在管理 UI 的 Clients 页面点击 Create join token 生成。
token="$(sudo env GOGINX_JOIN_SERVICE_HOST=control.example.com ./bin/goginx-admin create-client-join -id client-1 -user admin-1 -name home)"
```

如果只做本机试用，可以不创建 `config/goginx-server.env`，但生成的 token 通常不能直接给远程客户端使用。

`config/goginx-server.env` 仅承载运行进程环境变量，例如 `CF_DNS_API_TOKEN` 和 `GOGINX_JOIN_SERVICE_HOST`；`acme_enabled`、`acme_account_email`、`acme_terms_accepted` 等服务配置应写入 `config/server.json`。修改任一文件后执行 `sudo systemctl restart goginx-server`；仅在修改 unit 文件时才需要额外执行 `sudo systemctl daemon-reload`。

如果 enrollment 入口经过 HTTPS 反向代理，或控制通道端口不是默认值，请用显式地址生成 token：

```bash
token="$(sudo ./bin/goginx-admin create-client-join \
  -id client-1 \
  -user admin-1 \
  -name home \
  -enrollment-url "https://join.example.com/api/client/enroll" \
  -server-address "control.example.com:8443" \
  -server-tls-address "control.example.com:9443" \
  -server-name "go-ginx-control.local"
)"
```

### 客户端部署流程

```bash
sudo mkdir -p /opt/go-ginx
sudo tar -xzf <release-bundle>.tar.gz -C /opt/go-ginx --strip-components=1
cd /opt/go-ginx
sudo ./bin/goginx-client join "$token"
test -f data/client-state.json
test -f config/client.json
sudo cp systemd/goginx-client.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now goginx-client
```

`goginx-client` 服务默认读取部署根目录下的 `data/client-state.json`，同时 `./bin/goginx-client join <token>` 也会更新 `config/client.json` 供显式 `-config` 启动使用。

若先启动服务，会看到 `load client config: ... data/client-state.json ... cannot find the path specified`；处理方式是在客户端机器上先执行 join，然后再启动服务。若使用自定义路径，启动和 join 都需要显式传入对应 `-config`、`-state` 或 `-ca-file`。

如果不是 `systemd` 环境，也可以直接在 Release 包根目录运行 `./bin/goginx-server` 或 `./bin/goginx-client`，并由外部进程管理器负责守护进程生命周期。

### Windows 服务部署流程

Windows 发布包支持原生 Windows Service，不需要 WinSW 或 NSSM。以下示例假设发布包解压到 `C:\go-ginx`，并在管理员 PowerShell 中执行。

远程客户端部署时，推荐先写 `config/server.json` 并配置客户端可访问的 `join_service_host`。不要只依赖当前 PowerShell 中的临时环境变量，因为 Windows 服务运行环境不会稳定继承它们：

```powershell
cd C:\go-ginx
Copy-Item .\config\server.example.json .\config\server.json
# 编辑 config\server.json，把 join_service_host 设置为 control.example.com
```

安装并启动服务端服务：

```powershell
.\scripts\goginx-server-service.ps1 -Action install -Config config\server.json
.\scripts\goginx-server-service.ps1 -Action start
.\scripts\goginx-server-service.ps1 -Action status
```

也可以不用脚本，直接调用内置命令：

```powershell
.\bin\goginx-server.exe service install -config config\server.json
.\bin\goginx-server.exe service start
.\bin\goginx-server.exe service status
```

初始化管理员并生成客户端 join token 时，建议使用同一个 server 配置：

```powershell
.\bin\goginx-admin.exe init-admin -id admin-1 -username admin -password "<password>"
$token = .\bin\goginx-admin.exe create-client-join -server-config config\server.json -id client-1 -user admin-1 -name home
```

如果 enrollment 入口经过 HTTPS 反向代理，或控制通道端口不是默认值，请改用显式地址参数生成 token。

客户端机器上先执行 join，确认受管状态已写入，再安装服务：

```powershell
cd C:\go-ginx
.\bin\goginx-client.exe join "$token"
Test-Path .\data\client-state.json
.\scripts\goginx-client-service.ps1 -Action install
.\scripts\goginx-client-service.ps1 -Action start
.\scripts\goginx-client-service.ps1 -Action status
```

对应的内置命令是：

```powershell
.\bin\goginx-client.exe service install
.\bin\goginx-client.exe service start
.\bin\goginx-client.exe service status
```

常用服务操作：

```powershell
.\scripts\goginx-server-service.ps1 -Action restart
.\scripts\goginx-server-service.ps1 -Action stop
.\scripts\goginx-server-service.ps1 -Action uninstall
```

Windows 服务日志仍写入部署根目录下的 `logs/server.log` 和 `logs/client.log`，首版不注册 Windows Event Log source。首版安装命令也不提供自定义服务账户参数；如需自定义账户，可在服务安装后使用 Windows 原生服务管理工具调整。

升级 Windows 发布包时，先停止服务，替换 `bin/`、`admin-ui/`、`scripts/` 和示例配置等发布文件，保留 `data/`、已有 `config/server.json`、`config/client.json` 和证书/JWT 状态，然后再启动服务。回滚也按同样流程处理。

### 日志轮换与平台处理

默认日志轮换配置为单个当前文件 50 MiB、最多保留 10 个归档、归档保留 7 天并启用 gzip 压缩。Linux `systemd` 部署中，进程仍会写 stderr，journald 可以捕获服务日志；同时 `logs/server.log` 和 `logs/client.log` 由应用内轮换保护，通常不需要额外 `logrotate` 去重命名打开中的文件。Windows 部署默认依赖应用内轮换，因为外部 rename 型日志轮换工具通常不能稳定处理进程打开中的文件。Docker 或 Kubernetes 部署更推荐依赖 stdout/stderr 和容器运行时日志轮换，文件日志可作为显式部署选择或排障辅助。

如果磁盘占用异常增长，先检查 `log_max_size_mb`、`log_max_backups`、`log_retention_days`、`log_compress`，再确认服务账号有权限写入和删除部署根目录下的 `logs/` 归档文件。

### 8080 返回 404 的排查

Release 包的推荐启动方式是直接运行 `./bin/goginx-server`，或使用包内 `systemd/goginx-server.service`。这条路径会启用管理监听器，并从部署根目录下的 `admin-ui/` 提供 `/`、`/login`、`/dashboard` 等页面；该默认前端路径按二进制所在位置推导，不依赖启动时的当前工作目录。

如果访问 `8080` 返回 `404`，先检查实际访问到的是不是管理监听器：

```bash
curl -i http://127.0.0.1:8080/
curl -i http://127.0.0.1:8080/api/admin/session
```

正常情况下，第一个请求应返回管理前端 HTML，第二个请求应返回 JSON 会话状态。

常见原因和处理方式：

| 现象 | 处理 |
| --- | --- |
| 服务启动失败并提示 admin frontend 目录错误 | 确认 Release 根目录包含 `admin-ui/index.html`，或显式设置 `admin_frontend_dir`。 |
| 服务日志显示 `admin=disabled` | 说明不是按无配置 Release 路径启动，或显式配置关闭了 `admin_enabled`。 |
| 使用 `-config config/server.json` 后异常 | 确认 `admin_enabled` 为 `true`，并且控制通道证书文件已经存在。首次部署更推荐先使用无配置启动，让服务自动生成 `data/` 状态、控制通道 TLS 材料和 `data/admin-jwt.key`。 |
| 服务启动失败并指向 `admin_jwt_secret_file` | 检查 `data/admin-jwt.key` 是否存在、可由服务账号读取、内容未损坏且未被错误替换。恢复备份中的原文件可以保留未过期管理员 JWT；删除或轮换该文件会要求管理员重新登录。 |

更新管理前端时，重新构建 `admin-ui/dist`，把构建产物同步到 Release 根目录的 `admin-ui/`，然后重启 `goginx-server`。服务端不会热加载前端文件。
