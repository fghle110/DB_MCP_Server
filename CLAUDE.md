# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with this repository.

## 项目概述

**dbmcp** v0.2.0 — 一个用 Go 编写的数据库操作 MCP Server。通过模型上下文协议（stdio 传输）暴露 15 个工具，供 AI 工具与 MySQL、PostgreSQL、SQLite、Redis 数据库交互。支持多数据库连接、结构化配置、事务控制、配置热重载、SQL 注入防护和操作审计。

## 常用命令

```bash
# 构建（输出至 build/ 目录）
go build -o build/dbmcp.exe ./cmd/dbmcp

# 运行全部测试
go test ./... -v

# 仅运行单元测试（无需 Docker）
go test ./internal/config/... ./internal/security/... ./internal/permission/... -v

# 运行特定数据库集成测试
go test ./internal/database/... -run TestSQLite -v
go test ./internal/database/... -run TestMySQL -v
go test ./internal/mcp/... -v

# 构建指定平台版本
GOOS=linux GOARCH=amd64 go build -o build/dbmcp ./cmd/dbmcp
GOOS=darwin GOARCH=arm64 go build -o build/dbmcp-darwin ./cmd/dbmcp

# 启动 Redis（Docker）
docker-compose -f docker/docker-compose.redis.yml up -d

# 网络受限时配置代理
GOPROXY=https://goproxy.cn,direct go get <package>
```

> **注意**：SQLite 使用 `modernc.org/sqlite`（纯 Go 实现），无需 CGO。MySQL/PostgreSQL 集成测试需要 Docker 自动启动容器。

## 架构

### 模块结构

| 模块 | 职责 |
|------|------|
| `config/` | 配置加载（YAML）、结构化字段支持（host/port/username/password/database/options）、校验、fsnotify 热重载 |
| `database/` | `DatabaseDriver` 接口 + `DriverManager` 连接池 + 4 种驱动实现（MySQL/PG/SQLite/Redis）+ 事务支持 |
| `security/` | SQL 注入防护：关键字拦截、多语句检测（支持 `$$` dollar-quoted strings）、输入校验 |
| `permission/` | 权限校验：只读模式、数据库白名单、表黑名单、操作白名单、Redis 命令白名单/key 黑名单 |
| `logger/` | 审计日志，写入本地 SQLite（`~/.dbmcp/audit.db`），含 DSN 脱敏和高危操作标记 |
| `mcp/` | MCP Server：注册 15 个工具，串联所有模块的安全/权限管道 |

### MCP 工具列表

| 工具 | 说明 |
|------|------|
| `execute_query` | 执行 SELECT 查询 |
| `execute_update` | 执行 INSERT/UPDATE/DELETE/DDL |
| `execute_param_query` | 执行参数化查询 |
| `list_databases` | 列出已连接的数据库 |
| `list_tables` | 列出数据库中的表 |
| `describe_table` | 查看表结构 |
| `query_logs` | 查询审计日志 |
| `config_status` | 查看配置状态 |
| `begin_tx` | 开始事务 |
| `commit` | 提交事务 |
| `rollback` | 回滚事务 |
| `redis_command` | 执行 Redis 命令 |
| `redis_scan` | 安全扫描 key |
| `redis_info` | 服务器状态 |
| `redis_describe` | 查看 key 的类型/TTL/值 |

### 请求流程

每条 SQL 操作在 `internal/mcp/server.go` 中经过以下管道：

1. **安全检查**（`security.SQLGuard`）— 长度/编码/控制字符检查 → 多语句检测 → 危险关键字拦截
2. **权限校验**（`permission.Checker`）— 数据库白名单 → 表黑名单 → 操作白名单 + 只读模式检查
3. **数据库驱动**（`database.DatabaseDriver`）— 执行查询，30 秒超时；写操作自动包装在事务中，失败自动回滚
4. **审计日志**（`logger.AuditLogger`）— 记录操作至 SQLite，含 DSN 脱敏和高危标记

