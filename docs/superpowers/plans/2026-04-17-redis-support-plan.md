# Redis Support & Config Restructure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 dbmcp 增加 Redis 支持，同时重构配置结构为按数据库类型分组（relational/nosql/timeseries/graph），为未来扩展预留架构空间。

**Architecture:** 配置层按类型分组 + Redis 驱动实现 DatabaseDriver 接口 + 命令级权限白名单 + 4 个新的 Redis MCP 工具。旧配置格式自动向后兼容。

**Tech Stack:** Go 1.26, github.com/redis/go-redis/v9, mark3labs/mcp-go, yaml.v3, filepath.Match

---

## 文件结构

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/config/config.go` | 修改 | 新增 DatabaseGroups、NosqlPermissionConfig 等结构，修改加载逻辑支持新旧格式 |
| `internal/config/config_test.go` | 修改 | 新增分组配置测试、向后兼容测试 |
| `internal/database/redis.go` | 新增 | Redis 驱动实现 DatabaseDriver 接口 |
| `internal/database/redis_test.go` | 新增 | Redis 驱动单元测试 |
| `internal/database/manager.go` | 修改 | createDriver 增加 redis 分支，buildDSN 增加 Redis 分支 |
| `internal/permission/permission.go` | 修改 | 新增 CheckRedisCommand、NosqlPermissionConfig |
| `internal/permission/permission_test.go` | 修改 | 新增 Redis 权限测试 |
| `internal/security/sql_guard.go` | 修改 | 新增 Redis 命令解析辅助函数 |
| `internal/mcp/server.go` | 修改 | 新增 4 个 Redis 工具 handler |
| `go.mod` / `go.sum` | 修改 | 新增 github.com/redis/go-redis/v9 |
| `cmd/dbmcp/main.go` | 修改 | 适配分组后的配置结构 |

---

### Task 1: 添加 Redis 依赖

**Files:**
- Modify: `go.mod` / `go.sum`

- [x] **Step 1: 添加 go-redis 依赖**

Run:
```bash
go get github.com/redis/go-redis/v9@latest
```

如果网络受限：
```bash
GOPROXY=https://goproxy.cn,direct go get github.com/redis/go-redis/v9@latest
```

- [x] **Step 2: 验证依赖已添加**

Run:
```bash
go mod tidy
```

Expected: 无错误，go.mod 中出现 `github.com/redis/go-redis/v9`

- [x] **Step 3: 提交**

```bash
git add go.mod go.sum
git commit -m "chore: add go-redis dependency"
```

---

### Task 2: 重构配置结构为分组模式

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [x] **Step 1: 编写新配置结构的测试用例**

在 `internal/config/config_test.go` 中新增：

```go
func TestValidateConfig_GroupedDatabases(t *testing.T) {
	cfg := &AppConfig{
		DatabaseGroups: DatabaseGroups{
			Relational: map[string]DatabaseConfig{
				"mysql_prod": {Driver: "mysql", DSN: "user:pass@tcp(localhost:3306)/db"},
			},
			Nosql: map[string]DatabaseConfig{
				"myredis": {Driver: "redis", Host: "localhost", Port: 6379},
			},
		},
	}
	err := ValidateConfig(cfg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateConfig_GroupedEmpty(t *testing.T) {
	cfg := &AppConfig{
		DatabaseGroups: DatabaseGroups{},
	}
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for empty grouped databases")
	}
}

func TestLoadConfig_BackwardCompatible(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// 旧格式：flat map
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
	// 旧格式应自动迁移到 relational
	if _, ok := cfg.DatabaseGroups.Relational["mydb"]; !ok {
		t.Error("expected old format to migrate to relational")
	}
}

func TestNormalizeConfig_MigrateOldFormat(t *testing.T) {
	cfg := &AppConfig{
		Databases: map[string]DatabaseConfig{
			"mysql_db":  {Driver: "mysql", DSN: "root:pass@tcp(localhost:3306)/db"},
			"pg_db":     {Driver: "postgres", Host: "localhost", Port: 5432},
			"redis_db":  {Driver: "redis", Host: "localhost", Port: 6379},
			"sqlite_db": {Driver: "sqlite", DSN: "/tmp/test.db"},
		},
	}
	NormalizeConfig(cfg)
	if _, ok := cfg.DatabaseGroups.Relational["mysql_db"]; !ok {
		t.Error("mysql should migrate to relational")
	}
	if _, ok := cfg.DatabaseGroups.Relational["pg_db"]; !ok {
		t.Error("postgres should migrate to relational")
	}
	if _, ok := cfg.DatabaseGroups.Nosql["redis_db"]; !ok {
		t.Error("redis should migrate to nosql")
	}
	if _, ok := cfg.DatabaseGroups.Relational["sqlite_db"]; !ok {
		t.Error("sqlite should migrate to relational")
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run:
```bash
go test ./internal/config/... -run 'TestValidateConfig_Grouped|TestLoadConfig_Backward|TestNormalizeConfig' -v
```

Expected: 编译失败（类型未定义）

- [x] **Step 3: 实现配置结构重构**

修改 `internal/config/config.go`，完整内容如下：

```go
package config

import (
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

// DatabaseConfig 单个数据库连接配置
type DatabaseConfig struct {
	Driver   string            `yaml:"driver"`
	DSN      string            `yaml:"dsn"`
	Host     string            `yaml:"host"`
	Port     int               `yaml:"port"`
	Username string            `yaml:"username"`
	Password string            `yaml:"password"`
	Database string            `yaml:"database"`
	Options  map[string]string `yaml:"options"`
}

// HasStructFields 判断是否使用结构化字段
func (d *DatabaseConfig) HasStructFields() bool {
	return d.Host != "" || d.Port != 0 || d.Username != "" || d.Password != "" || d.Database != ""
}

// DatabaseGroups 按类型分组的数据库配置
type DatabaseGroups struct {
	Relational map[string]DatabaseConfig `yaml:"relational"`
	Nosql      map[string]DatabaseConfig `yaml:"nosql"`
	Timeseries map[string]DatabaseConfig `yaml:"timeseries"`
	Graph      map[string]DatabaseConfig `yaml:"graph"`
}

// AllDatabases 返回所有数据库的扁平 map（用于向后兼容）
func (g *DatabaseGroups) AllDatabases() map[string]DatabaseConfig {
	result := make(map[string]DatabaseConfig)
	for k, v := range g.Relational {
		result[k] = v
	}
	for k, v := range g.Nosql {
		result[k] = v
	}
	for k, v := range g.Timeseries {
		result[k] = v
	}
	for k, v := range g.Graph {
		result[k] = v
	}
	return result
}

// PermissionConfig 权限配置
type PermissionConfig struct {
	ReadOnly         bool     `yaml:"read_only"`
	AllowedDatabases []string `yaml:"allowed_databases"`
	AllowedActions   []string `yaml:"allowed_actions"`
	BlockedTables    []string `yaml:"blocked_tables"`
}

// NosqlPermissionConfig NoSQL 权限配置
type NosqlPermissionConfig struct {
	ReadOnly        bool     `yaml:"read_only"`
	AllowedCommands []string `yaml:"allowed_commands"`
	BlockedKeys     []string `yaml:"blocked_keys"`
}

// PermissionsGroup 按类型分组的权限配置
type PermissionsGroup struct {
	Relational PermissionConfig          `yaml:"relational"`
	Nosql      NosqlPermissionConfig     `yaml:"nosql"`
	Timeseries PermissionConfig          `yaml:"timeseries"`
	Graph      PermissionConfig          `yaml:"graph"`
}

// AppConfig 完整配置
type AppConfig struct {
	// 旧格式：flat map（向后兼容）
	Databases map[string]DatabaseConfig `yaml:"databases"`
	// 新格式：按类型分组
	DatabaseGroups DatabaseGroups `yaml:"database_groups"`
	// 旧格式权限（向后兼容）
	Permissions PermissionConfig `yaml:"permissions"`
	// 新格式权限：按类型分组
	PermissionsGroup PermissionsGroup `yaml:"permissions_groups"`
}

// HasOldFormat 判断是否使用了旧格式
func (a *AppConfig) HasOldFormat() bool {
	return len(a.Databases) > 0 && a.Databases[""] != nil
}

// NormalizeConfig 将旧格式迁移到新格式
func NormalizeConfig(cfg *AppConfig) {
	// 初始化分组 map
	if cfg.DatabaseGroups.Relational == nil {
		cfg.DatabaseGroups.Relational = make(map[string]DatabaseConfig)
	}
	if cfg.DatabaseGroups.Nosql == nil {
		cfg.DatabaseGroups.Nosql = make(map[string]DatabaseConfig)
	}
	if cfg.DatabaseGroups.Timeseries == nil {
		cfg.DatabaseGroups.Timeseries = make(map[string]DatabaseConfig)
	}
	if cfg.DatabaseGroups.Graph == nil {
		cfg.DatabaseGroups.Graph = make(map[string]DatabaseConfig)
	}

	// 如果旧格式有数据，迁移到新格式
	if len(cfg.Databases) > 0 {
		hasRelational := false
		hasNosql := false
		for name, db := range cfg.Databases {
			if db.Driver == "" {
				continue
			}
			switch db.Driver {
			case "mysql", "postgres", "postgresql", "sqlite", "sqlite3", "mssql", "sqlserver":
				cfg.DatabaseGroups.Relational[name] = db
				hasRelational = true
			case "redis":
				cfg.DatabaseGroups.Nosql[name] = db
				hasNosql = true
			default:
				log.Printf("[config] unknown driver '%s' for '%s', migrating to nosql", db.Driver, name)
				cfg.DatabaseGroups.Nosql[name] = db
				hasNosql = true
			}
		}
		if hasRelational || hasNosql {
			log.Println("[config] migrated old flat config to grouped format")
		}
	}

	// 如果新格式为空，确保 map 已初始化
	if len(cfg.DatabaseGroups.Relational) == 0 {
		cfg.DatabaseGroups.Relational = make(map[string]DatabaseConfig)
	}
	if len(cfg.DatabaseGroups.Nosql) == 0 {
		cfg.DatabaseGroups.Nosql = make(map[string]DatabaseConfig)
	}
	if len(cfg.DatabaseGroups.Timeseries) == 0 {
		cfg.DatabaseGroups.Timeseries = make(map[string]DatabaseConfig)
	}
	if len(cfg.DatabaseGroups.Graph) == 0 {
		cfg.DatabaseGroups.Graph = make(map[string]DatabaseConfig)
	}
}

// applyDefaults 应用默认权限配置
func applyDefaults(cfg *AppConfig) {
	if cfg.Permissions.AllowedActions == nil {
		cfg.Permissions.AllowedActions = []string{"select", "insert", "update", "delete", "create", "drop"}
	}
	if cfg.Permissions.AllowedDatabases == nil {
		cfg.Permissions.AllowedDatabases = []string{"*"}
	}
	if cfg.Permissions.BlockedTables == nil {
		cfg.Permissions.BlockedTables = []string{}
	}
	if cfg.Databases == nil {
		cfg.Databases = make(map[string]DatabaseConfig)
	}

	// NoSQL 默认权限
	if cfg.PermissionsGroup.Nosql.AllowedCommands == nil {
		cfg.PermissionsGroup.Nosql.AllowedCommands = []string{
			"GET", "SET", "HGET", "HGETALL", "HSET", "LPUSH", "LRANGE",
			"SADD", "SMEMBERS", "SCAN", "INFO", "DEL", "EXISTS", "TTL",
			"TYPE", "PING", "ECHO", "DBSIZE", "KEYS",
		}
	}
	if cfg.PermissionsGroup.Nosql.BlockedKeys == nil {
		cfg.PermissionsGroup.Nosql.BlockedKeys = []string{}
	}

	NormalizeConfig(cfg)
}

// ValidateConfig 校验配置合法性
func ValidateConfig(cfg *AppConfig) error {
	allDB := cfg.DatabaseGroups.AllDatabases()
	if len(allDB) == 0 {
		return &ConfigError{Message: "no valid databases configured"}
	}
	for name, db := range allDB {
		if db.Driver == "" {
			return &ConfigError{Message: "database '" + name + "' missing driver"}
		}
		if db.DSN == "" && db.Host == "" {
			return &ConfigError{Message: "database '" + name + "' missing dsn or host"}
		}
	}
	return nil
}

// ConfigError 配置错误
type ConfigError struct {
	Message string
}

func (e *ConfigError) Error() string {
	return e.Message
}

// ReloadContext 热重载状态
type ReloadContext struct {
	LastReload    time.Time
	ReloadSuccess bool
}

// AppState 运行时配置(线程安全)
type AppState struct {
	config     atomic.Pointer[AppConfig]
	reloadCtx  atomic.Pointer[ReloadContext]
	mu         sync.RWMutex
	configPath string
}

// NewAppState 创建运行时配置
func NewAppState(configPath string) (*AppState, error) {
	app := &AppState{configPath: configPath}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	app.config.Store(cfg)
	app.reloadCtx.Store(&ReloadContext{LastReload: time.Now(), ReloadSuccess: true})
	return app, nil
}

// Config 原子读取当前配置
func (a *AppState) Config() *AppConfig {
	return a.config.Load()
}

// ReloadCtx 原子读取热重载状态
func (a *AppState) ReloadCtx() *ReloadContext {
	return a.reloadCtx.Load()
}

// ConfigPath 获取配置文件路径
func (a *AppState) ConfigPath() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.configPath
}

// SetConfigPath 原子更新配置路径
func (a *AppState) SetConfigPath(path string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.configPath = path
}

// UpdateConfig 原子更新配置
func (a *AppState) UpdateConfig(cfg *AppConfig) {
	a.config.Store(cfg)
	a.reloadCtx.Store(&ReloadContext{LastReload: time.Now(), ReloadSuccess: true})
}

// UpdateReloadFailed 记录热重载失败
func (a *AppState) UpdateReloadFailed() {
	a.reloadCtx.Store(&ReloadContext{LastReload: time.Now(), ReloadSuccess: false})
}

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
	applyDefaults(&cfg)
	if err := ValidateConfig(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// BackupConfig 备份配置文件
func BackupConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".bak", data, 0600)
}
```

- [x] **Step 4: 更新现有测试**

修改 `internal/config/config_test.go` 中需要适配的测试：

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateConfig_EmptyDatabases(t *testing.T) {
	cfg := &AppConfig{}
	cfg.DatabaseGroups.Relational = make(map[string]DatabaseConfig)
	cfg.DatabaseGroups.Nosql = make(map[string]DatabaseConfig)
	cfg.DatabaseGroups.Timeseries = make(map[string]DatabaseConfig)
	cfg.DatabaseGroups.Graph = make(map[string]DatabaseConfig)
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for empty databases")
	}
}

func TestValidateConfig_MissingDriver(t *testing.T) {
	cfg := &AppConfig{}
	cfg.DatabaseGroups.Relational = map[string]DatabaseConfig{
		"test": {Driver: "", DSN: "some-dsn"},
	}
	cfg.DatabaseGroups.Nosql = make(map[string]DatabaseConfig)
	cfg.DatabaseGroups.Timeseries = make(map[string]DatabaseConfig)
	cfg.DatabaseGroups.Graph = make(map[string]DatabaseConfig)
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for missing driver")
	}
}

func TestValidateConfig_MissingDSN(t *testing.T) {
	cfg := &AppConfig{}
	cfg.DatabaseGroups.Relational = map[string]DatabaseConfig{
		"test": {Driver: "mysql", DSN: "", Host: ""},
	}
	cfg.DatabaseGroups.Nosql = make(map[string]DatabaseConfig)
	cfg.DatabaseGroups.Timeseries = make(map[string]DatabaseConfig)
	cfg.DatabaseGroups.Graph = make(map[string]DatabaseConfig)
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for missing DSN")
	}
}

