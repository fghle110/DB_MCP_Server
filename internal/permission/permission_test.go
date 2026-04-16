package permission

import (
	"testing"

	"github.com/dbmcp/dbmcp/internal/config"
)

func fullPermConfig() config.PermissionConfig {
	return config.PermissionConfig{
		ReadOnly:         false,
		AllowedDatabases: []string{"*"},
		AllowedActions:   []string{"SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "DROP"},
		BlockedTables:    []string{},
	}
}

func TestCheckSelect_Allowed(t *testing.T) {
	c := NewChecker(fullPermConfig())
	err := c.CheckSelect("testdb", "users")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCheckSelect_BlockedDatabase(t *testing.T) {
	cfg := fullPermConfig()
	cfg.AllowedDatabases = []string{"only_this_db"}
	c := NewChecker(cfg)
	err := c.CheckSelect("other_db", "users")
	if err == nil {
		t.Error("expected error for blocked database")
	}
}

func TestCheckSelect_BlockedTable(t *testing.T) {
	cfg := fullPermConfig()
	cfg.BlockedTables = []string{"secret_table"}
	c := NewChecker(cfg)
	err := c.CheckSelect("testdb", "secret_table")
	if err == nil {
		t.Error("expected error for blocked table")
	}
}

func TestCheckWrite_ReadOnlyMode(t *testing.T) {
	cfg := fullPermConfig()
	cfg.ReadOnly = true
	c := NewChecker(cfg)
	err := c.CheckWrite("testdb", "users", "INSERT")
	if err == nil {
		t.Error("expected error in read-only mode")
	}
}

func TestCheckWrite_Allowed(t *testing.T) {
	cfg := fullPermConfig()
	c := NewChecker(cfg)
	err := c.CheckWrite("testdb", "users", "INSERT")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCheckWrite_ActionNotAllowed(t *testing.T) {
	cfg := fullPermConfig()
	cfg.AllowedActions = []string{"SELECT"}
	c := NewChecker(cfg)
	err := c.CheckWrite("testdb", "users", "DELETE")
	if err == nil {
		t.Error("expected error for disallowed action")
	}
}

func TestUpdate_AppliedImmediately(t *testing.T) {
	c := NewChecker(fullPermConfig())
	err := c.CheckWrite("testdb", "users", "INSERT")
	if err != nil {
		t.Fatalf("expected write allowed before update: %v", err)
	}
	newCfg := fullPermConfig()
	newCfg.ReadOnly = true
	c.Update(newCfg)
	err = c.CheckWrite("testdb", "users", "INSERT")
	if err == nil {
		t.Error("expected error after switching to read-only mode")
	}
}
