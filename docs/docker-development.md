# Docker 开发环境

本仓库提供一套面向本地开发的 Docker Compose 配置，用于在 Linux 容器中运行 Go 后端、Vite Admin UI、Go 测试和前端验证。

## 启动

先确保 Docker Desktop 或 Docker Engine 已启用 Compose v2，然后在仓库根目录执行：

```powershell
docker compose up --build
```

开发镜像默认会使用 `https://goproxy.cn,direct` 下载 Go module，并使用 `https://registry.npmmirror.com` 下载 pnpm 依赖；这是为了避免部分网络环境下 `proxy.golang.org` 或 npm 官方 registry 连接失败。需要改回官方源时可以执行：

```powershell
docker compose build --build-arg GOPROXY=https://proxy.golang.org,direct --build-arg NPM_CONFIG_REGISTRY=https://registry.npmjs.org
```

启动后访问：

| 地址 | 用途 |
| --- | --- |
| `http://localhost:5173` | Admin UI Vite 开发服务器 |
| `localhost:8081` | 客户端 enrollment 入口 |
| `localhost:8443/udp` | 控制通道 QUIC |
| `localhost:9443` | 控制通道 TCP+TLS |
| `localhost:18080` | HTTP 代理入口 |
| `localhost:18443` | HTTPS 代理入口 |

管理 API 的 `8080` 端口只在 Compose 网络命名空间内监听。`admin-ui` 服务与 `server` 服务共享网络命名空间，并把 Vite 代理指向 `127.0.0.1:8080`，这样后端会把管理请求视为本地开发请求，不会触发 `PROTECTED_TRANSPORT_REQUIRED`。

## 初始化管理员

服务端启动后，在另一个终端执行：

```powershell
docker compose exec server ./.tmp/docker/bin/goginx-admin init-admin -id admin-1 -username admin -password "change-me"
```

然后打开 `http://localhost:5173`，使用上面的用户名和密码登录。

## 常用开发命令

进入带 Go、Node.js 和 pnpm 的开发 shell：

```powershell
docker compose run --rm dev bash
```

运行后端完整验证：

```powershell
docker compose run --rm dev go test ./...
```

运行前端测试和构建：

```powershell
docker compose run --rm dev bash -lc "cd admin-ui && pnpm test"
docker compose run --rm dev bash -lc "cd admin-ui && pnpm build"
```

GraphQL schema 或 operation 改动后刷新前端生成文件：

```powershell
docker compose run --rm dev bash -lc "cd admin-ui && pnpm graphql:refresh"
```

运行管理 TUI：

```powershell
docker compose exec server ./.tmp/docker/bin/goginx-admin tui
```

## 数据与清理

Compose 会把运行时 SQLite、证书、JWT 密钥和日志保存在 Docker volume 中，不写入 Git：

| Volume | 容器路径 | 用途 |
| --- | --- | --- |
| `goginx-dev-data` | `/workspace/.tmp/docker/data` | SQLite、证书、JWT 密钥 |
| `goginx-dev-logs` | `/workspace/.tmp/docker/logs` | server/client 日志 |
| `go-mod-cache` | `/go/pkg/mod` | Go module cache |
| `go-build-cache` | `/root/.cache/go-build` | Go build cache |
| `pnpm-store` | `/pnpm/store` | pnpm store |
| `admin-ui-node-modules` | `/workspace/admin-ui/node_modules` | 前端依赖目录 |

停止容器但保留数据：

```powershell
docker compose down
```

彻底重置开发数据和依赖缓存：

```powershell
docker compose down -v
```

`docker compose down -v` 会删除管理员、客户端、证书和本地密钥等开发状态，只在确认不需要保留这些数据时使用。
