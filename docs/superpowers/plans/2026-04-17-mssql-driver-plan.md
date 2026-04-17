# MSSQL Server 驱动实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 dbmcp 添加 Microsoft SQL Server 支持，实现基础 CRUD、元数据查询和事务控制。

**Architecture:** 在 `internal/database/` 包中新增 `mssql.go`，遵循现有 MySQL/PG 驱动的结构模式（`db *sql.DB` + `tx *sql.Tx`），实现 `DatabaseDriver` 接口。在 `manager.go` 中注册驱动类型和 DSN 构建函数。新增 `mssql_test.go` 使用 Docker 容器运行集成测试。

**Tech Stack:** Go 1.26, `github.com/microsoft/go-mssqldb`, `database/sql`, testcontainers-go/modules/mssql

---

## 文件结构

| 文件 | 操作 | 职责 |
|------|------|------|
| `go.mod` / `go.sum` | 修改 | 添加 `github.com/microsoft/go-mssqldb` 依赖 |
| `internal/database/mssql.go` | 新增 | MSSQLDriver 实现（Connect/Query/Exec/ListDatabases/ListTables/DescribeTable/事务/Close） |
| `internal/database/manager.go` | 修改 | createDriver 和 buildDSN 中增加 `mssql`/`sqlserver` case |
| `testhelpers/dbsetup.go` | 修改 | 增加 `SetupMSSQLContainer` 辅助函数 |
| `internal/database/mssql_test.go` | 新增 | 8 个集成测试用例 |

---

### Task 1: 添加 go-mssqldb 依赖

**Files:**
- `go.mod`

- [ ] **Step 1: 添加依赖**

Run:
```bash
go get github.com/microsoft/go-mssqldb@latest
```

Expected: `go.mod` 中新增 `github.com/microsoft/go-mssqldb` 及相关间接依赖，`go.sum` 更新。

- [ ] **Step 2: 验证依赖可解析**

Run:
```bash
go mod tidy
```

Expected: 无错误输出。

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add go-mssqldb dependency"
```

---

### Task 2: 在 manager.go 中注册 MSSQL 驱动

**Files:**
- Modify: `internal/database/manager.go` (lines 130-141 for createDriver, lines 144-161 for buildDSN)

- [ ] **Step 1: 在 createDriver 中新增 mssql case**

在 `internal/database/manager.go` 的 `createDriver` 函数中，`default` case 之前添加：

```go
case "mssql", "sqlserver":
    return NewMSSQLDriver(), nil
```

- [ ] **Step 2: 在 buildDSN 中新增 mssql case**

在 `buildDSN` 函数的 switch 中，`default` case 之前添加：

```go
case "mssql", "sqlserver":
    return buildMSSQLDSN(cfg)
```

- [ ] **Step 3: 添加 buildMSSQLDSN 函数**

在 `buildSQLiteDSN` 函数之后（文件末尾），添加：

```go
// buildMSSQLDSN 组装 MSSQL DSN: sqlserver://user:pass@host:port?database=dbname&opts
func buildMSSQLDSN(cfg config.DatabaseConfig) string {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	port := cfg.Port
	if port == 0 {
		port = 1433
	}
	user := cfg.Username
	if user == "" {
		user = "sa"
	}
	dbname := cfg.Database
	if dbname == "" {
		dbname = "master"
	}

	dsn := fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s",
		url.QueryEscape(user), url.QueryEscape(cfg.Password), host, port, url.QueryEscape(dbname))

	opts := make(url.Values)
	for k, v := range cfg.Options {
		opts.Set(k, v)
	}
	if encoded := opts.Encode(); encoded != "" {
		dsn += "&" + encoded
	}
	return dsn
}
```

- [ ] **Step 4: 验证编译通过**

Run:
```bash
go build ./...
```

Expected: 编译失败，提示 `NewMSSQLDriver` 未定义（因为我们还没创建 mssql.go）。这是预期行为，确认注册逻辑语法正确。

- [ ] **Step 5: Commit**

```bash
git add internal/database/manager.go
git commit -m "feat: register mssql driver type and DSN builder in manager"
```

---

### Task 3: 实现 MSSQLDriver（基础连接 + Query/Exec）

**Files:**
- Create: `internal/database/mssql.go`

- [ ] **Step 1: 创建 mssql.go 并实现 Connect/Query/Exec**

```go
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/microsoft/go-mssqldb"
)

