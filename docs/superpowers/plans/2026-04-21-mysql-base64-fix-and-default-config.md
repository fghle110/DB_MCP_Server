# MySQL Base64 Fix & Default Config Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix MySQL/PostgreSQL/MSSQL query results returning base64-encoded strings instead of readable text, and auto-generate a default config file on first run.

**Architecture:** Add a shared `convertBytes` helper in `database/interface.go` used by all three `Query` methods. Add `GenerateDefaultConfig` in `config.go`. Modify `main.go` to call it when config is missing.

**Tech Stack:** Go 1.26, database/sql, gopkg.in/yaml.v3

---

### Task 1: Add shared `convertBytes` helper and fix MySQL `Query`

**Files:**
- Modify: `internal/database/interface.go` — add `convertBytes` helper function
- Modify: `internal/database/mysql.go:40-64` — call `convertBytes` in `Query`
- Modify: `internal/database/mysql_test.go:77-80` — fix test assertion (now `string` not `[]byte`)
- Test: `internal/database/mysql_test.go` — existing test

- [ ] **Step 1: Add `convertBytes` helper to `interface.go`**

Add this function at the end of `internal/database/interface.go` (after the `DatabaseDriver` interface):

```go
// convertBytes converts []byte values to string.
// database/sql scans VARCHAR/TEXT/BLOB columns as []byte by default.
// Go's encoding/json then encodes []byte as base64. This function
// prevents that by converting to string before serialization.
func convertBytes(values []interface{}) []interface{} {
	for i, v := range values {
		if b, ok := v.([]byte); ok {
			values[i] = string(b)
		}
	}
	return values
}
```

- [ ] **Step 2: Fix MySQL `Query` to call `convertBytes`**

In `internal/database/mysql.go`, replace the `Query` method's row accumulation loop (lines 53-63). Current code:

```go
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
```

Change to:

```go
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
		result.Rows = append(result.Rows, convertBytes(values))
	}
```

The only change is `convertBytes(values)` instead of `values` on the append line.

- [ ] **Step 3: Fix MySQL test assertion**

In `internal/database/mysql_test.go`, lines 77-80. Current code:

```go
	name := string(result.Rows[0][1].([]byte))
	if name != "Alice" {
		t.Errorf("expected name 'Alice', got %v", name)
	}
```

Change to:

```go
	name, ok := result.Rows[0][1].(string)
	if !ok {
		t.Fatalf("expected name to be string, got %T", result.Rows[0][1])
	}
	if name != "Alice" {
		t.Errorf("expected name 'Alice', got %v", name)
	}
```

- [ ] **Step 4: Run tests to verify**

Run: `go test ./internal/database/... -run TestMySQL -v`
Expected: All MySQL tests PASS (requires Docker). If no Docker, run `go build ./...` to verify compilation at minimum.

- [ ] **Step 5: Commit**

```bash
git add internal/database/interface.go internal/database/mysql.go internal/database/mysql_test.go
git commit -m "fix: convert []byte to string in MySQL query results to prevent base64 encoding"
```

---

### Task 2: Fix PostgreSQL and MSSQL `Query` methods

**Files:**
- Modify: `internal/database/postgres.go:52-63` — call `convertBytes` in `Query`
- Modify: `internal/database/mssql.go:58-69` — call `convertBytes` in `Query`

- [ ] **Step 1: Fix PostgreSQL `Query`**

In `internal/database/postgres.go`, change line 62 from:

```go
		result.Rows = append(result.Rows, values)
```

To:

```go
		result.Rows = append(result.Rows, convertBytes(values))
```

- [ ] **Step 2: Fix MSSQL `Query`**

In `internal/database/mssql.go`, change line 68 from:

```go
		result.Rows = append(result.Rows, values)
```

To:

```go
		result.Rows = append(result.Rows, convertBytes(values))
```

- [ ] **Step 3: Run build to verify compilation**

