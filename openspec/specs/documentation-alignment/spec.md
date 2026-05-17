## Purpose

定义文档对齐流程，确保产品意图、设计方案、实现证据和 OpenSpec 需求之间的状态声明可追溯，并防止当前文档声称未实现能力已经可用。

## Requirements

### Requirement: Documentation source hierarchy
文档对齐流程 MUST 使用已文档化的来源层级，用于区分产品意图、设计方案、实现证据和 OpenSpec 需求。

#### Scenario: Classifying a documentation claim
- **WHEN** 路线图或规格声明描述产品范围、技术设计或实现状态
- **THEN** 该声明 MUST 标识其来源是产品需求、设计文档、实现证据还是 OpenSpec 需求

#### Scenario: Resolving implementation status
- **WHEN** 产品或设计文档描述某项能力，但当前进展文档没有实现证据
- **THEN** 该能力 MUST NOT 被标记为已实现

### Requirement: Roadmap gap matrix
文档对齐流程 MUST 为主要能力维护路线图/缺口矩阵，记录需求覆盖、设计覆盖、实现证据、当前状态和下一步动作。

#### Scenario: Recording a major capability
- **WHEN** 需求或设计文档中的主要能力被加入路线图/缺口矩阵
- **THEN** 矩阵 MUST 包含来源引用、状态、已实现时的证据，以及能力未完成时的下一步动作

#### Scenario: Marking implemented capability
- **WHEN** 某项能力在路线图/缺口矩阵中被标记为已实现
- **THEN** 矩阵 MUST 引用当前实现证据，例如进展记录、验证文档、测试、构建输出或活跃实现引用

### Requirement: Controlled status vocabulary
文档对齐流程 MUST 只使用 `required`、`designed`、`implemented`、`gap` 或 `out-of-scope` 状态值对路线图条目分类。

#### Scenario: Reviewing roadmap status
- **WHEN** 路线图条目被评审
- **THEN** 其状态 MUST 使用受控值之一，并且 MUST 与引用来源和证据一致

#### Scenario: Identifying missing implementation
- **WHEN** 某项能力已被要求并设计，但缺少实现证据
- **THEN** 除非该能力被明确排除在当前产品范围之外，否则其状态 MUST 是 `gap`

### Requirement: No unsupported feature claims
文档对齐流程 MUST 防止文档把未支持的生产特性声明为当前实现能力。

#### Scenario: Comparing design scope with current progress
- **WHEN** 设计文档包含当前进展文档列为缺失的能力
- **THEN** 对齐文档 MUST 把这些能力保留为缺口，而不是当前能力

#### Scenario: Updating implementation progress
- **WHEN** 未来工作完成此前缺失的能力
- **THEN** 对齐文档 MUST 在引用新的实现证据后才更新状态
