## Why

当前管理前端默认从服务端二进制内嵌资源加载，部署后即使替换或删除部署目录中的 `admin-ui/`，浏览器仍可能看到旧的管理页面。这不利于频繁更新 UI，也容易让操作者误判当前实际服务的前端版本。

## What Changes

- **BREAKING**：管理前端默认静态资源来源改为部署根目录下的 `admin-ui/` 目录，而不是内嵌资源或进程工作目录。
- `admin-ui/` 必须包含前端构建产物入口 `index.html`，并由 admin listener 同源服务浏览器路由和资产文件。
- `admin_frontend_dir` 保留为显式覆盖项，用于指定非默认的自定义前端目录。
- 服务端启动时如果启用了管理面但默认或配置的前端目录不可用，必须给出明确错误，避免悄悄回退到旧 UI。
- 部署包继续包含 `admin-ui/` 构建产物，使替换该目录即可更新管理前端。
- 内嵌前端资源不再作为基础部署的默认浏览器管理面来源；如保留，只能作为显式开发/测试备用路径。

## Capabilities

### New Capabilities

- 无。

### Modified Capabilities

- `admin-resource-management`：调整同源管理前端交付合同，使默认资源来源为部署目录 `admin-ui/`，并重新定义缺失前端目录和 `admin_frontend_dir` 覆盖行为。
- `deployment-operations`：调整部署包和 configless 启动验证合同，使 `admin-ui/` 构建产物成为默认部署布局中的运行时工件。

## Impact

- 影响 `internal/adminapi` 的前端资源加载顺序、缺失目录错误处理和同源静态路由。
- 影响 `internal/deploy` 的部署包配置与 `admin-ui/` 目录交付语义。
- 影响 `README.md`、`docs/daemon-runtime.md` 和相关部署文档中关于默认管理前端来源的说明。
- 需要更新 `internal/adminapi`、`internal/deploy`、`e2e` 或 daemon smoke tests 中对嵌入式默认前端的断言。
