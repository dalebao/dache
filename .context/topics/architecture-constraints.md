# 架构约束（Architecture Constraints）

## 核心理念

> 约束换自治：更严格且不可绕过的门禁 = 更少的猜测试错 = 更大的效率提升

AI Coding 场景下，架构约束不能依赖执行者"理解设计意图"，必须被编码为自动化规则。

## Pre-commit Hook 三层防线

### L0 — 通用卫生

格式、大文件、冲突、密钥泄露、分支保护等。所有项目通用。

推荐工具：`pre-commit` 框架 + `detect-secrets`、`git-secrets`

### L1 — 语言质量

按技术栈定制：编译检查、lint、类型检查、测试等。

| 技术栈 | 推荐工具 |
|--------|----------|
| Go | `golangci-lint`（聚合 govet/staticcheck/errcheck）、`go vet`、`go mod tidy` |
| Java | Checkstyle、PMD、SpotBugs、ArchUnit |
| TypeScript | ESLint + `@typescript-eslint`、Prettier |
| Python | ruff、mypy、pytest |

### L2 — AI 专项守护

- 占位符拦截（如 `TODO` / `FIXME` 未处理）
- 幻觉标记拦截（AI 生成的虚假引用/API）
- 文件行数上限（防止 AI 产出巨型文件）
- ExecPlan 完成度守护（验收标准未达成不允许提交）

### 三条关键设计原则

1. **报错即指导**：错误输出包含精确上下文与修复建议（路径/当前值/阈值/下一步命令），让 Hook 成为教练而非警察
2. **增量 delta 校验**：如果仓库已有大量存量问题，优先拦截增量而非全量。全量门槛会因 baseline 噪音导致误报，使执行者反复卡住
3. **自愈闭环**：Hook 拦截 → 执行者读取报错 → 自动修复 → 重新提交。人类只做定方向与 Review

## 架构不变量（Architecture Invariants）

定义"允许依赖方向"的规则，违反即阻断：

- **分层依赖规则**：Controller → Service → Repository，不允许跨层调用
- **模块依赖规则**：只允许向内/向下依赖，禁止循环依赖
- **契约优先**：先改接口/契约，再改实现。反向修改应该被门禁拦截

## 本地孪生（Local Twin）

把反馈链路从分钟级压缩到秒级，包含两方面：

1. **依赖治理**：不追求完全本地化，只追求"最小可启动子集"。判断标准：是否阻塞启动？阻塞则按难度分层处理（noop → 直连 → 容器替代 → Mock → 裁剪）
2. **本地观测栈**：业务代码不变，通过可插拔 Writer/OTLP endpoint 切换本地与线上。日志必须结构化（action/result/error_kind/trace_id），让执行者用查询语言定位而非 grep

## 确定性验证体系

AI 场景下验证的三个硬条件：
1. **零环境依赖**：不依赖外部服务/数据库
2. **确定性输出**：红/绿二值，可复现，可脚本化
3. **执行者可消费**：CLI 而非 GUI

验证链路（从快到慢）：单元测试 → 协议回放 → 组件验证 → E2E

## 协议录制回放（Cassette Tape）

不要手写 mock。录制一次真实交互，永久确定性重放。

关键做法：
- 精度可到 SSE 事件 payload 级别
- 消除模型响应随机性
- 把分钟级验证压缩到毫秒级

迁移提示：在评测/实验系统里，它等价于用"固定数据集/固定请求-响应语料/固定随机种子"替代在线不确定性。
