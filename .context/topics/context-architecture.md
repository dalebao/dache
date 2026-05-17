# 上下文体系架构

## 两大基石哲学

### 哲学一：仓库即上下文

所有 AI 需要的知识都存放在当前仓库中。这意味着：
- 入口文件（AGENTS.md / CLAUDE.md / .cursorrules 等）、项目配置、`.context/` 下的所有文件都在仓库内
- AI 不从外部 URL、全局配置中无关的目录获取知识
- 仓库是自描述的：clone 后 AI 能独立理解项目

### 哲学二：文档体系 + 执行计划

上下文工程分为两大子系统：

**文档体系** — 渐进式披露（Progressive Disclosure）
- 从 L0（入口）到 L4（可调用 Skill），逐步深入
- 入口薄化，知识下沉

**执行计划** — 设计文档 + 施工蓝图 + 进度日志
- 弱实现细节，强验收门槛
- 四条不可妥协原则（见 execution-plan.md）

## 三大支柱

Harness 由三大支柱 + 度量 + 优化构成完整闭环：

```
上下文工程（该做什么）→ 架构约束（不出错）→ 垃圾回收（不腐化）
                                      ↑           ↓
                               度量 ← 优化（改进飞轮）
```

详见 `three-pillars.md`。

## 分层职责

```
L0  AGENTS.md / CLAUDE.md / .cursorrules   路由 + 核心约束
L1  .context/topics/                       专题知识 ← 你在此
L2  .context/modules/                      模块入口
L3  .context/runbooks/                     稳定事实/操作手册
L4  .context/skills/                       可调用 Skill（能力包）
```

> `.context/` 是本系统的通用目录名，不依赖特定 AI 工具。你的 AI 工具可能使用不同文件名作为 L0 入口（如 Kilo 用 AGENTS.md，Claude Code 用 CLAUDE.md）。

## 文档流转

1. 优化闭环发现高频痛点 → 定向改进 → 产生新知识
2. 新知识验证后 → 提炼为 L3 稳定事实
3. 稳定事实积累 → 抽象为 L1 专题
4. 模块化 → 提取到 L2 模块文档
5. L0 入口保持薄，仅维护路由表
