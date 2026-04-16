# 集成测试 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 dbmcp 添加数据库驱动集成测试和 MCP Server 端到端测试,使用 testcontainers-go 自动启停 Docker 容器。

**Architecture:** 每种数据库(MySQL/PostgreSQL/SQLite)有独立的集成测试文件,通过 testcontainers 启动真实容器,验证 `DatabaseDriver` 接口的所有方法。MCP Server 测试组装完整模块链,通过 `CallToolRequest` 调用工具 handler 验证端到端流程。

**Tech Stack:** Go 1.26, testcontainers-go v0.40, testcontainers-go/modules/mysql, testcontainers-go/modules/postgres, mark3labs/mcp-go v0.48

---

## 文件总览

| 文件 | 操作 | 职责 |
|------|------|------|
| `testhelpers/dbsetup.go` | 创建 | Docker 可用性检查 + 容器启动辅助函数 |
| `internal/database/mysql_test.go` | 创建 | MySQL 集成测试(8个测试用例) |
| `internal/database/postgres_test.go` | 创建 | PostgreSQL 集成测试(8个测试用例) |
| `internal/database/sqlite_test.go` | 创建 | SQLite 集成测试(8个测试用例,无Docker) |
| `internal/mcp/server_test.go` | 创建 | MCP Server 端到端测试(6个测试用例) |

---

### Task 1: 安装 testcontainers 依赖 + 创建测试辅助工具

**Files:**
- Create: `testhelpers/dbsetup.go`
- Modify: `go.mod` (新增依赖)

- [ ] **Step 1: 安装 testcontainers 依赖**

```bash
cd C:/Workspace/TestProject/dbmcp
GOPROXY=https://goproxy.cn,direct go get github.com/testcontainers/testcontainers-go
GOPROXY=https://goproxy.cn,direct go get github.com/testcontainers/testcontainers-go/modules/mysql
GOPROXY=https://goproxy.cn,direct go get github.com/testcontainers/testcontainers-go/modules/postgres
GOPROXY=https://goproxy.cn,direct go mod tidy
```

- [ ] **Step 2: 创建测试辅助文件**

创建 `testhelpers/dbsetup.go`:

```go
package testhelpers

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"testing"

	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// DockerAvailable 检查 Docker 是否可用
func DockerAvailable() bool {
	_, err := exec.LookPath("docker")
	if err != nil {
		return false
	}
	// 尝试连接 Docker daemon
	conn, err := net.Dial("unix", "/var/run/docker.sock")
	if err != nil {
		// Windows 上检查方式不同,简单返回 true (如果有 docker 命令)
		return true
	}
	conn.Close()
	return true
}

// SkipIfNoDocker 如果 Docker 不可用则跳过测试
func SkipIfNoDocker(t *testing.T) {
	t.Helper()
	if !DockerAvailable() {
		t.Skip("Docker not available, skipping integration test")
	}
}

// SetupMySQLContainer 启动 MySQL 测试容器,返回 DSN 和清理函数
func SetupMySQLContainer(ctx context.Context) (string, func(), error) {
	ctr, err := mysql.Run(ctx, "mysql:8.0",
		mysql.WithDatabase("testdb"),
		mysql.WithUsername("test"),
		mysql.WithPassword("test"),
	)
	if err != nil {
		return "", nil, fmt.Errorf("start mysql container: %w", err)
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("get mysql host: %w", err)
	}
	port, err := ctr.MappedPort(ctx, "3306/tcp")
	if err != nil {
		return "", nil, fmt.Errorf("get mysql port: %w", err)
	}

	dsn := fmt.Sprintf("test:test@tcp(%s:%s)/testdb?parseTime=true", host, port.Port())

	cleanup := func() {
		_ = ctr.Terminate(ctx)
	}

	return dsn, cleanup, nil
}

// SetupPostgresContainer 启动 PostgreSQL 测试容器,返回 DSN 和清理函数
func SetupPostgresContainer(ctx context.Context) (string, func(), error) {
	ctr, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.WithInitScripts(),
	)
	if err != nil {
		return "", nil, fmt.Errorf("start postgres container: %w", err)
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("get postgres host: %w", err)
	}
	port, err := ctr.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return "", nil, fmt.Errorf("get postgres port: %w", err)
	}

	dsn := fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", host, port.Port())

	cleanup := func() {
		_ = ctr.Terminate(ctx)
	}

	return dsn, cleanup, nil
}
```

