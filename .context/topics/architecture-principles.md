# 架构设计哲学

## 语言自动选择规则

AI 根据仓库中检测到的语言自动选择对应的架构哲学版本：

| 检测条件 | 应用版本 |
|----------|----------|
| 仓库中存在 `*.go` 文件（不含 `vendor/`） | Golang 版 |
| 仓库中存在 `*.java` 文件（不含测试用例） | Java 版 |
| 两者同时存在 | 两者都加载，按文件后缀分别应用 |
| 两者都不存在 | 不加载本文件，无此约束 |

---

## Golang 版

### 0. Go 之道

> "Clear is better than clever." — Go Proverbs

Go 代码追求**极致的简单性**。不要为了"以后可能需要"而引入抽象。如果今天不需要接口，就不要定义接口。

### 1. 包设计（Package Design）

- **按职责分包**：一个包做一件事。包名就是它的职责声明（`auth`、`store`、`http`），包名全小写、单数
- **禁止循环依赖**：`a → b → a` 在任何层级都不允许。出现循环依赖说明职责划分错误
- **内部包隔离**：`internal/` 下的包对外不可见，利用 Go 编译器强制封装。所有非公开的实现细节都应放在 `internal/`
- **避免 `util` / `common` / `helper` 包**：这类包往往是职责不明的垃圾桶。每个函数都应该有更合理的归属

### 2. 接口（Interface）

- **小接口优先**：接口通常包含 1-3 个方法。`io.Reader` 是 Go 接口设计的典范
- **定义在消费者侧**：接口定义在使用它的包中，而非实现它的包。这是 Go 的"隐式实现"（duck typing）最强大的地方
- **接受接口，返回结构体**：函数参数用接口接受行为，返回值用具体类型，避免调用者被迫做类型断言
- **不要为了测试而提取接口**：如果只有一个实现，且没有外部替代者的可能，就不要定义接口。用 Concrete Type 面对测试

### 3. 错误处理

- **错误就是值**：错误是普通的值，用 `if err != nil` 处理，不用 `try-catch`
- **尽早返回，减少嵌套**：
  ```go
  // 好的
  if err != nil {
      return err
  }
  // 正常逻辑

  // 不好的
  if err == nil {
      // 正常逻辑
  } else {
      return err
  }
  ```
- **包装错误以增加上下文**：用 `fmt.Errorf("doing something: %w", err)` 包装错误，保留原始错误以便 `errors.Is` / `errors.As`
- **不要 panic**：panic 只用于真正的不可恢复状态（如初始化失败）。普通错误返回 error

### 4. 并发（Concurrency）

- **不要通过共享内存通信，通过通信共享内存**：优先使用 channel 而非 mutex
- **谁启动 goroutine 谁负责关闭**：启动 goroutine 的代码必须确保它能正确退出，包括 cancel 信号的传递
- **使用 errgroup 管理 goroutine 生命周期**：`golang.org/x/sync/errgroup` 是管理并发任务的标准工具
- **限制 goroutine 数量**：使用 worker pool 或信号量模式，禁止无限 spawning
- **channel 的方向是个契约**：函数参数中明确 channel 方向（`<-chan` / `chan<-`）

### 5. 结构体与方法

- **值类型 vs 指针接收者**：如果结构体包含 sync.Mutex 或类似不可复制字段，必须用指针接收者；否则保持一致
- **零值有用性**：让类型的零值可以直接使用（如 `bytes.Buffer`、`sync.WaitGroup`）。这叫"零值可用"
- **构造函数用 `New` 前缀**：`func NewXxx(...) *Xxx`

### 6. 测试

- **Table-driven tests**：Go 测试的标准模式
- **包内测试 vs 外部测试**：白盒测试用 `package xxx`，黑盒测试用 `package xxx_test`
- **测试文件名 `*_test.go`**：必须遵守 Go 工具链约定

---

## Java 版

### 0. Java 之道

> "Prefer well-named code over comments." — Clean Code

Java 代码追求**显式与可读性**。类型系统是你的盟友，利用它来表达不变性和契约。

### 1. 包与模块设计（Package & Module Design）

- **分层架构**：标准的 Controller → Service → Repository 三层，不允许跨层调用（Controller 不能直接调 Repository）
- **按功能分包**：顶层包按功能领域划分（`order`、`payment`、`user`），而非按技术层划分（`controllers`、`services`、`daos`）
  ```
  com.example.order/
  ├── OrderController.java
  ├── OrderService.java
  ├── OrderRepository.java
  └── Order.java
  ```
- **禁止循环依赖**：模块间依赖方向必须单向
- **封装内部实现**：用 `package-private`（默认可见性）隐藏内部类，仅暴露接口或抽象类

### 2. 面向对象原则（SOLID）

- **单一职责**：一个类只有一个变化理由。大於 300 行的类通常是警告信号
- **开闭原则**：对扩展开放，对修改封闭。用策略模式或模板方法代替 `if-else` 链
- **里氏替换**：子类不能削弱父类的前置条件，不能增强后置条件
- **接口隔离**：接口应小而专，不应强迫实现类依赖它们不需要的方法
- **依赖倒置**：依赖抽象（接口/抽象类），不依赖具体实现。结合 DI 容器（Spring / Guice）使用

### 3. 依赖注入

- **构造器注入 > Setter 注入 > Field 注入**：构造器注入明确表达依赖关系，且保证不可变性
- **Spring Boot 环境下优先使用构造器注入**：
  ```java
  @Service
  public class OrderService {
      private final OrderRepository repository;

      public OrderService(OrderRepository repository) {
          this.repository = repository;
      }
  }
  ```
- **避免 @Autowired on field**：使测试困难，隐藏依赖关系

### 4. 不可变性（Immutability）

- **优先使用 `record`（Java 16+）/ `@Value`（Lombok）**：值对象应该是不可变的
- **`final` 是默认选择**：所有字段、方法参数、局部变量默认 `final`，除非有修改理由
- **`List.of()`、`Map.of()` 优先于可变集合**：API 返回值应该不可变

### 5. 异常与错误处理

- **受检异常（Checked Exception）用于可恢复**：调用者可以合理处理的异常
- **非受检异常（RuntimeException）用于编程错误**：表示 bug，不应被 catch
- **异常不要吞没**：空的 catch 块是反模式。即使不处理也要记录日志
- **全局异常处理**：Web 应用使用 `@ControllerAdvice` + `ErrorResponse` 统一处理

### 6. 函数式风格（Java 8+）

- **Stream 优先于显式循环**：集合转换操作优先使用 `stream().map().filter().collect()`
- **Optional 用于返回值，不用作参数**：`Optional` 表示"可能有值"，只用于方法返回值
- **方法引用优先于 Lambda**：`list.stream().map(Foo::bar)` 优于 `list.stream().map(f -> f.bar())`

### 7. 测试

- **Given-When-Then 模式**：每个测试方法遵循 Arrange-Act-Assert
- **Mock 外部依赖**：使用 Mockito 隔离被测试单元
- **测试命名 `methodName_should_expectedBehavior_when_condition`**：
  ```java
  void calculateTotal_should_applyDiscount_when_orderExceedsThreshold()
  ```
- **ArchUnit 验证架构约束**：用 ArchUnit 在 CI 中自动检查分层/包依赖规则
