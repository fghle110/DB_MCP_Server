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

	if columns[0].Name != "id" {
		t.Errorf("expected column name 'id', got %s", columns[0].Name)
	}
	if columns[0].Key != "PRI" {
		t.Errorf("expected key 'PRI' for id column, got %s", columns[0].Key)
	}

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
