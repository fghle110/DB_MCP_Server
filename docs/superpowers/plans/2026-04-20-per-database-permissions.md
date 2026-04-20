# Per-Database Permission Control Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将权限模型从按类型全局共享改为按数据库名称独立控制，每个数据库有独立的 read_only、allowed_actions、blocked_tables 配置，旧配置自动迁移并写回文件。

**Architecture:** 修改 `PermissionsGroup` 结构体将 relational/nosql 从单一对象改为 `map[string]`，Checker 按数据库名查找权限，`LoadConfig` 检测旧格式后自动迁移写回。

**Tech Stack:** Go 1.26, yaml.v3, gopkg.in/yaml.v3 Marshal/Unmarshal

---

## File Structure

| File | Operation | Responsibility |
|------|-----------|----------------|
| `internal/config/config.go` | Modify | `PermissionsGroup` 结构体变更，新增 `HasOldFormat()` 检测，`LoadConfig` 增加迁移写回逻辑，`applyDefaults` 生成 per-database 权限 |
| `internal/config/config_test.go` | Modify | 新增 per-database 权限测试、旧格式迁移写回测试 |
| `internal/permission/permission.go` | Modify | `Checker` 存储结构变更，`CheckSelect`/`CheckWrite`/`CheckRedisCommand` 改为 per-database 查找，`Update`/`UpdateNosql` 改为按数据库名更新，新增 `UpdateFromConfig` |
| `internal/permission/permission_test.go` | Modify | 适配新 Checker 签名的所有测试 |
| `internal/mcp/server.go` | Modify | `CheckRedisCommand` 调用增加 `database` 参数 |
| `cmd/dbmcp/main.go` | Modify | `NewChecker` 调用适配新签名，热重载适配 |

---

### Task 1: 修改 config.go 结构体和默认值逻辑

**Files:**
- Modify: `internal/config/config.go`

- [x] **Step 1: 修改 PermissionsGroup 结构体**

将 `PermissionsGroup` 中的 `Relational` 和 `Nosql` 从单一对象改为 map：

```go
// PermissionsGroup 按类型分组的权限配置
type PermissionsGroup struct {
	Relational map[string]PermissionConfig      `yaml:"relational"`
	Nosql      map[string]NosqlPermissionConfig `yaml:"nosql"`
	Timeseries map[string]PermissionConfig      `yaml:"timeseries"`
	Graph      map[string]PermissionConfig      `yaml:"graph"`
}
```

- [x] **Step 2: 修改 applyDefaults 函数**

替换原有 `applyDefaults` 函数，为每个已存在的数据库生成独立权限配置：

```go
// applyDefaults 应用默认权限配置
func applyDefaults(cfg *AppConfig) {
	// 初始化权限 map
	if cfg.PermissionsGroup.Relational == nil {
		cfg.PermissionsGroup.Relational = make(map[string]PermissionConfig)
	}
	if cfg.PermissionsGroup.Nosql == nil {
		cfg.PermissionsGroup.Nosql = make(map[string]NosqlPermissionConfig)
	}
	if cfg.PermissionsGroup.Timeseries == nil {
		cfg.PermissionsGroup.Timeseries = make(map[string]PermissionConfig)
	}
	if cfg.PermissionsGroup.Graph == nil {
		cfg.PermissionsGroup.Graph = make(map[string]PermissionConfig)
	}

	// 为每个 relational 数据库生成默认权限
	for name := range cfg.DatabaseGroups.Relational {
		if _, exists := cfg.PermissionsGroup.Relational[name]; !exists {
			cfg.PermissionsGroup.Relational[name] = PermissionConfig{
				ReadOnly:         false,
				AllowedDatabases: []string{"*"},
				AllowedActions:   []string{"SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "DROP"},
				BlockedTables:    []string{},
			}
		}
	}

	// 为每个 nosql 数据库生成默认权限
	for name := range cfg.DatabaseGroups.Nosql {
		if _, exists := cfg.PermissionsGroup.Nosql[name]; !exists {
			cfg.PermissionsGroup.Nosql[name] = NosqlPermissionConfig{
				ReadOnly:        false,
				AllowedCommands: []string{
					"GET", "SET", "HGET", "HGETALL", "HSET", "LPUSH", "LRANGE",
					"SADD", "SMEMBERS", "SCAN", "INFO", "DEL", "EXISTS", "TTL",
					"TYPE", "PING", "ECHO", "DBSIZE", "KEYS",
				},
				BlockedKeys: []string{},
			}
		}
	}

	NormalizeConfig(cfg)
}
```

