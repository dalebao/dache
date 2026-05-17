# 三大支柱总览

Harness 上下文体系由三大支柱构成：上下文工程、架构约束、垃圾回收。三者形成闭环：

```
上下文工程（让执行者知道该做什么）
    ↓
架构约束（让执行者不出错、出错即知道怎么修）
    ↓
垃圾回收（让 Harness 自身不腐化）
    ↓
度量（知道效果如何）→ 优化（改进什么）
    ↓
（反馈到上下文工程，形成飞轮）
```

## 支柱一：上下文工程

**目标**：解决"Agent/执行者怎么知道该做什么"

核心机制：
- 文档体系（渐进式披露 L0-L4）— 见 `progressive-disclosure.md`
- ExecPlan（执行计划活文档）— 见 `execution-plan.md`
- 仓库即唯一真相源 — 见 `context-architecture.md`
- 语言感知的架构哲学 — 见 `architecture-principles.md`

## 支柱二：架构约束

**目标**：解决"Agent/执行者怎么不出错"

核心机制：
- Pre-commit Hook 三层防线（L0/L1/L2）— 见 `architecture-constraints.md`
- 架构不变量（分层/单向依赖/契约优先）— 见 `architecture-constraints.md`
- 本地孪生（最小可启动子集 + 本地观测）— 见 `architecture-constraints.md`
- 确定性验证体系（标准库优先/红绿二值/协议录制回放）— 见 `architecture-constraints.md`

## 支柱三：垃圾回收

**目标**：解决"Harness 自身怎么不腐化"

核心机制：
- 架构级技术债扫描 — 见 `garbage-collection.md`
- 文档新鲜度巡检 — 见 `garbage-collection.md`
- ExecPlan 验收标准强制文档更新 — 见 `execution-plan.md`

## 度量与优化（跨支柱）

- 分层指标（Local → CI → Efficiency）— 见 `metrics.md`
- ACI（AI Collaboration Index）— 见 `metrics.md`
- 优化飞轮（信号 → Taxonomy → 改进 → 验证）— 见 `optimization-loop.md`
