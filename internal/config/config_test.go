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