// MSSQLDriver MSSQL Server 数据库驱动
type MSSQLDriver struct {
	db *sql.DB
	tx *sql.Tx
}

// NewMSSQLDriver 创建 MSSQL 驱动实例
func NewMSSQLDriver() *MSSQLDriver {
	return &MSSQLDriver{}
}

// Connect 连接 MSSQL Server
func (d *MSSQLDriver) Connect(dsn string) error {
	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping mssql: %w", err)
	}
	d.db = db
	return nil
}

// Query 执行查询
func (d *MSSQLDriver) Query(ctx context.Context, sqlStr string) (*QueryResult, error) {
	rows, err := d.db.QueryContext(ctx, sqlStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	result := &QueryResult{Columns: columns}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}
		result.Rows = append(result.Rows, values)
	}
	return result, rows.Err()
}

// Exec 执行写入（使用事务，失败自动回滚）
func (d *MSSQLDriver) Exec(ctx context.Context, sqlStr string) (int64, error) {
	if d.tx != nil {
		return d.execInTx(ctx, sqlStr)
	}
	return d.execSingle(ctx, sqlStr)
}

func (d *MSSQLDriver) execInTx(ctx context.Context, sqlStr string) (int64, error) {
	res, err := d.tx.ExecContext(ctx, sqlStr)
	if err != nil {
		_ = d.tx.Rollback()
		d.tx = nil
		return 0, fmt.Errorf("exec failed: %w (rolled back)", err)
	}
	affected, _ := res.RowsAffected()
	return affected, nil
}

func (d *MSSQLDriver) execSingle(ctx context.Context, sqlStr string) (int64, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	res, execErr := tx.ExecContext(ctx, sqlStr)
	if execErr != nil {
		_ = tx.Rollback()
		return 0, execErr
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	affected, _ := res.RowsAffected()
	return affected, nil
}
```

- [ ] **Step 2: 验证编译通过**

Run:
```bash
go build ./...
```

Expected: 编译成功，无错误。

- [ ] **Step 3: Commit**

```bash
git add internal/database/mssql.go
git commit -m "feat: add MSSQLDriver with Connect/Query/Exec methods"
```

---

### Task 4: 实现元数据查询（ListDatabases/ListTables/DescribeTable）

**Files:**
- Modify: `internal/database/mssql.go`

- [ ] **Step 1: 在 mssql.go 中追加元数据方法**

在 `execSingle` 函数之后、`Close` 方法之前添加（`Close` 方法下一步实现）：

```go
// ListDatabases 列出数据库
func (d *MSSQLDriver) ListDatabases(ctx context.Context) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT name FROM sys.databases WHERE state_desc = 'ONLINE'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var databases []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		databases = append(databases, name)
	}
	return databases, nil
}

