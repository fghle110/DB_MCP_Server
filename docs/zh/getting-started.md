# dbmcp 接入指南

> 🇺🇸 [English Version](../en/getting-started.md)

## 环境要求

| 组件 | 版本 | 说明 |
|------|------|------|
| Go | 1.26+ | 运行时 |
| Docker（可选） | 最新版 | 运行 MySQL/PostgreSQL 集成测试 |

## 快速开始

### 1. 获取代码

```bash
git clone https://github.com/dbmcp/dbmcp.git
cd dbmcp
```

### 2. 安装依赖

```bash
go mod download
```

### 3. 构建

```bash
# Windows
go build -o build/dbmcp.exe ./cmd/dbmcp

# macOS / Linux
go build -o build/dbmcp ./cmd/dbmcp

# 跨平台构建
GOOS=linux GOARCH=amd64 go build -o build/dbmcp ./cmd/dbmcp
GOOS=darwin GOARCH=arm64 go build -o build/dbmcp-darwin ./cmd/dbmcp
```

> **⚠️ 达梦数据库构建**：如需包含达梦驱动，**必须添加 `-tags dm`** 参数：
>
> ```bash
> # Windows
> go build -tags dm -o build/dbmcp.exe ./cmd/dbmcp
>
> # Linux（跨平台）
> GOOS=linux GOARCH=amd64 go build -tags dm -o build/dbmcp ./cmd/dbmcp
> ```
>
> **注意**：达梦驱动仅支持 Linux amd64 平台，macOS/Darwin 不支持构建。

### 4. 配置

创建配置目录并编辑配置文件：

```bash
# Windows
mkdir %USERPROFILE%\.dbmcp
notepad %USERPROFILE%\.dbmcp\config.yaml

# macOS / Linux
mkdir -p ~/.dbmcp
nano ~/.dbmcp/config.yaml
```

最小配置示例（PostgreSQL）：

```yaml
databases:
  postgres:
    driver: postgres
    host: localhost
    port: 5432
    username: postgres
    password: "your_password"
    database: postgres
    options:
      sslmode: disable

permissions:
  read_only: false
  allowed_databases: ["*"]
  allowed_actions: [SELECT, INSERT, UPDATE, DELETE, CREATE, DROP]
  blocked_tables: []
```

> 详细配置说明见 [配置操作指南](configuration.md)

### 数据库配置示例

#### MySQL

```yaml
databases:
  mysql:
    driver: mysql
    host: localhost
    port: 3306
    username: root
    password: "your_password"
    database: mydb
    options:
      parseTime: "true"
```

或使用 DSN 字符串：

```yaml
databases:
  mysql:
    driver: mysql
    dsn: "root:password@tcp(localhost:3306)/mydb?parseTime=true"
```

#### MSSQL Server

```yaml
databases:
  mssql:
    driver: mssql
    host: localhost
    port: 1433
    username: sa
    password: "YourPassword123"
    database: AdventureWorks
    options:
      encrypt: "false"
      connection timeout: "30"
```

或使用 DSN 字符串：

```yaml
databases:
  mssql:
    driver: mssql
    dsn: "sqlserver://sa:YourPassword123@localhost:1433?database=AdventureWorks&encrypt=false"
```

> **注意**：`driver` 字段可使用 `mssql` 或 `sqlserver`，两者等效。

#### PostgreSQL

```yaml
databases:
  postgres:
    driver: postgres
    host: localhost
    port: 5432
    username: postgres
    password: "your_password"
    database: postgres
    options:
      sslmode: disable
```

#### SQLite

```yaml
databases:
  sqlite:
    driver: sqlite
    dsn: "C:/data/mydb.db"
```

> 多数据库配置可在同一 `databases` 节点下并列配置，dbmcp 会自动连接所有有效条目。

### 5. 运行

```bash
./build/dbmcp
# 或指定配置文件路径
./build/dbmcp --config /path/to/config.yaml
```

启动后 MCP Server 通过 stdio 与 AI 工具通信，无需手动连接。

## 集成到 AI 工具

### Claude Code

在项目根目录创建 `.mcp.json`：

```json
{
  "mcpServers": {
    "dbmcp": {
      "command": "/path/to/build/dbmcp",
      "args": ["--config", "/path/to/config.yaml"]
    }
  }
}
```

Windows 路径示例：

```json
{
  "mcpServers": {
    "dbmcp": {
      "command": "C:\\Workspace\\TestProject\\dbmcp\\build\\dbmcp.exe",
      "args": ["--config", "C:\\Users\\你的用户名\\.dbmcp\\config.yaml"]
    }
  }
}
```

> **注意**：Windows 路径中的反斜杠需转义为 `\\`。

### Cline / Roo Code

