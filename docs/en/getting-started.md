# dbmcp Getting Started

> 🇨🇳 [中文版](../zh/getting-started.md)

## Requirements

| Component | Version | Notes |
|-----------|---------|-------|
| Go | 1.26+ | Runtime |
| Docker (optional) | Latest | For MySQL/PostgreSQL integration tests |

## Quick Start

### 1. Clone

```bash
git clone https://github.com/dbmcp/dbmcp.git
cd dbmcp
```

### 2. Install Dependencies

```bash
go mod download
```

### 3. Build

```bash
# Windows
go build -o build/dbmcp.exe ./cmd/dbmcp

# macOS / Linux
go build -o build/dbmcp ./cmd/dbmcp

# Cross-platform
GOOS=linux GOARCH=amd64 go build -o build/dbmcp ./cmd/dbmcp
GOOS=darwin GOARCH=arm64 go build -o build/dbmcp-darwin ./cmd/dbmcp
```

> **⚠️ Dameng (DM) Build**: To include the Dameng driver, you **must add `-tags dm`**:
>
> ```bash
> # Windows
> go build -tags dm -o build/dbmcp.exe ./cmd/dbmcp
>
> # Linux (cross-platform)
> GOOS=linux GOARCH=amd64 go build -tags dm -o build/dbmcp ./cmd/dbmcp
> ```
>
> **Note**: The DM driver only supports Linux amd64. macOS/Darwin is not supported.

### 4. Configure

Create config directory and edit:

```bash
# Windows
mkdir %USERPROFILE%\.dbmcp
notepad %USERPROFILE%\.dbmcp\config.yaml

# macOS / Linux
mkdir -p ~/.dbmcp
nano ~/.dbmcp/config.yaml
```

Minimal config example (PostgreSQL):

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

> See [Configuration Guide](configuration.md) for details.

### Database Configuration Examples

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

Or using a DSN string:

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

Or using a DSN string:

```yaml
databases:
  mssql:
    driver: mssql
    dsn: "sqlserver://sa:YourPassword123@localhost:1433?database=AdventureWorks&encrypt=false"
```

> **Note**: The `driver` field accepts either `mssql` or `sqlserver` — both are equivalent.

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

> Multiple databases can be configured together under the same `databases` node. dbmcp will automatically connect to all valid entries.

### 5. Run

```bash
./build/dbmcp
# Or specify config path
./build/dbmcp --config /path/to/config.yaml
```

The MCP Server communicates with AI tools via stdio — no manual connection needed.

## Integration with AI Tools

### Claude Code

Create `.mcp.json` in project root:

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

Windows path example:

```json
{
  "mcpServers": {
    "dbmcp": {
      "command": "C:\\Workspace\\TestProject\\dbmcp\\build\\dbmcp.exe",
      "args": ["--config", "C:\\Users\\Username\\.dbmcp\\config.yaml"]
    }
  }
}
```

> **Note**: Backslashes in Windows paths must be escaped as `\\`.

### Cline / Roo Code

Add to the extension's MCP Servers config:

```json
{
  "dbmcp": {
    "command": "/path/to/build/dbmcp",
    "args": ["--config", "/path/to/config.yaml"]
  }
}
```

## Verify Installation

After starting the AI tool, call these tools to verify:

| Tool | Purpose | Expected Output |
|------|---------|-----------------|
| `config_status` | Check config status | DB count, read-only mode, reload status |
| `list_databases` | List connected databases | List of database names |
| `list_tables` | List tables | List of table names |

## Available Tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `execute_query` | Execute SELECT query | `sql`, `database` |
| `execute_update` | Execute INSERT/UPDATE/DELETE/DDL | `sql`, `database` |
| `execute_param_query` | Execute parameterized query | `sql`, `database`, `params` |
| `list_databases` | List connected databases | None |
| `list_tables` | List tables in a database | `database` |
| `describe_table` | Show table structure | `database`, `table` |
| `query_logs` | Query audit logs | `limit`, `database`, `action_type` |
| `config_status` | Show config status | None |
| `begin_tx` | Begin transaction | `database` |
| `commit` | Commit transaction | `database` |
| `rollback` | Rollback transaction | `database` |

## FAQ

### Config file not found

Ensure the config file exists at `~/.dbmcp/config.yaml` or specify the path via `--config`.

### Database connection failed

Verify the DSN or structured fields are correct and the database service is running.

### Hot-reload not working

The MCP Server watches the **directory** containing the config file, not the file itself. Changes are detected automatically.

### Permission denied

Ensure the `permissions` config allows the target database and table, and the action is in `allowed_actions`.

## Roadmap

dbmcp uses a database-agnostic architecture via the `DatabaseDriver` interface. Adding a new driver only requires implementing the interface and registering it.

### Planned Database Support

| Type | Database | Status | Notes |
|------|----------|--------|-------|
| Relational | MySQL | ✓ Supported | 5.7 / 8.0+ |
| Relational | PostgreSQL | ✓ Supported | 13 / 15 / 16 |
| Relational | SQLite | ✓ Supported | Pure Go driver |
| Relational | **MSSQL Server** | ✓ Supported | 2017 / 2019 / 2022 |
| Time-Series | InfluxDB | 📋 Planned | Time-series queries & management |
| Time-Series | TDengine | 📋 Planned | Chinese time-series database |
| Time-Series | Prometheus | 📋 Planned | PromQL query support |
| Graph | Neo4j | 📋 Planned | Cypher query language |
| Graph | NebulaGraph | 📋 Planned | Chinese distributed graph DB |
| Chinese | Dameng (DM) | ✓ Supported | DM8, **requires `-tags dm` build** |
| Chinese | KingBase | 📋 Planned | PostgreSQL-compatible |
| Chinese | OceanBase | 📋 Planned | MySQL-compatible mode |
| Chinese | TiDB | 📋 Planned | MySQL-compatible, distributed |
| NoSQL | Redis | ✓ Supported | Key-value & cache management |
| NoSQL | MongoDB | 📋 Planned | BSON document queries |
| Cloud | Snowflake | 📋 Planned | Cloud data warehouse |
| Cloud | ClickHouse | 📋 Planned | OLAP column database |

### Extending with New Drivers

Implement the interface in `internal/database/interface.go` to add a new database:

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

Then register the new driver type in the `createDriver` factory function in `internal/database/manager.go`.

> Have a specific database in mind? Feel free to submit an [Issue](https://github.com/dbmcp/dbmcp/issues) or [Pull Request](https://github.com/dbmcp/dbmcp/pulls).
