package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateConfig_EmptyDatabases(t *testing.T) {
	cfg := &AppConfig{
		Databases: map[string]DatabaseConfig{},
	}
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for empty databases")
	}
}

func TestValidateConfig_MissingDriver(t *testing.T) {
	cfg := &AppConfig{
		Databases: map[string]DatabaseConfig{
			"test": {Driver: "", DSN: "some-dsn"},
		},
	}
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for missing driver")
	}
}

func TestValidateConfig_MissingDSN(t *testing.T) {
	cfg := &AppConfig{
		Databases: map[string]DatabaseConfig{
			"test": {Driver: "mysql", DSN: ""},
		},
	}
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for missing DSN")
	}
}

func TestValidateConfig_Valid(t *testing.T) {
	cfg := &AppConfig{
		Databases: map[string]DatabaseConfig{
			"test": {Driver: "mysql", DSN: "user:pass@tcp(localhost:3306)/db"},
		},
	}
	err := ValidateConfig(cfg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
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
	if len(cfg.Databases) != 1 {
		t.Errorf("expected 1 database, got %d", len(cfg.Databases))
	}
	if !cfg.Permissions.ReadOnly {
		t.Error("expected read_only to be true")
	}
	if len(cfg.Permissions.AllowedActions) == 0 {
		t.Error("expected default allowed_actions")
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
