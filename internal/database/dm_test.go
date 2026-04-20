package database

import (
	"testing"

	"github.com/dbmcp/dbmcp/internal/config"
)

func TestDmDriver_StubReturnsError(t *testing.T) {
	drv, err := NewDmDriver()
	if err == nil {
		t.Fatal("expected error from stub")
	}
	if drv != nil {
		t.Fatal("expected nil driver from stub")
	}
}

func TestDmDriver_StubMethodsReturnError(t *testing.T) {
	_, err := NewDmDriver()
	if err == nil {
		t.Fatal("expected error from stub")
	}
	// Driver is nil, subsequent methods are not callable on nil pointer.
	// The stub is validated by createDriver in the manager integration tests.
}

func TestBuildDmDSN_Defaults(t *testing.T) {
	cfg := config.DatabaseConfig{
		Driver:   "dm",
		Username: "SYSDBA",
		Password: "SYSDBA",
		Database: "TEST",
	}
	dsn := buildDmDSN(cfg)
	// Database 字段不作为 schema 参数
	if dsn != "dm://SYSDBA:SYSDBA@localhost:5236?autoCommit=0" {
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
			"schema":         "my_schema",
		},
	}
	dsn := buildDmDSN(cfg)
	expected := "dm://admin:pass@192.168.1.100:5237?autoCommit=0&connectTimeout=30&schema=my_schema"
	if dsn != expected {
		t.Errorf("unexpected DSN: %s (expected: %s)", dsn, expected)
	}
}
