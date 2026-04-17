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
