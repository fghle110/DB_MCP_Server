package database

import (
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/dbmcp/dbmcp/internal/config"
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
func (dm *DriverManager) Register(name, driverType string, cfg config.DatabaseConfig) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if existing, ok := dm.drivers[name]; ok {
		existing.Close()
	}

	drv, err := createDriver(driverType)
	if err != nil {
		return fmt.Errorf("create driver '%s': %w", driverType, err)
	}

	dsn := buildDSN(driverType, cfg)
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
		dsn := buildDSN(dbCfg.Driver, dbCfg)
		if err := drv.Connect(dsn); err != nil {
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
	case "mssql", "sqlserver":
		return NewMSSQLDriver(), nil
	case "redis":
		return NewRedisDriver(), nil
	case "dm", "dmdbms", "dameng":
		return NewDmDriver(), nil
	default:
		return nil, fmt.Errorf("unsupported driver type: %s", driverType)
	}
}

// buildDSN 从结构化配置或原始 DSN 字符串构建连接串
func buildDSN(driverType string, cfg config.DatabaseConfig) string {
	// 如果提供了原始 DSN，直接使用
	if cfg.DSN != "" {
		return cfg.DSN
	}

	// 否则从结构化字段组装
	switch driverType {
	case "mysql":
		return buildMySQLDSN(cfg)
	case "postgres", "postgresql":
		return buildPostgresDSN(cfg)
	case "sqlite", "sqlite3":
		return buildSQLiteDSN(cfg)
	case "mssql", "sqlserver":
		return buildMSSQLDSN(cfg)
	case "redis":
		return buildRedisDSN(cfg)
	case "dm", "dmdbms", "dameng":
		return buildDmDSN(cfg)
	default:
		return ""
	}
}

// buildMySQLDSN 组装 MySQL DSN: user:pass@tcp(host:port)/dbname?opts
func buildMySQLDSN(cfg config.DatabaseConfig) string {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	port := cfg.Port
	if port == 0 {
		port = 3306
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	user := cfg.Username
	if user == "" {
		user = "root"
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s",
		user, url.QueryEscape(cfg.Password), addr, cfg.Database)

	// 组装 options
	opts := make(url.Values)
	for k, v := range cfg.Options {
		opts.Set(k, v)
	}
	if encoded := opts.Encode(); encoded != "" {
		dsn += "?" + encoded
	}
	return dsn
}

// buildPostgresDSN 组装 PostgreSQL DSN: postgres://user:pass@host:port/dbname?opts
func buildPostgresDSN(cfg config.DatabaseConfig) string {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	port := cfg.Port
	if port == 0 {
		port = 5432
	}
	user := cfg.Username
	if user == "" {
		user = "postgres"
	}
	dbname := cfg.Database
	if dbname == "" {
		dbname = "postgres"
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s",
		url.QueryEscape(user), url.QueryEscape(cfg.Password), host, port, url.QueryEscape(dbname))

	// 组装 options
	opts := make(url.Values)
	for k, v := range cfg.Options {
		opts.Set(k, v)
	}
	if encoded := opts.Encode(); encoded != "" {
		dsn += "?" + encoded
	}
	return dsn
}

// buildSQLiteDSN 组装 SQLite DSN: 直接返回文件路径
func buildSQLiteDSN(cfg config.DatabaseConfig) string {
	if cfg.DSN != "" {
		return cfg.DSN
	}
	// 如果 host 被当作文件路径使用
	if cfg.Host != "" {
		return cfg.Host
	}
	// 默认
	return ":memory:"
}

// buildMSSQLDSN 组装 MSSQL DSN: sqlserver://user:pass@host:port?database=dbname&opts
func buildMSSQLDSN(cfg config.DatabaseConfig) string {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	port := cfg.Port
	if port == 0 {
		port = 1433
	}
	user := cfg.Username
	if user == "" {
		user = "sa"
	}
	dbname := cfg.Database
	if dbname == "" {
		dbname = "master"
	}

	dsn := fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s",
		url.QueryEscape(user), url.QueryEscape(cfg.Password), host, port, url.QueryEscape(dbname))

	// 组装 options
	opts := make(url.Values)
	for k, v := range cfg.Options {
		opts.Set(k, v)
	}
	if encoded := opts.Encode(); encoded != "" {
		dsn += "&" + encoded
	}
	return dsn
}

// buildRedisDSN 组装 Redis DSN: redis://username:password@host:port/db
func buildRedisDSN(cfg config.DatabaseConfig) string {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	port := cfg.Port
	if port == 0 {
		port = 6379
	}

	dbNum := "0"
	if v, ok := cfg.Options["db"]; ok {
		dbNum = v
	}

	if cfg.Password != "" {
		return fmt.Sprintf("redis://%s:%s@%s:%d/%s",
			url.QueryEscape(cfg.Username), url.QueryEscape(cfg.Password), host, port, dbNum)
	}
	return fmt.Sprintf("redis://:%s@%s:%d/%s", url.QueryEscape(cfg.Password), host, port, dbNum)
}

// buildDmDSN 组装达梦 DSN: dm://user:password@host:port?opts
func buildDmDSN(cfg config.DatabaseConfig) string {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	port := cfg.Port
	if port == 0 {
		port = 5236
	}
	user := cfg.Username
	if user == "" {
		user = "SYSDBA"
	}
	password := cfg.Password

	dsn := fmt.Sprintf("dm://%s:%s@%s:%d",
		url.QueryEscape(user), url.QueryEscape(password), host, port)

	// 组装 options
	opts := make(url.Values)
	for k, v := range cfg.Options {
		// 跳过空值的 schema 参数
		if k == "schema" && v == "" {
			continue
		}
		opts.Set(k, v)
	}
	// 达梦默认启用 autocommit，为支持事务需要关闭
	if cfg.Driver == "dm" || cfg.Driver == "dmdbms" || cfg.Driver == "dameng" {
		if _, ok := opts["autoCommit"]; !ok {
			opts.Set("autoCommit", "0")
		}
	}
	if encoded := opts.Encode(); encoded != "" {
		dsn += "?" + encoded
	}
	return dsn
}
