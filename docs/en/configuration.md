# dbmcp Configuration Guide

> 🇨🇳 [中文版](../zh/configuration.md)

## Config File

Default path: `~/.dbmcp/config.yaml` (override with `--config`)

## Config Structure

```yaml
databases:          # Database connections (multiple supported)
  <name>:
    driver: mysql|postgres|sqlite
    # ... connection info (see below)

permissions:        # Access control
  read_only: false
  allowed_databases: ["*"]
  allowed_actions: [SELECT, INSERT, UPDATE, DELETE, CREATE, DROP]
  blocked_tables: []
```

## Database Connection

Two configuration styles, can be mixed:

### Style 1: Structured Fields (Recommended)

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

**Supported structured fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `driver` | string | Yes | DB type: `mysql` / `postgres` / `sqlite` |
| `host` | string | No* | Host address, default `localhost` |
| `port` | int | No* | Port, default MySQL:3306 / PG:5432 |
| `username` | string | No* | Username, default MySQL:root / PG:postgres |
| `password` | string | No | Password |
| `database` | string | No | DB name, default PG:postgres |
| `options` | map | No | Connection parameters key-value pairs |

> *When `dsn` is empty, at least `host` must be present for structured fields to work.

### Style 2: DSN String

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

### Priority

If both `dsn` and structured fields are present, **DSN takes priority**.

### SQLite Special Handling

```yaml
databases:
  sqlite_file:
    driver: sqlite
    host: "/path/to/database.db"  # host used as file path

  sqlite_memory:
    driver: sqlite                # uses :memory: if no host/dsn
```

## Multi-Database Connections

Configure multiple databases simultaneously, identified by name:

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

Select target via `database` parameter when using tools:

```
list_tables(database=pg_local)
execute_query(database=mysql_prod, sql="SELECT * FROM users")
```

## Access Control

```yaml
permissions:
  read_only: false           # Global read-only mode
  allowed_databases: ["*"]   # Allowed databases (* = all)
  allowed_actions:           # Allowed operation types
    - SELECT
    - INSERT
    - UPDATE
    - DELETE
    - CREATE
    - DROP
  blocked_tables:            # Table blacklist
    - sensitive_data
    - secrets
```

### Permission Check Order

1. Is database in `allowed_databases`?
2. Is table in `blocked_tables`?
3. Is action type in `allowed_actions`?
4. If `read_only: true`, reject all non-SELECT operations.

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

| Category | Planned Support |
|----------|----------------|
| Time-Series | InfluxDB, TDengine, Prometheus |
| Graph | Neo4j, NebulaGraph |
| Chinese | Dameng(DM), KingBase, OceanBase, TiDB |
| NoSQL | Redis, MongoDB |
| Cloud | Snowflake, ClickHouse |

> See [Getting Started](getting-started.md#roadmap) for how to extend with new drivers.
