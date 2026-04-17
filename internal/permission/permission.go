package permission

import (
	"fmt"
	"path/filepath"
	"sync/atomic"

	"github.com/dbmcp/dbmcp/internal/config"
)

// Checker 权限校验器
type Checker struct {
	cfg      atomic.Pointer[config.PermissionConfig]
	nosqlCfg atomic.Pointer[config.NosqlPermissionConfig]
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

// CheckRedisCommand 检查 Redis 的命令权限
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
