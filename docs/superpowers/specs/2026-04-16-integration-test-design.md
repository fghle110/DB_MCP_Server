---
title: dbmcp 集成测试设计
date: 2026-04-16
status: draft
---

# 集成测试设计

## 1. 概述

为 dbmcp 添加集成测试,验证三种数据库驱动(MySQL、PostgreSQL、SQLite)的实际连接和操作方法,以及 MCP Server 端到端调用流程。测试使用 testcontainers-go 自动启停 Docker 容器,`go test ./...` 一键运行。

## 2. 测试范围

### 2.1 数据库驱动集成测试

每种数据库(MySQL、PostgreSQL、SQLite)运行以下测试:

| 测试 | 验证内容 |
|------|----------|
| `TestConnect` | 建立连接并 ping 成功 |
| `TestCreateTable` | 执行 CREATE TABLE |
| `TestInsertAndSelect` | INSERT 数据后 SELECT 返回正确结果 |
| `TestListDatabases` | 列出数据库名称 |
| `TestListTables` | 列出创建的表 |
| `TestDescribeTable` | 返回正确的字段信息 |
| `TestDropTable` | 执行 DROP TABLE |
| `TestExecRowsAffected` | 返回正确的影响行数 |

### 2.2 MCP Server 集成测试

| 测试 | 验证内容 |
|------|----------|
| `TestListDatabasesTool` | 调用 `list_databases` 返回数据库列表 |
| `TestListTablesTool` | 调用 `list_tables` 返回表列表 |
| `TestDescribeTableTool` | 调用 `describe_table` 返回表结构 |
| `TestExecuteQueryTool` | 调用 `execute_query` 执行 SELECT |
| `TestExecuteUpdateTool` | 调用 `execute_update` 执行 CREATE/INSERT |
| `TestConfigStatusTool` | 调用 `config_status` 返回配置信息 |

## 3. 技术选型

| 组件 | 选择 | 理由 |
|------|------|------|
| 容器管理 | testcontainers-go | Go 原生支持,测试自包含 |
| MySQL 容器 | testcontainers/modules/mysql | 官方模块,配置简单 |
| PostgreSQL 容器 | testcontainers/modules/postgres | 官方模块 |
| SQLite | 内存数据库(`file::memory:`) | 无需容器 |

## 4. 目录结构

```
dbmcp/
├── internal/
│   ├── database/
│   │   ├── mysql_test.go          # MySQL 集成测试
│   │   ├── postgres_test.go       # PostgreSQL 集成测试
│   │   └── sqlite_test.go         # SQLite 集成测试
│   └── mcp/
│       └── server_test.go         # MCP Server 集成测试
└── testhelpers/
    └── dbsetup.go                 # 共享工具:创建测试用配置、清理资源
```

## 5. 实现细节

### 5.1 数据库测试模式

每种数据库测试遵循相同模式:

```go
func TestXxx_Connect(t *testing.T) {
    // 1. 启动容器 (MySQL/PostgreSQL) 或创建内存库 (SQLite)
    // 2. 创建驱动实例并 Connect
    // 3. 验证无错误
    // 4. 清理 (容器自动停止)
}
```

### 5.2 MySQL 容器

```go
ctr, err := mysql.Run(ctx, "mysql:8.0",
    mysql.WithDatabase("testdb"),
    mysql.WithUsername("test"),
    mysql.WithPassword("test"),
)
dsn := mustConnectionString(ctr)
// → "test:test@tcp(host:port)/testdb?parseTime=true"
```

### 5.3 PostgreSQL 容器

```go
ctr, err := postgres.Run(ctx, "postgres:16-alpine",
    postgres.WithDatabase("testdb"),
    postgres.WithUsername("test"),
    postgres.WithPassword("test"),
)
dsn := mustConnectionString(ctr)
// → "postgres://test:test@host:port/testdb?sslmode=disable"
```

### 5.4 SQLite 内存库

```go
dsn := "file:testdb?mode=memory&cache=shared"
```

### 5.5 MCP Server 测试

```go
func TestMCP_ExecuteQuery(t *testing.T) {
    // 1. 启动 MySQL 容器
    // 2. 创建 DriverManager, 注册数据库
    // 3. 创建 MCP Server (含 security, permission, logger)
    // 4. 通过 CallToolRequest 调用 execute_query
    // 5. 验证返回结果
}
```

## 6. 跳过机制

如果 Docker 不可用,集成测试自动跳过而非失败:

```go
func TestMySQL_Integration(t *testing.T) {
    if !dockerAvailable() {
        t.Skip("Docker not available, skipping integration test")
    }
    // ...
}
```

## 7. 新增依赖

```
github.com/testcontainers/testcontainers-go
github.com/testcontainers/testcontainers-go/modules/mysql
github.com/testcontainers/testcontainers-go/modules/postgres
```