在扩展设置的 MCP Servers 配置中添加：

```json
{
  "dbmcp": {
    "command": "/path/to/build/dbmcp",
    "args": ["--config", "/path/to/config.yaml"]
  }
}
```

## 验证安装

启动 AI 工具后，调用以下工具验证：

| 工具 | 用途 | 预期输出 |
|------|------|----------|
| `config_status` | 查看配置状态 | 数据库连接数、权限模式、重载状态 |
| `list_databases` | 列出已连接数据库 | 数据库名称列表 |
| `list_tables` | 列出表 | 表名列表 |

## 可用工具

| 工具 | 说明 | 参数 |
|------|------|------|
| `execute_query` | 执行 SELECT 查询 | `sql`, `database` |
| `execute_update` | 执行 INSERT/UPDATE/DELETE/DDL | `sql`, `database` |
| `execute_param_query` | 执行参数化查询 | `sql`, `database`, `params` |
| `list_databases` | 列出已连接的数据库 | 无 |
| `list_tables` | 列出表 | `database` |
| `describe_table` | 查看表结构 | `database`, `table` |
| `query_logs` | 查询审计日志 | `limit`, `database`, `action_type` |
| `config_status` | 查看配置状态 | 无 |
| `begin_tx` | 开始事务 | `database` |
| `commit` | 提交事务 | `database` |
| `rollback` | 回滚事务 | `database` |

## 常见问题

### 配置文件未找到

确保配置文件存在于 `~/.dbmcp/config.yaml` 或通过 `--config` 指定正确路径。

### 数据库连接失败

检查 DSN 或结构化配置字段是否正确，确保数据库服务正在运行。

### 热重载未生效

MCP Server 监听配置文件所在**目录**而非文件本身。修改文件后会自动检测并重载。

### 权限拒绝

确认 `permissions` 配置允许目标数据库和表，且操作类型在 `allowed_actions` 中。

## 未来规划

dbmcp 通过 `DatabaseDriver` 接口实现数据库无关的架构，扩展新驱动只需实现接口并注册即可。

### 计划支持的数据库类型

| 类型 | 数据库 | 状态 | 说明 |
|------|--------|------|------|
| 关系型 | MySQL | ✓ 已支持 | 5.7 / 8.0+ |
| 关系型 | PostgreSQL | ✓ 已支持 | 13 / 15 / 16 |
| 关系型 | SQLite | ✓ 已支持 | 纯 Go 驱动 |
| 关系型 | **MSSQL Server** | ✓ 已支持 | 2017 / 2019 / 2022 |
| 时序数据库 | InfluxDB | 📋 计划中 | 时序数据查询与管理 |
| 时序数据库 | TDengine | 📋 计划中 | 涛思数据，国产时序数据库 |
| 时序数据库 | Prometheus | 📋 计划中 | PromQL 查询支持 |
| 图数据库 | Neo4j | 📋 计划中 | Cypher 查询语言支持 |
| 图数据库 | NebulaGraph | 📋 计划中 | 国产分布式图数据库 |
| 关系型 | **达梦 (DM)** | ✓ 已支持 | DM8，**需要 `-tags dm` 构建** |
| 国产数据库 | 人大金仓 (KingBase) | 📋 计划中 | PostgreSQL 兼容 |
| 国产数据库 | OceanBase | 📋 计划中 | MySQL 兼容模式 |
| 国产数据库 | TiDB | 📋 计划中 | MySQL 兼容，分布式 |
| NoSQL | Redis | ✓ 已支持 | 键值查询与缓存管理 |
| NoSQL | MongoDB | 📋 计划中 | BSON 文档查询 |
| 云数据库 | Snowflake | 📋 计划中 | 云数据仓库 |
| 云数据库 | ClickHouse | 📋 计划中 | OLAP 列式数据库 |

### 扩展新驱动

实现 `internal/database/interface.go` 中的接口即可接入新数据库：

```go
type DatabaseDriver interface {
    Connect(dsn string) error
    Query(ctx context.Context, sql string) (*QueryResult, error)
    Exec(ctx context.Context, sql string) (int64, error)
    ListDatabases(ctx context.Context) ([]string, error)
    ListTables(ctx context.Context, database string) ([]string, error)
    DescribeTable(ctx context.Context, database, table string) ([]Column, error)
    Close() error
    BeginTx(ctx context.Context) error
    Commit() error
    Rollback() error
}
```

然后在 `internal/database/manager.go` 的 `createDriver` 工厂函数中注册新驱动类型。

> 有特定数据库接入需求？欢迎提交 [Issue](https://github.com/dbmcp/dbmcp/issues) 或 [Pull Request](https://github.com/dbmcp/dbmcp/pulls)。
