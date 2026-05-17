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

### Step 5：测试 + 示例

**操作**：
- 为每个组件写单元测试（table-driven）
- 写 example_test.go 或 example 目录
- 跑全量测试

**验收**：
- `go test -count=1 ./...` 全部 Pass

## 知识沉淀

计划完成后更新：
- `.context/topics/architecture-principles.md` — 验证 Golang 版原则在本项目的适用性
- `.context/runbooks/` — 记录 Go 项目特有的构建/测试命令
