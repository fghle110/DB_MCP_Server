# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with this repository.

## 项目概述

**dbmcp** — 一个用 Go 编写的数据库操作 MCP Server。通过模型上下文协议（stdio 传输）暴露 8 个工具，供 AI 工具与 MySQL、PostgreSQL、SQLite 数据库交互。

## 常用命令

```bash
# 构建
go build -o dbmcp.exe ./cmd/dbmcp

# 运行全部测试
go test ./... -v

# 运行单个包的测试
go test ./internal/config/... -v

# 运行特定数据库集成测试
go test ./internal/database/... -run TestSQLite -v
go test ./internal/mcp/... -run TestMCP -v

# 构建指定平台版本
GOOS=linux GOARCH=amd64 go build -o dbmcp ./cmd/dbmcp

# 网络受限时配置代理
GOPROXY=https://goproxy.cn,direct go get <package>
```

> **注意**：SQLite 集成测试需要 CGO（C 编译器）。如果没有 gcc，可以使用 `modernc.org/sqlite` 替代 `mattn/go-sqlite3`，或跳过 SQLite 测试。

## 架构

### 模块结构

`internal/` 下每个模块职责清晰、相互独立：

| 模块 | 职责 |
|------|------|
| `config/` | 配置加载（YAML）、校验、基于 fsnotify 的热重载 |
| `database/` | `DatabaseDriver` 接口 + `DriverManager` 连接池 + 3 种驱动实现 |
| `security/` | SQL 注入防护：关键字拦截、多语句检测、输入校验 |
| `permission/` | 权限校验：只读模式、数据库白名单、表黑名单、操作白名单 |
| `logger/` | 审计日志，写入本地 SQLite（`~/.dbmcp/audit.db`） |
| `mcp/` | MCP Server：注册 8 个工具，串联所有模块的安全/权限管道 |

### 请求流程

每条 SQL 操作在 `internal/mcp/server.go` 中经过以下管道：

1. **安全检查**（`security.SQLGuard`）— 长度/编码/控制字符检查 → 多语句检测 → 危险关键字拦截
2. **权限校验**（`permission.Checker`）— 数据库白名单 → 表黑名单 → 操作白名单 + 只读模式检查
3. **数据库驱动**（`database.DatabaseDriver`）— 执行查询，30 秒超时
4. **审计日志**（`logger.AuditLogger`）— 记录操作至 SQLite

### 配置热重载

`config/watcher.go` 使用 fsnotify 监听配置文件。变更时：
1. 加载并校验新配置
2. 通过 `AppState.UpdateConfig()` 原子替换
3. 通过 `DriverManager.SyncFromConfig()` 同步数据库连接（旧连接 30 秒后优雅关闭）
4. 通过 `permission.Checker.Update()` 更新权限

校验失败时保留旧配置。

### 添加新数据库驱动

实现 `internal/database/interface.go` 中的 `DatabaseDriver` 接口：

```go
type DatabaseDriver interface {
    Connect(dsn string) error
    Query(ctx context.Context, sql string) (*QueryResult, error)
    Exec(ctx context.Context, sql string) (int64, error)
    ListDatabases(ctx context.Context) ([]string, error)
    ListTables(ctx context.Context, database string) ([]string, error)
    DescribeTable(ctx context.Context, database, table string) ([]Column, error)
    Close() error
}
```

然后在 `manager.go` 的 `createDriver` 工厂函数中注册。

### 模块依赖

```
cmd/dbmcp/main.go (入口)
  └── config → database → permission → security → logger → mcp
```

无循环依赖。各模块仅导入 `config` 包的共享类型。

## 技术栈

Go 1.26, mark3labs/mcp-go v0.48, go-sql-driver/mysql, jackc/pgx/v5, mattn/go-sqlite3, fsnotify, yaml.v3
