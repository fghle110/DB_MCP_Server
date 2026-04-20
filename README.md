# dbmcp

一个用 Go 编写的数据库操作 MCP Server，允许 AI 工具（Claude Code、Cline 等）通过模型上下文协议与数据库进行交互。

> 📚 [接入指南](docs/zh/getting-started.md) | [配置操作指南](docs/zh/configuration.md) | [数据库支持](docs/zh/databases.md) | [English Docs](docs/en/getting-started.md)

## 功能特性

- 多数据库支持：MySQL、PostgreSQL、SQLite、SQL Server、Redis、达梦(DM)
- 配置热重载：修改配置无需重启
- 权限控制：只读模式、数据库白名单、表黑名单
- 安全防护：SQL 注入防护、危险操作拦截、输入校验
- 审计日志：所有 AI 操作记录至本地 SQLite

## 快速开始

### 1. 环境要求

- Go 1.26 或更高版本
- Windows 10/11（也支持 macOS/Linux）

### 2. 构建

```bash
# Windows (PowerShell)
go build -o build/dbmcp.exe ./cmd/dbmcp

# macOS / Linux
go build -o build/dbmcp ./cmd/dbmcp

# 达梦支持版本（需要 dm 构建标签）
go build -tags dm -o build/dbmcp-dm.exe ./cmd/dbmcp

# 或使用 go install
go install ./cmd/dbmcp
```

构建产物输出至 `build/` 目录。达梦版本使用官方 Go 驱动（`third_party/dm/`），通过 `replace` 指令引入，需加 `-tags dm` 编译。

> **注意**：SQLite 使用 `modernc.org/sqlite`（纯 Go 实现），无需 CGO。

### 3. 配置

在 Windows 上，配置文件默认位于 `%USERPROFILE%\.dbmcp\config.yaml`（即 `C:\Users\你的用户名\.dbmcp\config.yaml`）。

```powershell
# 创建配置目录
mkdir $env:USERPROFILE\.dbmcp

# 复制配置模板
Copy-Item config\config.yaml.example $env:USERPROFILE\.dbmcp\config.yaml

# 编辑配置文件
notepad $env:USERPROFILE\.dbmcp\config.yaml
```

示例配置（`config.yaml`），支持两种格式：

**新格式（按类型分组，推荐）：**

```yaml
database_groups:
  relational:
    my_mysql:
      driver: mysql
      host: localhost
      port: 3306
      username: user
      password: password
      database: dbname
      options:
        parseTime: "true"
    my_sqlite:
      driver: sqlite
      host: "C:/data/mydb.db"
  nosql:
    my_redis:
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
      - CREATE
      - DROP
    blocked_tables: []
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

旧格式（扁平 `databases` map）仍然有效，加载时会自动迁移到新格式。详见 [数据库支持文档](docs/zh/databases.md)。

> **注意**：Windows 路径在 YAML 中使用正斜杠 `/` 或双反斜杠 `\\`。

### 4. 运行

```powershell
# 使用默认配置文件 (%USERPROFILE%\.dbmcp\config.yaml)
.\dbmcp.exe

# 指定配置文件路径
.\dbmcp.exe --config C:\path\to\config.yaml
```

### 5. 集成到 AI 工具

#### Claude Code

编辑 Claude Code 的设置（`~/.claude/settings.json` 或通过 `/settings` 命令），添加：

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

> **注意**：Windows 路径中的反斜杠需要转义为双反斜杠 `\\`。

#### Cline / Roo Code

在扩展设置中找到 MCP Servers 配置，添加：

```json
{
  "dbmcp": {
    "command": "C:\\path\\to\\dbmcp\\build\\dbmcp.exe",
    "args": ["--config", "C:\\Users\\你的用户名\\.dbmcp\\config.yaml"]
  }
}
```

### 6. 验证

启动 AI 工具后，可以通过以下命令验证 dbmcp 是否正常工作：

- 调用 `config_status` 查看配置状态
- 调用 `list_databases` 查看已连接的数据库

## 可用工具

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

## 配置热重载

dbmcp 会监听配置文件变更。修改 `config.yaml` 后自动生效，无效配置将被拒绝并保留上一版本配置。

## 安全防护

- SQL 注入防护：关键字黑名单 + 多语句检测
- 输入校验：长度限制（64KB）、仅 UTF-8、禁止控制字符
- 权限系统：只读模式、数据库白名单、表黑名单、操作白名单
- 所有操作记录至 `~/.dbmcp/audit.db`

## 测试

### 运行测试

```powershell
# 运行全部测试(包括集成测试,需要 Docker)
go test ./... -v

# 仅运行单元测试
go test ./internal/config/... ./internal/security/... ./internal/permission/... -v

# 运行特定数据库集成测试
go test ./internal/database/... -run TestSQLite -v
go test ./internal/database/... -run TestMySQL -v
go test ./internal/database/... -run TestPostgres -v

# 运行 MCP Server 端到端测试
go test ./internal/mcp/... -v
```

### 集成测试

| 测试 | 依赖 | 说明 |
|------|------|------|
| `internal/database/sqlite_test.go` | 无 | SQLite 内存数据库,8 个测试用例 |
| `internal/database/mysql_test.go` | Docker | MySQL 8.0 容器,8 个测试用例 |
| `internal/database/postgres_test.go` | Docker | PostgreSQL 16 容器,8 个测试用例 |
| `internal/database/mssql_test.go` | Docker | SQL Server 2022 容器,8 个测试用例 |
| `internal/mcp/server_test.go` | 无 | MCP Server 端到端,6 个测试用例 |

如果 Docker 不可用,MySQL、PostgreSQL 和 SQL Server 测试会自动跳过。

## 未来规划

dbmcp 通过 `DatabaseDriver` 接口实现数据库无关的架构，扩展新驱动只需实现接口并注册。

| 类型 | 数据库 | 状态 |
|------|--------|------|
| 关系型 | MySQL | ✓ 已支持 |
| 关系型 | PostgreSQL | ✓ 已支持 |
| 关系型 | SQLite | ✓ 已支持 |
| 关系型 | SQL Server | ✓ 已支持 |
| NoSQL | Redis | ✓ 已支持 |
| 国产数据库 | 达梦(DM) | ✓ 已支持 (需 `-tags dm`) |
| 时序数据库 | InfluxDB, TDengine, Prometheus | 📋 计划中 |
| 图数据库 | Neo4j, NebulaGraph | 📋 计划中 |
| 国产数据库 | 人大金仓(KingBase), OceanBase, TiDB | 📋 计划中 |
| NoSQL | MongoDB | 📋 计划中 |
| 云数据库 | Snowflake, ClickHouse | 📋 计划中 |

> 详见 [数据库支持文档](docs/zh/databases.md)

## 许可证

MIT
