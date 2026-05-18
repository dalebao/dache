# 执行计划规范

## 定义

执行计划 = 设计文档 + 施工蓝图 + 进度日志

弱实现细节，强验收门槛。不关心"怎么实现"，只关心"验收标准是什么"。

## 四条不可妥协原则

### 1. 自包含

新执行者仅凭本计划可接力完成，无需回溯上下文。

- 开头必须有 Background 说明上下文
- 所有术语需解释（或链接到专题文档）
- 路径使用仓库相对路径

### 2. 活文档

边干边更新进度、证据与决策理由。

- 每完成一个 Step 立即更新状态
- 记录关键决策和理由
- 附上证据（日志、截图、测试结果）

### 3. 结果导向

用可观测行为描述验收标准。

模板：`[命令/操作] → [预期输出/行为]`

例如：
- ❌ "修复 bug"（不可观测）
- ✅ "`cargo test` → tests全部 Pass，0 skipped"

### 4. 新手优化

- 术语首次出现时解释（括号或脚注）
- 所有文件路径使用仓库相对路径，如 `src/main.rs` 而非 `/absolute/path/to/src/main.rs`
- 命令必须是可复现的完整命令

## 与 ADR 的关系

ADR（Architecture Decision Record，Michael Nygard 提出）是与 ExecPlan 互补的文档形式：

| 维度 | ExecPlan | ADR |
|------|----------|-----|
| 关注点 | 做什么 + 怎么验证 | 为什么这么决策 |
| 生命周期 | 从开始到验收 | 永久存档 |
| 更新频率 | 高频（边做边更新） | 低频（只在决策时写） |
| 核心结构 | Background + 验收 + 蓝图 + 日志 | Context + Decision + Consequences |

推荐做法：重大架构决策（引入新框架、改变数据流、变更模块边界）应写 ADR 并存于 `.context/runbooks/adr/`。ExecPlan 中的 Step 决策理由可引用 ADR。

ADR 基本格式（1-2 页）：

```
# ADR-N：标题

## Context
（描述当前问题、技术约束、备选方案）

## Decision
（用了什么方案，为什么）

## Status
[Proposed / Accepted / Deprecated]

## Consequences
（正面和负面结果）
```

## 模板

见 `.context/plans/plan-template.md`