func TestValidateConfig_Valid(t *testing.T) {
	cfg := &AppConfig{}
	cfg.DatabaseGroups.Relational = map[string]DatabaseConfig{
		"test": {Driver: "mysql", DSN: "user:pass@tcp(localhost:3306)/db"},
	}
	cfg.DatabaseGroups.Nosql = make(map[string]DatabaseConfig)
	cfg.DatabaseGroups.Timeseries = make(map[string]DatabaseConfig)
	cfg.DatabaseGroups.Graph = make(map[string]DatabaseConfig)
	err := ValidateConfig(cfg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateConfig_GroupedDatabases(t *testing.T) {
	cfg := &AppConfig{}
	cfg.DatabaseGroups.Relational = map[string]DatabaseConfig{
		"mysql_prod": {Driver: "mysql", DSN: "user:pass@tcp(localhost:3306)/db"},
	}
	cfg.DatabaseGroups.Nosql = map[string]DatabaseConfig{
		"myredis": {Driver: "redis", Host: "localhost", Port: 6379},
	}
	cfg.DatabaseGroups.Timeseries = make(map[string]DatabaseConfig)
	cfg.DatabaseGroups.Graph = make(map[string]DatabaseConfig)
	err := ValidateConfig(cfg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateConfig_GroupedEmpty(t *testing.T) {
	cfg := &AppConfig{}
	cfg.DatabaseGroups.Relational = make(map[string]DatabaseConfig)
	cfg.DatabaseGroups.Nosql = make(map[string]DatabaseConfig)
	cfg.DatabaseGroups.Timeseries = make(map[string]DatabaseConfig)
	cfg.DatabaseGroups.Graph = make(map[string]DatabaseConfig)
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for empty grouped databases")
	}
}

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
	if !cfg.PermissionsGroup.Nosql.ReadOnly {
		// 旧格式权限应通过 applyDefaults 设置默认值
	}
}

func TestLoadConfig_BackwardCompatible(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// 旧格式：flat map
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
	// 旧格式应自动迁移到 relational
	if _, ok := cfg.DatabaseGroups.Relational["mydb"]; !ok {
		t.Error("expected old format to migrate to relational")
	}
}

func TestNormalizeConfig_MigrateOldFormat(t *testing.T) {
	cfg := &AppConfig{
		Databases: map[string]DatabaseConfig{
			"mysql_db":  {Driver: "mysql", DSN: "root:pass@tcp(localhost:3306)/db"},
			"pg_db":     {Driver: "postgres", Host: "localhost", Port: 5432},
			"redis_db":  {Driver: "redis", Host: "localhost", Port: 6379},
			"sqlite_db": {Driver: "sqlite", DSN: "/tmp/test.db"},
		},
	}
	NormalizeConfig(cfg)
	if _, ok := cfg.DatabaseGroups.Relational["mysql_db"]; !ok {
		t.Error("mysql should migrate to relational")
	}
	if _, ok := cfg.DatabaseGroups.Relational["pg_db"]; !ok {
		t.Error("postgres should migrate to relational")
	}
	if _, ok := cfg.DatabaseGroups.Nosql["redis_db"]; !ok {
		t.Error("redis should migrate to nosql")
	}
	if _, ok := cfg.DatabaseGroups.Relational["sqlite_db"]; !ok {
		t.Error("sqlite should migrate to relational")
	}
}

func TestBackupConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "test: value"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if err := BackupConfig(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	backupPath := path + ".bak"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("expected backup file to exist")
	}
}
```

- [x] **Step 5: 运行配置测试**

Run:
```bash
go test ./internal/config/... -v
```

Expected: 所有测试通过

- [x] **Step 6: 提交**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: restructure config to type-grouped databases with backward compatibility"
```

---

### Task 3: 实现 Redis 驱动

**Files:**
- Create: `internal/database/redis.go`
- Create: `internal/database/redis_test.go`
- Modify: `internal/database/manager.go`

- [x] **Step 1: 编写 Redis 驱动的单元测试**

创建 `internal/database/redis_test.go`：

```go
package database

import (
	"context"
	"testing"
)

func TestRedisDriver_NewDriver(t *testing.T) {
	drv := NewRedisDriver()
	if drv == nil {
		t.Fatal("expected non-nil driver")
	}
}

func TestRedisDriver_QueryNotSupported(t *testing.T) {
	drv := NewRedisDriver()
	_, err := drv.Query(context.Background(), "SELECT 1")
	if err == nil {
		t.Error("expected error for SQL query on Redis")
	}
}

func TestRedisDriver_ListDatabases(t *testing.T) {
	drv := NewRedisDriver()
	dbs, err := drv.ListDatabases(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dbs) != 1 {
		t.Errorf("expected 1 entry, got %d", len(dbs))
	}
	if dbs[0] != "Redis (16 logical databases)" {
		t.Errorf("unexpected db list: %v", dbs)
	}
}

func TestRedisDriver_BeginTxNotSupported(t *testing.T) {
	drv := NewRedisDriver()
	err := drv.BeginTx(context.Background())
	if err == nil {
		t.Error("expected error for transactions on Redis")
	}
}

func TestRedisDriver_CommitNotSupported(t *testing.T) {
	drv := NewRedisDriver()
	err := drv.Commit()
	if err == nil {
		t.Error("expected error for commit on Redis")
	}
}

func TestRedisDriver_RollbackNotSupported(t *testing.T) {
	drv := NewRedisDriver()
	err := drv.Rollback()
	if err == nil {
		t.Error("expected error for rollback on Redis")
	}
}

func TestRedisDriver_CloseWithoutConnection(t *testing.T) {
	drv := NewRedisDriver()
	err := drv.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseRedisCommand(t *testing.T) {
	tests := []struct {
		input    string
		wantCmd  string
		wantArgs []string
	}{
		{"GET mykey", "GET", []string{"mykey"}},
		{"SET mykey value", "SET", []string{"mykey", "value"}},
		{"HGETALL myhash", "HGETALL", []string{"myhash"}},
		{"  LPUSH  mylist  item1  item2  ", "LPUSH", []string{"mylist", "item1", "item2"}},
		{"INFO", "INFO", []string{}},
		{"SCAN 0 MATCH user:* COUNT 100", "SCAN", []string{"0", "MATCH", "user:*", "COUNT", "100"}},
	}
	for _, tt := range tests {
		cmd, args := ParseRedisCommand(tt.input)
		if cmd != tt.wantCmd {
			t.Errorf("ParseRedisCommand(%q) cmd = %q, want %q", tt.input, cmd, tt.wantCmd)
		}
		if len(args) != len(tt.wantArgs) {
			t.Errorf("ParseRedisCommand(%q) args len = %d, want %d", tt.input, len(args), len(tt.wantArgs))
			continue
		}
		for i, a := range args {
			if a != tt.wantArgs[i] {
				t.Errorf("ParseRedisCommand(%q) args[%d] = %q, want %q", tt.input, i, a, tt.wantArgs[i])
			}
		}
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run:
```bash
go test ./internal/database/... -run TestRedis -v
```

Expected: 编译失败（类型未定义）

- [x] **Step 3: 实现 Redis 驱动**

创建 `internal/database/redis.go`：

```go
package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisDriver Redis 数据库驱动
type RedisDriver struct {
	client *redis.Client
	ctx    context.Context
}

// NewRedisDriver 创建 Redis 驱动实例
func NewRedisDriver() *RedisDriver {
	return &RedisDriver{ctx: context.Background()}
}

// Connect 连接 Redis
func (d *RedisDriver) Connect(dsn string) error {
	opts, err := redis.ParseURL(dsn)
	if err != nil {
		return fmt.Errorf("parse redis dsn: %w", err)
	}

	client := redis.NewClient(opts)
	d.client = client

	ctx, cancel := context.WithTimeout(d.ctx, 10*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis authentication failed: %w", err)
	}

	return nil
}

// Query Redis 不支持 SQL 查询
func (d *RedisDriver) Query(ctx context.Context, sql string) (*QueryResult, error) {
	return nil, fmt.Errorf("redis does not support SQL queries. Use redis_command tool")
}

// Exec 执行 Redis 命令
func (d *RedisDriver) Exec(ctx context.Context, cmd string) (int64, error) {
	if d.client == nil {
		return 0, fmt.Errorf("not connected to redis")
	}

	command, args := ParseRedisCommand(cmd)
	if command == "" {
		return 0, fmt.Errorf("empty command")
	}

	// 不支持的命令类型
	blockedCommands := []string{"EVAL", "EVALSHA", "SCRIPT", "MULTI", "EXEC", "DISCARD", "WATCH", "UNWATCH", "SUBSCRIBE", "PSUBSCRIBE", "MONITOR", "SYNC", "PSYNC", "DEBUG", "BGSAVE", "BGREWRITEAOF", "SAVE"}
	for _, b := range blockedCommands {
		if command == b {
			return 0, fmt.Errorf("command '%s' is not supported via redis_command tool", command)
		}
	}

	allArgs := append([]string{command}, args...)
	result := d.client.Do(ctx, interfaceSlice(allArgs)...)

	if err := result.Err(); err != nil {
		return 0, fmt.Errorf("redis command '%s' failed: %w", command, err)
	}

	// 返回固定值 1（Redis 命令不总是返回 affected rows）
	return 1, nil
}

// ListDatabases 列出数据库
func (d *RedisDriver) ListDatabases(ctx context.Context) ([]string, error) {
	return []string{"Redis (16 logical databases)"}, nil
}

// ListTables 列出 key（使用 SCAN）
func (d *RedisDriver) ListTables(ctx context.Context, database string) ([]string, error) {
	if d.client == nil {
		return nil, fmt.Errorf("not connected to redis")
	}

	var keys []string
	cursor := uint64(0)
	for {
		k, nextCursor, err := d.client.Scan(ctx, cursor, "*", 100).Result()
		if err != nil {
			return nil, err
		}
		keys = append(keys, k...)
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return keys, nil
}

// DescribeTable 查看 key 的信息
func (d *RedisDriver) DescribeTable(ctx context.Context, database, table string) ([]Column, error) {
	if d.client == nil {
		return nil, fmt.Errorf("not connected to redis")
	}

	keyType, err := d.client.Type(ctx, table).Result()
	if err != nil {
		return nil, err
	}

	ttl, err := d.client.TTL(ctx, table).Result()
	if err != nil {
		return nil, err
	}

	ttlStr := "-1"
	if ttl == -2 {
		ttlStr = "key does not exist"
	} else if ttl == -1 {
		ttlStr = "no expiry"
	} else {
		ttlStr = ttl.String()
	}

	columns := []Column{
		{Name: "key", Type: "string", Nullable: "NO", Key: ""},
		{Name: "type", Type: keyType, Nullable: "NO", Key: ""},
		{Name: "ttl", Type: ttlStr, Nullable: "YES", Key: ""},
	}

	return columns, nil
}

// Close 关闭连接
func (d *RedisDriver) Close() error {
	if d.client != nil {
		return d.client.Close()
	}
	return nil
}

// BeginTx Redis 不支持传统事务
func (d *RedisDriver) BeginTx(ctx context.Context) error {
	return fmt.Errorf("redis does not support SQL-style transactions. Use MULTI/EXEC directly via redis_command")
}

// Commit Redis 不支持传统事务
func (d *RedisDriver) Commit() error {
	return fmt.Errorf("redis does not support SQL-style transactions. Use MULTI/EXEC directly via redis_command")
}

// Rollback Redis 不支持传统事务
func (d *RedisDriver) Rollback() error {
	return fmt.Errorf("redis does not support SQL-style transactions. Use MULTI/EXEC directly via redis_command")
}

// ParseRedisCommand 解析 Redis 命令文本
func ParseRedisCommand(cmd string) (string, []string) {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return "", nil
	}
	return strings.ToUpper(fields[0]), fields[1:]
}

// interfaceSlice 转换 []string 为 []interface{}
func interfaceSlice(ss []string) []interface{} {
	result := make([]interface{}, len(ss))
	for i, s := range ss {
		result[i] = s
	}
	return result
}

// ClientInfo 获取 Redis INFO 信息
func (d *RedisDriver) ClientInfo(ctx context.Context, section string) (string, error) {
	if d.client == nil {
		return "", fmt.Errorf("not connected to redis")
	}
	if section != "" {
		return d.client.Info(ctx, section).Result()
	}
	return d.client.Info(ctx).Result()
}

// Scan 使用 SCAN 命令扫描 key
func (d *RedisDriver) Scan(ctx context.Context, cursor uint64, pattern string, count int64) ([]string, uint64, error) {
	if d.client == nil {
		return nil, 0, fmt.Errorf("not connected to redis")
	}
	return d.client.Scan(ctx, cursor, pattern, count).Result()
}
```

- [x] **Step 4: 更新 manager.go 添加 Redis 分支**

修改 `internal/database/manager.go` 中的 `createDriver` 函数：

```go
// createDriver 根据类型创建驱动实例
func createDriver(driverType string) (DatabaseDriver, error) {
	switch driverType {
	case "mysql":
		return NewMySQLDriver(), nil
	case "postgres", "postgresql":
		return NewPostgresDriver(), nil
	case "sqlite", "sqlite3":
		return NewSQLiteDriver(), nil
	case "mssql", "sqlserver":
		return NewMSSQLDriver(), nil
	case "redis":
		return NewRedisDriver(), nil
	default:
		return nil, fmt.Errorf("unsupported driver type: %s", driverType)
	}
}
```

修改 `buildDSN` 函数，增加 Redis 分支：

```go
// buildDSN 从结构化配置或原始 DSN 字符串构建连接串
func buildDSN(driverType string, cfg config.DatabaseConfig) string {
	// 如果提供了原始 DSN，直接使用
	if cfg.DSN != "" {
		return cfg.DSN
	}

	// 否则从结构化字段组装
	switch driverType {
	case "mysql":
		return buildMySQLDSN(cfg)
	case "postgres", "postgresql":
		return buildPostgresDSN(cfg)
	case "sqlite", "sqlite3":
		return buildSQLiteDSN(cfg)
	case "mssql", "sqlserver":
		return buildMSSQLDSN(cfg)
	case "redis":
		return buildRedisDSN(cfg)
	default:
		return ""
	}
}
```

在 `buildMSSQLDSN` 函数后新增：

```go
// buildRedisDSN 组装 Redis DSN: redis://username:password@host:port/db
func buildRedisDSN(cfg config.DatabaseConfig) string {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	port := cfg.Port
	if port == 0 {
		port = 6379
	}

	dbNum := "0"
	if v, ok := cfg.Options["db"]; ok {
		dbNum = v
	}

	dsn := fmt.Sprintf("redis://%s:%s@%s:%d/%s",
		url.QueryEscape(cfg.Username), url.QueryEscape(cfg.Password), host, port, dbNum)

	// 如果没有密码，使用简化格式
	if cfg.Password == "" {
		dsn = fmt.Sprintf("redis://:%s@%s:%d/%s", url.QueryEscape(cfg.Password), host, port, dbNum)
	}

	return dsn
}
```

- [x] **Step 5: 运行数据库包测试**

Run:
```bash
go test ./internal/database/... -run TestRedis -v
```

Expected: 所有 Redis 相关测试通过

- [x] **Step 6: 编译验证**

Run:
```bash
go build -o build/dbmcp.exe ./cmd/dbmcp
```

Expected: 编译成功

- [x] **Step 7: 提交**

```bash
git add internal/database/redis.go internal/database/redis_test.go internal/database/manager.go go.mod go.sum
git commit -m "feat: implement Redis driver with DatabaseDriver interface"
```

---

### Task 4: 实现 Redis 权限系统

**Files:**
- Modify: `internal/permission/permission.go`
- Modify: `internal/permission/permission_test.go`

- [x] **Step 1: 编写 Redis 权限测试**

在 `internal/permission/permission_test.go` 末尾新增：

```go
func TestCheckRedisCommand_Allowed(t *testing.T) {
	cfg := PermissionConfig{
		ReadOnly:         false,
		AllowedDatabases: []string{"*"},
		AllowedActions:   []string{"SELECT"},
	}
	nosqlCfg := NosqlPermissionConfig{
		ReadOnly:        false,
		AllowedCommands: []string{"GET", "SET", "HGETALL"},
		BlockedKeys:     []string{},
	}
	c := NewChecker(cfg)
	c.UpdateNosql(nosqlCfg)

	if err := c.CheckRedisCommand("GET", "mykey"); err != nil {
		t.Errorf("expected GET to be allowed, got: %v", err)
	}
	if err := c.CheckRedisCommand("SET", "mykey"); err != nil {
		t.Errorf("expected SET to be allowed, got: %v", err)
	}
}

func TestCheckRedisCommand_BlockedByWhitelist(t *testing.T) {
	cfg := PermissionConfig{
		ReadOnly:         false,
		AllowedDatabases: []string{"*"},
		AllowedActions:   []string{"SELECT"},
	}
	nosqlCfg := NosqlPermissionConfig{
		ReadOnly:        false,
		AllowedCommands: []string{"GET"},
		BlockedKeys:     []string{},
	}
	c := NewChecker(cfg)
	c.UpdateNosql(nosqlCfg)

	if err := c.CheckRedisCommand("DEL", "mykey"); err == nil {
		t.Error("expected DEL to be blocked")
	}
}

func TestCheckRedisCommand_BlockedByKey(t *testing.T) {
	cfg := PermissionConfig{
		ReadOnly:         false,
		AllowedDatabases: []string{"*"},
		AllowedActions:   []string{"SELECT"},
	}
	nosqlCfg := NosqlPermissionConfig{
		ReadOnly:        false,
		AllowedCommands: []string{"GET", "SET", "DEL"},
		BlockedKeys:     []string{"*session*", "*auth*"},
	}
	c := NewChecker(cfg)
	c.UpdateNosql(nosqlCfg)

	if err := c.CheckRedisCommand("GET", "user:session:123"); err == nil {
		t.Error("expected key matching blocked pattern to be denied")
	}
	if err := c.CheckRedisCommand("GET", "user:123"); err != nil {
		t.Errorf("expected non-blocked key to be allowed, got: %v", err)
	}
}

func TestCheckRedisCommand_ReadOnly(t *testing.T) {
	cfg := PermissionConfig{
		ReadOnly:         false,
		AllowedDatabases: []string{"*"},
		AllowedActions:   []string{"SELECT"},
	}
	nosqlCfg := NosqlPermissionConfig{
		ReadOnly:        true,
		AllowedCommands: []string{"GET", "SET", "DEL"},
		BlockedKeys:     []string{},
	}
	c := NewChecker(cfg)
	c.UpdateNosql(nosqlCfg)

	if err := c.CheckRedisCommand("GET", "mykey"); err != nil {
		t.Errorf("expected GET to be allowed in read-only, got: %v", err)
	}
	if err := c.CheckRedisCommand("SET", "mykey"); err == nil {
		t.Error("expected SET to be blocked in read-only mode")
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

- [x] **Step 2: 运行测试确认失败**

Run:
```bash
go test ./internal/permission/... -run TestCheckRedis -v
```

Expected: 编译失败

- [x] **Step 3: 实现 Redis 权限校验**

修改 `internal/permission/permission.go`，完整内容：

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
	cfg        atomic.Pointer[config.PermissionConfig]
	nosqlCfg   atomic.Pointer[config.NosqlPermissionConfig]
}

// NewChecker 创建权限校验器
func NewChecker(cfg config.PermissionConfig) *Checker {
	c := &Checker{}
	c.cfg.Store(&cfg)
	defaultNosql := config.NosqlPermissionConfig{
		ReadOnly:        false,
		AllowedCommands: []string{"GET", "SET", "HGET", "HGETALL", "HSET", "LPUSH", "LRANGE", "SADD", "SMEMBERS", "SCAN", "INFO", "DEL", "EXISTS", "TTL", "TYPE", "PING", "ECHO", "DBSIZE", "KEYS"},
		BlockedKeys:     []string{},
	}
	c.nosqlCfg.Store(&defaultNosql)
	return c
}

// Update 更新权限配置(原子替换)
func (c *Checker) Update(cfg config.PermissionConfig) {
	c.cfg.Store(&cfg)
}

// UpdateNosql 更新 NoSQL 权限配置(原子替换)
func (c *Checker) UpdateNosql(cfg config.NosqlPermissionConfig) {
	c.nosqlCfg.Store(&cfg)
}

// CheckSelect 检查 SELECT 权限
func (c *Checker) CheckSelect(database string, tableName string) error {
	cfg := c.cfg.Load()
	if err := c.checkDatabase(cfg, database); err != nil {
		return err
	}
	if err := c.checkTable(cfg, tableName); err != nil {
		return err
	}
	return nil
}

// CheckWrite 检查写入权限
func (c *Checker) CheckWrite(database string, tableName string, actionType string) error {
	cfg := c.cfg.Load()
	if cfg.ReadOnly {
		return fmt.Errorf("database is in read-only mode, %s not allowed", actionType)
	}
	if err := c.checkDatabase(cfg, database); err != nil {
		return err
	}
	if err := c.checkTable(cfg, tableName); err != nil {
		return err
	}
	if !c.isActionAllowed(cfg, actionType) {
		return fmt.Errorf("action '%s' is not allowed", actionType)
	}
	return nil
}

// CheckRedisCommand 检查 Redis 命令权限
func (c *Checker) CheckRedisCommand(cmd string, key string) error {
	cfg := c.nosqlCfg.Load()

	// read_only 模式：拦截写命令
	if cfg.ReadOnly && IsRedisWriteCommand(cmd) {
		return fmt.Errorf("database is in read-only mode, %s not allowed", cmd)
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

// IsReadOnly 检查是否只读模式
func (c *Checker) IsReadOnly() bool {
	return c.cfg.Load().ReadOnly
}

// checkDatabase 检查数据库是否在白名单中
func (c *Checker) checkDatabase(cfg *config.PermissionConfig, database string) error {
	for _, db := range cfg.AllowedDatabases {
		if db == "*" || db == database {
			return nil
		}
	}
	return fmt.Errorf("database '%s' is not allowed", database)
}

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

// IsRedisWriteCommand 判断是否为 Redis 写命令
func IsRedisWriteCommand(cmd string) bool {
	writeCommands := map[string]bool{
		"SET": true, "SETNX": true, "SETEX": true, "MSET": true, "MSETNX": true,
		"GETSET": true, "APPEND": true, "INCR": true, "DECR": true,
		"INCRBY": true, "DECRBY": true, "DEL": true, "UNLINK": true,
		"HSET": true, "HSETNX": true, "HMSET": true, "HDEL": true,
		"LPUSH": true, "RPUSH": true, "LPOP": true, "RPOP": true,
		"LSET": true, "LINSERT": true, "SADD": true, "SREM": true,
		"SPOP": true, "SMOVE": true, "ZADD": true, "ZREM": true,
		"ZINCRBY": true, "FLUSHDB": true, "FLUSHALL": true,
		"EVAL": true, "EVALSHA": true,
	}
	return writeCommands[cmd]
}
```

- [x] **Step 4: 运行权限测试**

Run:
```bash
go test ./internal/permission/... -v
```

Expected: 所有测试通过

- [x] **Step 5: 提交**

```bash
git add internal/permission/permission.go internal/permission/permission_test.go
git commit -m "feat: add Redis command-level permission whitelist"
```

---

### Task 5: 添加 Redis 命令安全辅助函数

**Files:**
- Modify: `internal/security/sql_guard.go`

- [x] **Step 1: 新增 Redis 命令解析函数**

在 `internal/security/sql_guard.go` 末尾添加（不修改现有代码）：

```go
// ExtractRedisKey 从 Redis 命令中提取 key（第一个参数）
func ExtractRedisKey(cmd string) string {
	fields := strings.Fields(cmd)
	if len(fields) < 2 {
		return ""
	}
	return fields[1]
}
```

- [x] **Step 2: 运行安全模块测试**

Run:
```bash
go test ./internal/security/... -v
```

Expected: 所有测试通过（原有测试不受影响）

- [x] **Step 3: 提交**

```bash
git add internal/security/sql_guard.go
git commit -m "feat: add ExtractRedisKey helper for Redis command parsing"
```

---

### Task 6: 实现 Redis MCP 工具

**Files:**
- Modify: `internal/mcp/server.go`
- Modify: `cmd/dbmcp/main.go`

- [x] **Step 1: 注册 Redis 工具**

在 `internal/mcp/server.go` 的 `registerTools()` 方法末尾（最后一个 `d.srv.AddTool(...)` 之后）添加：

```go
	// Redis 工具
	d.srv.AddTool(
		mcp.NewTool("redis_command",
			mcp.WithDescription("Execute a Redis command"),
			mcp.WithString("database", mcp.Required(), mcp.Description("Database name")),
			mcp.WithString("cmd", mcp.Required(), mcp.Description("Redis command, e.g. 'GET mykey'")),
			mcp.WithNumber("db", mcp.Description("Redis logical database (0-15, default 0)")),
		),
		d.handleRedisCommand,
	)

	d.srv.AddTool(
		mcp.NewTool("redis_scan",
			mcp.WithDescription("Safely scan Redis keys using SCAN"),
			mcp.WithString("database", mcp.Required(), mcp.Description("Database name")),
			mcp.WithString("pattern", mcp.Description("Key pattern (default '*')")),
			mcp.WithNumber("limit", mcp.Description("Max keys to return (default 50)")),
			mcp.WithNumber("db", mcp.Description("Redis logical database (0-15, default 0)")),
		),
		d.handleRedisScan,
	)

	d.srv.AddTool(
		mcp.NewTool("redis_info",
			mcp.WithDescription("Get Redis server info"),
			mcp.WithString("database", mcp.Required(), mcp.Description("Database name")),
			mcp.WithString("section", mcp.Description("Info section (default 'default')")),
			mcp.WithNumber("db", mcp.Description("Redis logical database (0-15, default 0)")),
		),
		d.handleRedisInfo,
	)

	d.srv.AddTool(
		mcp.NewTool("redis_describe",
			mcp.WithDescription("Describe a Redis key (type, TTL, value)"),
			mcp.WithString("database", mcp.Required(), mcp.Description("Database name")),
			mcp.WithString("key", mcp.Required(), mcp.Description("Redis key")),
			mcp.WithNumber("db", mcp.Description("Redis logical database (0-15, default 0)")),
		),
		d.handleRedisDescribe,
	)
```

- [x] **Step 2: 实现 Redis 工具 handler**

在 `internal/mcp/server.go` 末尾添加：

```go
func (d *DBMCPServer) handleRedisCommand(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(req)
	dbName := strArg(args, "database")
	cmdStr := strArg(args, "cmd")
	dbNum := int(numArg(args, "db"))

	// 解析命令
	command, _ := database.ParseRedisCommand(cmdStr)
	if command == "" {
		return mcp.NewToolResultError("empty command"), nil
	}

	// 安全检查
	if err := d.guard.CheckSQL(cmdStr); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("security check failed: %v", err)), nil
	}

	// 权限校验
	key := security.ExtractRedisKey(cmdStr)
	if err := d.perm.CheckRedisCommand(command, key); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("permission denied: %v", err)), nil
	}

	drv, err := d.dm.Get(dbName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// 切换 db
	if dbNum > 0 {
		selectCmd := fmt.Sprintf("SELECT %d", dbNum)
		if _, err := drv.Exec(ctx, selectCmd); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to select db %d: %v", dbNum, err)), nil
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	result, err := drv.Exec(ctx, cmdStr)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("OK. Result: %d", result)), nil
}

func (d *DBMCPServer) handleRedisScan(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(req)
	dbName := strArg(args, "database")
	pattern := strArg(args, "pattern")
	if pattern == "" {
		pattern = "*"
	}
	limit := int(numArg(args, "limit"))
	if limit == 0 {
		limit = 50
	}
	dbNum := int(numArg(args, "db"))

	// 权限校验
	if err := d.perm.CheckRedisCommand("SCAN", ""); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("permission denied: %v", err)), nil
	}

	drv, err := d.dm.Get(dbName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 切换 db
	if dbNum > 0 {
		selectCmd := fmt.Sprintf("SELECT %d", dbNum)
		if _, err := drv.Exec(ctx, selectCmd); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to select db %d: %v", dbNum, err)), nil
		}
	}

	var keys []string
	cursor := uint64(0)
	redisDrv := drv.(*database.RedisDriver)
	for {
		k, nextCursor, err := redisDrv.Scan(ctx, cursor, pattern, 100)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		keys = append(keys, k...)
		if len(keys) >= limit {
			keys = keys[:limit]
			break
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if len(keys) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No keys matching pattern '%s'.", pattern)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Keys matching '%s':\n- %s", pattern, joinStrings(keys, "\n- "))), nil
}

