package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dbmcp/dbmcp/internal/config"
	"github.com/dbmcp/dbmcp/internal/database"
	"github.com/dbmcp/dbmcp/internal/logger"
	"github.com/dbmcp/dbmcp/internal/permission"
	"github.com/dbmcp/dbmcp/internal/security"
)

// setupTestServer creates a full MCP Server with SQLite memory database
func setupTestServer(t *testing.T) (*DBMCPServer, func()) {
	t.Helper()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configContent := `databases:
  testdb:
    driver: sqlite
    dsn: "file:mcp_e2e_test?mode=memory&cache=shared"
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
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	app, err := config.NewAppState(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	dm := database.NewDriverManager()
	cfg := app.Config()
	for name, dbCfg := range cfg.Databases {
		if err := dm.Register(name, dbCfg.Driver, dbCfg); err != nil {
			t.Fatalf("register db %s: %v", name, err)
		}
	}

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

// callTool calls an MCP tool via JSON-RPC HandleMessage and returns the response map
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

// assertToolSuccess asserts the tool call succeeded and returns the text content
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

func TestMCP_ExecuteQuery_Select(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Setup: create table and insert data
	callTool(t, srv, "execute_update", map[string]any{
		"sql":      "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)",
		"database": "testdb",
	})
	callTool(t, srv, "execute_update", map[string]any{
		"sql":      "INSERT INTO users (name, email) VALUES ('Alice', 'alice@test.com')",
		"database": "testdb",
	})

	// Query
	resp := callTool(t, srv, "execute_query", map[string]any{
		"sql":      "SELECT id, name, email FROM users WHERE name = 'Alice'",
		"database": "testdb",
	})
	text := assertToolSuccess(t, resp)

	var qr database.QueryResult
	if err := json.Unmarshal([]byte(text), &qr); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(qr.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(qr.Rows))
	}
}

func TestMCP_ListTablesTool(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	callTool(t, srv, "execute_update", map[string]any{
		"sql":      "CREATE TABLE test_table (id INTEGER PRIMARY KEY)",
		"database": "testdb",
	})

	resp := callTool(t, srv, "list_tables", map[string]any{
		"database": "testdb",
	})
	text := assertToolSuccess(t, resp)
	if text == "" {
		t.Error("expected non-empty table list")
	}
}

func TestMCP_DescribeTableTool(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	callTool(t, srv, "execute_update", map[string]any{
		"sql":      "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)",
		"database": "testdb",
	})

	resp := callTool(t, srv, "describe_table", map[string]any{
		"database": "testdb",
		"table":    "users",
	})
	text := assertToolSuccess(t, resp)
	if text == "" {
		t.Error("expected non-empty table description")
	}
}

func TestMCP_ConfigStatusTool(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	resp := callTool(t, srv, "config_status", map[string]any{})
	text := assertToolSuccess(t, resp)
	if text == "" {
		t.Error("expected non-empty config status")
	}
}
