## MODIFIED Requirements

### Requirement: Same-origin frontend delivery baseline
系统 MUST 把专用管理员前端和管理员 API 以一个外部同源呈现给浏览器，并且 configless 基础部署 MUST 默认使用部署根目录中的 `admin-ui/` 前端构建目录，而不是要求配置 `admin_frontend_dir`、依赖进程工作目录或静默使用二进制内嵌前端资源。

#### Scenario: Default admin-ui directory is served
- **WHEN** 管理面启用、未配置 `admin_frontend_dir`，且服务端二进制所在部署根目录中的 `admin-ui/` 包含专用前端构建产物入口 `index.html`
- **THEN** admin listener 从该部署根目录的 `admin-ui/` 目录在同源上服务 `/`、`/login`、`/dashboard`、`/users`、`/clients`、`/proxies`、`/certificates`、`/audit` 和受支持深链，同时继续保留 `/api/admin/*` 作为后端 API 命名空间

#### Scenario: Configured frontend directory overrides default admin-ui directory
- **WHEN** `admin_frontend_dir` 显式指向包含 `index.html` 的专用前端构建目录
- **THEN** admin listener 使用该目录服务前端路由和资源，而不是使用默认 `admin-ui/` 目录

#### Scenario: Missing selected frontend fails clearly
- **WHEN** 管理面启用，且当前选定的前端目录缺失、不是目录或缺少 `index.html`
- **THEN** 系统启动失败或拒绝启用管理 listener，并返回明确的 admin frontend 目录错误，而不是继续服务旧的内嵌前端资源

#### Scenario: Embedded frontend assets are not the default fallback
- **WHEN** 未配置 `admin_frontend_dir` 且默认 `admin-ui/` 目录不可用，即使服务端二进制包含内嵌前端资源
- **THEN** 系统也不得静默回退到内嵌前端作为基础部署的浏览器管理面来源

#### Scenario: Missing asset-like paths return not found
- **WHEN** 浏览器请求当前选定前端目录中不存在的资源型路径，例如 `/assets/missing.js`
- **THEN** admin listener 返回 `404 Not Found`，而不是把缺失资源错误伪装成前端深链或回退到其他前端资源来源