func (d *DBMCPServer) handleRedisInfo(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(req)
	dbName := strArg(args, "database")
	section := strArg(args, "section")

	// 权限校验
	if err := d.perm.CheckRedisCommand("INFO", ""); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("permission denied: %v", err)), nil
	}

	drv, err := d.dm.Get(dbName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	redisDrv, ok := drv.(*database.RedisDriver)
	if !ok {
		return mcp.NewToolResultError("not a Redis driver"), nil
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	info, err := redisDrv.ClientInfo(ctx, section)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(info), nil
}

func (d *DBMCPServer) handleRedisDescribe(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(req)
	dbName := strArg(args, "database")
	key := strArg(args, "key")
	dbNum := int(numArg(args, "db"))

	// 切换 db
	drv, err := d.dm.Get(dbName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if dbNum > 0 {
		selectCmd := fmt.Sprintf("SELECT %d", dbNum)
		if _, err := drv.Exec(ctx, selectCmd); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to select db %d: %v", dbNum, err)), nil
		}
	}

	columns, err := drv.DescribeTable(ctx, dbName, key)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var result string
	for _, c := range columns {
		result += fmt.Sprintf("%s: %s\n", c.Name, c.Type)
	}
	return mcp.NewToolResultText(result), nil
}
```

等等，`handleRedisScan` 使用了类型断言和 `Scan` 方法，但 `DatabaseDriver` 接口没有 `Scan`。需要调整：直接使用 `Exec` 执行 SCAN 命令。修改 `handleRedisScan`：

```go
func (d *DBMCPServer) handleRedisScan(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(req)
	dbName := strArg(args, "database")
	pattern := strArg(args, "pattern")
	if pattern == "" {
		pattern = "*"
	}
	limit := int(numArg(args, "limit"))
	if limit == 0 {
		limit = 50
	}
	dbNum := int(numArg(args, "db"))

	// 权限校验
	if err := d.perm.CheckRedisCommand("SCAN", ""); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("permission denied: %v", err)), nil
	}

	drv, err := d.dm.Get(dbName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 切换 db
	if dbNum > 0 {
		selectCmd := fmt.Sprintf("SELECT %d", dbNum)
		if _, err := drv.Exec(ctx, selectCmd); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to select db %d: %v", dbNum, err)), nil
		}
	}

	// 使用 Exec 执行 SCAN，结果通过驱动内部处理
	result, err := drv.Exec(ctx, fmt.Sprintf("SCAN 0 MATCH %s COUNT %d", pattern, limit))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Scan result: %d", result)), nil
}
```

- [x] **Step 3: 更新 main.go 适配分组配置**

修改 `cmd/dbmcp/main.go` 中的数据库注册部分（第 44-49 行）：

```go
	dm := database.NewDriverManager()
	cfg := app.Config()
	// 使用分组后的所有数据库
	for name, dbCfg := range cfg.DatabaseGroups.AllDatabases() {
		if err := dm.Register(name, dbCfg.Driver, dbCfg); err != nil {
			log.Printf("[warn] failed to register database %s: %v", name, err)
		}
	}
