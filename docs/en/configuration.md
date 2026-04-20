# dbmcp Configuration Guide

> 🇨🇳 [中文版](../zh/configuration.md)

## Config File

Default path: `~/.dbmcp/config.yaml` (override with `--config`)

## Config Structure (New Format)

```yaml
database_groups:        # Database connections grouped by type
  relational:           # Relational DBs (MySQL/PostgreSQL/SQLite/MSSQL/DM)
    <name>:
      driver: mysql|postgres|sqlite|mssql|dm
      # ... connection info (see below)
  nosql:                # NoSQL DBs (Redis, etc.)
    <name>:
      driver: redis
      # ... connection info

permissions_groups:     # Per-database permissions grouped by type
  relational:
    <name>:             # DB name, matching keys in database_groups
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

> **Backward Compatible**: The old format (flat `databases` + `permissions`) still works. On startup, it is automatically migrated to the new format with a backup saved as `config.yaml.bak`.

## Database Connection

Two configuration styles, can be mixed:

### Style 1: Structured Fields (Recommended)

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

**Supported structured fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `driver` | string | Yes | DB type: `mysql` / `postgres` / `sqlite` / `mssql` / `dm` / `redis` |
| `host` | string | No* | Host address, default `localhost` |
| `port` | int | No* | Port, default MySQL:3306 / PG:5432 / MSSQL:1433 / DM:5236 / Redis:6379 |
| `username` | string | No* | Username |
| `password` | string | No | Password |
| `database` | string | No | DB name |
| `options` | map | No | Connection parameters key-value pairs |

> *When `dsn` is empty, at least `host` must be present for structured fields to work.

### Style 2: DSN String

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

### Priority

If both `dsn` and structured fields are present, **DSN takes priority**.

### SQLite Special Handling

```yaml
database_groups:
  relational:
    sqlite_file:
      driver: sqlite
      host: "/path/to/database.db"  # host used as file path

    sqlite_memory:
      driver: sqlite                # uses :memory: if no host/dsn
```

### Dameng (DM) Database

The DM driver requires the `-tags dm` build flag:

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

## Multi-Database Connections

Configure multiple databases simultaneously, identified by name:

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

Select target via `database` parameter when using tools:

```
list_tables(database=pg_local)
execute_query(database=mysql_prod, sql="SELECT * FROM users")
```

## Access Control (Per-Database)

Each database has independent permissions:

```yaml
permissions_groups:
  relational:
    mysql_prod:
      read_only: true              # Read-only mode
      allowed_databases: ["*"]     # Allowed databases (* = all)
      allowed_actions:             # Allowed operation types
        - SELECT
      blocked_tables:              # Table blacklist
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
      allowed_commands:            # Command whitelist
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
      blocked_keys: []             # Key blacklist (supports wildcards)
```

### Permission Check Order

**Relational databases:**
1. Is database in `allowed_databases`?
2. Is table in `blocked_tables`?
3. Is action type in `allowed_actions`?
4. If `read_only: true`, reject all non-SELECT operations.

**Redis:**
1. Is command in `allowed_commands`?
2. Is key in `blocked_keys`? (supports wildcards)
3. If `read_only: true`, reject all write commands.

### Old Format Auto-Migration

Old format configs (flat `databases` + `permissions`) are automatically migrated on startup:

1. **Backup** — Original file saved as `config.yaml.bak`
2. **Migrate** — `databases` → `database_groups`, `permissions` → type-mapped into `permissions_groups`
3. **Expand** — Single old-format permission object expanded to per-database config
4. **Write-back** — Config file updated to new format

```yaml
# Old format (still works, auto-migrates)
databases:
  mydb:
    driver: mysql
    dsn: "user:pass@tcp(localhost:3306)/testdb"
permissions:
  read_only: true
  allowed_actions: [SELECT]
```

## Config Hot-Reload

The MCP Server watches the config file for changes:

1. Edit `~/.dbmcp/config.yaml`
2. Changes detected automatically (directory-level events)
3. Invalid config keeps old config active
4. Valid config atomically replaces (no disruption to in-flight requests)

### Hot-Reload Triggers

| Event Type | Triggers Reload |
|------------|-----------------|
| File Write | Yes |
| File Create | Yes |
| File Rename | Yes |
| File Delete | No |

### Hot-Reload Effects

| Component | Behavior |
|-----------|----------|
| DB Connections | New connections registered, removed connections gracefully closed after 30s |
| Permissions | Atomic swap, no effect on in-flight requests |
| Security Rules | Immediately effective |

## DSN Masking

DSNs in audit logs are automatically masked:

```
Raw:    postgres://admin:secret123@localhost:5432/db
Logged: postgres://admin:***@localhost:5432/db
```

## High-Risk Operation Marking

The following operations are marked `[HIGH_RISK]` in audit logs:

| Type | Examples |
|------|----------|
| `DROP TABLE` / `DROP DATABASE` | Delete table/database |
| `TRUNCATE` | Truncate table |
| `ALTER TABLE ... DROP` | Drop column/constraint |
| `DELETE/UPDATE ... WHERE 1=1` | Full-table operations |

## Empty Entry Skipping

Empty or commented-out entries in config are silently skipped:

```yaml
databases:
  mysql_prod:
    driver: mysql
    host: prod.example.com
    port: 3306
    username: root
    password: ""
    database: ""

  # mysql_backup:           # Commented-out entries are skipped

  pg_disabled:              # driver-only entries without host/dsn are also skipped
    driver: postgres
```

Validation requires at least one complete, valid database entry.

---

## Extending to More Databases

dbmcp's architecture supports unlimited new database types — just implement the `DatabaseDriver` interface.

| Category | Supported | Planned |
|----------|-----------|---------|
| Relational | MySQL, PostgreSQL, SQLite, SQL Server, **Dameng (DM)** | KingBase, OceanBase, TiDB |
| NoSQL | **Redis** | MongoDB |
| Time-Series | — | InfluxDB, TDengine, Prometheus |
| Graph | — | Neo4j, NebulaGraph |
| Cloud | — | Snowflake, ClickHouse |

> DM build requires `go build -tags dm`. See [Getting Started](getting-started.md#roadmap) for how to extend with new drivers.
