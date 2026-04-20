# dbmcp 配置操作指南

> 🇺🇸 [English Version](../en/configuration.md)

## 配置文件

默认路径：`~/.dbmcp/config.yaml`（可通过 `--config` 指定）

## 配置结构（新格式）

```yaml
database_groups:        # 按类型分组的数据库配置
  relational:           # 关系型数据库（MySQL/PostgreSQL/SQLite/MSSQL/达梦）
    <name>:
      driver: mysql|postgres|sqlite|mssql|dm
      # ... 连接信息（见下方）
  nosql:                # NoSQL 数据库（Redis 等）
    <name>:
      driver: redis
      # ... 连接信息

permissions_groups:     # 按类型分组的权限配置（per-database）
  relational:
    <name>:             # 数据库名称，与 database_groups 中的 key 对应
      read_only: false
      allowed_databases: ["*"]
      allowed_actions: [SELECT, INSERT, UPDATE, DELETE, CREATE, DROP]
      blocked_tables: []
  nosql:
    <name>:
      read_only: false
      allowed_commands: [GET, SET, ...]
      blocked_keys: []
```

> **旧格式兼容**：旧格式（扁平 `databases` + `permissions`）仍然有效，启动时会自动迁移到新格式并备份为 `config.yaml.bak`。

## 数据库连接

支持两种配置风格，可混用：

### 方式一：结构化字段（推荐）

```yaml
database_groups:
  relational:
    mysql_prod:
      driver: mysql
      host: prod-db.example.com
      port: 3306
      username: app_user
      password: "secure_password"
      database: myapp
      options:
        parseTime: "true"
        charset: utf8mb4

    pg_local:
      driver: postgres
      host: localhost
      port: 5432
      username: postgres
      password: ""
      database: postgres
      options:
        sslmode: disable
```

**支持的结构化字段：**

| 字段 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `driver` | string | 是 | 数据库类型：`mysql` / `postgres` / `sqlite` / `mssql` / `dm` / `redis` |
| `host` | string | 否* | 主机地址，默认 `localhost` |
| `port` | int | 否* | 端口号，默认 MySQL:3306 / PG:5432 / MSSQL:1433 / DM:5236 / Redis:6379 |
| `username` | string | 否* | 用户名 |
| `password` | string | 否 | 密码 |
| `database` | string | 否 | 数据库名 |
| `options` | map | 否 | 连接参数键值对 |

> *`dsn` 为空时，至少需要 `host` 存在才能使用结构化字段。

### 方式二：DSN 字符串

```yaml
database_groups:
  relational:
    mysql_dsn:
      driver: mysql
      dsn: "user:pass@tcp(localhost:3306)/mydb?parseTime=true"
    pg_dsn:
      driver: postgres
      dsn: "postgres://user:pass@localhost:5432/mydb?sslmode=disable"
    sqlite_dsn:
      driver: sqlite
      dsn: "/path/to/database.db"
```

### 优先级

如果同时配置了 `dsn` 和结构化字段，**DSN 优先**。

### SQLite 特殊处理

```yaml
database_groups:
  relational:
    sqlite_file:
      driver: sqlite
      host: "/path/to/database.db"  # host 作为文件路径

    sqlite_memory:
      driver: sqlite                # 无 host/dsn 时使用内存库
```

### 达梦数据库

达梦驱动需要构建时添加 `-tags dm` 参数：

```bash
go build -tags dm -o build/dbmcp.exe ./cmd/dbmcp
```

```yaml
database_groups:
  relational:
    dm_db:
      driver: dm
      host: localhost
      port: 5236
      username: SYSDBA
      password: SYSDBA
      database: dbmcp
      options:
        autoCommit: "0"
        schema: "dbmcp"
```

## 多数据库连接

可同时配置多个数据库，通过名称区分：

```yaml
database_groups:
  relational:
    mysql_prod:
      driver: mysql
      host: prod-db.example.com
      port: 3306
      username: root
      password: ""
      database: production

    pg_local:
      driver: postgres
      host: localhost
      port: 5432
      username: postgres
      password: "local_pass"
      database: dev_db

    dm_db:
      driver: dm
      host: localhost
      port: 5236
      username: SYSDBA
      password: SYSDBA
      database: dbmcp

  nosql:
    local_redis:
      driver: redis
      host: localhost
      port: 6379
```

使用时通过 `database` 参数指定目标：

```
list_tables(database=pg_local)
execute_query(database=mysql_prod, sql="SELECT * FROM users")
```

## 权限控制（Per-Database）

