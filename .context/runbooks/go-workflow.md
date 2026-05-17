# Go 项目开发工作流 — L3 稳定事实

## 构建

```bash
go build ./...
```

## 测试

```bash
# 所有测试，禁用缓存
go test -count=1 ./...

# 带 race detector
go test -race -count=1 ./...

# 覆盖率
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

## 代码质量

```bash
go vet ./...
gofmt -l -s -w .
```

## 移除外来依赖

```bash
go mod tidy
```

## 项目约定

- 单包结构：root package = `localcache`，对外暴露 `package localcache`
- 测试文件使用 `_test.go` 后缀，外测模式 `package localcache_test`
- 公开 API 优先用泛型（Go 1.18+），内部用接口擦除类型参数
- 当多个泛型类型的实例需要被统一管理时，定义非泛型接口解耦
