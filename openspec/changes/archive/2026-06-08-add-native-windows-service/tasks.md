## 1. Runtime Entry Refactor

- [x] 1.1 将 `cmd/goginx-server/main.go` 的配置加载、启动、阻塞和关闭逻辑抽成可由控制台模式与 Windows Service 模式共用的 `runServer(ctx, configPath)`。
- [x] 1.2 将 `cmd/goginx-client/main.go` 的客户端运行逻辑抽成可由控制台模式与 Windows Service 模式共用的 `runClient(ctx, configPath)`。
- [x] 1.3 保持现有控制台模式行为不变，包括 `join` 子命令、`-config` 参数、日志输出和信号停止。

## 2. Windows Service Core

- [x] 2.1 新增 Windows build-tag 的服务管理包，封装 `svc.Run`、`mgr.CreateService`、服务启动、停止、重启、状态查询和卸载。
- [x] 2.2 新增非 Windows build-tag fallback，使 service 命令在非 Windows 平台返回清晰的不支持错误。
- [x] 2.3 实现通用 service handler，将 SCM 的 stop/shutdown 请求映射为 context cancel，并等待 server/client runner 退出。
- [x] 2.4 定义 server/client 默认服务名、显示名、描述、启动类型和 `service run` binPath 生成规则。

## 3. Command Integration

- [x] 3.1 为 `goginx-server` 增加 `service install|uninstall|start|stop|restart|status|run` 子命令。
- [x] 3.2 为 `goginx-client` 增加 `service install|uninstall|start|stop|restart|status|run` 子命令。
- [x] 3.3 支持服务安装参数：`-name`、`-display-name`、`-description`、`-startup` 和可选 `-config`。
- [x] 3.4 在 `goginx-client service install` 默认 managed 模式下校验 `data/client-state.json` 存在；显式 `-config` 模式下校验配置可加载。

## 4. Packaging And Documentation

- [x] 4.1 为 Windows 发布包新增 PowerShell 辅助脚本，封装 server/client 服务安装、启动、停止、重启、状态查询和卸载流程。
- [x] 4.2 更新 Windows 发布包说明，覆盖内置 `service` 子命令和 PowerShell 辅助脚本两种使用方式。
- [x] 4.3 更新 README 或部署文档，强调 Windows server 服务部署时优先使用 `config/server.json` 的 `join_service_host` 或显式 token 地址。
- [x] 4.4 补充 Windows 客户端服务部署顺序：先执行 `join`，确认 `data/client-state.json`，再安装并启动服务。
- [x] 4.5 补充 Windows 服务升级、回滚、文件日志、权限、防火墙和常见故障排查说明，明确首版不支持自定义服务账户且不接入 Windows Event Log。

## 5. Validation

- [x] 5.1 增加 service 命令解析和 binPath 构造单元测试，覆盖默认参数、显式 `-config`、服务名和启动类型。
- [x] 5.2 增加 service handler 单元测试，模拟 stop/shutdown 并验证 runner 收到 context cancel。
- [x] 5.3 增加客户端服务安装前置校验测试，覆盖缺少受管状态、存在受管状态和显式配置路径。
- [x] 5.4 运行 `go test ./...`，并至少验证 Windows 目标可编译：`GOOS=windows go test ./cmd/goginx-server ./cmd/goginx-client`。