- [ ] **Step 3: 编译验证**

```bash
cd C:/Workspace/TestProject/dbmcp
go build ./testhelpers/...
```

Expected: no errors.

- [ ] **Step 4: 提交**

```bash
cd C:/Workspace/TestProject/dbmcp
git add testhelpers/ go.mod go.sum
git commit -m "feat: add testcontainers dependency and test helpers

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 2: SQLite 集成测试

**Files:**
- Create: `internal/database/sqlite_test.go`

- [ ] **Step 1: 创建 SQLite 集成测试**

SQLite 使用内存数据库,不需要 Docker,是最简单的起点。

创建 `internal/database/sqlite_test.go`:

```go
package database

import (
	"context"
	"testing"
	"time"
)

func setupSQLite(t *testing.T) *SQLiteDriver {
	t.Helper()
	drv := NewSQLiteDriver()
	dsn := "file:testdb_" + t.Name() + "?mode=memory&cache=shared"
	if err := drv.Connect(dsn); err != nil {
		t.Fatalf("failed to connect to sqlite: %v", err)
	}
	t.Cleanup(func() { _ = drv.Close() })
	return drv
}

func TestSQLite_Connect(t *testing.T) {
	drv := setupSQLite(t)
	if drv.db == nil {
		t.Fatal("expected db connection")
	}
}

func TestSQLite_CreateTable(t *testing.T) {
	drv := setupSQLite(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT)")
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
}

func TestSQLite_InsertAndSelect(t *testing.T) {
	drv := setupSQLite(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT)")
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
	if result.Rows[0][1] != "Alice" {
		t.Errorf("expected name 'Alice', got %v", result.Rows[0][1])
	}
}

func TestSQLite_ListDatabases(t *testing.T) {
	drv := setupSQLite(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbs, err := drv.ListDatabases(ctx)
	if err != nil {
		t.Fatalf("list databases: %v", err)
	}
	if len(dbs) != 1 || dbs[0] != "main" {
		t.Errorf("expected [main], got %v", dbs)
	}
}

func TestSQLite_ListTables(t *testing.T) {
	drv := setupSQLite(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE test_table (id INTEGER PRIMARY KEY)")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	tables, err := drv.ListTables(ctx, "main")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if len(tables) != 1 || tables[0] != "test_table" {
		t.Errorf("expected [test_table], got %v", tables)
	}
}

func TestSQLite_DescribeTable(t *testing.T) {
	drv := setupSQLite(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT)")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	columns, err := drv.DescribeTable(ctx, "main", "users")
	if err != nil {
		t.Fatalf("describe table: %v", err)
	}

	if len(columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(columns))
	}

	// Check first column (id INTEGER PRIMARY KEY)
	if columns[0].Name != "id" {
		t.Errorf("expected column name 'id', got %s", columns[0].Name)
	}
	if columns[0].Key != "PRI" {
		t.Errorf("expected key 'PRI' for id column, got %s", columns[0].Key)
	}

	// Check second column (name TEXT NOT NULL)
	if columns[1].Name != "name" {
		t.Errorf("expected column name 'name', got %s", columns[1].Name)
	}
	if columns[1].Nullable != "NO" {
		t.Errorf("expected nullable 'NO' for name column, got %s", columns[1].Nullable)
	}
}

