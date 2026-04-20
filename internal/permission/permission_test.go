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
