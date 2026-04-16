---
title: dbmcp - Go Database MCP Server Design
date: 2026-04-16
status: draft
---

# dbmcp: Go Database MCP Server

## 1. 概述

使用 Go 开发一个数据库操作的 MCP Server,供 AI 工具(如 Claude Code)通过 stdio 协议调用。支持多数据库适配(Mysql、PostgreSQL、SQLite 及国产数据库),提供 SQL 执行、表结构管理、权限控制和操作日志功能,配置文件支持文件监听自动热重载。

## 2. 技术选型

| 组件 | 选择 | 理由 |
|------|------|------|
| MCP SDK | github.com/mark3labs/mcp-go | 社区活跃,功能全,支持 MCP 规范到 2025-11-25 |
| 传输方式 | stdio | AI 工具本地集成 |
| MySQL 驱动 | github.com/go-sql-driver/mysql | Go 生态最成熟 |
| PostgreSQL 驱动 | github.com/jackc/pgx/v5 | 性能好,支持 PG 特性全 |
| SQLite 驱动 | github.com/mattn/go-sqlite3 | 零配置,广泛使用 |
| 配置热重载 | github.com/fsnotify/fsnotify | 跨平台文件监听 |
| 配置解析 | gopkg.in/yaml.v3 | YAML 配置文件解析 |
| SQL 解析 | github.com/xwb1989/sqlparser | SQL 语法分析,危险操作识别 |

## 3. 架构设计

### 3.1 整体架构

```
AI Client (Claude等)
        │ stdio (stdin/stdout JSON-RPC)
        ▼
┌─────────────────────────────────────┐
│           MCP Server                 │
│  ┌───────────────────────────────┐  │
│  │  MCP Framework (mcp-go)       │  │
│  │  Tool注册 / 参数校验 / JSON-RPC│  │
│  └───────────┬───────────────────┘  │
│              │                       │
│  ┌───────────▼───────────────────┐  │
│  │       Database Manager        │  │
│  │  连接池 / 多数据库适配         │  │
│  └───┬───────────┬───────────┬───┘  │
│      │           │           │       │
│  ┌───▼──┐   ┌───▼──┐  ┌───▼──┐     │
│  │MySQL │   │  PG  │  │SQLite│ ...  │
│  └──────┘   └──────┘  └──────┘     │
│              │                       │
│  ┌───────────▼───────────────────┐  │
│  │      Permission Engine        │  │
│  │  白名单/黑名单 / 只读模式      │  │
│  └───────────────────────────────┘  │
│              │                       │
│  ┌───────────▼───────────────────┐  │
│  │      Operation Logger         │  │
│  │  操作日志 / 可查询 / 可导出    │  │
│  └───────────────────────────────┘  │
│              ▲                       │
│  ┌───────────┴───────────────────┐  │
│  │       Config Watcher          │  │
│  │  fsnotify / 原子替换 / 回滚   │  │
│  └───────────────────────────────┘  │
└─────────────────────────────────────┘
```

### 3.2 目录结构

```
dbmcp/
├── cmd/
│   └── dbmcp/
│       └── main.go              # 入口,初始化各组件并启动 MCP Server
├── internal/
│   ├── mcp/
│   │   └── server.go            # MCP Server 初始化,Tool 注册
│   ├── database/
│   │   ├── interface.go         # 统一数据库接口 DatabaseDriver
│   │   ├── manager.go           # 连接池管理,动态增删连接
│   │   ├── mysql.go             # MySQL 驱动实现
│   │   ├── postgres.go          # PostgreSQL 驱动实现
│   │   └── sqlite.go            # SQLite 驱动实现
│   ├── permission/
│   │   └── permission.go        # 权限校验引擎,原子替换配置
│   ├── security/
│   │   ├── sql_guard.go         # SQL 注入防护:关键字拦截,多语句检测
│   │   └── input_check.go       # 输入校验:长度/编码/控制字符
│   ├── logger/
│   │   └── logger.go            # 操作日志,写入本地 SQLite
│   └── config/
│       ├── config.go            # 配置结构体定义
│       └── watcher.go           # fsnotify 热重载,含回滚逻辑
├── config/
│   └── config.yaml.example      # 配置文件模板
├── go.mod
├── go.sum
└── README.md
```

## 4. 核心功能设计

### 4.1 MCP Tools

| Tool 名称 | 功能 | 参数 | 返回值 |
|-----------|------|------|--------|
| `execute_query` | 执行 SELECT 查询 | `sql`(string), `database`(string) | 结果集(JSON数组) |
| `execute_update` | 执行 INSERT/UPDATE/DELETE/DDL | `sql`(string), `database`(string) | 影响行数,成功/失败 |
| `execute_param_query` | 参数化查询(防注入) | `sql`(string), `database`(string), `params`(array) | 结果集(JSON数组) |
| `list_databases` | 列出当前已连接数据库 | 无 | 数据库名称列表 |
| `list_tables` | 列出指定数据库的表 | `database`(string) | 表名列表 |
| `describe_table` | 查看表结构 | `database`(string), `table`(string) | 字段信息(名,类型,是否空,键) |
| `query_logs` | 查询 AI 操作日志 | `limit`(int,默认50), `database`(string,可选), `action_type`(string,可选) | 操作日志列表 |
| `config_status` | 查看当前配置状态 | 无 | 数据库连接数,权限模式,上次热重载时间 |

