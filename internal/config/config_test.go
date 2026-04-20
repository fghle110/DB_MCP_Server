package config

import (
	"os"
	"path/filepath"
	"strings"
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
	// 验证 per-database 权限已生成
	perm, ok := cfg.PermissionsGroup.Relational["mydb"]
	if !ok {
		t.Error("expected per-database permission for mydb")
	}
	if !perm.ReadOnly {
		t.Error("expected read_only to be true for mydb")
	}
}

func TestLoadConfig_BackwardCompatible(t *testing.T) {
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

func TestLoadConfig_CreatesBackup(t *testing.T) {
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

	_, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	backupPath := path + ".bak"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("expected backup file config.yaml.bak to exist")
	} else {
		data, _ := os.ReadFile(backupPath)
		if string(data) != content {
			t.Error("expected backup to contain original old-format content")
		}
	}
}

func TestLoadConfig_WritesNewFormat(t *testing.T) {
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

	_, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 重新读取磁盘文件，验证写回的是新格式
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fileContent := string(data)
	if fileContent == content {
		t.Error("expected file to be rewritten with new format, not contain old format")
	}
	// 新格式应包含 database_groups
	if !strings.Contains(fileContent, "database_groups") {
		t.Error("expected file to contain 'database_groups' (new format)")
	}
	if !strings.Contains(fileContent, "permissions_groups") {
		t.Error("expected file to contain 'permissions_groups' (new format)")
	}
}

func TestLoadConfig_NoMigrationForNewFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `database_groups:
  relational:
    mydb:
      driver: mysql
      host: localhost
      port: 3306
      username: user
      password: pass
      database: testdb
permissions_groups:
  relational:
    mydb:
      read_only: false
      allowed_actions: [SELECT, INSERT]
      allowed_databases: ["*"]
      blocked_tables: []
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 新格式不应有备份（因为没有迁移）
	backupPath := path + ".bak"
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Error("expected no backup file for new format config")
	}
}

func TestLoadConfig_MigrateSingleObjectPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `database_groups:
  relational:
    db1:
      driver: mysql
      host: localhost
      port: 3306
      username: root
      password: ""
      database: test
  nosql:
    redis1:
      driver: redis
      host: localhost
      port: 6379
permissions_groups:
  relational:
    read_only: true
    allowed_actions: [SELECT]
  nosql:
    read_only: false
    allowed_commands: [GET, SET]
    blocked_keys: []
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 单一对象应展开为 per-database map
	perm, ok := cfg.PermissionsGroup.Relational["db1"]
	if !ok {
		t.Fatal("expected per-database permission for db1")
	}
	if !perm.ReadOnly {
		t.Error("expected db1 to inherit read_only from single object")
	}
	if len(perm.AllowedActions) != 1 || perm.AllowedActions[0] != "SELECT" {
		t.Errorf("expected db1 actions [SELECT], got %v", perm.AllowedActions)
	}

	// nosql 也应展开
	nosqlPerm, ok := cfg.PermissionsGroup.Nosql["redis1"]
	if !ok {
		t.Fatal("expected per-nosql permission for redis1")
	}
	if len(nosqlPerm.AllowedCommands) != 2 {
		t.Errorf("expected redis1 to have 2 commands, got %d", len(nosqlPerm.AllowedCommands))
	}
}

func TestLoadConfig_MigrateMultipleDatabases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `databases:
  mysql_prod:
    driver: mysql
    dsn: "root:pass@tcp(localhost:3306)/prod"
  pg_dev:
    driver: postgres
    host: localhost
    port: 5432
    username: dev
    password: dev
    database: devdb
  myredis:
    driver: redis
    host: localhost
    port: 6379
permissions:
  read_only: true
  allowed_actions: [SELECT, INSERT]
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证所有数据库都迁移了
	if _, ok := cfg.DatabaseGroups.Relational["mysql_prod"]; !ok {
		t.Error("mysql_prod should migrate to relational")
	}
	if _, ok := cfg.DatabaseGroups.Relational["pg_dev"]; !ok {
		t.Error("pg_dev should migrate to relational")
	}
	if _, ok := cfg.DatabaseGroups.Nosql["myredis"]; !ok {
		t.Error("myredis should migrate to nosql")
	}

	// 验证每个 relational 数据库都有权限
	for _, name := range []string{"mysql_prod", "pg_dev"} {
		perm, ok := cfg.PermissionsGroup.Relational[name]
		if !ok {
			t.Errorf("expected permission for %s", name)
		} else {
			if !perm.ReadOnly {
				t.Errorf("expected %s to be read-only", name)
			}
		}
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(":::invalid yaml{{{"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoadConfig_UnknownDriver(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `databases:
  unknown_db:
    driver: oracle
    dsn: "some-dsn"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 未知驱动应归入 nosql 并打印日志
	if _, ok := cfg.DatabaseGroups.Nosql["unknown_db"]; !ok {
		t.Error("expected unknown driver to migrate to nosql")
	}
}

func TestNormalizeConfig_DoesNotOverwriteExistingPermissions(t *testing.T) {
	cfg := &AppConfig{
		Databases: map[string]DatabaseConfig{
			"mydb": {Driver: "mysql", DSN: "root:pass@tcp(localhost:3306)/db"},
		},
		Permissions: PermissionConfig{
			ReadOnly:       true,
			AllowedActions: []string{"SELECT"},
		},
		PermissionsGroup: PermissionsGroup{
			Relational: map[string]PermissionConfig{
				"mydb": {ReadOnly: false, AllowedActions: []string{"SELECT", "INSERT"}},
			},
		},
	}
	NormalizeConfig(cfg)

	// 已有权限不应被覆盖
	perm := cfg.PermissionsGroup.Relational["mydb"]
	if perm.ReadOnly {
		t.Error("expected existing read_only=false to be preserved")
	}
	if len(perm.AllowedActions) != 2 {
		t.Errorf("expected 2 actions preserved, got %d", len(perm.AllowedActions))
	}
}

