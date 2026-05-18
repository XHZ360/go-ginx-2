## Why

当前 configless/join 体验、客户端资源生命周期和 admin-ui 弹窗交互仍有几个高频操作断点：join token 缺少可靠默认服务地址，客户端无法完全删除，从客户端创建代理需要重复选择上下文，弹窗在小屏或长 token 情况下容易溢出，并且蒙版关闭行为可能误关正在操作的弹窗。

本变更把这些行为收敛为明确合同，使基础部署和管理员日常资源管理更顺滑、更可预测。

## What Changes

- 服务端配置加载和启动阶段确认当前服务域名或 IP，并将其作为后续生成 join/enrollment token 的默认服务地址；显式配置仍可覆盖自动确认结果。
- 管理员客户端管理支持“完全删除”客户端资源，并定义删除前置条件、关联资源处理和审计行为。
- admin-ui 支持从客户端详情或列表直接跳转到创建代理流程，创建代理时默认携带对应 `clientId` 与 `userId`，并仍提供用户与客户端选择器允许管理员修改。
- admin-ui 所有弹窗设置最大高度和内部滚动，避免内容溢出视口；token/secret 展示区域最多显示 3 行，超出时在区域内滚动。
- admin-ui 弹窗蒙版只在鼠标按下和释放都发生在蒙版层时关闭；若从弹窗内部按下并在蒙版层释放，不关闭弹窗。

## Capabilities

### New Capabilities

- 无

### Modified Capabilities

- `control-channel`: join/enrollment 材料生成使用服务端确认的默认服务地址，并允许显式覆盖。
- `deployment-operations`: configless 或显式配置启动时确认当前服务域名或 IP，作为后续 join 默认值的运行时状态。
- `admin-resource-management`: 客户端完全删除、客户端到代理创建的上下文跳转，以及弹窗滚动/关闭交互合同。

## Impact

- 服务端配置与启动路径：需要解析显式服务域名/IP配置、自动推断默认地址，并在生成 join token 时复用。
- Join/enrollment 生成 API 或命令：默认服务地址来源从临时输入扩展为“已确认服务地址”，并保留管理员覆盖入口。
- Admin GraphQL/API 与资源服务：新增或补齐客户端删除 mutation/command、校验关联代理和审计记录。
- Admin 前端：更新 Clients、ClientDetail、Proxy 创建表单、用户/客户端选择器、Modal/Dialog 组件与一次性 token 展示组件。
- 测试：覆盖服务地址默认值、客户端删除约束、proxy 创建默认上下文、弹窗滚动和蒙版点击语义。
