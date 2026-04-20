# 数据库支持

dbmcp 通过统一的 `DatabaseDriver` 接口支持多种数据库，每种数据库在配置文件中通过 `driver` 字段区分。

## 支持的数据库

| 数据库 | driver 值 | 默认端口 | 依赖库 | 构建标签 |
|--------|-----------|----------|--------|----------|
| MySQL | `mysql` | 3306 | `github.com/go-sql-driver/mysql` | 无 |
| PostgreSQL | `postgres`, `postgresql` | 5432 | `github.com/jackc/pgx/v5` | 无 |
| SQLite | `sqlite`, `sqlite3` | — | `modernc.org/sqlite` | 无 |
| SQL Server | `mssql`, `sqlserver` | 1433 | `github.com/microsoft/go-mssqldb` | 无 |
| Redis | `redis` | 6379 | `github.com/redis/go-redis/v9` | 无 |
| 达梦 DM | `dm`, `dmdbms`, `dameng` | 5236 | `third_party/dm/dm/`（本地替换） | `dm` |

> **达梦驱动**需要 `dm` 构建标签才能编译，CI 和 release 构建中默认排除。本地构建达梦版本时使用：`go build -tags dm -o build/dbmcp-dm ./cmd/dbmcp`。

## DSN 格式

各数据库的连接串格式如下。配置支持两种形式：结构化字段（host/port/username/password/database/options）或原始 `dsn` 字符串。

### MySQL

```
user:password@tcp(host:port)/database?parseTime=true&loc=Local
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| host | `localhost` | 服务器地址 |
| port | `3306` | 端口号 |
| username | `root` | 用户名 |
| password | — | 密码 |
| database | — | 数据库名（必填） |
| options | — | 额外连接参数 |

### PostgreSQL

```
postgres://user:password@host:port/database?sslmode=disable
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| host | `localhost` | 服务器地址 |
| port | `5432` | 端口号 |
| username | `postgres` | 用户名 |
| password | — | 密码 |
| database | `postgres` | 数据库名 |
| options | — | 额外连接参数（如 sslmode） |

### SQLite

```
/path/to/database.db  或  :memory:
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| dsn | — | 数据库文件路径 |
| host | — | 也可作为文件路径使用 |
| 默认 | `:memory:` | 未指定时使用内存数据库 |

SQLite 使用纯 Go 实现（`modernc.org/sqlite`），无需 CGO。

### SQL Server

```
sqlserver://user:password@host:port?database=dbname&encrypt=disable
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| host | `localhost` | 服务器地址 |
| port | `1433` | 端口号 |
| username | `sa` | 用户名 |
| password | — | 密码 |
| database | `master` | 数据库名 |
| options | — | 额外连接参数 |

### Redis

```
redis://user:password@host:port/db
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| host | `localhost` | 服务器地址 |
| port | `6379` | 端口号 |
| password | — | 密码 |
| options.db | `0` | 逻辑数据库（0-15） |

### 达梦 DM

```
dm://user:password@host:port?autoCommit=0&schema=模式名
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| host | `localhost` | 服务器地址 |
| port | `5236` | 端口号 |
| username | `SYSDBA` | 用户名 |
| password | — | 密码 |
| options.autoCommit | `0` | 自动提交（0=关闭，支持事务） |
| options.schema | — | 默认模式名 |

> 达梦默认启用 `autocommit`，为支持事务，驱动自动设置 `autoCommit=0`（除非配置中已显式指定）。

## 功能支持矩阵

| 功能 | MySQL | PostgreSQL | SQLite | SQL Server | Redis | 达梦 DM |
|------|:-----:|:----------:|:------:|:----------:|:-----:|:-------:|
| SQL 查询 | ✓ | ✓ | ✓ | ✓ | ✗ | ✓ |
| 写操作 | ✓ | ✓ | ✓ | ✓ | ✓* | ✓ |
| 事务控制 | ✓ | ✓ | ✓ | ✓ | ✗ | ✓ |
| 参数化查询 | ✓ | ✓ | ✓ | ✓ | ✗ | ✓** |
| 列出数据库 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| 列出表 | ✓ | ✓ | ✓ | ✓ | ✓*** | ✓ |
| 表结构查询 | ✓ | ✓ | ✓ | ✓ | ✓**** | ✓ |

- ✓* Redis 写操作通过 `redis_command` 工具执行，不走 `execute_update`
- ✓** 达梦的参数化查询通过 `QueryWithParams` 方法实现，需使用 `execute_param_query` 工具
- ✓*** Redis 列出表返回的是 key 列表（使用 SCAN）
- ✓**** Redis 表结构返回 key 的类型/TTL

## 事务支持

关系型数据库（MySQL、PostgreSQL、SQLite、SQL Server、达梦）均通过 `DatabaseDriver` 接口的 `BeginTx`/`Commit`/`Rollback` 方法支持事务。