- [x] **Step 3: 编译验证**

Run:
```bash
cd C:/Workspace/TestProject/dbmcp
go build ./internal/config/...
```

Expected: 编译失败（因为 permission.go 和 main.go 还在使用旧结构），这是预期行为。仅 config 包本身应能编译：

```bash
cd C:/Workspace/TestProject/dbmcp
go build ./internal/config/config.go
```

Expected: 无错误。

- [x] **Step 4: Commit**

```bash
cd C:/Workspace/TestProject/dbmcp
git add internal/config/config.go
git commit -m "feat: change PermissionsGroup to per-database map structure"
```

---

### Task 2: 实现配置自动迁移与文件写回

**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 1: 新增 HasOldFormat 检测函数**

在 `NormalizeConfig` 函数之前添加：

```go
// HasOldFormat 判断是否使用了旧格式（扁平 permissions 或单一对象 permissions_groups）
func (cfg *AppConfig) HasOldFormat() bool {
	// 检查扁平 permissions 是否有实际值
	if cfg.Permissions.ReadOnly || len(cfg.Permissions.AllowedActions) > 0 || len(cfg.Permissions.AllowedDatabases) > 0 || len(cfg.Permissions.BlockedTables) > 0 {
		return true
	}
	return false
}
```

- [ ] **Step 2: 修改 NormalizeConfig 将旧权限迁移到 per-database**

在现有数据库迁移逻辑之后（`NormalizeConfig` 函数末尾，`// 确保新格式 map 已初始化` 之前），添加 per-database 权限迁移：

```go
	// 迁移旧 permissions 到 per-database 权限
	if len(cfg.Databases) > 0 && (cfg.Permissions.ReadOnly || len(cfg.Permissions.AllowedActions) > 0) {
		oldPerm := cfg.Permissions
		for name := range cfg.DatabaseGroups.Relational {
			if _, exists := cfg.PermissionsGroup.Relational[name]; !exists {
				cfg.PermissionsGroup.Relational[name] = PermissionConfig{
					ReadOnly:         oldPerm.ReadOnly,
					AllowedDatabases: oldPerm.AllowedDatabases,
					AllowedActions:   oldPerm.AllowedActions,
					BlockedTables:    oldPerm.BlockedTables,
				}
			}
		}
	}
```

- [ ] **Step 3: 修改 LoadConfig 增加迁移写回逻辑**

替换现有 `LoadConfig` 函数：

```go
// LoadConfig 从文件加载配置
func LoadConfig(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// 检测是否需要迁移
	needsMigration := cfg.HasOldFormat()
	if needsMigration {
		applyDefaults(&cfg)
		// 序列化为新格式
		newData, err := yaml.Marshal(&cfg)
		if err != nil {
			return nil, fmt.Errorf("marshal migrated config: %w", err)
		}
		// 先备份
		if err := BackupConfig(path); err != nil {
			log.Printf("[config] backup failed before migration: %v", err)
		}
		// 写回新格式
		if err := os.WriteFile(path, newData, 0644); err != nil {
			return nil, fmt.Errorf("write migrated config: %w", err)
		}
		log.Println("[config] migrated config to per-database permissions")
	} else {
		applyDefaults(&cfg)
	}

	if err := ValidateConfig(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
```

注意：需要在 `config.go` 的 import 中增加 `"fmt"` 和 `"log"`（如果尚未导入）。

- [ ] **Step 4: 运行配置测试**

Run:
```bash
cd C:/Workspace/TestProject/dbmcp
go test ./internal/config/... -v
```

Expected: 所有测试通过，包括 `TestLoadConfig_FromFile` 中验证 per-database 权限已生成。

- [ ] **Step 5: Commit**