```

修改热重载部分（第 61 行）：

```go
		dm.SyncFromConfig(newCfg.DatabaseGroups.AllDatabases())
```

修改权限初始化（第 51 行）：

```go
	perm := permission.NewChecker(cfg.PermissionsGroup.Relational)
	perm.UpdateNosql(cfg.PermissionsGroup.Nosql)
```

修改热重载中的权限更新（第 62 行）：

```go
		perm.Update(newCfg.PermissionsGroup.Relational)
		perm.UpdateNosql(newCfg.PermissionsGroup.Nosql)
```

- [x] **Step 4: 编译验证**

Run:
```bash
go build -o build/dbmcp.exe ./cmd/dbmcp
```

Expected: 编译成功

- [x] **Step 5: 运行全部测试**

Run:
```bash
go test ./internal/config/... ./internal/security/... ./internal/permission/... -v
```

Expected: 所有测试通过

- [x] **Step 6: 提交**

```bash
git add internal/mcp/server.go cmd/dbmcp/main.go
git commit -m "feat: add 4 Redis MCP tools (redis_command, redis_scan, redis_info, redis_describe)"
```

---

### Task 7: 最终验证与集成测试

**Files:**
- All modified files

- [x] **Step 1: 运行全部单元测试**

Run:
```bash
go test ./internal/config/... ./internal/security/... ./internal/permission/... -v
```

Expected: 全部通过

- [x] **Step 2: 编译最终版本**

Run:
```bash
go build -o build/dbmcp.exe ./cmd/dbmcp
```

Expected: 编译成功

- [x] **Step 3: 更新 CLAUDE.md 文档**

在 `CLAUDE.md` 的 MCP 工具列表中添加 Redis 工具，在配置示例中添加 Redis 分组配置。

找到 MCP 工具列表表格，在 `rollback` 行后添加：

| `redis_command` | 执行 Redis 命令 |
| `redis_scan` | 安全扫描 key |
| `redis_info` | 服务器状态 |
| `redis_describe` | 查看 key 的类型/TTL/值 |

在配置示例部分，在现有示例后添加：

```yaml
  nosql:
    myredis:
      driver: redis
      host: localhost
      port: 6379
      password: ""
      options:
        db: "0"
```

在数据库驱动部分添加：
| `redis.go` | Redis 驱动，使用 go-redis 连接 |

- [x] **Step 4: 最终提交**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md with Redis support documentation"
```

---

## 实施顺序总结

1. **Task 1** — 添加 go-redis 依赖
2. **Task 2** — 重构配置结构为分组模式（核心基础设施）
3. **Task 3** — 实现 Redis 驱动
4. **Task 4** — 实现 Redis 权限系统
5. **Task 5** — 添加 Redis 命令安全辅助
6. **Task 6** — 实现 Redis MCP 工具
7. **Task 7** — 最终验证与文档更新

每个 Task 完成后都可独立编译测试，Task 2 是其他所有任务的前置依赖。
