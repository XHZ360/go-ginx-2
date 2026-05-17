## 1. Configless Runtime Foundation

- [x] 1.1 调整 `goginx-server` 和 `goginx-client` 命令入口，使未传 `-config` 时进入 configless/受管状态模式，显式传入 `-config` 时继续使用严格 JSON 配置加载。
- [x] 1.2 在 `internal/config` 中拆分默认配置、显式配置加载和受管状态加载逻辑，保持未知 JSON 字段拒绝和现有字段校验。
- [x] 1.3 为服务端启动添加受管目录初始化，确保默认 `data/`、`data/certs/`、SQLite 路径和后续状态路径在启动前可用。
- [x] 1.4 为 configless 与显式配置两条路径添加配置单元测试，覆盖缺失默认配置文件不再阻塞 configless 启动，以及显式配置文件缺失仍返回清晰错误。

## 2. Control TLS Bootstrap

- [x] 2.1 添加控制通道 TLS bootstrap 服务，在缺少受管控制 CA、证书或私钥时生成材料，并在重启时复用已有材料。
- [x] 2.2 为生成的控制面证书引入稳定 TLS server name，并把实际拨号地址与 TLS 校验名称分离。
- [x] 2.3 调整服务端 TLS 加载路径，使显式配置的证书文件优先，configless 模式使用受管 TLS 材料。
- [x] 2.4 添加证书 bootstrap 测试，覆盖首次生成、重启复用、私钥不写入 SQLite、证书可被客户端信任池校验。

## 3. SQLite Administrator Authentication

- [x] 3.1 将 admin API 登录校验抽象为凭据来源接口，并实现 SQLite 管理员用户凭据来源。
- [x] 3.2 添加 `goginx-admin init-admin` 本地初始化命令，支持创建或更新启用的管理员用户并写入 bcrypt 密码哈希。
- [x] 3.3 更新 admin API 启动逻辑，使 configless 模式不要求 `admin_credentials_file`，并只允许启用的 `RoleAdmin` 用户登录管理面。
- [x] 3.4 保留或明确废弃 `admin_credentials_file` 兼容路径，并添加对应文档和测试。
- [x] 3.5 添加登录与会话测试，覆盖未初始化管理员无默认密码、普通用户不能登录、管理员登录成功和受保护传输要求不变。

## 4. Embedded Admin Frontend

- [x] 4.1 为 admin UI 构建产物添加可嵌入文件系统入口，服务端未配置 `admin_frontend_dir` 时优先服务内置前端。
- [x] 4.2 保留 `admin_frontend_dir` 外部目录覆盖，并保持缺失 asset-like 路径返回 `404 Not Found` 的行为。
- [x] 4.3 更新 bundle 生成逻辑，使 admin UI 不再必须通过配置字段指向外部目录；需要时仍可包含外部前端资源作为覆盖。
- [x] 4.4 添加 admin frontend delivery 测试，覆盖内置资源、外部目录覆盖、无内置资源 fallback 或失败语义。

## 5. Client Join And Managed State

- [x] 5.1 为一次性 client enrollment/join token 添加领域模型、SQLite 持久化和过期/已使用状态跟踪。
- [x] 5.2 添加管理员服务能力和 CLI 命令，用于生成短期或单次使用的客户端 join 材料，并避免日志输出可重放 secret。
- [x] 5.3 实现 `goginx-client join <token>` 流程，解析或交换 join 材料并写入客户端本地受管状态。
- [x] 5.4 调整 `goginx-client` 无参数启动路径，使其从本地受管状态读取服务端地址、TLS 信任材料、server name、client ID、credential、协议和重连参数。
- [x] 5.5 添加 join token 测试，覆盖生成、消费、过期/重复消费拒绝、secret 脱敏和加入后控制通道证书校验。

## 6. Packaging, Services, And Documentation

- [x] 6.1 更新 `deploy/systemd` 服务模板，使默认 `ExecStart` 不传 `-config`，并记录如何用 override 恢复显式配置路径。
- [x] 6.2 更新部署包生成和 bundle 测试，使 server/client JSON 示例变成可选覆盖资料，而不是基础启动依赖。
- [x] 6.3 更新 README 和 daemon runtime 文档，描述 configless server 启动、管理员初始化、client join、受管状态位置和高级配置覆盖。
- [x] 6.4 更新 troubleshooting 文档，覆盖端口冲突、受管 TLS 材料缺失/损坏、未初始化管理员、join token 过期、客户端受管状态损坏等失败类别。

## 7. End-to-End Validation

- [x] 7.1 添加外部进程 smoke 测试，证明无 `server.json`、`client.json`、`admin_credentials_file` 和 `admin_frontend_dir` 时服务端可以启动并初始化受管状态。
- [x] 7.2 扩展 E2E 流程，使用 `init-admin` 初始化管理员并验证 admin 登录/session/bootstrap 行为。
- [x] 7.3 扩展 E2E 流程，生成 client join 材料、执行 `goginx-client join`、无 `-config` 启动客户端并完成控制通道认证。
- [x] 7.4 验证显式 JSON 配置路径仍可运行现有 server/client smoke 流程，避免破坏兼容部署。
- [x] 7.5 运行完整 Go 测试和相关 admin UI 测试，确认 configless 路径、兼容路径、bundle 和文档示例保持一致。
