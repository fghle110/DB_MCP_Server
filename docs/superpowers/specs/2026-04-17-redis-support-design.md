# Redis 支持设计文档

**日期**: 2026-04-17  
**作者**: Claude Code  
**状态**: 待审核

---

## 目标

为 dbmcp 增加 Redis 支持，同时重构配置结构以支持多种数据库类型（关系型、NoSQL、时序、图数据库），为未来扩展预留架构空间。

## 核心决策

1. **配置按类型分组** — `databases.relational`/`nosql`/`timeseries`/`graph`
2. **命令级权限白名单** — Redis 命令白名单 + key 黑名单 + read_only 模式
3. **工具层面动态切换 Redis db** — `redis_command` 增加 `db` 参数（0-15）
4. **通用命令工具为主** — `redis_command` 是核心，辅以 `redis_scan`/`redis_info`/`redis_describe`
5. **向后兼容** — 旧格式自动迁移，打印警告

---

## 架构

### 配置结构

```yaml
databases:
  relational:
    mysql_prod:
      driver: mysql
      host: ...
    pg_analytics:
      driver: postgres
      host: ...
  
  nosql:
    myredis:
      driver: redis
      host: localhost
      port: 6379
      username: ""       # Redis 6.0+ ACL 用户名（可选）
      password: ""       # 密码验证（必需时填写）
      options:
        ssl: "false"
        db: "0"          # 默认逻辑数据库
  
  timeseries:       # 预留
  graph:           # 预留

permissions:
  relational:
    read_only: false
    allowed_databases: ["*"]
    blocked_tables: []
    allowed_actions: ["SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "DROP"]
  
  nosql:
    read_only: false
    allowed_commands: ["GET", "SET", "HGET", "HGETALL", "HSET", "LPUSH", "LRANGE", "SADD", "SMEMBERS", "SCAN", "INFO", "DEL", "EXISTS", "TTL", "TYPE", "PING", "ECHO", "DBSIZE", "KEYS"]
    blocked_keys: []
  
  timeseries:       # 预留
  graph:           # 预留
```

### Redis 驱动实现

文件：`internal/database/redis.go`

实现 `DatabaseDriver` 接口：

- `Connect(dsn string)` — 使用 `github.com/redis/go-redis/v9`，DSN 格式 `redis://username:password@host:port/0`
  - 密码验证：通过 DSN 中的 password 字段，连接时自动执行 `AUTH` 命令
  - Redis 6.0+ ACL：支持 `username:password` 格式
  - 无密码时跳过 AUTH（兼容 `requirepass no` 的情况）
  - 连接失败时返回明确错误："Redis authentication failed"
- `Query(ctx, sql)` — 返回错误 "Redis does not support SQL queries. Use redis_command tool."
- `Exec(ctx, cmd)` — 解析 Redis 命令文本，调用 `go-redis` 执行
- `ListDatabases(ctx)` — 返回 `["Redis (16 logical databases)"]`
- `ListTables(ctx, database)` — 使用 `SCAN` 返回当前 db 的 key 列表
- `DescribeTable(ctx, database, table)` — `TYPE` + `TTL` + `GET`/`HGETALL` 查看 key 内容
- `BeginTx`/`Commit`/`Rollback` — Redis MULTI/EXEC 语义不同，返回空操作或错误提示
- `Close()` — 关闭 Redis 连接

**命令执行细节**：

`Exec` 接收命令文本如 `"GET user:123"`，用 `strings.Fields` 分割：
- 第一个词是命令名（大写），用于路由到 `go-redis` 的对应方法
- 剩余词是参数
- 不支持的参数类型（如 Lua 脚本、管道）返回错误

### 权限系统

文件：`internal/permission/permission.go`

新增 `NosqlPermissionConfig`：

```go
type NosqlPermissionConfig struct {
    ReadOnly        bool     `yaml:"read_only"`
    AllowedCommands []string `yaml:"allowed_commands"`
    BlockedKeys     []string `yaml:"blocked_keys"` // 通配符匹配，使用 filepath.Match
}
```

新增方法：

```go
func (c *Checker) CheckRedisCommand(cmd string, key string) error
```

校验逻辑：
1. 检查 `read_only` — 写命令被拦截。写命令完整列表：`SET`, `SETNX`, `SETEX`, `MSET`, `MSETNX`, `GETSET`, `APPEND`, `INCR`, `DECR`, `INCRBY`, `DECRBY`, `DEL`, `UNLINK`, `HSET`, `HSETNX`, `HMSET`, `HDEL`, `LPUSH`, `RPUSH`, `LPOP`, `RPOP`, `LSET`, `LINSERT`, `SADD`, `SREM`, `SPOP`, `SMOVE`, `ZADD`, `ZREM`, `ZINCRBY`, `FLUSHDB`, `FLUSHALL`, `EVAL`, `EVALSHA`
2. 检查 `cmd` 是否在 `allowed_commands` 中
3. 检查 `key` 是否匹配 `blocked_keys` 中的任意模式（使用 `filepath.Match`）

### MCP 工具

文件：`internal/mcp/server.go`

| 工具 | 参数 | 说明 |
|------|------|------|
| `redis_command` | `database`(必填), `cmd`(必填), `db`(可选, 默认0) | 执行 Redis 命令 |
| `redis_scan` | `database`(必填), `pattern`(可选, 默认`*`), `limit`(可选, 默认50), `db`(可选) | 安全扫描 key |
| `redis_info` | `database`(必填), `section`(可选), `db`(可选) | 服务器状态 |
| `redis_describe` | `database`(必填), `key`(必填), `db`(可选) | 查看 key 的类型/TTL/值 |

**`redis_command` 处理流程**：

```
用户输入 cmd → 安全检查(长度/控制字符) → 命令白名单校验(cmd in allowed_commands) → key 黑名单校验 → 执行 → 审计日志
```

### 模块变更清单

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `internal/config/config.go` | 修改 | 新增 `DatabaseGroup` 结构，支持按类型分组 |
| `internal/config/config_test.go` | 修改 | 更新测试 |
| `internal/database/redis.go` | 新增 | Redis 驱动实现 |
| `internal/database/redis_test.go` | 新增 | Redis 驱动测试 |
| `internal/database/manager.go` | 修改 | `createDriver` 增加 redis 分支，`buildDSN` 增加 Redis 分支 |
| `internal/permission/permission.go` | 修改 | 新增 Redis 权限校验 |
| `internal/permission/permission_test.go` | 修改 | 新增测试 |
| `internal/security/sql_guard.go` | 修改 | 新增 `CheckRedisCommand` 辅助方法 |
| `internal/mcp/server.go` | 修改 | 新增 4 个 Redis 工具 handler |
| `cmd/dbmcp/main.go` | 检查 | 确认无需修改 |
| `go.mod` / `go.sum` | 修改 | 新增 `github.com/redis/go-redis/v9` 依赖 |

### 向后兼容

配置加载时检测旧格式（`databases` 是 flat map，value 有 `driver` 字段），自动归类：

- `mysql`/`postgres`/`sqlite`/`mssql` → `relational`
- `redis` → `nosql`
- 未知类型 → 打印警告，放入 `nosql` 分组

旧格式仍然有效，但会打印迁移建议日志。

### 未来扩展点

- **时序数据库** — 新增 `timeseries` 分组，驱动实现 `DatabaseDriver` 接口，专用查询语法
- **图数据库** — 新增 `graph` 分组，驱动实现 `DatabaseDriver` 接口，Cypher/Gremlin 语法
- 当前配置结构已预留 `timeseries`/`graph` 分组
