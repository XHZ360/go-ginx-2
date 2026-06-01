## Context

当前 `admin_frontend_dir` 为空时，管理监听器从服务端二进制内嵌的 `embedded_admin` 资源服务前端。部署包也可能包含 `admin-ui/`，但默认配置不会使用它，因此操作者替换部署目录中的 `admin-ui/` 后，实际页面仍可能来自旧二进制。

这次变更把默认运行时前端来源改为部署根目录中的 `admin-ui/`。在受支持的部署包中，服务端二进制位于 `<install-root>/bin/`，因此部署根目录是 `bin/` 的上一级；替换 `/opt/go-ginx/admin-ui/` 并重启服务即可更新管理 UI，而不依赖进程当前工作目录。

## Goals / Non-Goals

**Goals:**

- 让 configless 管理面默认从部署目录 `admin-ui/` 加载前端构建产物。
- 保留 `admin_frontend_dir` 作为显式覆盖，优先级高于默认 `admin-ui/`。
- 缺失或无效前端目录必须产生明确错误，避免继续服务二进制中的旧 UI。
- 部署包必须交付 `admin-ui/` 构建产物，并验证无额外配置时可以服务管理页面。
- 文档明确 UI 更新方式：替换 `admin-ui/` 构建目录并重启服务。

**Non-Goals:**

- 不改变 `/api/admin/*`、会话、GraphQL 或管理资源合同。
- 不引入运行时热加载、文件监听或无需重启的 UI 更新机制。
- 不要求 `goginx-server` 自动执行 `npm install` 或 `npm run build`。
- 不扩大前端功能范围。

## Decisions

1. 默认目录使用部署根目录下的 `admin-ui`

   `admin_frontend_dir` 为空时，服务端根据当前可执行文件路径推导部署根目录：若二进制位于 `bin/` 目录内，则使用 `bin/` 的父目录；否则使用二进制所在目录。随后加载 `<deployment-root>/admin-ui/`。备选方案是继续依赖进程工作目录、继续嵌入前端或默认使用 `admin-ui/dist`；依赖工作目录会让服务管理器或手动启动方式改变资源来源，嵌入前端无法满足部署后替换 UI，`admin-ui/dist` 会把源码仓库结构泄漏到安装布局。

2. 显式配置优先于默认目录

   如果 `admin_frontend_dir` 非空，继续使用该目录。这样保留开发、自定义部署和迁移路径，同时让默认部署无需手写配置。

3. 不做静默嵌入回退

   当选定目录缺失、不是目录或缺少 `index.html` 时，管理服务启动失败并返回清晰错误。静默回退到嵌入资源会重新制造“替换了目录但页面没变”的问题。

4. 部署包构建要求已有前端构建产物

   `build-deploy-bundle` 应从仓库的 `admin-ui/dist` 复制到输出根目录的 `admin-ui/`。如果构建产物不存在，应失败并提示先构建前端，而不是生成一个启动后必然缺少管理页面的包。

## Risks / Trade-offs

- [Risk] 从旧版本升级后安装根目录没有 `admin-ui/`，服务重启会失败。→ 文档和错误信息必须说明需要随新部署包同步 `admin-ui/`，回滚时恢复旧二进制和旧布局。
- [Risk] 本地通过 `go run` 启动时，可执行文件位于临时目录，默认部署根目录无法指向源码树。→ 文档要求本地开发使用发布布局二进制，或通过显式配置的 `admin_frontend_dir` 指向构建产物目录。
- [Risk] 部署包构建现在依赖前端构建先完成。→ 不在 Go 打包命令里自动安装 Node 依赖，保持构建边界清晰，并给出可执行的失败提示。
- [Risk] 保留嵌入文件会让行为再次混淆。→ 若代码暂时保留嵌入资源，只允许测试或显式路径使用，默认路径不得调用嵌入回退。

## Migration Plan

1. 发布包生成流程先运行 `admin-ui` 前端构建，再运行 `build-deploy-bundle`。
2. 安装或升级时把部署包中的 `admin-ui/` 目录同步到安装根目录。
3. 重启 `goginx-server` 后，通过管理根路径和 `/api/admin/session` 验证前端与 API 同源可用。
4. 回滚时恢复上一版二进制与上一版 `admin-ui/` 目录，避免新旧前端和后端合同不匹配。

## Open Questions

- 是否需要额外增加 `GOGINX_ADMIN_FRONTEND_DIR` 环境变量覆盖，方便 configless 服务单元在不写 JSON 配置时指定非默认目录？本变更先不要求该能力。
