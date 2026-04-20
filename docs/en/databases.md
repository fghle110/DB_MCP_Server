# Database Support

dbmcp supports multiple databases through a unified `DatabaseDriver` interface. Each database is distinguished by the `driver` field in the configuration file.

## Supported Databases

| Database | driver Values | Default Port | Dependency | Build Tag |
|----------|---------------|--------------|------------|-----------|
| MySQL | `mysql` | 3306 | `github.com/go-sql-driver/mysql` | none |
| PostgreSQL | `postgres`, `postgresql` | 5432 | `github.com/jackc/pgx/v5` | none |
| SQLite | `sqlite`, `sqlite3` | — | `modernc.org/sqlite` | none |
| SQL Server | `mssql`, `sqlserver` | 1433 | `github.com/microsoft/go-mssqldb` | none |
| Redis | `redis` | 6379 | `github.com/redis/go-redis/v9` | none |
| DM (Dameng) | `dm`, `dmdbms`, `dameng` | 5236 | `third_party/dm/dm/` (local replace) | `dm` |

> **DM driver** requires the `dm` build tag and is excluded from CI/release builds by default. To build locally with DM support: `go build -tags dm -o build/dbmcp-dm ./cmd/dbmcp`

## DSN Formats

Each database supports two configuration forms: structured fields (host/port/username/password/database/options) or a raw `dsn` string.

### MySQL

```
user:password@tcp(host:port)/database?parseTime=true&loc=Local
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| host | `localhost` | Server address |
| port | `3306` | Port number |
| username | `root` | Username |
| password | — | Password |
| database | — | Database name (required) |
| options | — | Additional connection parameters |

### PostgreSQL

```
postgres://user:password@host:port/database?sslmode=disable
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| host | `localhost` | Server address |
| port | `5432` | Port number |
| username | `postgres` | Username |
| password | — | Password |
| database | `postgres` | Database name |
| options | — | Additional connection parameters (e.g. sslmode) |

### SQLite

```
/path/to/database.db  or  :memory:
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| dsn | — | Database file path |
| host | — | Can also be used as file path |
| default | `:memory:` | In-memory database if unspecified |

SQLite uses a pure Go implementation (`modernc.org/sqlite`), no CGO required.

### SQL Server

```
sqlserver://user:password@host:port?database=dbname&encrypt=disable
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| host | `localhost` | Server address |
| port | `1433` | Port number |
| username | `sa` | Username |
| password | — | Password |
| database | `master` | Database name |
| options | — | Additional connection parameters |

### Redis

```
redis://user:password@host:port/db
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| host | `localhost` | Server address |
| port | `6379` | Port number |
| password | — | Password |
| options.db | `0` | Logical database (0-15) |

### DM (Dameng)

```
dm://user:password@host:port?autoCommit=0&schema=schema_name
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| host | `localhost` | Server address |
| port | `5236` | Port number |
| username | `SYSDBA` | Username |
| password | — | Password |
| options.autoCommit | `0` | Auto-commit (0=off, enables transactions) |
| options.schema | — | Default schema name |

> DM enables `autocommit` by default. For transaction support, the driver automatically sets `autoCommit=0` (unless explicitly configured).

## Feature Matrix

| Feature | MySQL | PostgreSQL | SQLite | SQL Server | Redis | DM |
|---------|:-----:|:----------:|:------:|:----------:|:-----:|:--:|
| SQL Queries | ✓ | ✓ | ✓ | ✓ | ✗ | ✓ |
| Write Ops | ✓ | ✓ | ✓ | ✓ | ✓* | ✓ |
| Transactions | ✓ | ✓ | ✓ | ✓ | ✗ | ✓ |
| Param Queries | ✓ | ✓ | ✓ | ✓ | ✗ | ✓** |
| List Databases | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| List Tables | ✓ | ✓ | ✓ | ✓ | ✓*** | ✓ |
| Describe Table | ✓ | ✓ | ✓ | ✓ | ✓**** | ✓ |

