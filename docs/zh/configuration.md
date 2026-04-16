# dbmcp 配置操作指南

> 🇺🇸 [English Version](../en/configuration.md)

## 配置文件

默认路径：`~/.dbmcp/config.yaml`（可通过 `--config` 指定）

## 配置结构

```yaml
databases:          # 数据库连接配置（支持多个）
  <name>:
    driver: mysql|postgres|sqlite
    # ... 连接信息（见下方）

permissions:        # 权限控制
  read_only: false
  allowed_databases: ["*"]
  allowed_actions: [SELECT, INSERT, UPDATE, DELETE, CREATE, DROP]
  blocked_tables: []
```

## 数据库连接

支持两种配置风格，可混用：

### 方式一：结构化字段（推荐）

```yaml
databases:
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
| `driver` | string | 是 | 数据库类型：`mysql` / `postgres` / `sqlite` |
| `host` | string | 否* | 主机地址，默认 `localhost` |
| `port` | int | 否* | 端口号，默认 MySQL:3306 / PG:5432 |
| `username` | string | 否* | 用户名，默认 MySQL:root / PG:postgres |
| `password` | string | 否 | 密码 |
| `database` | string | 否 | 数据库名，默认 PG:postgres |
| `options` | map | 否 | 连接参数键值对 |

> *`dsn` 为空时，至少需要 `host` 存在才能使用结构化字段。

### 方式二：DSN 字符串

```yaml
databases:
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
databases:
  sqlite_file:
    driver: sqlite
    host: "/path/to/database.db"  # host 作为文件路径

  sqlite_memory:
    driver: sqlite                # 无 host/dsn 时使用内存库
```

## 多数据库连接

可同时配置多个数据库，通过名称区分：

```yaml
databases:
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

  sqlite_test:
    driver: sqlite
    host: "/tmp/test.db"
```

使用时通过 `database` 参数指定目标：

```
list_tables(database=pg_local)
execute_query(database=mysql_prod, sql="SELECT * FROM users")
```

## 权限控制

```yaml
permissions:
  read_only: false           # 全局只读模式
  allowed_databases: ["*"]   # 允许操作的数据库（* 表示全部）
  allowed_actions:           # 允许的操作类型
    - SELECT
    - INSERT
    - UPDATE
    - DELETE
    - CREATE
    - DROP
  blocked_tables:            # 表黑名单
    - sensitive_data
    - secrets
```

### 权限校验顺序

1. 数据库是否在 `allowed_databases` 中
2. 表是否在 `blocked_tables` 中
3. 操作类型是否在 `allowed_actions` 中
4. `read_only: true` 时拒绝所有非 SELECT 操作

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

| 类别 | 计划支持 |
|------|----------|
| 时序数据库 | InfluxDB, TDengine, Prometheus |
| 图数据库 | Neo4j, NebulaGraph |
| 国产数据库 | 达梦(DM), 人大金仓(KingBase), OceanBase, TiDB |
| NoSQL | Redis, MongoDB |
| 云数据库 | Snowflake, ClickHouse |

> 详见 [接入指南](getting-started.md#未来规划) 了解如何扩展新驱动。
