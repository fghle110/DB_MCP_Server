package permission

import (
	"fmt"
	"sync/atomic"

	"github.com/dbmcp/dbmcp/internal/config"
)

// Checker 权限校验器
type Checker struct {
	cfg atomic.Pointer[config.PermissionConfig]
}

// NewChecker 创建权限校验器
func NewChecker(cfg config.PermissionConfig) *Checker {
	c := &Checker{}
	c.cfg.Store(&cfg)
	return c
}

// Update 更新权限配置(原子替换)
func (c *Checker) Update(cfg config.PermissionConfig) {
	c.cfg.Store(&cfg)
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