```bash
cd C:/Workspace/TestProject/dbmcp
git add internal/config/config.go
git commit -m "feat: auto-migrate old config format to per-database permissions with file write-back"
```

---

### Task 3: 修改 permission.go Checker 实现

**Files:**
- Modify: `internal/permission/permission.go`

- [x] **Step 1: 修改 Checker 结构体和 NewChecker**

```go
package permission

import (
	"fmt"
	"path/filepath"
	"sync/atomic"

	"github.com/dbmcp/dbmcp/internal/config"
)

// Checker 权限校验器
type Checker struct {
	relationalPerms atomic.Pointer[map[string]config.PermissionConfig]
	nosqlPerms      atomic.Pointer[map[string]config.NosqlPermissionConfig]
}

// NewChecker 创建权限校验器
func NewChecker(perms config.PermissionsGroup) *Checker {
	c := &Checker{}
	c.relationalPerms.Store(&perms.Relational)
	c.nosqlPerms.Store(&perms.Nosql)
	return c
}
```

- [x] **Step 2: 修改 CheckSelect 和 CheckWrite**

```go
// CheckSelect 检查 SELECT 权限
func (c *Checker) CheckSelect(database string, tableName string) error {
	perms := c.relationalPerms.Load()
	cfg, ok := (*perms)[database]
	if !ok {
		return fmt.Errorf("database '%s' has no permission config", database)
	}
	if err := c.checkTable(&cfg, tableName); err != nil {
		return err
	}
	return nil
}

// CheckWrite 检查写入权限
func (c *Checker) CheckWrite(database string, tableName string, actionType string) error {
	perms := c.relationalPerms.Load()
	cfg, ok := (*perms)[database]
	if !ok {
		return fmt.Errorf("database '%s' has no permission config", database)
	}
	if cfg.ReadOnly {
		return fmt.Errorf("database '%s' is in read-only mode, %s not allowed", database, actionType)
	}
	if err := c.checkTable(&cfg, tableName); err != nil {
		return err
	}
	if !c.isActionAllowed(&cfg, actionType) {
		return fmt.Errorf("action '%s' is not allowed for database '%s'", actionType, database)
	}
	return nil
}
```

- [x] **Step 3: 修改 CheckRedisCommand**

增加 `database` 参数：

```go
// CheckRedisCommand 检查 Redis 的命令权限
func (c *Checker) CheckRedisCommand(database string, cmd string, key string) error {
	perms := c.nosqlPerms.Load()
	cfg, ok := (*perms)[database]
	if !ok {
		return fmt.Errorf("database '%s' has no nosql permission config", database)
	}

	// read_only 模式：拦截写命令
	if cfg.ReadOnly && IsRedisWriteCommand(cmd) {
		return fmt.Errorf("database '%s' is in read-only mode, %s not allowed", database, cmd)
	}

	// 命令白名单
	allowed := false
	for _, a := range cfg.AllowedCommands {
		if a == cmd {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("redis command '%s' is not allowed", cmd)
	}

	// key 黑名单
	if key != "" {
		for _, pattern := range cfg.BlockedKeys {
			if matched, _ := filepath.Match(pattern, key); matched {
				return fmt.Errorf("key '%s' matches blocked pattern '%s'", key, pattern)
			}
		}
	}

	return nil
}
```

- [x] **Step 4: 修改 Update/UpdateNosql/IsReadOnly 方法**

```go
// Update 更新指定关系型数据库的权限配置(原子替换)
func (c *Checker) Update(database string, cfg config.PermissionConfig) {
	perms := c.relationalPerms.Load()
	newPerms := make(map[string]config.PermissionConfig)
	for k, v := range *perms {
		newPerms[k] = v
	}
	newPerms[database] = cfg
	c.relationalPerms.Store(&newPerms)
}

// UpdateNosql 更新指定 NoSQL 数据库的权限配置(原子替换)
func (c *Checker) UpdateNosql(database string, cfg config.NosqlPermissionConfig) {
	perms := c.nosqlPerms.Load()
	newPerms := make(map[string]config.NosqlPermissionConfig)
	for k, v := range *perms {
		newPerms[k] = v
	}
	newPerms[database] = cfg
	c.nosqlPerms.Store(&newPerms)
}

// UpdateFromConfig 从配置全量更新权限（用于热重载）
func (c *Checker) UpdateFromConfig(perms config.PermissionsGroup) {
	c.relationalPerms.Store(&perms.Relational)
	c.nosqlPerms.Store(&perms.Nosql)
}

// IsReadOnly 检查指定数据库是否只读模式
func (c *Checker) IsReadOnly(database string) bool {
	perms := c.relationalPerms.Load()
	cfg, ok := (*perms)[database]
	if !ok {
		return false
	}
	return cfg.ReadOnly
}
```