Run: `go build ./internal/database/...`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add internal/database/postgres.go internal/database/mssql.go
git commit -m "fix: convert []byte to string in PostgreSQL and MSSQL query results"
```

---

### Task 3: Add `GenerateDefaultConfig` function

**Files:**
- Modify: `internal/config/config.go` — add `GenerateDefaultConfig` function

- [ ] **Step 1: Add `GenerateDefaultConfig` function**

Add to the end of `internal/config/config.go` (after the `BackupConfig` function):

```go
// GenerateDefaultConfig creates a default config file with placeholder
// database entries for common database types.
func GenerateDefaultConfig(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	cfg := &AppConfig{
		DatabaseGroups: DatabaseGroups{
			Relational: map[string]DatabaseConfig{
				"my_mysql": {
					Driver:   "mysql",
					Host:     "localhost",
					Port:     3306,
					Username: "root",
					Password: "",
					Database: "",
					Options:  map[string]string{"parseTime": "true"},
				},
				"my_postgres": {
					Driver:   "postgres",
					Host:     "localhost",
					Port:     5432,
					Username: "postgres",
					Password: "",
					Database: "postgres",
					Options:  map[string]string{"sslmode": "disable"},
				},
			},
			Nosql: map[string]DatabaseConfig{
				"my_redis": {
					Driver:   "redis",
					Host:     "localhost",
					Port:     6379,
					Password: "",
					Options:  map[string]string{"db": "0"},
				},
			},
			Timeseries: make(map[string]DatabaseConfig),
			Graph:      make(map[string]DatabaseConfig),
		},
		PermissionsGroup: PermissionsGroup{
			Relational: map[string]PermissionConfig{
				"my_mysql": {
					ReadOnly:         false,
					AllowedDatabases: []string{"*"},
					AllowedActions:   []string{"SELECT", "INSERT", "UPDATE", "DELETE"},
					BlockedTables:    []string{},
				},
				"my_postgres": {
					ReadOnly:         false,
					AllowedDatabases: []string{"*"},
					AllowedActions:   []string{"SELECT", "INSERT", "UPDATE", "DELETE"},
					BlockedTables:    []string{},
				},
			},
			Nosql: map[string]NosqlPermissionConfig{
				"my_redis": {
					ReadOnly: false,
					AllowedCommands: []string{
						"GET", "SET", "HGET", "HGETALL", "HSET",
						"LPUSH", "LRANGE", "SCAN", "INFO", "DEL",
						"EXISTS", "TTL", "TYPE", "PING",
					},
					BlockedKeys: []string{},
				},
			},
			Timeseries: make(map[string]PermissionConfig),
			Graph:      make(map[string]PermissionConfig),
		},
	}

	NormalizeConfig(cfg)
	applyDefaults(cfg)

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal default config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write default config: %w", err)
	}

	return nil
}
```

Also need to add the `path/filepath` import to the existing imports in `config.go`. Current imports:

```go
import (
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)
```

Add `"path/filepath"` to the standard library imports:

```go
import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)
```

- [ ] **Step 2: Add unit test for `GenerateDefaultConfig`**

Add to `internal/config/config_test.go`:

```go
func TestGenerateDefaultConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	err := GenerateDefaultConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("expected default config file to be created")
	}

	// Verify it can be loaded
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("failed to load generated default config: %v", err)
	}

	// Verify expected databases exist
	if _, ok := cfg.DatabaseGroups.Relational["my_mysql"]; !ok {
		t.Error("expected my_mysql in relational databases")
	}
	if _, ok := cfg.DatabaseGroups.Relational["my_postgres"]; !ok {
		t.Error("expected my_postgres in relational databases")
	}
	if _, ok := cfg.DatabaseGroups.Nosql["my_redis"]; !ok {
		t.Error("expected my_redis in nosql databases")
	}

	// Verify permissions exist for each database
	if _, ok := cfg.PermissionsGroup.Relational["my_mysql"]; !ok {
		t.Error("expected per-database permission for my_mysql")
	}
	if _, ok := cfg.PermissionsGroup.Nosql["my_redis"]; !ok {
		t.Error("expected per-nosql permission for my_redis")
	}
}

