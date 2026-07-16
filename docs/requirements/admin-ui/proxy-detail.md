# 代理详情页 UI 设计

## 1. 页面定位

代理详情页用于查看代理完整配置、运行状态、证书状态以及生命周期操作。

## 2. 路由

- ` /proxies/:id `

## 3. 页面目标

- 展示代理的配置与统计信息
- 支持编辑代理
- 支持启用、禁用、删除代理
- HTTP/HTTPS 展示并编辑路径路由
- HTTPS 代理场景下展示关联证书摘要与访问激活操作

## 4. 页面结构

- 返回导航区
- 基础信息卡片
- 配置卡片
- 运行状态卡片
- 统计卡片
- 操作区
- 路径路由区（HTTP/HTTPS 时显示）
- 访问认证区（HTTPS 时显示）
- 证书信息区（HTTPS 时显示）

## 5. 信息展示

- 代理 ID
- 名称
- 类型
- 所属用户
- 所属客户端
- 状态
- RuntimeStatus
- 描述
- EntryBindHost / EntryPort
- HTTP Host 或 HTTPS SNI 域名
- TargetHost / TargetPort
- HTTPS 绑定证书（`certificateId`，未绑定时显示“未绑定”；不再展示 CertFile / KeyFile 文件路径）
- 活跃连接数
- 上传流量
- 下载流量
- 错误计数
- 创建时间
- 更新时间

## 6. 交互设计

- 页面进入后请求详情数据
- 页面保持 5 秒轮询刷新运行态与统计
- 支持点击“编辑代理”打开编辑表单
- 支持启用、禁用、删除操作
- 删除前需确认当前代理已禁用

## 7. 编辑表单规则

- 允许编辑支持的字段
- 不允许修改代理类型
- BindHost 使用后端提供的监听地址选项，不使用自由输入
- TCP/UDP 编辑 BindHost、EntryPort、TargetHost、TargetPort
- HTTP/HTTPS 编辑 BindHost、EntryPort、域名、TargetHost、TargetPort（默认 `/` 后端）
- HTTP/HTTPS 支持路径路由编辑器：Path Prefix、Client、Target Host/Port、StripPrefix、Upstream Path Prefix
- 路径前缀不得占用 `/.well-known/goginx/`；同一 Domain+PathPrefix 可保存多个代理，但任一时刻最多一个可启用，创建、更新或启用时仅与已启用代理校验冲突；跨用户 Client 拒绝
- HTTPS 通过证书选择控件改绑证书，不再编辑 CertFile / KeyFile 文件路径
- 更新或启用后若发生监听冲突或 listener 启动失败，展示 `ENTRY_CONFLICT`
- 错误需定位到对应字段或操作区

### 7.1 HTTPS 证书选择

HTTPS 代理编辑表单通过证书选择控件改绑证书：

- 证书选择器只列出“与该 Domain Host 兼容且可绑定”的证书；同一证书可绑定多个 Domain，选择器不因证书已被其他 Domain 引用而排除。
- 选中证书展示证书摘要（provider、hostnames、有效期、服务状态，Origin CA 还展示部署提示）。
- 当前选中证书若已不在可用列表（例如失去可服务能力），仍保留展示并提示原因。
- 提交时只传 `certificateId`，不再提交 `certFile` / `keyFile`。

### 7.2 跳转创建证书与草稿恢复

编辑表单中点击“创建证书”跳转证书页创建流程：

- 跳转前保存编辑表单草稿（不含 secret material），链接携带 `returnTo=/proxies/<id>`、`draftId` 和 `host`。
- 证书创建成功后返回详情页，重新打开编辑对话框，还原草稿字段并自动选中新证书。
- 草稿缺失、过期或解析失败时安全降级：回退到当前 proxy 配置并预选新证书，提示草稿已失效。

## 7.3 HTTPS 访问认证

HTTPS 详情页提供访问认证区域：

- 展示当前认证开关状态
- “开启认证并生成激活链接”：原子开启并返回一次性 URL
- “生成新激活链接”：已开启时可用
- “统一撤销全部访问”
- “关闭认证”（执行统一撤销语义）

激活链接对话框展示完整 URL、复制按钮、二维码与过期时间，并提示“链接仅展示一次”。关闭对话框后清理前端内存中的 URL，不得写入长期缓存、草稿或 storage。

## 8. 证书区（Web Proxy）

Web Proxy 不持有权威证书绑定；证书绑定在所属 Domain 上。本页可只读展示 Domain 证书摘要（若有）：

- Serving / Operation 状态
- Host / hostnames
- 到期时间与最近操作时间
- 失败次数、指纹、最近错误摘要

证书的创建、删除、签发、续期、轮换、同步、撤销在证书页执行；绑定/解绑在 Domain 详情页执行。

### 8.1 绑定与运行时解析

- 权威绑定为 `domains.certificate_id`；证书与 Domain 为 1:n（一证可服务多 Domain）。
- HTTPS 按 SNI 选择 Domain 与证书后终止 TLS，再按 Path 选择本 Proxy。
- Domain 未绑定可服务证书或证书失效时，该 Domain 的 HTTPS entry fail closed。
- 在证书页删除仍被 Domain 引用的证书会解除相关 Domain 绑定，HTTPS 在重新绑定前不可用。

## 9. 状态设计

### 9.1 初始加载
- 详情骨架屏

### 9.2 不存在态
- 显示“代理不存在”

### 9.3 冲突态
- 单独显示监听冲突信息

### 9.4 删除前置失败
- 显示“删除前需先禁用代理”

### 9.5 操作失败
- 更新、启停、删除失败分别反馈

## 10. 可用性要求

- 危险操作与普通操作需明显分区
- 编辑后的刷新仅影响当前详情，不强制全页重载
- 证书区仅在 HTTPS 场景可见

## 11. 验收标准

- 能展示完整代理详情
- 能执行编辑、启停、删除
- 不允许原地修改类型
- 删除前必须先禁用
- 冲突错误有明确视觉反馈
- HTTPS 代理只通过证书选择器改绑证书或跳转创建证书，不再编辑 CertFile / KeyFile
- 跳转创建证书后能恢复编辑表单草稿并自动选中新证书；草稿失效时安全降级
- 证书摘要为只读，证书生命周期动作集中在证书页
- HTTP/HTTPS 可管理路径路由；跨用户 Client 与保留路径校验有明确错误
- HTTPS 可开启访问激活、生成一次性链接/二维码，并统一撤销；URL 仅展示一次
