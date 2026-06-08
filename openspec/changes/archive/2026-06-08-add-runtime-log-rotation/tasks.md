## 1. 配置与默认值

- [x] 1.1 为 server/client 定义共享日志轮换配置字段、默认值和 JSON 映射，保留 server 现有 `log_retention_days` 语义
- [x] 1.2 增加配置校验，覆盖轮换大小、保留数量、保留天数和压缩配置的无效值
- [x] 1.3 更新 managed/default 配置加载路径，使 configless server/client 都获得一致日志轮换默认值

## 2. 共享日志轮换实现

- [x] 2.1 新增 `internal/logging` 共享日志输出初始化，接收部署根、日志文件名和轮换配置并返回关闭函数
- [x] 2.2 实现或接入应用内大小轮换、时间戳归档、保留天数、保留数量和可选压缩行为
- [x] 2.3 确保日志输出继续同时写入 stderr，并且文件日志初始化失败时不会掩盖后续启动错误
- [x] 2.4 增加归档清理和压缩失败处理，确保失败只记录诊断信息且后续日志继续写入当前文件

## 3. Server/Client 接入

- [x] 3.1 替换 `cmd/goginx-server` 中重复的 `setupLogOutput`，改用共享日志初始化并传入 server 配置
- [x] 3.2 替换 `cmd/goginx-client` 中重复的 `setupLogOutput`，改用共享日志初始化并传入 client 配置
- [x] 3.3 处理配置加载前的早期日志输出，确保配置错误、路径错误和轮换初始化错误仍能被 stderr 捕获

## 4. 测试覆盖

- [x] 4.1 增加配置默认值与校验测试，覆盖 server/client 日志轮换字段
- [x] 4.2 增加日志轮换单元测试，验证达到大小阈值后当前文件重建、归档命名和后续写入
- [x] 4.3 增加保留策略测试，验证过期归档和超过数量限制的归档会被清理，当前日志不会被删除
- [x] 4.4 增加 stderr 旁路测试或入口级测试，验证启用文件轮换时 stderr 仍收到日志
- [x] 4.5 在可行范围内覆盖 Windows 兼容路径，避免依赖外部 rename 型 logrotate 行为

## 5. 文档与示例

- [x] 5.1 更新 README 和 `docs/daemon-runtime.md`，说明默认日志轮换、配置字段、归档命名和故障排查
- [x] 5.2 更新部署包示例配置，加入日志轮换字段并标注为高级覆盖
- [x] 5.3 补充 Linux/systemd、Windows、macOS 和容器部署下的推荐日志处理方式

## 6. 验证

- [x] 6.1 运行相关 Go 单元测试，至少覆盖配置、日志轮换实现和 server/client 入口路径
- [x] 6.2 运行 `openspec validate --type change add-runtime-log-rotation --strict` 确认变更规格仍通过