### 单语句写操作

`Exec` 方法默认将单条写操作包装在事务中执行，失败自动回滚：

```
开始事务 → 执行 SQL → 提交/回滚
```

### 多语句事务

通过 MCP 工具手动控制：

```
begin_tx → execute_update (多次) → commit / rollback
```

### Redis 事务

Redis 不支持 SQL 风格事务。`BeginTx`/`Commit`/`Rollback` 均返回错误提示。如需 Redis 原生事务，直接使用 `MULTI`/`EXEC` 命令通过 `redis_command` 工具操作。

### 达梦事务特殊性

达梦驱动在事务模式下使用专用连接（`conn.ExecContext`）而非 `tx.ExecContext`，这是因为达梦的 `database/sql` 事务抽象在某些版本中存在兼容性问题。`BeginTx` 获取专用连接，`Commit`/`Rollback` 后自动释放连接。

## 配置示例

```yaml
database_groups:
  relational:
    prod_mysql:
      driver: mysql
      host: 192.168.1.100
      port: 3306
      username: app_user
      password: "secure_password"
      database: production_db
      options:
        parseTime: "true"
        loc: Local

    pg_analytics:
      driver: postgres
      host: 192.168.1.101
      port: 5432
      username: analyst
      password: "secure_password"
      database: analytics
      options:
        sslmode: require

    local_cache:
      driver: sqlite
      host: /var/lib/dbmcp/cache.db

    sqlserver:
      driver: mssql
      host: 192.168.1.102
      port: 1433
      username: sa
      password: "SecurePass123"
      database: reports
      options:
        encrypt: disable

    dm_prod:
      driver: dm
      host: 192.168.1.103
      port: 5236
      username: dbmcp
      password: "dm_password"
      options:
        autoCommit: "0"
        schema: "dbmcp"

  nosql:
    local_redis:
      driver: redis
      host: localhost
      port: 6379
      password: ""
      options:
        db: "0"
```

## 驱动架构

所有驱动实现统一的 `DatabaseDriver` 接口：

```go
type DatabaseDriver interface {
    Connect(dsn string) error
    Query(ctx context.Context, sql string) (*QueryResult, error)
    Exec(ctx context.Context, sql string) (int64, error)
    ListDatabases(ctx context.Context) ([]string, error)
    ListTables(ctx context.Context, database string) ([]string, error)
    DescribeTable(ctx context.Context, database, table string) ([]Column, error)
    Close() error
    // 事务
    BeginTx(ctx context.Context) error
    Commit() error
    Rollback() error
}
```

`DriverManager` 负责驱动的创建、连接池管理、热重载同步和生命周期管理。新驱动类型只需：

1. 实现 `DatabaseDriver` 接口
2. 在 `manager.go` 的 `createDriver` 函数中注册类型
3. 在 `buildDSN` 函数中添加 DSN 组装逻辑

## 连接池配置

| 数据库 | MaxOpen | MaxIdle | MaxLifetime | 说明 |
|--------|---------|---------|-------------|------|
| MySQL | 10 | 5 | 5 分钟 | 常规连接池 |
| PostgreSQL | 10 | 5 | 5 分钟 | 常规连接池 |
| SQLite | 1 | 1 | 5 分钟 | 单连接（文件锁限制） |
| SQL Server | 1 | 1 | 5 分钟 | 单连接 |
| Redis | — | — | — | 由 go-redis 管理 |
| 达梦 DM | 10 | 5 | 5 分钟 | 常规连接池 |

## 安全与权限

所有数据库操作在执行前经过以下管道：

1. **SQL 注入防护** — 长度/编码检查 → 多语句检测（支持 PostgreSQL `$$...$$`）→ 危险关键字拦截
2. **权限校验** — 数据库白名单 → 表黑名单 → 操作白名单 + 只读模式
3. **审计日志** — 所有操作记录至本地 SQLite 审计库（`~/.dbmcp/audit.db`），DSN 自动脱敏

Redis 命令使用独立的权限管道：

1. 命令白名单校验（`permission.Checker.CheckRedisCommand`）
2. Key 黑名单校验（支持通配符匹配）
3. 只读模式检查

## 占位符差异

| 数据库 | 占位符 | 示例 |
|--------|--------|------|
| MySQL | `?` | `SELECT * FROM users WHERE id = ?` |
| PostgreSQL | `$1, $2, ...` | `SELECT * FROM users WHERE id = $1` |
| SQLite | `?` | `SELECT * FROM users WHERE id = ?` |
| SQL Server | `@p1, @p2, ...` | `SELECT * FROM users WHERE id = @p1` |
| 达梦 DM | `?` | `SELECT * FROM users WHERE id = ?` |

参数化查询统一通过 `execute_param_query` MCP 工具执行。
