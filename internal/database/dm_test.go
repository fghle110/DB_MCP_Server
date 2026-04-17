package database

import (
	"context"
	"testing"

	"github.com/dbmcp/dbmcp/internal/config"
)

func TestDmDriver_NewDriver(t *testing.T) {
	drv := NewDmDriver()
	if drv == nil {
		t.Fatal("expected non-nil driver")
	}
}

func TestDmDriver_QueryNotSupported(t *testing.T) {
	drv := NewDmDriver()
	_, err := drv.Query(context.Background(), "SELECT 1")
	if err == nil {
		t.Error("expected error for unconnected driver")
	}
}

func TestDmDriver_ListDatabasesNotConnected(t *testing.T) {
	drv := NewDmDriver()
	_, err := drv.ListDatabases(context.Background())
	if err == nil {
		t.Error("expected error for unconnected driver")
	}
}

func TestDmDriver_BeginTxNotSupported(t *testing.T) {
	drv := NewDmDriver()
	err := drv.BeginTx(context.Background())
	if err == nil {
		t.Error("expected error for transactions on unconnected driver")
	}
}

func TestDmDriver_CommitNotSupported(t *testing.T) {
	drv := NewDmDriver()
	err := drv.Commit()
	if err == nil {
		t.Error("expected error for commit on unconnected driver")
	}
}

func TestDmDriver_RollbackNotSupported(t *testing.T) {
	drv := NewDmDriver()
	err := drv.Rollback()
	if err == nil {
		t.Error("expected error for rollback on unconnected driver")
	}
}

func TestDmDriver_CloseWithoutConnection(t *testing.T) {
	drv := NewDmDriver()
	err := drv.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuildDmDSN_Defaults(t *testing.T) {
	cfg := config.DatabaseConfig{
		Driver:   "dm",
		Username: "SYSDBA",
		Password: "SYSDBA",
		Database: "TEST",
	}
	dsn := buildDmDSN(cfg)
	if dsn != "dm://SYSDBA:SYSDBA@localhost:5236?schema=TEST" {
		t.Errorf("unexpected DSN: %s", dsn)
	}
}

func TestBuildDmDSN_WithOptions(t *testing.T) {
	cfg := config.DatabaseConfig{
		Driver:   "dm",
		Host:     "192.168.1.100",
		Port:     5237,
		Username: "admin",
		Password: "pass",
		Database: "MYDB",
		Options: map[string]string{
			"connectTimeout": "30",
		},
	}
	dsn := buildDmDSN(cfg)
	expected := "dm://admin:pass@192.168.1.100:5237?schema=MYDB&connectTimeout=30"
	if dsn != expected {
		t.Errorf("unexpected DSN: %s (expected: %s)", dsn, expected)
	}
}