辅助方法 `checkDatabase` 不再需要（已在查找逻辑中处理），`checkTable` 和 `isActionAllowed` 保持不变：

```go
// checkTable 检查表是否在黑名单中
func (c *Checker) checkTable(cfg *config.PermissionConfig, tableName string) error {
	if tableName == "" {
		return nil
	}
	for _, t := range cfg.BlockedTables {
		if t == tableName {
			return fmt.Errorf("table '%s' is blocked", tableName)
		}
	}
	return nil
}

// isActionAllowed 检查操作类型是否允许
func (c *Checker) isActionAllowed(cfg *config.PermissionConfig, actionType string) bool {
	for _, a := range cfg.AllowedActions {
		if a == actionType {
			return true
		}
	}
	return false
}
```

`IsRedisWriteCommand` 函数保持不变。

- [x] **Step 5: Commit**

```bash
cd C:/Workspace/TestProject/dbmcp
git add internal/permission/permission.go
git commit -m "feat: refactor Checker to per-database permission lookup"
```

---

### Task 4: 修改 server.go 中 CheckRedisCommand 调用

**Files:**
- Modify: `internal/mcp/server.go`

- [x] **Step 1: 更新所有 CheckRedisCommand 调用**

`CheckRedisCommand` 签名从 `(cmd, key)` 改为 `(database, cmd, key)`。找到所有调用点并添加 `dbName` 参数：

在 `handleRedisCommand` 中（约 line 602）：
```go
// Before
if err := d.perm.CheckRedisCommand(command, key); err != nil {

// After
if err := d.perm.CheckRedisCommand(dbName, command, key); err != nil {
```

在 `handleRedisScan` 中（约 line 649）：
```go
// Before
if err := d.perm.CheckRedisCommand("SCAN", ""); err != nil {

// After
if err := d.perm.CheckRedisCommand(dbName, "SCAN", ""); err != nil {
```

在 `handleRedisInfo` 中（约 line 705）：
```go
// Before
if err := d.perm.CheckRedisCommand("INFO", ""); err != nil {

// After
if err := d.perm.CheckRedisCommand(dbName, "INFO", ""); err != nil {
```

- [x] **Step 2: 编译验证**

Run:
```bash
cd C:/Workspace/TestProject/dbmcp
go build ./internal/mcp/...
```

Expected: 编译失败（main.go 还未适配），但 server.go 本身无语法错误。

- [x] **Step 3: Commit**

```bash
cd C:/Workspace/TestProject/dbmcp
git add internal/mcp/server.go
git commit -m "feat: update CheckRedisCommand calls with database parameter"
```

---

### Task 5: 修改 main.go 适配新签名

**Files:**
- Modify: `cmd/dbmcp/main.go`

- [x] **Step 1: 修改权限初始化**

```go
// Before (line 51-52)
perm := permission.NewChecker(cfg.PermissionsGroup.Relational)
perm.UpdateNosql(cfg.PermissionsGroup.Nosql)

// After
perm := permission.NewChecker(cfg.PermissionsGroup)
```

- [x] **Step 2: 修改热重载部分**

```go
// Before (line 63-64)
perm.Update(newCfg.PermissionsGroup.Relational)
perm.UpdateNosql(newCfg.PermissionsGroup.Nosql)

// After
perm.UpdateFromConfig(newCfg.PermissionsGroup)
```

- [x] **Step 3: 编译验证**

Run:
```bash
cd C:/Workspace/TestProject/dbmcp
go build ./...
```

