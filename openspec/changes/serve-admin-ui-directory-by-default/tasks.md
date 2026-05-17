## 1. 管理前端加载行为

- [x] 1.1 调整 `internal/adminapi` 的前端加载逻辑，使 `admin_frontend_dir` 为空时默认解析部署根目录下的 `admin-ui/`
- [x] 1.2 保留 `admin_frontend_dir` 显式覆盖优先级，并确保默认路径不依赖当前进程工作目录
- [x] 1.3 删除或隔离默认嵌入式前端回退路径，确保缺失默认目录时不会服务旧的内嵌 UI
- [x] 1.4 为缺失目录、非目录和缺少 `index.html` 的情况返回可定位的 admin frontend 错误

## 2. 部署包与运行布局

- [x] 2.1 调整 `build-deploy-bundle`，从仓库 `admin-ui/dist` 复制构建产物到输出根目录 `admin-ui/`
- [x] 2.2 当前端构建产物缺失时让打包流程失败，并提示先构建管理前端
- [x] 2.3 确认生成的默认 server 配置继续保持 `admin_frontend_dir` 为空，由部署根目录 `admin-ui/` 承担默认资源来源
- [x] 2.4 检查 systemd 模板和二进制布局与默认 `admin-ui/` 解析路径一致

## 3. 测试覆盖

- [x] 3.1 更新 `internal/adminapi` 测试，覆盖默认 `admin-ui/` 目录服务浏览器路由和资源
- [x] 3.2 更新 `internal/adminapi` 测试，覆盖 `admin_frontend_dir` 覆盖默认目录
- [x] 3.3 增加缺失默认 `admin-ui/` 或缺少 `index.html` 时不回退内嵌资源的失败测试
- [x] 3.4 更新 `internal/deploy` 和 `cmd/goginx-admin` 打包测试，覆盖 `admin-ui/` 必须被复制以及缺失 dist 时打包失败
- [x] 3.5 更新外部进程 smoke/e2e 测试，证明无 `admin_frontend_dir` 配置时部署包通过默认 `admin-ui/` 服务管理前端

## 4. 文档与验证

- [x] 4.1 更新 README 和部署文档，把默认管理前端来源改为部署根目录 `admin-ui/`
- [x] 4.2 文档化 UI 更新流程：替换 `admin-ui/` 构建目录并重启 `goginx-server`
- [x] 4.3 文档化发布打包前需要先构建 `admin-ui/dist`
- [x] 4.4 运行相关 Go 测试和前端构建/测试，确认变更满足规格
