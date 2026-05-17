# Harness 上下文体系 — L0 入口

## 核心约束（不可违反）

1. **仓库即上下文**：AI 所有知识从当前仓库获取。禁止依赖外部搜索、全局配置中与本项目无关的上下文。新 clone 者应能凭仓库自描述文件理解项目。
2. **文档体系 + 执行计划二分法**：上下文工程分为文档体系和执行计划。文档体系遵循渐进式披露（L0-L4）；执行计划 = 设计文档 + 施工蓝图 + 进度日志。
3. **语言感知的架构哲学**：AI 根据仓库中检测到的语言，自动应用 `.context/topics/architecture-principles.md` 中对应的架构设计哲学。检测到 `.go` 文件则应用 Golang 版，`.java` 文件则应用 Java 版，同时存在则按文件后缀分别应用。

## 路由表

| 层级 | 路径 | 用途 |
|------|------|------|
| L0 | `AGENTS.md` | 入口：路由 + 核心约束（当前文件） |
| L1 | `.context/topics/` | 专题知识，按主题导航 |
| L2 | `.context/modules/` | 模块入口，只放本模块命令与约束 |
| L3 | `.context/runbooks/` | 稳定事实 / 操作手册 / 执行计划 |
| L4 | `.context/skills/` | 可调用的 Skill（能力包） |
| — | `.context/plans/` | 执行计划：设计 + 蓝图 + 进度 |

## 三大支柱导航

| 支柱 | 专题文档 | 操作手册 |
|------|----------|----------|
| 上下文工程 | `topics/context-architecture.md`、`topics/progressive-disclosure.md`、`topics/execution-plan.md`、`topics/architecture-principles.md` | — |
| 架构约束 | `topics/architecture-constraints.md` | `runbooks/pre-commit-hooks.md` |
| 垃圾回收 | `topics/garbage-collection.md` | — |
| 度量 | `topics/metrics.md` | — |
| 优化 | `topics/optimization-loop.md` | — |

## 命令/智能体入口

- `.context/command/*.md` — AI 自定义命令
- `.context/agent/*.md` — AI 自定义智能体/角色

> 注：本系统为工具无关的通用上下文体系。在 Kilo 中等效于 `.kilo/command/` 和 `.kilo/agent/`，在 Claude Code 中等效于 `CLAUDE.md`。本仓库使用 Kilo，因此以 `AGENTS.md` 为 L0 入口。

## AGENTS.md 维护原则

- 入口薄化：只承载路由/约束/命令入口，知识本地下沉到专题文档
- 维护策略：把文档更新写进执行计划的验收标准，巡检只做兜底