Expected: 编译成功。

Run:
```bash
cd C:/Workspace/TestProject/dbmcp
go build -o build/dbmcp.exe ./cmd/dbmcp
```

Expected: 生成 `build/dbmcp.exe`。

- [x] **Step 4: Commit**

```bash
cd C:/Workspace/TestProject/dbmcp
git add cmd/dbmcp/main.go
git commit -m "feat: adapt main.go to per-database permission checker"
```

---

### Task 6: 更新 config 测试

**Files:**
- Modify: `internal/config/config_test.go`

- [x] **Step 1: 更新现有测试以适配新结构**

所有需要初始化 `PermissionsGroup` 的测试需要确保 map 已初始化。但当前测试中没有直接构造 `PermissionsGroup` 的，所以主要更新 `TestLoadConfig_FromFile` 和 `TestLoadConfig_BackwardCompatible` 来验证 per-database 权限迁移。

修改 `TestLoadConfig_FromFile`：

```go
func TestLoadConfig_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `databases:
  mydb:
    driver: mysql
    dsn: "user:pass@tcp(localhost:3306)/testdb"
permissions:
  read_only: true
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := cfg.DatabaseGroups.Relational["mydb"]; !ok {
		t.Errorf("expected mydb to migrate to relational")
	}
	// 验证 per-database 权限已生成
	perm, ok := cfg.PermissionsGroup.Relational["mydb"]
	if !ok {
		t.Error("expected per-database permission for mydb")
	}
	if !perm.ReadOnly {
		t.Error("expected read_only to be true for mydb")
	}
}
```

- [x] **Step 2: 新增 per-database 权限测试**

```go
func TestApplyDefaults_PerDatabasePermissions(t *testing.T) {
	cfg := &AppConfig{}
	cfg.DatabaseGroups.Relational = map[string]DatabaseConfig{
		"mysql_prod": {Driver: "mysql", DSN: "user:pass@tcp(localhost:3306)/db"},
		"mysql_dev":  {Driver: "mysql", DSN: "user:pass@tcp(localhost:3307)/db"},
	}
	cfg.DatabaseGroups.Nosql = map[string]DatabaseConfig{
		"myredis": {Driver: "redis", Host: "localhost", Port: 6379},
	}
	cfg.DatabaseGroups.Timeseries = make(map[string]DatabaseConfig)
	cfg.DatabaseGroups.Graph = make(map[string]DatabaseConfig)
	cfg.PermissionsGroup.Relational = map[string]PermissionConfig{
		"mysql_prod": {ReadOnly: true, AllowedActions: []string{"SELECT"}},
	}

	applyDefaults(cfg)

	// mysql_prod 已有配置，不应覆盖
	if perm, ok := cfg.PermissionsGroup.Relational["mysql_prod"]; !ok {
		t.Error("expected mysql_prod permission")
	} else if !perm.ReadOnly {
		t.Error("expected mysql_prod to remain read-only")
	}

	// mysql_dev 无配置，应生成默认值
	if perm, ok := cfg.PermissionsGroup.Relational["mysql_dev"]; !ok {
		t.Error("expected mysql_dev default permission")
	} else if perm.ReadOnly {
		t.Error("expected mysql_dev to not be read-only by default")
	}

	// myredis 无配置，应生成默认值
	if perm, ok := cfg.PermissionsGroup.Nosql["myredis"]; !ok {
		t.Error("expected myredis default permission")
	} else if len(perm.AllowedCommands) == 0 {
		t.Error("expected myredis to have default commands")
	}
}

