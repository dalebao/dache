# AI 自运营项目经验 — 可移植操作手册

本文档记录 AI 在没有人工介入的情况下，自主完成 Go SDK 项目开发、测试、问题发现与修复的全流程经验。可移植到任意其他项目。

---

## 1. 核心理念

AI 自运营 = 一个人工指令 + AI 自主规划 + 迭代验证 + 知识沉淀。

关键模式：

```
指令 → 执行计划 → 分步实现 → 编译验证 → 测试
→ 代码审计发现 bug → 修复 → 重新验证
→ 知识沉淀 → 文档化
```

## 2. 通用工作流

### 2.1 创建执行计划 (ExecPlan)

在 `.context/plans/` 下创建管理计划，包含：
- Background（自包含上下文）
- 验收标准（结果导向：可观测的命令/输出）
- 施工蓝图（分步操作 + 每步验收）
- 进度日志（活文档：边干边更新）

### 2.2 分步实现

按 ExecPlan 的 Step 顺序实现。每步完成后：
1. 编译验证：`go build ./...`
2. 测试验证：`go test -count=1 -v ./...`
3. `go vet ./...`
4. 更新进度日志中的状态

### 2.3 代码审计

实现完成后，进行系统性审计。审计清单：

| 审计项 | 检查方法 | 常见问题 |
|--------|----------|----------|
| 编译错误 | `go build ./...` | 泛型类型推断失败、import 遗漏 |
| 静态分析 | `go vet ./...` | 未使用变量、死代码 |
| 测试挂起 | `go test -timeout 30s` | 阻塞调用、无限循环 |
| 竞态 | `go test -race` | 并发安全 |
| 泛型限制 | 检查方法上的类型参数 | Go ≤ 1.22 不支持方法类型参数 |
| 接口实现 | 检查接口可被不同泛型实例使用 | 异构泛型需要非泛型接口解耦 |

### 2.4 常见 Go 泛型陷阱

1. **方法不能有类型参数**：需要改为包级函数
   ```go
   // ❌ 不行
   func (v *View) Query[K, V any](name string) *Query[K, V]
   // ✅ 改为包级函数
   func QueryView[K, V any](v *View, name string) *Query[K, V]
   ```

2. **类型推断在参数中缺失类型参数时失败**：当泛型函数的部分类型参数未被参数使用时，需要显式指定
   ```go
   // ❌ New 的 K 无法从 WithIndex 的参数推断
   New(WithKey(kfn), WithIndex(...))
   // ✅ 显式指定
   New[int, User](WithKey(kfn), WithIndex[int, User, int](...))
   ```

3. **异构泛型集合**：不同 K/V 的 `Cache[K1,V1]` 和 `Cache[K2,V2]` 不能存入同一切片
   ```go
   // ❌ 不行
   var caches []*Cache[comparable, any]
   // ✅ 定义非泛型接口，让泛型类型实现它
   type Adder interface { LoadFrom(...) error }
   type Table[K, V] struct { ... } // 实现 Adder
   ```

## 3. 测试策略

### 3.1 Mock 模式

对接口依赖使用 Mock 实现：

```go
type mockConnector struct {
    scanFn func(ctx context.Context, table string, dest any) error
}
func (m *mockConnector) ScanAll(ctx context.Context, table string, dest any) error {
    return m.scanFn(ctx, table, dest)
}
```

### 3.2 覆盖关键路径

| 路径 | 测试场景 |
|------|----------|
| 正常路径 | 主数据源成功 → 数据加载 → 查询 |
| 回退路径 | 主数据源失败 → 备源成功 → 数据可用 |
| 全失败路径 | 所有源失败 → 错误返回 → 降级标志 |
| 原子性 | 多表刷新 → 一表失败 → 所有不更新 |
| 版本戳 | 刷新后版本递增 |

### 3.3 竞态测试

```go
func TestConcurrency(t *testing.T) {
    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := 0; j < 100; j++ {
                cache.Get(key)
                cache.Query().Where(...).Execute(ctx)
            }
        }()
    }
    wg.Wait()
}
```

## 4. 问题发现与修复

### 4.1 阻塞问题检测

当 `go test` 超时时，检查：
- `sync.Once.Do` 中是否包含无限循环（常见错误）
- 是否缺少 goroutine 启动（`go func()`）
- channel 是否在正确时机关闭

示例修复：

```go
// ❌ 阻塞：once.Do 不会返回
rg.once.Do(func() {
    for { select { ... } }  // 无限循环
})

// ✅ 非阻塞：once.Do 启动 goroutine
rg.once.Do(func() {
    go func() {
        for { select { ... } }
    }()
})
```

### 4.2 字段/方法名冲突

当 interface 方法名与 struct 字段名相同时，Go 无法区分：

```go
// ❌ 编译错误
type indexer interface { name() string }
type simpleIndex struct { name string }  // 冲突
// ✅ 改方法名
type indexer interface { indexName() string }
```

### 4.3 降级状态管理

DataSource 模式中，降级标志必须在回退成功后重置：

```go
// ❌ 错误：回退成功但未重置标志
for _, fb := range fallbacks {
    if fb.ScanAll(...) == nil {
        return nil  // 忘记 errCount = 0
    }
}
// ✅ 正确
for _, fb := range fallbacks {
    if fb.ScanAll(...) == nil {
        ds.errCount = 0
        return nil
    }
}
```

## 5. 工具链注意事项

### 5.1 Go 版本兼容性

| 功能 | Go 1.22 | Go 1.23+ |
|------|---------|----------|
| 泛型 | ✅ 函数/类型 | ✅ 函数/类型 |
| 方法类型参数 | ❌ | ❌ (Go 1.24 也未支持) |
| range-over-func | ❌ | ✅ 实验性 |

### 5.2 GitHub API

使用 GitHub CLI (`gh`) 比直接调用 API 更可靠：

```bash
gh issue create --title "bug" --body "description" --label "bug"
```

如不可用，可创建 `.github/issues/*.md` 文件作为 issue 草稿。

### 5.3 网络环境

Go module 下载可能因 IPv6 问题失败。解决方法：

```bash
# 跳过代理，直接连接
GOPROXY=direct go mod tidy

# 或临时下载 Go
curl -sL https://go.dev/dl/go1.22.5.linux-amd64.tar.gz | tar -xz -C /tmp
export PATH="/tmp/go/bin:$PATH"
```

## 6. 文档沉淀

每轮迭代结束后更新：
1. **`.context/runbooks/`** — 稳定事实和操作命令
2. **`.context/plans/`** — 执行计划进度和学到的经验
3. **`.github/issues/`** — 发现的问题和修复方案

## 7. 检查清单（可复用到任意项目）

```markdown
- [ ] ExecPlan 创建（Background + 验收 + 蓝图）
- [ ] 代码实现完成
- [ ] go build ./... 通过
- [ ] go vet ./... 通过
- [ ] go test -count=1 -v ./... 全部 PASS
- [ ] go test -race ./... 通过
- [ ] 代码审计完成（泛型限制、阻塞问题、竞态）
- [ ] 发现的 bug 已修复
- [ ] 知识沉淀到 runbook
- [ ] 推送 GitHub
- [ ] CI 配置完成
```
