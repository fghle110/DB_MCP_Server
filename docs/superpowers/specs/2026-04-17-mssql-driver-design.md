# MSSQL Server 驱动设计

**日期**: 2026-04-17
**状态**: 待实现

## 概述

为 dbmcp 添加 Microsoft SQL Server（MSSQL）支持，作为第四个数据库驱动。采用渐进式实现，第一阶段聚焦基础 CRUD 功能，与现有 MySQL/PG/SQLite 驱动能力对齐。

## 技术选型

| 项目 | 选择 |
|------|------|
| Go 驱动 | `github.com/microsoft/go-mssqldb`（微软官方，纯 Go） |
| 驱动注册名 | `sqlserver` |
| 默认端口 | 1433 |
| DSN 格式 | `sqlserver://user:pass@host:port?database=dbname&options` |

## 架构

### 文件结构

```
internal/database/
├── mssql.go         ← 新增：MSSQLDriver 实现
├── manager.go       ← 修改：+1 case in createDriver, +1 case in buildDSN
├── interface.go     ← 不变
└── mysql.go / postgres.go / sqlite.go  ← 不变
```

### MSSQLDriver 结构体

```go
type MSSQLDriver struct {
    db *sql.DB
    tx *sql.Tx
}
```

与 MySQL/PG 驱动完全一致：`db` 存储连接池，`tx` 存储当前事务。

### 实现的接口方法

| 方法 | 实现说明 |
|------|----------|
| `Connect(dsn string)` | `sql.Open("sqlserver", dsn)` + Ping 验证 |
| `Query(ctx, sql)` | 标准 `db.QueryContext`，结果扫描为 `QueryResult` |
| `Exec(ctx, sql)` | 事务包装：`execInTx` + `execSingle`，与 MySQL 模式一致 |
| `ListDatabases(ctx)` | `SELECT name FROM sys.databases WHERE state_desc = 'ONLINE'` |
| `ListTables(ctx, db)` | `SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_TYPE = 'BASE TABLE' AND TABLE_CATALOG = N'db'` |
| `DescribeTable(ctx, db, table)` | `INFORMATION_SCHEMA.COLUMNS` 联合 `KEY_COLUMN_USAGE` 查询主键信息 |
| `BeginTx(ctx)` | `db.BeginTx(ctx, nil)` 存入 `d.tx` |
| `Commit()` | `tx.Commit()` 并清空 `d.tx` |
| `Rollback()` | `tx.Rollback()` 并清空 `d.tx` |
| `Close()` | 关闭 `db`，回滚未提交事务 |

### MSSQL 特有 SQL 模板

**DescribeTable**（主键联合查询）：

```sql
SELECT c.COLUMN_NAME, c.DATA_TYPE, c.IS_NULLABLE,
    CASE WHEN pk.COLUMN_NAME IS NOT NULL THEN 'PRI' ELSE '' END AS COLUMN_KEY
FROM INFORMATION_SCHEMA.COLUMNS c
LEFT JOIN (
    SELECT ku.TABLE_CATALOG, ku.TABLE_NAME, ku.COLUMN_NAME
    FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
    JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE ku ON tc.CONSTRAINT_NAME = ku.CONSTRAINT_NAME
    WHERE tc.CONSTRAINT_TYPE = 'PRIMARY KEY'
) pk ON c.TABLE_CATALOG = pk.TABLE_CATALOG AND c.TABLE_NAME = pk.TABLE_NAME AND c.COLUMN_NAME = pk.COLUMN_NAME
WHERE c.TABLE_CATALOG = N'dbname' AND c.TABLE_NAME = N'tablename'
ORDER BY c.ORDINAL_POSITION
```

### 标识符处理

MSSQL 使用方括号 `[]` 包裹标识符。数据库名和表名在 SQL 模板中使用 `N'...'` Unicode 字符串前缀，支持中文库名/表名。

## DSN 构建

### buildMSSQLDSN

从 `config.DatabaseConfig` 构建 MSSQL DSN：

- 默认 host: `localhost`
- 默认 port: `1433`
- 默认 username: `sa`
- 默认 database: `master`
- options 参数通过 URL query 追加到 DSN

### manager.go 修改

`createDriver` 新增 case：
```go
case "mssql", "sqlserver":
    return NewMSSQLDriver(), nil
```

`buildDSN` 新增 case：
```go
case "mssql", "sqlserver":
    return buildMSSQLDSN(cfg)
```

## 集成测试

新增 `internal/database/mssql_test.go`，8 个测试用例：

| 测试 | 说明 |
|------|------|
| `TestMSSQLConnect` | 连接验证和 Ping |
| `TestMSSQLQuery` | SELECT 查询 |
| `TestMSSQLExec` | INSERT/UPDATE/DELETE |
| `TestMSSQLTransaction` | 完整事务流程 |
| `TestMSSQLTransactionRollback` | 事务回滚 |
| `TestMSSQLListDatabases` | 数据库列表 |
| `TestMSSQLListTables` | 表列表 |
| `TestMSSQLDescribeTable` | 表结构 |

使用 Docker 容器 `mcr.microsoft.com/mssql/server:2022-latest`，Docker 不可用时自动跳过。

## 错误处理与边缘情况

1. **连接超时**：通过 `connection timeout` 选项控制，默认 30 秒
2. **Unicode 兼容**：元数据查询使用 `N'...'` 前缀支持中文标识符
3. **空结果**：`ListTables` 在空库中返回空切片（非 nil）
4. **SQL 安全检查**：系统视图查询不受 SQLGuard 拦截

## 配置示例

```yaml
databases:
  my_mssql:
    driver: mssql
    host: 192.168.1.100
    port: 1433
    username: sa
    password: "YourPassword123"
    database: AdventureWorks
    options:
      encrypt: "true"
      connection timeout: "30"
```

## 范围说明

本设计仅包含第一阶段基础功能（CRUD + 元数据查询 + 事务）。高级特性（存储过程、Windows 认证、AlwaysOn）不在本设计范围内，后续另行设计。