func TestLoadConfig_PerDatabasePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `database_groups:
  relational:
    mysql_prod:
      driver: mysql
      host: localhost
      port: 3306
      username: user
      password: pass
      database: prod
    mysql_dev:
      driver: mysql
      host: localhost
      port: 3307
      username: user
      password: pass
      database: dev
permissions_groups:
  relational:
    mysql_prod:
      read_only: true
      allowed_actions: [SELECT]
      blocked_tables: []
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// mysql_prod 应有自定义权限
	perm, ok := cfg.PermissionsGroup.Relational["mysql_prod"]
	if !ok {
		t.Fatal("expected mysql_prod permission")
	}
	if !perm.ReadOnly {
		t.Error("expected mysql_prod to be read-only")
	}
	if len(perm.AllowedActions) != 1 || perm.AllowedActions[0] != "SELECT" {
		t.Errorf("expected mysql_prod actions [SELECT], got %v", perm.AllowedActions)
	}

	// mysql_dev 应有默认权限
	permDev, ok := cfg.PermissionsGroup.Relational["mysql_dev"]
	if !ok {
		t.Fatal("expected mysql_dev permission")
	}
	if permDev.ReadOnly {
		t.Error("expected mysql_dev to not be read-only")
	}
}
```

- [x] **Step 3: 运行配置测试**

Run:
```bash
cd C:/Workspace/TestProject/dbmcp
go test ./internal/config/... -v
```

Expected: 所有测试通过。

- [x] **Step 4: Commit**

```bash
cd C:/Workspace/TestProject/dbmcp
git add internal/config/config_test.go
git commit -m "test: add per-database permission config tests"
```

---

### Task 7: 更新 permission 测试

**Files:**
- Modify: `internal/permission/permission_test.go`

- [x] **Step 1: 重写所有测试以适配新 Checker 签名**

完整重写 `permission_test.go`：

