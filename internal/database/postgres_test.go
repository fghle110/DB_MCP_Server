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
	name, ok := result.Rows[0][1].(string)
	if !ok {
		t.Fatalf("expected name to be string, got %T", result.Rows[0][1])
	}
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
