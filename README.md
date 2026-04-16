# dbmcp

A database operation MCP Server written in Go, allowing AI tools (Claude Code, Cline, etc.) to interact with databases via the Model Context Protocol.

## Features

- Multi-database support: MySQL, PostgreSQL, SQLite (extensible to other databases)
- Configuration hot-reload: modify config without restarting
- Permission control: read-only mode, database whitelist, table blacklist
- Security: SQL injection prevention, dangerous operation blocking, input validation
- Audit logging: all AI operations recorded to local SQLite

## Quick Start

### 1. Build

```bash
go build -o dbmcp ./cmd/dbmcp
```

### 2. Configure

```bash
mkdir -p ~/.dbmcp
cp config/config.yaml.example ~/.dbmcp/config.yaml
# Edit ~/.dbmcp/config.yaml with your database connections
```

### 3. Run

```bash
./dbmcp
# Or specify config path:
./dbmcp --config /path/to/config.yaml
```

### 4. Integrate with Claude Code

Add to your Claude Code MCP configuration:

```json
{
  "mcpServers": {
    "dbmcp": {
      "command": "/path/to/dbmcp",
      "args": ["--config", "/path/to/config.yaml"]
    }
  }
}
```

## Available Tools

| Tool | Description |
|------|-------------|
| `execute_query` | Execute SELECT query |
| `execute_update` | Execute INSERT/UPDATE/DELETE/DDL |
| `execute_param_query` | Execute parameterized query |
| `list_databases` | List connected databases |
| `list_tables` | List tables in a database |
| `describe_table` | Show table structure |
| `query_logs` | Query audit logs |
| `config_status` | Show configuration status |

## Config Hot-Reload

dbmcp watches the config file for changes. Modify `config.yaml` and changes take effect automatically. Invalid configs are rejected and the previous config is kept.

## Security

- SQL injection protection via keyword blocking and multi-statement detection
- Input validation: length limit (64KB), UTF-8 only, no control characters
- Permission system: read-only mode, database whitelist, table blacklist, action whitelist
- All operations logged to `~/.dbmcp/audit.db`

## License

MIT