- ✓* Redis writes go through `redis_command` tool, not `execute_update`
- ✓** DM param queries use `QueryWithParams`, via `execute_param_query` tool
- ✓*** Redis list tables returns keys (via SCAN)
- ✓**** Redis describe returns key type/TTL

## Transaction Support

Relational databases (MySQL, PostgreSQL, SQLite, SQL Server, DM) support transactions via `BeginTx`/`Commit`/`Rollback`.

### Single-Statement Writes

`Exec` wraps each write in a transaction by default:

```
BEGIN → Execute SQL → COMMIT / ROLLBACK on failure
```

### Multi-Statement Transactions

Manual control via MCP tools:

```
begin_tx → execute_update (multiple times) → commit / rollback
```

### Redis Transactions

Redis does not support SQL-style transactions. `BeginTx`/`Commit`/`Rollback` return errors. For Redis native transactions, use `MULTI`/`EXEC` via `redis_command`.

### DM Transaction Quirks

The DM driver uses a dedicated connection (`conn.ExecContext`) for transactions instead of `tx.ExecContext` due to compatibility issues with `database/sql` transaction abstraction in certain DM versions. `BeginTx` acquires a dedicated connection; `Commit`/`Rollback` releases it.

## Configuration Example

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

## Driver Architecture

All drivers implement the unified `DatabaseDriver` interface:

```go
type DatabaseDriver interface {
    Connect(dsn string) error
    Query(ctx context.Context, sql string) (*QueryResult, error)
    Exec(ctx context.Context, sql string) (int64, error)
    ListDatabases(ctx context.Context) ([]string, error)
    ListTables(ctx context.Context, database string) ([]string, error)
    DescribeTable(ctx context.Context, database, table string) ([]Column, error)
    Close() error
    // Transactions
    BeginTx(ctx context.Context) error
    Commit() error
    Rollback() error
}
```

`DriverManager` handles driver creation, connection pooling, hot-reload sync, and lifecycle management. To add a new driver:

1. Implement the `DatabaseDriver` interface
2. Register the type in `createDriver` in `manager.go`
3. Add DSN assembly logic in `buildDSN`

## Connection Pool Settings

| Database | MaxOpen | MaxIdle | MaxLifetime | Notes |
|----------|---------|---------|-------------|-------|
| MySQL | 10 | 5 | 5 min | Standard pool |
| PostgreSQL | 10 | 5 | 5 min | Standard pool |
| SQLite | 1 | 1 | 5 min | Single connection (file lock) |
| SQL Server | 1 | 1 | 5 min | Single connection |
| Redis | — | — | — | Managed by go-redis |
| DM | 10 | 5 | 5 min | Standard pool |

## Security Pipeline

All database operations pass through a security pipeline before execution:

1. **SQL Injection Guard** — length/encoding checks → multi-statement detection (supports PG `$$...$$`) → dangerous keyword interception
2. **Permission Check** — database whitelist → table blacklist → action whitelist + read-only mode
3. **Audit Logging** — all operations logged to local SQLite (`~/.dbmcp/audit.db`), DSNs are redacted

Redis commands use a separate pipeline:

1. Command whitelist (`permission.Checker.CheckRedisCommand`)
2. Key blacklist (with glob pattern matching)
3. Read-only mode check

## Placeholder Syntax

| Database | Placeholder | Example |
|----------|-------------|---------|
| MySQL | `?` | `SELECT * FROM users WHERE id = ?` |
| PostgreSQL | `$1, $2, ...` | `SELECT * FROM users WHERE id = $1` |
| SQLite | `?` | `SELECT * FROM users WHERE id = ?` |
| SQL Server | `@p1, @p2, ...` | `SELECT * FROM users WHERE id = @p1` |
| DM | `?` | `SELECT * FROM users WHERE id = ?` |

Parameterized queries are executed via the `execute_param_query` MCP tool.