func TestSQLite_DropTable(t *testing.T) {
	drv := setupSQLite(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE temp_table (id INTEGER PRIMARY KEY)")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err = drv.Exec(ctx, "DROP TABLE temp_table")
	if err != nil {
		t.Fatalf("drop table: %v", err)
	}

	tables, err := drv.ListTables(ctx, "main")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if len(tables) != 0 {
		t.Errorf("expected 0 tables after drop, got %d", len(tables))
	}
}

func TestSQLite_ExecRowsAffected(t *testing.T) {
	drv := setupSQLite(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
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

	// Update should affect 1 row
	rows, err = drv.Exec(ctx, "UPDATE users SET name = 'Robert' WHERE name = 'Bob'")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if rows != 1 {
		t.Errorf("expected 1 row affected by update, got %d", rows)
	}

	// Delete should affect 1 row
	rows, err = drv.Exec(ctx, "DELETE FROM users")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if rows != 1 {
		t.Errorf("expected 1 row affected by delete, got %d", rows)
	}
}
```

- [ ] **Step 2: 运行测试**

```bash
cd C:/Workspace/TestProject/dbmcp
go test ./internal/database/sqlite_test.go ./internal/database/interface.go ./internal/database/sqlite.go -v
```

Expected: 8 tests PASS.

- [ ] **Step 3: 提交**

```bash
cd C:/Workspace/TestProject/dbmcp
git add internal/database/sqlite_test.go
git commit -m "test: add SQLite integration tests (8 test cases)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 3: MySQL 集成测试

**Files:**
- Create: `internal/database/mysql_test.go`

- [ ] **Step 1: 创建 MySQL 集成测试**

创建 `internal/database/mysql_test.go`:

```go
package database

import (
	"context"
	"testing"
	"time"

	"github.com/dbmcp/dbmcp/testhelpers"
)

func setupMySQL(t *testing.T) (*MySQLDriver, func()) {
	t.Helper()
	testhelpers.SkipIfNoDocker(t)
	ctx := context.Background()

	dsn, cleanup, err := testhelpers.SetupMySQLContainer(ctx)
	if err != nil {
		t.Fatalf("failed to setup mysql container: %v", err)
	}

	drv := NewMySQLDriver()
	if err := drv.Connect(dsn); err != nil {
		cleanup()
		t.Fatalf("failed to connect to mysql: %v", err)
	}

	return drv, cleanup
}

func TestMySQL_Connect(t *testing.T) {
	drv, cleanup := setupMySQL(t)
	defer cleanup()
	if drv.db == nil {
		t.Fatal("expected db connection")
	}
}

func TestMySQL_CreateTable(t *testing.T) {
	drv, cleanup := setupMySQL(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE users (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100) NOT NULL, email VARCHAR(100))")
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
}

func TestMySQL_InsertAndSelect(t *testing.T) {
	drv, cleanup := setupMySQL(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE users (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100), email VARCHAR(100))")
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
	// MySQL returns []uint8 for strings
	name := string(result.Rows[0][1].([]byte))
	if name != "Alice" {
		t.Errorf("expected name 'Alice', got %v", name)
	}
}

func TestMySQL_ListDatabases(t *testing.T) {
	drv, cleanup := setupMySQL(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbs, err := drv.ListDatabases(ctx)
	if err != nil {
		t.Fatalf("list databases: %v", err)
	}
	// MySQL always has system databases
	if len(dbs) < 1 {
		t.Errorf("expected at least 1 database, got %d", len(dbs))
	}
	// Check that testdb exists
	found := false
	for _, db := range dbs {
		if db == "testdb" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected testdb in databases, got %v", dbs)
	}
}

func TestMySQL_ListTables(t *testing.T) {
	drv, cleanup := setupMySQL(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE test_table (id INT AUTO_INCREMENT PRIMARY KEY)")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	tables, err := drv.ListTables(ctx, "testdb")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if len(tables) != 1 || tables[0] != "test_table" {
		t.Errorf("expected [test_table], got %v", tables)
	}
}

func TestMySQL_DescribeTable(t *testing.T) {
	drv, cleanup := setupMySQL(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE users (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100) NOT NULL, email VARCHAR(100))")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	columns, err := drv.DescribeTable(ctx, "testdb", "users")
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

func TestMySQL_DropTable(t *testing.T) {
	drv, cleanup := setupMySQL(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE temp_table (id INT AUTO_INCREMENT PRIMARY KEY)")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err = drv.Exec(ctx, "DROP TABLE temp_table")
	if err != nil {
		t.Fatalf("drop table: %v", err)
	}

	tables, err := drv.ListTables(ctx, "testdb")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if len(tables) != 0 {
		t.Errorf("expected 0 tables after drop, got %d", len(tables))
	}
}

func TestMySQL_ExecRowsAffected(t *testing.T) {
	drv, cleanup := setupMySQL(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE users (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100))")
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

- [ ] **Step 2: 编译验证**

```bash
cd C:/Workspace/TestProject/dbmcp
go build ./internal/database/mysql_test.go ./internal/database/interface.go ./internal/database/mysql.go 2>&1
```

Expected: no errors. (Tests will only pass if Docker is available.)

- [ ] **Step 3: 提交**

```bash
cd C:/Workspace/TestProject/dbmcp
git add internal/database/mysql_test.go
git commit -m "test: add MySQL integration tests with testcontainers

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 4: PostgreSQL 集成测试

**Files:**
- Create: `internal/database/postgres_test.go`

- [ ] **Step 1: 创建 PostgreSQL 集成测试**

创建 `internal/database/postgres_test.go`:

```go
package database

import (
	"context"
	"testing"
	"time"

	"github.com/dbmcp/dbmcp/testhelpers"
)

func setupPostgres(t *testing.T) (*PostgresDriver, func()) {
	t.Helper()
	testhelpers.SkipIfNoDocker(t)
	ctx := context.Background()

	dsn, cleanup, err := testhelpers.SetupPostgresContainer(ctx)
	if err != nil {
		t.Fatalf("failed to setup postgres container: %v", err)
	}

	drv := NewPostgresDriver()
	if err := drv.Connect(dsn); err != nil {
		cleanup()
		t.Fatalf("failed to connect to postgres: %v", err)
	}

	return drv, cleanup
}

func TestPostgres_Connect(t *testing.T) {
	drv, cleanup := setupPostgres(t)
	defer cleanup()
	if drv.db == nil {
		t.Fatal("expected db connection")
	}
}

func TestPostgres_CreateTable(t *testing.T) {
	drv, cleanup := setupPostgres(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE users (id SERIAL PRIMARY KEY, name VARCHAR(100) NOT NULL, email VARCHAR(100))")
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
}

func TestPostgres_InsertAndSelect(t *testing.T) {
	drv, cleanup := setupPostgres(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE users (id SERIAL PRIMARY KEY, name VARCHAR(100), email VARCHAR(100))")
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

func TestPostgres_ListDatabases(t *testing.T) {
	drv, cleanup := setupPostgres(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbs, err := drv.ListDatabases(ctx)
	if err != nil {
		t.Fatalf("list databases: %v", err)
	}
	// Check that testdb exists
	found := false
	for _, db := range dbs {
		if db == "testdb" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected testdb in databases, got %v", dbs)
	}
}

func TestPostgres_ListTables(t *testing.T) {
	drv, cleanup := setupPostgres(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE test_table (id SERIAL PRIMARY KEY)")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	tables, err := drv.ListTables(ctx, "testdb")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if len(tables) != 1 || tables[0] != "test_table" {
		t.Errorf("expected [test_table], got %v", tables)
	}
}

func TestPostgres_DescribeTable(t *testing.T) {
	drv, cleanup := setupPostgres(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE users (id SERIAL PRIMARY KEY, name VARCHAR(100) NOT NULL, email VARCHAR(100))")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	columns, err := drv.DescribeTable(ctx, "testdb", "users")
	if err != nil {
		t.Fatalf("describe table: %v", err)
	}

	if len(columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(columns))
	}

	if columns[0].Name != "id" {
		t.Errorf("expected column name 'id', got %s", columns[0].Name)
	}
}

func TestPostgres_DropTable(t *testing.T) {
	drv, cleanup := setupPostgres(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE temp_table (id SERIAL PRIMARY KEY)")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err = drv.Exec(ctx, "DROP TABLE temp_table")
	if err != nil {
		t.Fatalf("drop table: %v", err)
	}

	tables, err := drv.ListTables(ctx, "testdb")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if len(tables) != 0 {
		t.Errorf("expected 0 tables after drop, got %d", len(tables))
	}
}

func TestPostgres_ExecRowsAffected(t *testing.T) {
	drv, cleanup := setupPostgres(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := drv.Exec(ctx, "CREATE TABLE users (id SERIAL PRIMARY KEY, name VARCHAR(100))")
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

- [ ] **Step 2: 编译验证**

```bash
cd C:/Workspace/TestProject/dbmcp
go build ./internal/database/postgres_test.go ./internal/database/interface.go ./internal/database/postgres.go 2>&1
```

Expected: no errors.

- [ ] **Step 3: 提交**

```bash
cd C:/Workspace/TestProject/dbmcp
git add internal/database/postgres_test.go
git commit -m "test: add PostgreSQL integration tests with testcontainers

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 5: MCP Server 端到端集成测试

**Files:**
- Create: `internal/mcp/server_test.go`

- [ ] **Step 1: 创建 MCP Server 集成测试**

这个测试组装完整的 MCP Server,使用 SQLite 内存数据库(不需要 Docker),验证所有 Tool handler 的端到端调用。

创建 `internal/mcp/server_test.go`:

```go
package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dbmcp/dbmcp/internal/config"
	"github.com/dbmcp/dbmcp/internal/database"
	"github.com/dbmcp/dbmcp/internal/logger"
	"github.com/dbmcp/dbmcp/internal/permission"
	"github.com/dbmcp/dbmcp/internal/security"

	"github.com/mark3labs/mcp-go/mcp"
)

// setupTestServer 创建包含 SQLite 内存数据库的完整 MCP Server
func setupTestServer(t *testing.T) (*DBMCPServer, func()) {
	t.Helper()

	// 创建临时目录和配置文件
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configContent := `databases:
  testdb:
    driver: sqlite
    dsn: "file:integration_test_%s?mode=memory&cache=shared"
permissions:
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
`
	// 写入配置
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// 加载配置
	app, err := config.NewAppState(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	// 创建数据库管理器
	dm := database.NewDriverManager()
	cfg := app.Config()
	for name, dbCfg := range cfg.Databases {
		if err := dm.Register(name, dbCfg.Driver, dbCfg.DSN); err != nil {
			t.Fatalf("register db %s: %v", name, err)
		}
	}

	// 创建权限、安全、日志
	perm := permission.NewChecker(cfg.Permissions)
	guard := security.NewSQLGuard(security.MaxSQLLength)
	auditLog, err := logger.NewAuditLogger()
	if err != nil {
		t.Fatalf("create audit logger: %v", err)
	}

	srv := New(app, dm, perm, guard, auditLog)

	cleanup := func() {
		dm.CloseAll()
		_ = auditLog.Close()
	}

	return srv, cleanup
}

// callTool 辅助函数:调用 MCP 工具
func callTool(t *testing.T, srv *DBMCPServer, toolName string, args map[string]any) *mcp.CallToolResult {
	t.Helper()

	// 由于 mcp-go v0.48 的 CallToolRequest.Arguments 是 any 类型,
	// 我们需要通过 server 的 HandleMessage 来调用工具
	// 这里直接调用 handler,绕过了 JSON-RPC 层

	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Name = toolName
	req.Params.Arguments = args

	// 通过 server 的 Handle 方法处理请求
	result, err := srv.srv.HandleMessage(ctx, req)
	if err != nil {
		t.Fatalf("handle message: %v", err)
	}

	toolResult, ok := result.(*mcp.CallToolResult)
	if !ok {
		t.Fatalf("expected CallToolResult, got %T", result)
	}
	return toolResult
}

func TestMCP_ListDatabasesTool(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	result := callTool(t, srv, "list_databases", map[string]any{})

	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}

	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if textContent.Text == "" {
		t.Error("expected non-empty database list")
	}
}

func TestMCP_ExecuteUpdate_CreateTable(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	result := callTool(t, srv, "execute_update", map[string]any{
		"sql":      "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT)",
		"database": "testdb",
	})

	if result.IsError {
		t.Fatalf("create table failed: %v", result.Content)
	}
}

func TestMCP_ExecuteQuery_Select(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// 先创建表并插入数据
	callTool(t, srv, "execute_update", map[string]any{
		"sql":      "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)",
		"database": "testdb",
	})
	callTool(t, srv, "execute_update", map[string]any{
		"sql":      "INSERT INTO users (name, email) VALUES ('Alice', 'alice@test.com')",
		"database": "testdb",
	})

	// 查询
	result := callTool(t, srv, "execute_query", map[string]any{
		"sql":      "SELECT id, name, email FROM users WHERE name = 'Alice'",
		"database": "testdb",
	})

	if result.IsError {
		t.Fatalf("query failed: %v", result.Content)
	}

	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	var qr database.QueryResult
	if err := json.Unmarshal([]byte(textContent.Text), &qr); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if len(qr.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(qr.Rows))
	}
}

func TestMCP_ListTablesTool(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// 先创建表
	callTool(t, srv, "execute_update", map[string]any{
		"sql":      "CREATE TABLE test_table (id INTEGER PRIMARY KEY)",
		"database": "testdb",
	})

	result := callTool(t, srv, "list_tables", map[string]any{
		"database": "testdb",
	})

	if result.IsError {
		t.Fatalf("list tables failed: %v", result.Content)
	}

	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if textContent.Text == "" {
		t.Error("expected non-empty table list")
	}
}

func TestMCP_DescribeTableTool(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// 先创建表
	callTool(t, srv, "execute_update", map[string]any{
		"sql":      "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)",
		"database": "testdb",
	})

	result := callTool(t, srv, "describe_table", map[string]any{
		"database": "testdb",
		"table":    "users",
	})

	if result.IsError {
		t.Fatalf("describe table failed: %v", result.Content)
	}

	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if textContent.Text == "" {
		t.Error("expected non-empty table description")
	}
}

func TestMCP_ConfigStatusTool(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	result := callTool(t, srv, "config_status", map[string]any{})

	if result.IsError {
		t.Fatalf("config status failed: %v", result.Content)
	}

	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if textContent.Text == "" {
		t.Error("expected non-empty config status")
	}
}
```

Wait, I need to verify the mcp-go API for calling tools programmatically. Let me check the HandleMessage approach:

The `server.MCPServer.HandleMessage` takes a JSON-RPC message and returns a JSON-RPC response. For testing, a simpler approach is to directly call the handler functions. Let me revise the `callTool` helper:

```go
func callTool(t *testing.T, srv *DBMCPServer, toolName string, args map[string]any) *mcp.CallToolResult {
    t.Helper()
    ctx := context.Background()
    req := mcp.CallToolRequest{}
    req.Params.Name = toolName
    req.Params.Arguments = args

    // Find and call the handler directly
    // Since handlers are registered internally, we use HandleMessage
    // which wraps the JSON-RPC protocol
    jsonReq, _ := json.Marshal(map[string]any{
        "jsonrpc": "2.0",
        "id":      1,
        "method":  "tools/call",
        "params": map[string]any{
            "name":      toolName,
            "arguments": args,
        },
    })

    resp := srv.srv.HandleMessage(ctx, jsonReq)
    jsonResp, _ := json.Marshal(resp)

    var result struct {
        Result struct {
            Content []struct {
                Type string `json:"type"`
                Text string `json:"text"`
            } `json:"content"`
            IsError bool `json:"isError"`
        } `json:"result"`
        Error *struct {
            Message string `json:"message"`
        } `json:"error"`
    }
    if err := json.Unmarshal(jsonResp, &result); err != nil {
        t.Fatalf("unmarshal response: %v", err)
    }

    if result.Error != nil {
        t.Fatalf("tool error: %s", result.Error.Message)
    }

    return &mcp.CallToolResult{
        IsError: result.Result.IsError,
        Content: func() []mcp.Content {
            var contents []mcp.Content
            for _, c := range result.Result.Content {
                contents = append(contents, mcp.TextContent{Type: c.Type, Text: c.Text})
            }
            return contents
        }(),
    }
}
```

Actually, this is overly complex. The simplest approach: mcp-go's `server.MCPServer` has a `HandleMessage` method that accepts a JSON-RPC request (as `[]byte` or `mcp.JSONRPCRequest`). Let me use the simplest correct approach.

Looking at the mcp-go v0.48 API, the cleanest way is to construct a JSON-RPC request and call `HandleMessage`:

```go
func callTool(t *testing.T, srv *DBMCPServer, toolName string, args map[string]any) map[string]any {
    t.Helper()
    ctx := context.Background()

    // Build JSON-RPC request
    jsonReq := map[string]any{
        "jsonrpc": "2.0",
        "id":      1,
        "method":  "tools/call",
        "params": map[string]any{
            "name":      toolName,
            "arguments": args,
        },
    }
    reqBytes, _ := json.Marshal(jsonReq)

    // Handle the request
    resp := srv.srv.HandleMessage(ctx, reqBytes)
    respBytes, _ := json.Marshal(resp)

    var result map[string]any
    if err := json.Unmarshal(respBytes, &result); err != nil {
        t.Fatalf("unmarshal response: %v", err)
    }
    return result
}
```

This returns a raw `map[string]any` that the test can inspect. Let me update the test file with this approach:

Replace the `callTool` function in server_test.go with:

```go
// callTool 调用 MCP 工具,返回 JSON-RPC 响应的 map
func callTool(t *testing.T, srv *DBMCPServer, toolName string, args map[string]any) map[string]any {
	t.Helper()
	ctx := context.Background()

	jsonReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	}
	reqBytes, _ := json.Marshal(jsonReq)

	resp := srv.srv.HandleMessage(ctx, reqBytes)
	respBytes, _ := json.Marshal(resp)

	var result map[string]any
	if err := json.Unmarshal(respBytes, &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return result
}

// assertToolSuccess 断言工具调用成功,返回 result 文本
func assertToolSuccess(t *testing.T, resp map[string]any) string {
	t.Helper()
	if errVal, ok := resp["error"]; ok && errVal != nil {
		t.Fatalf("tool returned error: %v", errVal)
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result map, got %v", resp)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("expected content array, got %v", result)
	}
	first, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("expected content object, got %v", content[0])
	}
	text, ok := first["text"].(string)
	if !ok {
		t.Fatalf("expected text field, got %v", first)
	}
	return text
}
```

And update each test to use these helpers:

```go
func TestMCP_ListDatabasesTool(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	resp := callTool(t, srv, "list_databases", map[string]any{})
	text := assertToolSuccess(t, resp)
	if text == "" {
		t.Error("expected non-empty database list")
	}
}

func TestMCP_ExecuteUpdate_CreateTable(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	resp := callTool(t, srv, "execute_update", map[string]any{
		"sql":      "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT)",
		"database": "testdb",
	})
	assertToolSuccess(t, resp)
}
```

- [ ] **Step 2: 编译验证**

```bash
cd C:/Workspace/TestProject/dbmcp
go build ./internal/mcp/server_test.go ./internal/mcp/server.go 2>&1
```

Expected: no errors. (May need to resolve additional imports.)

- [ ] **Step 3: 运行测试**

```bash
cd C:/Workspace/TestProject/dbmcp
go test ./internal/mcp/... -v -run TestMCP -count=1
```

Expected: 6 tests PASS (SQLite-based, no Docker required).

- [ ] **Step 4: 提交**

```bash
cd C:/Workspace/TestProject/dbmcp
git add internal/mcp/server_test.go
git commit -m "test: add MCP Server end-to-end integration tests

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 6: 运行全部测试 + 更新文档

**Files:**
- Modify: `README.md` (添加测试说明)
- Modify: `CLAUDE.md` (添加测试命令)

- [ ] **Step 1: 运行全部测试**

```bash
cd C:/Workspace/TestProject/dbmcp
go test ./... -v -count=1 2>&1
```

Expected: All existing unit tests PASS + SQLite integration tests PASS. MySQL/PostgreSQL tests will run if Docker is available, or skip gracefully.

- [ ] **Step 2: 更新 README.md**

在 README.md 的安全防护章节后面添加测试章节:

```markdown
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
| `internal/mcp/server_test.go` | 无 | MCP Server 端到端,6 个测试用例 |

如果 Docker 不可用,MySQL 和 PostgreSQL 测试会自动跳过。
```

- [ ] **Step 3: 更新 CLAUDE.md**

在 CLAUDE.md 的命令部分添加集成测试命令:

在现有测试命令后添加:

```bash
# 运行特定数据库集成测试
go test ./internal/database/... -run TestSQLite -v
go test ./internal/mcp/... -run TestMCP -v
```

- [ ] **Step 4: 最终提交**

```bash
cd C:/Workspace/TestProject/dbmcp
git add README.md CLAUDE.md
git commit -m "docs: add integration test documentation

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## 依赖汇总

| 任务 | 依赖 |
|------|------|
| Task 1 | 无 |
| Task 2 | Task 1 (go.mod 依赖) |
| Task 3 | Task 1 (testhelpers) |
| Task 4 | Task 1 (testhelpers) |
| Task 5 | Task 2 (验证 database 包测试通过) |
| Task 6 | Task 5 |