```go
package permission

import (
	"testing"

	"github.com/dbmcp/dbmcp/internal/config"
)

func TestCheckSelect_Allowed(t *testing.T) {
	perms := config.PermissionsGroup{
		Relational: map[string]config.PermissionConfig{
			"testdb": {
				ReadOnly:         false,
				AllowedDatabases: []string{"*"},
				AllowedActions:   []string{"SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "DROP"},
				BlockedTables:    []string{},
			},
		},
		Nosql: map[string]config.NosqlPermissionConfig{},
	}
	c := NewChecker(perms)
	err := c.CheckSelect("testdb", "users")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCheckSelect_BlockedTable(t *testing.T) {
	perms := config.PermissionsGroup{
		Relational: map[string]config.PermissionConfig{
			"testdb": {
				ReadOnly:         false,
				AllowedDatabases: []string{"*"},
				AllowedActions:   []string{"SELECT"},
				BlockedTables:    []string{"secret_table"},
			},
		},
		Nosql: map[string]config.NosqlPermissionConfig{},
	}
	c := NewChecker(perms)
	err := c.CheckSelect("testdb", "secret_table")
	if err == nil {
		t.Error("expected error for blocked table")
	}
}

func TestCheckSelect_DatabaseNotFound(t *testing.T) {
	perms := config.PermissionsGroup{
		Relational: map[string]config.PermissionConfig{
			"testdb": {ReadOnly: false},
		},
		Nosql: map[string]config.NosqlPermissionConfig{},
	}
	c := NewChecker(perms)
	err := c.CheckSelect("other_db", "users")
	if err == nil {
		t.Error("expected error for database not found")
	}
}

func TestCheckWrite_ReadOnlyMode(t *testing.T) {
	perms := config.PermissionsGroup{
		Relational: map[string]config.PermissionConfig{
			"testdb": {
				ReadOnly:         true,
				AllowedDatabases: []string{"*"},
				AllowedActions:   []string{"SELECT"},
				BlockedTables:    []string{},
			},
		},
		Nosql: map[string]config.NosqlPermissionConfig{},
	}
	c := NewChecker(perms)
	err := c.CheckWrite("testdb", "users", "INSERT")
	if err == nil {
		t.Error("expected error in read-only mode")
	}
}

func TestCheckWrite_Allowed(t *testing.T) {
	perms := config.PermissionsGroup{
		Relational: map[string]config.PermissionConfig{
			"testdb": {
				ReadOnly:         false,
				AllowedDatabases: []string{"*"},
				AllowedActions:   []string{"SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "DROP"},
				BlockedTables:    []string{},
			},
		},
		Nosql: map[string]config.NosqlPermissionConfig{},
	}
	c := NewChecker(perms)
	err := c.CheckWrite("testdb", "users", "INSERT")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCheckWrite_ActionNotAllowed(t *testing.T) {
	perms := config.PermissionsGroup{
		Relational: map[string]config.PermissionConfig{
			"testdb": {
				ReadOnly:         false,
				AllowedDatabases: []string{"*"},
				AllowedActions:   []string{"SELECT"},
				BlockedTables:    []string{},
			},
		},
		Nosql: map[string]config.NosqlPermissionConfig{},
	}
	c := NewChecker(perms)
	err := c.CheckWrite("testdb", "users", "DELETE")
	if err == nil {
		t.Error("expected error for disallowed action")
	}
}

func TestUpdate_AppliedImmediately(t *testing.T) {
	perms := config.PermissionsGroup{
		Relational: map[string]config.PermissionConfig{
			"testdb": {
				ReadOnly:         false,
				AllowedDatabases: []string{"*"},
				AllowedActions:   []string{"SELECT", "INSERT", "UPDATE", "DELETE"},
				BlockedTables:    []string{},
			},
		},
		Nosql: map[string]config.NosqlPermissionConfig{},
	}
	c := NewChecker(perms)

	err := c.CheckWrite("testdb", "users", "INSERT")
	if err != nil {
		t.Fatalf("expected write allowed before update: %v", err)
	}

	c.Update("testdb", config.PermissionConfig{
		ReadOnly:         true,
		AllowedDatabases: []string{"*"},
		AllowedActions:   []string{"SELECT"},
		BlockedTables:    []string{},
	})

	err = c.CheckWrite("testdb", "users", "INSERT")
	if err == nil {
		t.Error("expected error after switching to read-only mode")
	}
}

func TestCheckRedisCommand_Allowed(t *testing.T) {
	perms := config.PermissionsGroup{
		Relational: map[string]config.PermissionConfig{},
		Nosql: map[string]config.NosqlPermissionConfig{
			"myredis": {
				ReadOnly:        false,
				AllowedCommands: []string{"GET", "SET", "HGETALL"},
				BlockedKeys:     []string{},
			},
		},
	}
	c := NewChecker(perms)

	if err := c.CheckRedisCommand("myredis", "GET", "mykey"); err != nil {
		t.Errorf("expected GET to be allowed, got: %v", err)
	}
	if err := c.CheckRedisCommand("myredis", "SET", "mykey"); err != nil {
		t.Errorf("expected SET to be allowed, got: %v", err)
	}
}

func TestCheckRedisCommand_BlockedByWhitelist(t *testing.T) {
	perms := config.PermissionsGroup{
		Relational: map[string]config.PermissionConfig{},
		Nosql: map[string]config.NosqlPermissionConfig{
			"myredis": {
				ReadOnly:        false,
				AllowedCommands: []string{"GET"},
				BlockedKeys:     []string{},
			},
		},
	}
	c := NewChecker(perms)

	if err := c.CheckRedisCommand("myredis", "DEL", "mykey"); err == nil {
		t.Error("expected DEL to be blocked")
	}
}

func TestCheckRedisCommand_BlockedByKey(t *testing.T) {
	perms := config.PermissionsGroup{
		Relational: map[string]config.PermissionConfig{},
		Nosql: map[string]config.NosqlPermissionConfig{
			"myredis": {
				ReadOnly:        false,
				AllowedCommands: []string{"GET", "SET", "DEL"},
				BlockedKeys:     []string{"*session*", "*auth*"},
			},
		},
	}
	c := NewChecker(perms)

	if err := c.CheckRedisCommand("myredis", "GET", "user:session:123"); err == nil {
		t.Error("expected key matching blocked pattern to be denied")
	}
	if err := c.CheckRedisCommand("myredis", "GET", "user:123"); err != nil {
		t.Errorf("expected non-blocked key to be allowed, got: %v", err)
	}
}

func TestCheckRedisCommand_ReadOnly(t *testing.T) {
	perms := config.PermissionsGroup{
		Relational: map[string]config.PermissionConfig{},
		Nosql: map[string]config.NosqlPermissionConfig{
			"myredis": {
				ReadOnly:        true,
				AllowedCommands: []string{"GET", "SET", "DEL"},
				BlockedKeys:     []string{},
			},
		},
	}
	c := NewChecker(perms)

	if err := c.CheckRedisCommand("myredis", "GET", "mykey"); err != nil {
		t.Errorf("expected GET to be allowed in read-only, got: %v", err)
	}
	if err := c.CheckRedisCommand("myredis", "SET", "mykey"); err == nil {
		t.Error("expected SET to be blocked in read-only mode")
	}
}

func TestCheckRedisCommand_DatabaseNotFound(t *testing.T) {
	perms := config.PermissionsGroup{
		Relational: map[string]config.PermissionConfig{},
		Nosql:      map[string]config.NosqlPermissionConfig{},
	}
	c := NewChecker(perms)

	if err := c.CheckRedisCommand("unknown_redis", "GET", "mykey"); err == nil {
		t.Error("expected error for database not found")
	}
}

func TestIsRedisWriteCommand(t *testing.T) {
	writeCommands := []string{"SET", "SETNX", "SETEX", "MSET", "MSETNX", "GETSET", "APPEND", "INCR", "DECR", "INCRBY", "DECRBY", "DEL", "UNLINK", "HSET", "HSETNX", "HMSET", "HDEL", "LPUSH", "RPUSH", "LPOP", "RPOP", "LSET", "LINSERT", "SADD", "SREM", "SPOP", "SMOVE", "ZADD", "ZREM", "ZINCRBY", "FLUSHDB", "FLUSHALL", "EVAL", "EVALSHA"}
	for _, cmd := range writeCommands {
		if !IsRedisWriteCommand(cmd) {
			t.Errorf("expected %s to be a write command", cmd)
		}
	}

	readCommands := []string{"GET", "MGET", "HGET", "HGETALL", "HKEYS", "HVALS", "LRANGE", "SMEMBERS", "SISMEMBER", "ZCARD", "ZRANGE", "SCAN", "INFO", "EXISTS", "TTL", "TYPE", "PING", "ECHO", "DBSIZE", "KEYS"}
	for _, cmd := range readCommands {
		if IsRedisWriteCommand(cmd) {
			t.Errorf("expected %s to NOT be a write command", cmd)
		}
	}
}
```

