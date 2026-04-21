# 设计文档: MySQL base64 修复与默认配置自动生成

**日期**: 2026-04-21
**问题**:
1. MySQL 查询返回 base64 编码字符串而非可读文本
2. 配置文件不存在时程序直接退出，不会自动生成默认配置

---

## 问题 1: MySQL 查询返回 base64

### 根因分析

`internal/database/mysql.go` 的 `Query` 方法使用 `[]interface{}` 接收行数据（第 54-58 行）：

```go
values := make([]interface{}, len(columns))
valuePtrs := make([]interface{}, len(columns))
for i := range values {
    valuePtrs[i] = &values[i]
}
```

`database/sql` 的 `rows.Scan` 会将 MySQL 的字符串/二进制列（VARCHAR、TEXT、BLOB 等）扫描为 `[]byte`。Go 的 `encoding/json` 包在 `json.MarshalIndent` 时自动将 `[]byte` 编码为 base64（这是 Go 标准行为）。

调用链：`MySQLDriver.Query` → `server.go:queryResultToText` → `json.MarshalIndent` → base64 输出。

### 修复方案

在 `MySQLDriver.Query` 中，`rows.Scan` 之后、追加到 `result.Rows` 之前，遍历 `values` 数组，将 `[]byte` 转为 `string`：

```go
func convertBytes(values []interface{}) []interface{} {
    for i, v := range values {
        if b, ok := v.([]byte); ok {
            values[i] = string(b)
        }
    }
    return values
}
```

### 影响范围

PostgreSQL（`postgres.go`）和 MSSQL（`mssql.go`）使用相同的数据接收模式，同样受影响。一并修复三个驱动以保持一致性。

SQLite 使用纯 Go 驱动 `modernc.org/sqlite`，行为不同（直接返回 string），不需要修改。

---

## 问题 2: 配置文件不存在时不自动生成

### 根因分析

`cmd/dbmcp/main.go:34-36`：

```go
if _, err := os.Stat(*configPath); os.IsNotExist(err) {
    log.Fatalf("config file not found: %s\nRun with --config to specify a custom path.", *configPath)
}
```

直接退出，没有任何 fallback 逻辑。

### 修复方案

**`internal/config/config.go`** — 新增 `GenerateDefaultConfig(path string) error` 函数：

1. 使用 `os.MkdirAll` 创建配置目录（`~/.dbmcp/`）
2. 构建默认 `AppConfig`，包含：
   - MySQL 占位配置（`my_mysql`，host localhost:3306）
   - PostgreSQL 占位配置（`my_postgres`，host localhost:5432）
   - Redis 占位配置（`my_redis`，host localhost:6379）
   - 对应 per-database 权限（默认全开放，供本地开发使用）
3. `yaml.Marshal` 并写入文件

**`cmd/dbmcp/main.go`** — 修改启动逻辑：

1. 检测到配置文件不存在时，调用 `GenerateDefaultConfig`
2. 打印提示信息：配置已生成，请编辑后重新启动
3. 调用 `os.Exit(0)` 正常退出（不启动 MCP Server，因为默认配置的密码都是空的）

### 用户体验

首次运行：
```
Config file not found: /home/user/.dbmcp/config.yaml
Generating default config file...
Default config created: /home/user/.dbmcp/config.yaml
Please edit the config file with your database credentials, then restart dbmcp.
```

编辑配置后重新启动即可正常工作。

---

## 变更文件清单

| 文件 | 变更 |
|------|------|
| `internal/database/mysql.go` | 新增 `convertBytes` 函数，`Query` 中调用 |
| `internal/database/postgres.go` | 同上 |
| `internal/database/mssql.go` | 同上 |
| `internal/config/config.go` | 新增 `GenerateDefaultConfig` 函数 |
| `cmd/dbmcp/main.go` | 替换 `log.Fatalf` 为调用 `GenerateDefaultConfig` |