每个数据库独立配置权限：

```yaml
permissions_groups:
  relational:
    mysql_prod:
      read_only: true              # 只读模式
      allowed_databases: ["*"]     # 允许操作的数据库（* 表示全部）
      allowed_actions:             # 允许的操作类型
        - SELECT
      blocked_tables:              # 表黑名单
        - sensitive_data
    mysql_dev:
      read_only: false
      allowed_databases: ["*"]
      allowed_actions:
        - SELECT
        - INSERT
        - UPDATE
        - DELETE
        - CREATE
        - DROP
      blocked_tables: []
  nosql:
    local_redis:
      read_only: false
      allowed_commands:            # 命令白名单
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
      blocked_keys: []             # Key 黑名单（支持通配符）
```

### 权限校验顺序

**关系型数据库：**
1. 数据库是否在 `allowed_databases` 中
2. 表是否在 `blocked_tables` 中
3. 操作类型是否在 `allowed_actions` 中
4. `read_only: true` 时拒绝所有非 SELECT 操作

**Redis：**
1. 命令是否在 `allowed_commands` 中
2. Key 是否在 `blocked_keys` 中（支持通配符）
3. `read_only: true` 时拒绝所有写命令

### 旧格式自动迁移

旧格式配置（扁平 `databases` + `permissions`）启动时会自动：

1. **备份** — 原文件保存为 `config.yaml.bak`
2. **迁移** — `databases` → `database_groups`，`permissions` → 按数据库类型分配到 `permissions_groups`
3. **展开** — 旧格式单一权限对象自动展开为 per-database 配置
4. **写回** — 配置文件更新为新格式

```yaml
# 旧格式（仍然可用，会自动迁移）
databases:
  mydb:
    driver: mysql
    dsn: "user:pass@tcp(localhost:3306)/testdb"
permissions:
  read_only: true
  allowed_actions: [SELECT]
```

## 配置热重载

MCP Server 运行时会监听配置文件变更：

1. 编辑 `~/.dbmcp/config.yaml`
2. 自动检测并重载（监听目录级事件）
3. 校验失败时保留旧配置
4. 成功时原子替换（不影响正在执行的请求）

### 热重载触发条件

| 事件类型 | 触发 |
|----------|------|
| 文件写入 (Write) | ✓ |
| 文件创建 (Create) | ✓ |
| 文件重命名 (Rename) | ✓ |
| 文件删除 | ✗ |

### 热重载影响

| 组件 | 行为 |
|------|------|
| 数据库连接 | 新增连接自动注册，移除的连接 30 秒后优雅关闭 |
| 权限 | 立即原子替换，不影响正在执行的请求 |
| 安全策略 | 立即生效 |

## DSN 脱敏

审计日志中的 DSN 会自动脱敏密码：

```
原始: postgres://admin:secret123@localhost:5432/db
日志: postgres://admin:***@localhost:5432/db
```

## 高危操作标记

以下操作会在审计日志中标记为 `[HIGH_RISK]`：

| 类型 | 示例 |
|------|------|
| `DROP TABLE` / `DROP DATABASE` | 删除表/库 |
| `TRUNCATE` | 清空表 |
| `ALTER TABLE ... DROP` | 删除列/约束 |
| `DELETE/UPDATE ... WHERE 1=1` | 全表操作 |

## 空条目跳过

配置中允许存在注释掉或空的条目，不会被当作错误：

```yaml
databases:
  mysql_prod:
    driver: mysql
    host: prod.example.com
    port: 3306
    username: root
    password: ""
    database: ""

  # mysql_backup:          # 注释掉的条目会被跳过
  #   driver: mysql

  pg_disabled:             # 只有 driver 没有 host/dsn 也会跳过
    driver: postgres
```

校验要求：至少一个完整有效的数据库条目。

---

## 扩展更多数据库

dbmcp 的架构支持无限扩展新数据库类型，只需实现 `DatabaseDriver` 接口即可。

| 类别 | 已支持 | 计划支持 |
|------|--------|----------|
| 关系型 | MySQL, PostgreSQL, SQLite, SQL Server, **达梦(DM)** | 人大金仓(KingBase), OceanBase, TiDB |
| NoSQL | **Redis** | MongoDB |
| 时序数据库 | — | InfluxDB, TDengine, Prometheus |
| 图数据库 | — | Neo4j, NebulaGraph |
| 云数据库 | — | Snowflake, ClickHouse |

> 达梦构建需要 `go build -tags dm`。详见 [接入指南](getting-started.md) 了解如何扩展新驱动。
