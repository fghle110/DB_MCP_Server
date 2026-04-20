package permission

import (
	"fmt"
	"path/filepath"
	"sync/atomic"

	"github.com/dbmcp/dbmcp/internal/config"
)

// Checker 权限校验器
type Checker struct {
	relationalPerms atomic.Pointer[map[string]config.PermissionConfig]
	nosqlPerms      atomic.Pointer[map[string]config.NosqlPermissionConfig]
}

// NewChecker 创建权限校验器
func NewChecker(perms config.PermissionsGroup) *Checker {
	c := &Checker{}
	c.relationalPerms.Store(&perms.Relational)
	c.nosqlPerms.Store(&perms.Nosql)
	return c
}

// Update 更新指定关系型数据库的权限配置(原子替换)
func (c *Checker) Update(database string, cfg config.PermissionConfig) {
	perms := c.relationalPerms.Load()
	newPerms := make(map[string]config.PermissionConfig)
	for k, v := range *perms {
		newPerms[k] = v
	}
	newPerms[database] = cfg
	c.relationalPerms.Store(&newPerms)
}

// UpdateNosql 更新指定 NoSQL 数据库的权限配置(原子替换)
func (c *Checker) UpdateNosql(database string, cfg config.NosqlPermissionConfig) {
	perms := c.nosqlPerms.Load()
	newPerms := make(map[string]config.NosqlPermissionConfig)
	for k, v := range *perms {
		newPerms[k] = v
	}
	newPerms[database] = cfg
	c.nosqlPerms.Store(&newPerms)
}

// UpdateFromConfig 从配置全量更新权限（用于热重载）
func (c *Checker) UpdateFromConfig(perms config.PermissionsGroup) {
	c.relationalPerms.Store(&perms.Relational)
	c.nosqlPerms.Store(&perms.Nosql)
}

// CheckSelect 检查 SELECT 权限
func (c *Checker) CheckSelect(database string, tableName string) error {
	perms := c.relationalPerms.Load()
	cfg, ok := (*perms)[database]
	if !ok {
		return fmt.Errorf("database '%s' has no permission config", database)
	}
	if err := c.checkTable(&cfg, tableName); err != nil {
		return err
	}
	return nil
}

// CheckWrite 检查写入权限
func (c *Checker) CheckWrite(database string, tableName string, actionType string) error {
	perms := c.relationalPerms.Load()
	cfg, ok := (*perms)[database]
	if !ok {
		return fmt.Errorf("database '%s' has no permission config", database)
	}
	if cfg.ReadOnly {
		return fmt.Errorf("database '%s' is in read-only mode, %s not allowed", database, actionType)
	}
	if err := c.checkTable(&cfg, tableName); err != nil {
		return err
	}
	if !c.isActionAllowed(&cfg, actionType) {
		return fmt.Errorf("action '%s' is not allowed for database '%s'", actionType, database)
	}
	return nil
}

// CheckRedisCommand 检查 Redis 的命令权限
func (c *Checker) CheckRedisCommand(database string, cmd string, key string) error {
	perms := c.nosqlPerms.Load()
	cfg, ok := (*perms)[database]
	if !ok {
		return fmt.Errorf("database '%s' has no nosql permission config", database)
	}

	// read_only 模式：拦截写命令
	if cfg.ReadOnly && IsRedisWriteCommand(cmd) {
		return fmt.Errorf("database '%s' is in read-only mode, %s not allowed", database, cmd)
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

// IsReadOnly 检查指定数据库是否只读模式
func (c *Checker) IsReadOnly(database string) bool {
	perms := c.relationalPerms.Load()
	cfg, ok := (*perms)[database]
	if !ok {
		return false
	}
	return cfg.ReadOnly
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
