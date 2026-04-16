package database

import (
	"fmt"
	"log"
	"sync"
	"time"

	"dbmcp/internal/config"
)

// DriverManager 数据库连接池管理器
type DriverManager struct {
	mu      sync.RWMutex
	drivers map[string]DatabaseDriver
}

// NewDriverManager 创建连接池管理器
func NewDriverManager() *DriverManager {
	return &DriverManager{
		drivers: make(map[string]DatabaseDriver),
	}
}

// Register 注册数据库连接
func (dm *DriverManager) Register(name, driverType, dsn string) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if existing, ok := dm.drivers[name]; ok {
		existing.Close()
	}

	drv, err := createDriver(driverType)
	if err != nil {
		return fmt.Errorf("create driver '%s': %w", driverType, err)
	}

	if err := drv.Connect(dsn); err != nil {
		return fmt.Errorf("connect '%s': %w", name, err)
	}

	dm.drivers[name] = drv
	log.Printf("[db] registered database: %s (%s)", name, driverType)
	return nil
}

// Get 获取数据库驱动
func (dm *DriverManager) Get(name string) (DatabaseDriver, error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	drv, ok := dm.drivers[name]
	if !ok {
		return nil, fmt.Errorf("database '%s' not found", name)
	}
	return drv, nil
}

// Remove 移除数据库连接
func (dm *DriverManager) Remove(name string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	if drv, ok := dm.drivers[name]; ok {
		drv.Close()
		delete(dm.drivers, name)
		log.Printf("[db] removed database: %s", name)
	}
}

// List 列出所有已注册的数据库
func (dm *DriverManager) List() []string {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	names := make([]string, 0, len(dm.drivers))
	for name := range dm.drivers {
		names = append(names, name)
	}
	return names
}

// CloseAll 关闭所有连接
func (dm *DriverManager) CloseAll() {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	for name, drv := range dm.drivers {
		if err := drv.Close(); err != nil {
			log.Printf("[db] error closing %s: %v", name, err)
		}
	}
	dm.drivers = make(map[string]DatabaseDriver)
}

// SyncFromConfig 从配置同步连接(用于热重载)
func (dm *DriverManager) SyncFromConfig(databases map[string]config.DatabaseConfig) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	for name, dbCfg := range databases {
		drv, err := createDriver(dbCfg.Driver)
		if err != nil {
			log.Printf("[db] skip %s: %v", name, err)
			continue
		}
		if err := drv.Connect(dbCfg.DSN); err != nil {
			log.Printf("[db] connect %s failed: %v", name, err)
			continue
		}
		if old, ok := dm.drivers[name]; ok {
			go func(d DatabaseDriver) {
				time.Sleep(30 * time.Second)
				d.Close()
			}(old)
		}
		dm.drivers[name] = drv
	}

	for name := range dm.drivers {
		if _, ok := databases[name]; !ok {
			dm.drivers[name].Close()
			delete(dm.drivers, name)
			log.Printf("[db] removed stale database: %s", name)
		}
	}
}

// createDriver 根据类型创建驱动实例
func createDriver(driverType string) (DatabaseDriver, error) {
	switch driverType {
	case "mysql":
		return NewMySQLDriver(), nil
	case "postgres", "postgresql":
		return NewPostgresDriver(), nil
	case "sqlite", "sqlite3":
		return NewSQLiteDriver(), nil
	default:
		return nil, fmt.Errorf("unsupported driver type: %s", driverType)
	}
}
