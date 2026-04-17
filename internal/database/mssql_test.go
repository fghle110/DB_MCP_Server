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