- [x] **Step 2: 运行权限测试**

Run:
```bash
cd C:/Workspace/TestProject/dbmcp
go test ./internal/permission/... -v
```

Expected: 所有测试通过。

- [x] **Step 3: Commit**

```bash
cd C:/Workspace/TestProject/dbmcp
git add internal/permission/permission_test.go
git commit -m "test: rewrite permission tests for per-database model"
```

---

### Task 8: 运行全部测试验证

**Files:**
- All modified files

- [ ] **Step 1: 运行全部单元测试**

Run:
```bash
cd C:/Workspace/TestProject/dbmcp
go test ./internal/config/... ./internal/permission/... ./internal/security/... -v
```

Expected: 所有测试通过。

- [ ] **Step 2: 编译最终版本**

Run:
```bash
cd C:/Workspace/TestProject/dbmcp
go build -o build/dbmcp.exe ./cmd/dbmcp
```

Expected: 编译成功，生成 `build/dbmcp.exe`。

- [ ] **Step 3: 运行全部测试（含集成）**

Run:
```bash
cd C:/Workspace/TestProject/dbmcp
go test ./... -v -count=1
```

Expected: 所有测试通过（SQLite/MCP 测试无 Docker 依赖自动通过，MySQL/PG/MSSQL 需要 Docker 或自动跳过）。

- [ ] **Step 4: Commit（如有修复）**

```bash
cd C:/Workspace/TestProject/dbmcp
git add -A
git commit -m "fix: address test issues from per-database permission refactor"
```

---

## 依赖汇总

| 任务 | 依赖 |
|------|------|
| Task 1 | 无 |
| Task 2 | Task 1（结构体变更） |
| Task 3 | Task 1（结构体变更） |
| Task 4 | Task 3（Checker 签名变更） |
| Task 5 | Task 3, 4（Checker + server 签名） |
| Task 6 | Task 1, 2（config 结构体 + 迁移） |
| Task 7 | Task 3（Checker 签名变更） |
| Task 8 | Task 1-7 |