Redis 命令走独立管道：安全检查 → 命令白名单校验（`permission.Checker.CheckRedisCommand`）→ key 黑名单校验 → `RedisDriver.ExecResult` 返回格式化结果 → 审计日志。

### 配置

配置文件位于 `~/.dbmcp/config.yaml`，使用按类型分组的配置结构：

```yaml
database_groups:
  relational:
    mysql:
      driver: mysql
      host: localhost
      port: 3306
      username: root
      password: ""
      database: ""
      options:
        parseTime: "true"
    postgres:
      driver: postgres
      host: localhost
      port: 5432
      username: postgres
      password: ""
      database: postgres
      options:
        sslmode: disable

  nosql:
    local_redis:
      driver: redis
      host: localhost
      port: 6379
      password: ""
      options:
        db: "0"

permissions_groups:
  relational:
    read_only: false
    allowed_databases: ["*"]
    allowed_actions:
      - SELECT
      - INSERT
      - UPDATE
      - DELETE
  nosql:
    read_only: false
    allowed_commands:
      - GET
      - SET
      - HGET
      - HGETALL
      - HSET
      - LPUSH
      - LRANGE
      - SCAN
      - INFO
      - DEL
      - EXISTS
      - TTL
      - TYPE
      - PING
    blocked_keys: []
```

旧格式（扁平 `databases` map）仍然有效，加载时会自动迁移到分组结构。

### 配置热重载

`config/watcher.go` 监听配置文件所在目录（非文件本身），处理 Write/Create/Rename 事件。变更时：
1. 加载并校验新配置
2. 通过 `AppState.UpdateConfig()` 原子替换
3. 通过 `DriverManager.SyncFromConfig()` 同步数据库连接（旧连接 30 秒后优雅关闭）
4. 通过 `permission.Checker.Update()` 更新权限

校验失败时保留旧配置。

### 事务支持

`DatabaseDriver` 接口提供 `BeginTx`/`Commit`/`Rollback` 方法。写操作（`Exec`）默认使用单语句事务，失败自动回滚。多语句场景可通过 `begin_tx` → 多次 `execute_update` → `commit`/`rollback` 控制。

### 安全模块

`security/sql_guard.go` 支持 PostgreSQL `$$...$$` dollar-quoted strings，`$$` 内的 `;` 不被误判为多语句。

### Redis 支持

Redis 驱动（`database/redis.go`）实现 `DatabaseDriver` 接口，提供以下能力：

- **命令执行** — `redis_command` 工具接收文本命令（如 `GET mykey`），通过 `ExecResult` 返回格式化结果
- **返回格式** — `ExecResult` 将 Redis 原始响应转为可读文本：
  - 字符串 → 直接返回（`"hello"`）
  - 整数 → `(integer) 42`
  - 数组 → 编号列表（`1) a\n2) b`）
  - Hash 键值对 → `key: value` 格式
  - nil → `(nil)`
- **权限控制** — 命令白名单 + key 黑名单（通配符匹配）+ read_only 模式
- **逻辑数据库** — 通过 `db` 参数切换 0-15
- **不支持** — SQL 查询、事务、EVAL/SCRIPT 脚本、SUBSCRIBE 订阅

### 添加新数据库驱动

实现 `internal/database/interface.go` 中的 `DatabaseDriver` 接口（含事务方法），然后在 `manager.go` 的 `createDriver` 工厂函数中注册。

### 模块依赖

```
cmd/dbmcp/main.go (入口)
  └── config → database → permission → security → logger → mcp
```

无循环依赖。各模块仅导入 `config` 包的共享类型。

## 技术栈

Go 1.26, mark3labs/mcp-go, go-sql-driver/mysql, jackc/pgx/v5, modernc.org/sqlite, github.com/redis/go-redis/v9, fsnotify, yaml.v3