### 4.2 权限控制

通过 `config.yaml` 的 `permissions` 字段控制:

```yaml
permissions:
  read_only: false              # 全局只读模式,开启后拒绝所有写入操作
  allowed_databases: ["*"]      # 允许操作的数据库列表, * 表示全部
  allowed_actions:              # 允许的操作类型
    - select
    - insert
    - update
    - delete
    - create
    - drop
  blocked_tables: []            # 禁止操作的表黑名单
```

权限校验在 `execute_query` 和 `execute_update` 执行前进行:
1. 检查数据库是否在 `allowed_databases` 中
2. 检查表是否在 `blocked_tables` 中
3. 解析 SQL 前缀判断操作类型,检查是否在 `allowed_actions` 中
4. 如果 `read_only: true`,拒绝所有非 SELECT 操作

权限配置通过 `sync/atomic` 实现原子替换,热重载时不影响正在执行的请求。

### 4.3 操作日志

所有 AI 执行的操作记录到本地 SQLite 文件 `~/.dbmcp/audit.db`:

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PRIMARY KEY | 自增 ID |
| timestamp | DATETIME | 操作时间 |
| database | TEXT | 目标数据库名称 |
| action | TEXT | 操作类型(SELECT/INSERT/UPDATE/DELETE/CREATE/DROP等) |
| sql | TEXT | 执行的 SQL,最大 4096 字符,超出截断 |
| result | TEXT | 执行结果(success/error) |
| error_message | TEXT | 错误信息(失败时) |
| duration_ms | INTEGER | 执行耗时(毫秒) |

日志通过 `database/sql` 写入本地 SQLite,不依赖外部数据库。

## 5. 配置热重载设计

### 5.1 流程

```
配置文件变动(fsnotify)
        │
        ▼
  读取新配置(YAML解析)
        │
        ▼
  校验配置合法性
        │
   ┌────┴────┐
   │         │
  有效      无效
   │         │
   ▼         ▼
 原子替换   记录错误日志
 新配置     保持旧配置不变
   │
   ▼
 ├─ 数据库: 新建连接池 / 关闭旧连接池(优雅关闭)
 ├─ 权限: 原子替换内存指针
 └─ 通知 MCP Server 工具列表变更
```

### 5.2 连接池管理

- **新增数据库**: 自动创建新的连接池并验证连接可用
- **删除数据库**: 优雅关闭旧连接池,等待正在执行的查询完成(最大等待 30 秒)
- **修改数据库**: 先关闭旧连接,再创建新连接

### 5.3 回滚机制

- 配置文件格式错误或连接失败时,自动回滚到上一版本配置
- 记录错误到日志,不中断 MCP Server 运行
- 保留配置文件修改前的备份(`config.yaml.bak`)

### 5.4 并发安全

- 配置对象使用 `sync.RWMutex` 保护读操作
- 权限配置使用 `atomic.Pointer` 实现无锁读
- 数据库连接池使用 `sync.Map` 管理命名连接

## 6. 安全设计

### 6.1 SQL 注入防护

**原则: MCP Server 绝不信任任何输入的 SQL,即使是 AI 生成的。**

AI 可能被 prompt injection 攻击,间接生成恶意 SQL。因此需要在服务端做多层防护:

#### 6.1.1 SQL 解析与危险操作拦截

在 SQL 执行前,通过 SQL 解析器提取操作类型和关键字,拦截以下危险操作:

| 危险类别 | 拦截关键字 | 说明 |
|----------|-----------|------|
| 文件读写 | `LOAD_FILE`, `INTO OUTFILE`, `INTO DUMPFILE`, `LOAD DATA`, `BULK INSERT` | 防止读写服务器文件 |
| 系统命令 | `xp_cmdshell`, `sys_exec`, `system` | 防止执行系统命令 |
| 网络操作 | `UTL_HTTP`, `HTTPURITYPE`, `curl`, `wget` | 防止外发请求 |
| 提权操作 | `GRANT`, `REVOKE`, `ALTER USER`, `CREATE USER` | 防止修改用户权限 |
| 存储过程执行 | `CALL`, `EXEC`, `EXECUTE` | 防止调用危险存储过程 |
| 多语句执行 | `;` (语句分隔符) | 防止一次传入多条 SQL |
| 注释注入 | `--`, `#`, `/* */` | 允许但记录,结合其他策略判断 |
| 数据库特定危险操作 | MySQL: `SHOW GRANTS`, PG: `COPY ... FROM PROGRAM` | 各数据库特有的危险语法 |

实现方式: 使用 SQL 解析库(如 `github.com/xwb1989/sqlparser` 或正则预扫描)提取 SQL 特征,在权限校验阶段一并拦截。

#### 6.1.2 参数化查询支持

提供独立的参数化工具 `execute_param_query`,AI 传入 SQL 模板和参数:

```json
{
  "tool": "execute_param_query",
  "args": {
    "database": "my_mysql",
    "sql": "SELECT * FROM users WHERE name = ? AND age > ?",
    "params": ["Alice", 18]
  }
}
```

参数通过 `database/sql` 的预处理机制绑定,彻底杜绝字符串拼接注入。

#### 6.1.3 多语句拦截

严格禁止在一个请求中执行多条 SQL(以 `;` 分隔)。实现:
- 去除 SQL 中的注释后,检查 `;` 出现次数,超过 1 次(结尾的除外)直接拒绝
- 返回错误: `multiple statements not allowed, split into separate calls`

### 6.2 代码注入防护

#### 6.2.1 输入校验

| 防护点 | 措施 |
|--------|------|
| SQL 长度限制 | 单条 SQL 最大 64KB,超出拒绝 |
| 非法字符 | 禁止控制字符(ASCII 0-31,排除换行和制表符) |
| 编码注入 | 所有输入强制 UTF-8,拒绝非 UTF-8 字节序列 |
| YAML 解析 | 使用 `yaml.SafeUnmarshal`,禁止 `!!binary`, `!!go` 等危险类型 |

#### 6.2.2 DSN 安全

| 措施 | 说明 |
|------|------|
| 配置文件权限 | 启动时检查 `config.yaml` 文件权限,警告过于开放(如 777) |
| DSN 格式校验 | 严格解析 DSN,禁止 `allowAllFiles=true`, `multiStatements=true` 等危险参数 |
| 密码保护 | 启动时日志中脱敏打印 DSN(密码显示为 `***`) |

#### 6.2.3 进程安全

| 措施 | 说明 |
|------|------|
| 不执行系统命令 | MCP Server 内部不调用 `exec.Command` |
| 不加载外部代码 | 不通过 `plugin.Open` 加载动态库 |
| 最小权限运行 | 建议以普通用户运行,非 root |

### 6.3 YAML Bomb 防护

YAML 解析时防范 Billion Laughs 攻击:
- 限制 YAML 解析深度(最大 10 层嵌套)
- 限制解析大小(最大 1MB)

### 6.4 数据库安全配置

| 措施 | 说明 |
|------|------|
| 连接超时 | 默认 10 秒,防止慢连接耗尽资源 |
| 最大连接数 | 每数据库默认最大 10 连接,防止连接池耗尽 |
| 查询超时 | 默认 30 秒,通过 `context.WithTimeout` 强制终止 |
| 只读模式 | 配置 `read_only: true` 时,底层数据库连接也设置为只读 |

## 6.5 安全层执行顺序

```
SQL 输入
    │
    ▼
1. 长度校验(≤64KB)
    │
    ▼
2. 编码校验(UTF-8)
    │
    ▼
3. 控制字符检查
    │
    ▼
4. 多语句检测(;)
    │
    ▼
5. 危险操作关键字拦截
    │
    ▼
6. 权限校验(数据库/表/操作类型)
    │
    ▼
7. 执行(参数化绑定)
    │
    ▼
8. 结果集大小限制(≤10MB)
    │
    ▼
返回结果 / 记录日志
```

## 7. 错误处理

| 场景 | 处理方式 |
|------|----------|
| SQL 语法错误 | 返回错误信息给 AI,记录到操作日志 |
| SQL 被安全拦截 | 返回安全拦截原因,记录到操作日志(标记为 security_block) |
| 数据库连接断开 | 自动重试(最多3次),重试失败返回错误 |
| 配置文件格式错误 | 热重载失败,回滚旧配置,记录错误日志 |
| 权限拒绝 | 返回权限错误,记录到操作日志 |
| 超时 | 查询超时默认 30 秒,可配置 |
| 结果集过大 | 截断到 10MB,提示 AI 添加 LIMIT |

## 8. 部署

### 8.1 本地开发

```bash
go run cmd/dbmcp/main.go
```

### 8.2 编译

```bash
# Windows
GOOS=windows GOARCH=amd64 go build -o dbmcp.exe ./cmd/dbmcp

# macOS
GOOS=darwin GOARCH=arm64 go build -o dbmcp ./cmd/dbmcp

# Linux
GOOS=linux GOARCH=amd64 go build -o dbmcp ./cmd/dbmcp
```

### 8.3 AI 工具集成(Claude Code)

在 Claude Code 的配置文件中添加:

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

### 8.4 配置文件路径

默认读取 `~/.dbmcp/config.yaml`,可通过 `--config` 参数指定。

## 9. 国产数据库扩展

通过实现 `internal/database/interface.go` 中定义的 `DatabaseDriver` 接口,可以接入国产数据库:

```go
type DatabaseDriver interface {
    Connect(dsn string) error
    Query(ctx context.Context, sql string) ([]map[string]interface{}, error)
    Exec(ctx context.Context, sql string) (int64, error)
    ListDatabases(ctx context.Context) ([]string, error)
    ListTables(ctx context.Context, database string) ([]string, error)
    DescribeTable(ctx context.Context, database, table string) ([]Column, error)
    Close() error
}
```

支持的国产数据库驱动(如达梦 DM、人大金仓 KingBase)可通过其 Go 驱动实现此接口接入。
