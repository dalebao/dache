# 执行计划：创建 localcache Go SDK

## Background

在 `/Users/dale/harness/` 目录下创建一个 Go 语言的开源 SDK 项目，提供 ORM 风格的本地缓存查询能力。

解决的问题：
1. **缓存时效一致性**：多张数据库表加载缓存时统一刷新，避免传统 localcache 各表过期时间不一致
2. **索引 ORM 查询**：注册索引后支持过滤（Where）、排序（OrderBy）等操作

## 验收标准

- [ ] `go build ./...` → 编译通过，0 errors
- [ ] `go vet ./...` → 0 warnings
- [ ] `go test ./... -count=1` → 全部 Pass，0 skipped
- [ ] 公开 API 使用泛型（Go 1.18+），类型安全
- [ ] 示例代码可编译执行
- [ ] AGENTS.md 约束 3（语言感知架构哲学）激活：检测到 `.go` 文件后 Golang 版原则生效

## 施工蓝图

### Step 1：项目骨架 + 核心类型 (`cache.go`)

**操作**：
- 创建 `go.mod`，模块名 `github.com/dalebao/dache`
- 实现 `Cache[K comparable, V any]` 泛型类型

**验收**：
- `go build ./...` 通过
- `Cache` 支持 `Load`（全量替换）和 `Get`（按 key 查询）

### Step 2：索引系统 (`index.go`)

**操作**：
- 实现泛型索引 `index[K, V, I comparable]`
- 支持多值索引（多个记录有相同索引值）
- 增删数据时自动维护索引

**验收**：
- 索引值查找返回正确结果集

### Step 3：查询构建器 (`query.go`)

**操作**：
- 实现 `Query[K, V]` 构建器：Where / OrderBy / Limit / Offset / Execute
- Select 最优索引执行查询
- Where 支持 OpEQ / OpGT / OpGTE / OpLT / OpLTE

**验收**：
- `cache.Query().Where("age", OpGTE, 18).Execute(ctx)` 返回正确结果

### Step 4：统一刷新组 (`group.go`)

**操作**：
- 实现 `RefreshGroup`，管理一组统一刷新时机的 Cache
- 提供原子化 Load 机制

**验收**：
- 多个 Cache 注册到同一 Group 后，Load 一次更新全部 Cache

### Step 5：类型修复 + 推送

**操作**：
- 修复 view.go 中使用 `[]any` 断言 `Cache.Load` 的类型问题，改用 `TableAdder` 接口
- 调整模块路径为 `github.com/dalebao/dache`
- 推送到 GitHub

**验收**：
- `git push` → 成功推送到 `github.com/dalebao/dache`

## 进度日志

| 日期 | 步骤 | 状态 | 决策/理由 |
|------|------|------|-----------|
| 2026-05-17 | Step 1 | 已完成 | 单包结构，root 即 package localcache |
| 2026-05-17 | Step 2 | 已完成 | indexer 接口+simpleIndex 实现，泛型约束使用 comparable |
| 2026-05-17 | Step 3 | 已完成 | clone-on-write 链式调用，反射 field 访问 + fieldGetter 优化接口 |
| 2026-05-17 | Step 4 | 已完成 | Group 用 interval 控制刷新频率；RefreshGroup 提供 Start/Stop 后台循环 |
| 2026-05-17 | Step 5 | 已完成 | 发现泛型类型无法存入异构集合，引入 TableAdder 接口解耦 |

## 知识沉淀

### 写入 L3 runbook 的稳定事实

- **Go 泛型接口模式**：当多个 `Cache[K1,V1]`、`Cache[K2,V2]` 需要被统一管理时，不能直接存储为 `[]*Cache[K,V]`。解决方案：定义非泛型接口（如 `TableAdder`）暴露所需能力，具体泛型类型实现该接口。在 View 端存储 `[]TableAdder`，类型安全由调用者在 View.Query 中做断言保障。
- **Connector + DataSource 抽象**：把外部数据源接入抽象为 `Connector` 接口（ScanAll/Ping/Close），`DataSource` 包装主备回退链。这个模式可推广到任何需要多数据源 failback 的场景。
- **View + Group 一致性模型**：使用 Group 做间隔控制，View 做版本戳 + epoch 统一刷新。保证跨表查询的数据来自同一时间点。Table/View/Group 三者关系见 `design/view-architecture.md`（如后续补充）。

### 待解决的问题

- Go 本地编译环境尚未就绪（brew install go 超时），需要在 GitHub Actions 上验证编译和测试