// ListTables 列出表
func (d *MSSQLDriver) ListTables(ctx context.Context, database string) ([]string, error) {
	rows, err := d.db.QueryContext(ctx,
		"SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_TYPE = 'BASE TABLE' AND TABLE_CATALOG = N'"+database+"'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, nil
}

// DescribeTable 查看表结构
func (d *MSSQLDriver) DescribeTable(ctx context.Context, database, table string) ([]Column, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT c.COLUMN_NAME, c.DATA_TYPE, c.IS_NULLABLE,
		        CASE WHEN pk.COLUMN_NAME IS NOT NULL THEN 'PRI' ELSE '' END AS COLUMN_KEY
		 FROM INFORMATION_SCHEMA.COLUMNS c
		 LEFT JOIN (
		     SELECT ku.TABLE_CATALOG, ku.TABLE_NAME, ku.COLUMN_NAME
		     FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
		     JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE ku ON tc.CONSTRAINT_NAME = ku.CONSTRAINT_NAME
		     WHERE tc.CONSTRAINT_TYPE = 'PRIMARY KEY'
		 ) pk ON c.TABLE_CATALOG = pk.TABLE_CATALOG AND c.TABLE_NAME = pk.TABLE_NAME AND c.COLUMN_NAME = pk.COLUMN_NAME
		 WHERE c.TABLE_CATALOG = N'`+database+`' AND c.TABLE_NAME = N'`+table+`'
		 ORDER BY c.ORDINAL_POSITION`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []Column
	for rows.Next() {
		var c Column
		if err := rows.Scan(&c.Name, &c.Type, &c.Nullable, &c.Key); err != nil {
			return nil, err
		}
		columns = append(columns, c)
	}
	return columns, nil
}
```

- [ ] **Step 2: 验证编译通过**

Run:
```bash
go build ./...
```

Expected: 编译成功。

- [ ] **Step 3: Commit**

```bash
git add internal/database/mssql.go
git commit -m "feat: add MSSQL ListDatabases/ListTables/DescribeTable methods"
```

---

### Task 5: 实现事务和 Close 方法

**Files:**
- Modify: `internal/database/mssql.go`

- [ ] **Step 1: 追加事务和 Close 方法**

在文件末尾添加：

```go
// Close 关闭连接
func (d *MSSQLDriver) Close() error {
	if d.tx != nil {
		_ = d.tx.Rollback()
		d.tx = nil
	}
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// BeginTx 开始事务
func (d *MSSQLDriver) BeginTx(ctx context.Context) error {
	if d.tx != nil {
		return fmt.Errorf("transaction already in progress")
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	d.tx = tx
	return nil
}

// Commit 提交事务
func (d *MSSQLDriver) Commit() error {
	if d.tx == nil {
		return fmt.Errorf("no transaction in progress")
	}
	tx := d.tx
	d.tx = nil
	return tx.Commit()
}

// Rollback 回滚事务
func (d *MSSQLDriver) Rollback() error {
	if d.tx == nil {
		return fmt.Errorf("no transaction in progress")
	}
	err := d.tx.Rollback()
	d.tx = nil
	return err
}
```

- [ ] **Step 2: 验证编译和接口实现**

Run:
```bash
go build ./...
```

Expected: 编译成功。同时验证 `MSSQLDriver` 实现了 `DatabaseDriver` 接口——如果有任何方法缺失，编译器会报错。

- [ ] **Step 3: Commit**

```bash
git add internal/database/mssql.go
git commit -m "feat: add MSSQL transaction and Close methods"
```

---

### Task 6: 添加 MSSQL 测试容器辅助函数

**Files:**
- Modify: `testhelpers/dbsetup.go`

- [ ] **Step 1: 添加 mssql testcontainers 依赖**

Run:
```bash
go get github.com/testcontainers/testcontainers-go/modules/mssql@latest
```

- [ ] **Step 2: 在 dbsetup.go 中添加 SetupMSSQLContainer 函数**

在 `SetupPostgresContainer` 函数之后添加：

```go
import "github.com/testcontainers/testcontainers-go/modules/mssql"

// SetupMSSQLContainer starts a MSSQL Server test container, returns DSN and cleanup function
func SetupMSSQLContainer(ctx context.Context) (string, func(), error) {
	ctr, err := mssql.Run(ctx, "mcr.microsoft.com/mssql/server:2022-latest",
		mssql.WithAcceptEula(),
		mssql.WithPassword("Test@12345"),
	)
	if err != nil {
		return "", nil, fmt.Errorf("start mssql container: %w", err)
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("get mssql host: %w", err)
	}
	port, err := ctr.MappedPort(ctx, "1433/tcp")
	if err != nil {
		return "", nil, fmt.Errorf("get mssql port: %w", err)
	}

	dsn := fmt.Sprintf("sqlserver://sa:%s@%s:%s?database=master",
		url.QueryEscape("Test@12345"), host, port.Port())

	cleanup := func() {
		_ = ctr.Terminate(ctx)
	}

	return dsn, cleanup, nil
}
```

注意：需要在文件顶部 import 中添加 `"net/url"` 和 `"github.com/testcontainers/testcontainers-go/modules/mssql"`。

- [ ] **Step 3: 验证编译通过**

Run:
```bash
go build ./...
```

Expected: 编译成功。

- [ ] **Step 4: Commit**

```bash
git add testhelpers/dbsetup.go go.mod go.sum
git commit -m "feat: add MSSQL test container helper"
```

---

### Task 7: 编写 MSSQL 集成测试

**Files:**
- Create: `internal/database/mssql_test.go`

- [ ] **Step 1: 创建 mssql_test.go**

```go
package database

import (
	"context"
	"testing"
	"time"

	"github.com/dbmcp/dbmcp/testhelpers"
)

func setupMSSQL(t *testing.T) (*MSSQLDriver, func()) {
	t.Helper()
	testhelpers.SkipIfNoDocker(t)
	ctx := context.Background()

	dsn, cleanup, err := testhelpers.SetupMSSQLContainer(ctx)
	if err != nil {
		t.Fatalf("failed to setup mssql container: %v", err)
	}

	drv := NewMSSQLDriver()
	if err := drv.Connect(dsn); err != nil {
		cleanup()
		t.Fatalf("failed to connect to mssql: %v", err)
	}

	return drv, cleanup
}

func TestMSSQL_Connect(t *testing.T) {
	drv, cleanup := setupMSSQL(t)
	defer cleanup()
	if drv.db == nil {
		t.Fatal("expected db connection")
	}
}

func TestMSSQL_CreateTable(t *testing.T) {
	drv, cleanup := setupMSSQL(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE users (id INT IDENTITY PRIMARY KEY, name NVARCHAR(100) NOT NULL, email NVARCHAR(100))")
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
}

func TestMSSQL_InsertAndSelect(t *testing.T) {
	drv, cleanup := setupMSSQL(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE users (id INT IDENTITY PRIMARY KEY, name NVARCHAR(100), email NVARCHAR(100))")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err = drv.Exec(ctx, "INSERT INTO users (name, email) VALUES ('Alice', 'alice@test.com')")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	result, err := drv.Query(ctx, "SELECT id, name, email FROM users WHERE name = 'Alice'")
	if err != nil {
		t.Fatalf("select: %v", err)
	}

	if len(result.Columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(result.Columns))
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	name := string(result.Rows[0][1].([]byte))
	if name != "Alice" {
		t.Errorf("expected name 'Alice', got %v", name)
	}
}

func TestMSSQL_ListDatabases(t *testing.T) {
	drv, cleanup := setupMSSQL(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbs, err := drv.ListDatabases(ctx)
	if err != nil {
		t.Fatalf("list databases: %v", err)
	}
	if len(dbs) < 1 {
		t.Errorf("expected at least 1 database, got %d", len(dbs))
	}
	found := false
	for _, db := range dbs {
		if db == "master" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected master in databases, got %v", dbs)
	}
}

func TestMSSQL_ListTables(t *testing.T) {
	drv, cleanup := setupMSSQL(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE test_table (id INT IDENTITY PRIMARY KEY)")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	tables, err := drv.ListTables(ctx, "master")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	found := false
	for _, table := range tables {
		if table == "test_table" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected test_table in tables, got %v", tables)
	}
}

func TestMSSQL_DescribeTable(t *testing.T) {
	drv, cleanup := setupMSSQL(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE users (id INT IDENTITY PRIMARY KEY, name NVARCHAR(100) NOT NULL, email NVARCHAR(100))")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	columns, err := drv.DescribeTable(ctx, "master", "users")
	if err != nil {
		t.Fatalf("describe table: %v", err)
	}

	if len(columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(columns))
	}

	if columns[0].Name != "id" {
		t.Errorf("expected column name 'id', got %s", columns[0].Name)
	}
	if columns[0].Key != "PRI" {
		t.Errorf("expected key 'PRI' for id column, got %s", columns[0].Key)
	}
}

func TestMSSQL_DropTable(t *testing.T) {
	drv, cleanup := setupMSSQL(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE temp_table (id INT IDENTITY PRIMARY KEY)")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err = drv.Exec(ctx, "DROP TABLE temp_table")
	if err != nil {
		t.Fatalf("drop table: %v", err)
	}

	tables, err := drv.ListTables(ctx, "master")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	found := false
	for _, table := range tables {
		if table == "temp_table" {
			found = true
			break
		}
	}
	if found {
		t.Errorf("expected temp_table dropped, still found in %v", tables)
	}
}

func TestMSSQL_ExecRowsAffected(t *testing.T) {
	drv, cleanup := setupMSSQL(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE users (id INT IDENTITY PRIMARY KEY, name NVARCHAR(100))")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	rows, err := drv.Exec(ctx, "INSERT INTO users (name) VALUES ('Bob')")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if rows != 1 {
		t.Errorf("expected 1 row affected, got %d", rows)
	}

	rows, err = drv.Exec(ctx, "UPDATE users SET name = 'Robert' WHERE name = 'Bob'")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if rows != 1 {
		t.Errorf("expected 1 row affected by update, got %d", rows)
	}

	rows, err = drv.Exec(ctx, "DELETE FROM users")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if rows != 1 {
		t.Errorf("expected 1 row affected by delete, got %d", rows)
	}
}
```

- [ ] **Step 2: 验证编译通过**

Run:
```bash
go build ./...
```

Expected: 编译成功。

- [ ] **Step 3: Commit**

```bash
git add internal/database/mssql_test.go
git commit -m "test: add MSSQL integration tests (8 test cases)"
```

---

### Task 8: 运行测试验证

**Files:**
- `internal/database/mssql_test.go`

- [ ] **Step 1: 运行 MSSQL 集成测试**

Run:
```bash
go test ./internal/database/... -run TestMSSQL -v
```

Expected:
- 8 个测试全部 PASS（Docker 可用时）
- 或全部 SKIP（Docker 不可用时）
- 无 FAIL

- [ ] **Step 2: 运行全部测试确保无回归**

Run:
```bash
go test ./... -v
```

Expected: 所有现有测试 PASS，MSSQL 测试 PASS 或 SKIP。

- [ ] **Step 3: 最终 Commit（如有测试修复）**

```bash
git add -A
git commit -m "fix: address MSSQL test issues"
```

---

## 规范覆盖检查

| 规范要求 | 对应 Task |
|----------|-----------|
| `github.com/microsoft/go-mssqldb` 纯 Go 驱动 | Task 1 |
| `MSSQLDriver` 结构体 `db` + `tx` 字段 | Task 3 |
| `Connect` 使用 `sqlserver` 驱动名 | Task 3 |
| `Query` / `Exec` 对齐现有模式 | Task 3 |
| `ListDatabases` 使用 `sys.databases` | Task 4 |
| `ListTables` 使用 `INFORMATION_SCHEMA.TABLES` | Task 4 |
| `DescribeTable` 联合查询主键信息 | Task 4 |
| `BeginTx` / `Commit` / `Rollback` / `Close` | Task 5 |
| `createDriver` + `buildDSN` 注册 | Task 2 |
| `buildMSSQLDSN` 默认端口 1433 | Task 2 |
| Docker 集成测试 8 个用例 | Task 7 |
| `N'...'` Unicode 前缀 | Task 4 (DescribeTable/ListTables) |

无遗漏。

## 占位符扫描

计划中无 "TBD"、"TODO"、"implement later" 或 "add tests for the above" 等占位符。每个步骤包含完整代码和预期输出。

## 类型一致性

- `QueryResult` / `Column` 来自 `interface.go`，与现有驱动使用方式一致
- `setupMSSQL` 函数签名与 `setupMySQL`/`setupPostgres` 完全一致
- 测试用例结构（`setupMSSQL` → `defer cleanup` → `ctx` → `defer cancel`）与其他测试文件模式对齐
- DSN 构建使用 `url.QueryEscape` 与 MySQL/PG 一致