func TestGenerateDefaultConfig_InvalidPath(t *testing.T) {
	// On most systems, writing to root-level directory without
	// permission should fail
	err := GenerateDefaultConfig("/proc/impossible/config.yaml")
	if err == nil {
		t.Error("expected error for unwritable path")
	}
}
```

- [ ] **Step 3: Run tests to verify**

Run: `go test ./internal/config/... -v`
Expected: All config tests PASS, including the new `TestGenerateDefaultConfig` and `TestGenerateDefaultConfig_InvalidPath`.

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add GenerateDefaultConfig function for first-run experience"
```

---

### Task 4: Modify `main.go` to auto-generate config on first run

**Files:**
- Modify: `cmd/dbmcp/main.go:23-36` — replace `log.Fatalf` with config generation logic

- [ ] **Step 1: Update `main.go` startup logic**

Current code (lines 23-36):

```go
	if *configPath == "" {
		home := os.Getenv("HOME")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
		if home == "" {
			log.Fatal("cannot determine home directory, use --config flag")
		}
		*configPath = home + "/.dbmcp/config.yaml"
	}

	if _, err := os.Stat(*configPath); os.IsNotExist(err) {
		log.Fatalf("config file not found: %s\nRun with --config to specify a custom path.", *configPath)
	}
```

Replace with:

```go
	if *configPath == "" {
		home := os.Getenv("HOME")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
		if home == "" {
			log.Fatal("cannot determine home directory, use --config flag")
		}
		*configPath = home + string(os.PathSeparator) + ".dbmcp" + string(os.PathSeparator) + "config.yaml"
	}

	if _, err := os.Stat(*configPath); os.IsNotExist(err) {
		log.Printf("Config file not found: %s", *configPath)
		log.Println("Generating default config file...")
		if err := config.GenerateDefaultConfig(*configPath); err != nil {
			log.Fatalf("Failed to generate default config: %v", err)
		}
		log.Printf("Default config created: %s", *configPath)
		log.Println("Please edit the config file with your database credentials, then restart dbmcp.")
		os.Exit(0)
	}
```

Key changes:
- Use `os.PathSeparator` instead of hardcoded `/` for cross-platform compatibility (Windows uses `\`)
- Replace `log.Fatalf` with `config.GenerateDefaultConfig` call, then print instructions and `os.Exit(0)`

- [ ] **Step 2: Run build to verify**

Run: `go build -o build/dbmcp.exe ./cmd/dbmcp`
Expected: No errors, binary created at `build/dbmcp.exe`

- [ ] **Step 3: Manual test — first run without config**

Run: `./build/dbmcp.exe` (without any config file existing)
Expected output:
```
Config file not found: C:\Users\<user>\.dbmcp\config.yaml
Generating default config file...
Default config created: C:\Users\<user>\.dbmcp\config.yaml
Please edit the config file with your database credentials, then restart dbmcp.
```

Then verify the file exists and contains valid YAML with the expected database entries.

- [ ] **Step 4: Commit**

```bash
git add cmd/dbmcp/main.go
git commit -m "feat: auto-generate default config on first run instead of exiting"
```

---

### Task 5: Run full test suite and verify

**Files:**
- All of the above

- [ ] **Step 1: Run full test suite**

Run: `go test ./... -v`
Expected: All tests PASS (integration tests requiring Docker will be skipped if Docker is not available).

- [ ] **Step 2: Verify build**

Run: `go build -o build/dbmcp.exe ./cmd/dbmcp`
Expected: No errors.

- [ ] **Step 3: Final commit**

```bash
git status
git log --oneline -5
```

Verify all commits are clean and messages are descriptive.
