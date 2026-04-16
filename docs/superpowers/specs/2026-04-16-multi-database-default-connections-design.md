---
title: Multi-Database Default Connections Design
date: 2026-04-16
status: approved
---

# Multi-Database Default Connections

## 1. 目标

在 `~/.dbmcp/config.yaml` 中预置多种数据库类型和多个版本的默认连接配置，使 MCP Server 启动时自动建立所有可用连接。

## 2. 配置设计

### 2.1 命名规范

`<driver><version>` 格式：

| 名称 | 类型 | 版本 | 默认端口 |
|------|------|------|---------|
| `mysql57` | MySQL | 5.7 | 5370 |
| `mysql80` | MySQL | 8.0 | 8306 |
| `pg13` | PostgreSQL | 13 | 6432 |
| `pg15` | PostgreSQL | 15 | 6433 |
| `sqlite` | SQLite | latest | N/A |

### 2.2 配置文件

`~/.dbmcp/config.yaml`：

```yaml
databases:
  mysql57:
    driver: mysql
    dsn: "root:root@tcp(localhost:5370)/?parseTime=true"
  mysql80:
    driver: mysql
    dsn: "root:root@tcp(localhost:8306)/?parseTime=true"
  pg13:
    driver: postgres
    dsn: "postgres://postgres:postgres@localhost:6432/postgres?sslmode=disable"
  pg15:
    driver: postgres
    dsn: "postgres://postgres:postgres@localhost:6433/postgres?sslmode=disable"
  sqlite:
    driver: sqlite
    dsn: "/tmp/test.db"

permissions:
  read_only: false
  allowed_databases: ["*"]
  allowed_actions: [SELECT, INSERT, UPDATE, DELETE, CREATE, DROP]
  blocked_tables: []
```

## 3. 行为

- 启动时遍历 `databases` 中所有条目，逐个注册连接
- 连接失败的数据库记录警告日志，不阻塞其他数据库
- `list_databases` 只显示成功连接的数据库
- 通过 `database` 参数指定目标数据库（如 `mysql57`、`pg15`）

## 4. 不需要代码变更

项目已支持多数据库配置（`DriverManager.Register` 遍历 config map），只需更新配置文件即可。
