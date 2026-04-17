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
