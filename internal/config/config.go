package config

import (
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

// DatabaseConfig 单个数据库连接配置
type DatabaseConfig struct {
	Driver   string            `yaml:"driver"`
	DSN      string            `yaml:"dsn"`
	Host     string            `yaml:"host"`
	Port     int               `yaml:"port"`
	Username string            `yaml:"username"`
	Password string            `yaml:"password"`
	Database string            `yaml:"database"`
	Options  map[string]string `yaml:"options"`
}

// HasStructFields 判断是否使用结构化字段
func (d *DatabaseConfig) HasStructFields() bool {
	return d.Host != "" || d.Port != 0 || d.Username != "" || d.Password != "" || d.Database != ""
}

// DatabaseGroups 按类型分组的数据库配置
type DatabaseGroups struct {
	Relational map[string]DatabaseConfig `yaml:"relational"`
	Nosql      map[string]DatabaseConfig `yaml:"nosql"`
	Timeseries map[string]DatabaseConfig `yaml:"timeseries"`
	Graph      map[string]DatabaseConfig `yaml:"graph"`
}

// AllDatabases 返回所有数据库的扁平 map（用于向后兼容）
func (g *DatabaseGroups) AllDatabases() map[string]DatabaseConfig {
	result := make(map[string]DatabaseConfig)
	for k, v := range g.Relational {
		result[k] = v
	}
	for k, v := range g.Nosql {
		result[k] = v
	}
	for k, v := range g.Timeseries {
		result[k] = v
	}
	for k, v := range g.Graph {
		result[k] = v
	}
	return result
}

// PermissionConfig 权限配置
type PermissionConfig struct {
	ReadOnly         bool     `yaml:"read_only"`
	AllowedDatabases []string `yaml:"allowed_databases"`
	AllowedActions   []string `yaml:"allowed_actions"`
	BlockedTables    []string `yaml:"blocked_tables"`
}

// NosqlPermissionConfig NoSQL 权限配置
type NosqlPermissionConfig struct {
	ReadOnly        bool     `yaml:"read_only"`
	AllowedCommands []string `yaml:"allowed_commands"`
	BlockedKeys     []string `yaml:"blocked_keys"`
}

// PermissionsGroup 按类型分组的权限配置
type PermissionsGroup struct {
	Relational map[string]PermissionConfig      `yaml:"relational"`
	Nosql      map[string]NosqlPermissionConfig `yaml:"nosql"`
	Timeseries map[string]PermissionConfig      `yaml:"timeseries"`
	Graph      map[string]PermissionConfig      `yaml:"graph"`
}

// UnmarshalYAML 自定义解析，支持旧格式（单一对象）和新格式（map）
func (pg *PermissionsGroup) UnmarshalYAML(value *yaml.Node) error {
	// 先尝试用中间结构解析：每种类型可能是单一对象或 map
	type rawGroup struct {
		Relational yaml.Node `yaml:"relational"`
		Nosql      yaml.Node `yaml:"nosql"`
		Timeseries yaml.Node `yaml:"timeseries"`
		Graph      yaml.Node `yaml:"graph"`
	}
	var r rawGroup
	if err := value.Decode(&r); err != nil {
		return err
	}

	pg.Relational = make(map[string]PermissionConfig)
	pg.Nosql = make(map[string]NosqlPermissionConfig)
	pg.Timeseries = make(map[string]PermissionConfig)
	pg.Graph = make(map[string]PermissionConfig)

	// 解析 relational：可能是单个 PermissionConfig 或 map[string]PermissionConfig
	if r.Relational.Kind != 0 {
		// 先尝试解析为单个 PermissionConfig（旧格式）
		var single PermissionConfig
		if err := r.Relational.Decode(&single); err == nil && single.ReadOnly || len(single.AllowedActions) > 0 {
			// 这是旧格式单一对象，暂存标记，后续 applyDefaults 会处理
			pg.Relational[""] = single
		} else {
			// 尝试解析为 map
			var m map[string]PermissionConfig
			if err := r.Relational.Decode(&m); err == nil {
				pg.Relational = m
			} else {
				pg.Relational[""] = single
			}
		}
	}

	// 解析 nosql：可能是单个 NosqlPermissionConfig 或 map
	if r.Nosql.Kind != 0 {
		var single NosqlPermissionConfig
		if err := r.Nosql.Decode(&single); err == nil && single.ReadOnly || len(single.AllowedCommands) > 0 {
			pg.Nosql[""] = single
		} else {
			var m map[string]NosqlPermissionConfig
			if err := r.Nosql.Decode(&m); err == nil {
				pg.Nosql = m
			} else {
				pg.Nosql[""] = single
			}
		}
	}

	return nil
}

// AppConfig 完整配置
type AppConfig struct {
	// 旧格式：flat map（向后兼容）
	Databases map[string]DatabaseConfig `yaml:"databases"`
	// 新格式：按类型分组
	DatabaseGroups DatabaseGroups `yaml:"database_groups"`
	// 旧格式权限（向后兼容）
	Permissions PermissionConfig `yaml:"permissions"`
	// 新格式权限：按类型分组
	PermissionsGroup PermissionsGroup `yaml:"permissions_groups"`
}

// HasOldFormat 判断是否使用了旧格式（扁平 permissions 有实际值，或 permissions_groups 是单一对象）
func (cfg *AppConfig) HasOldFormat() bool {
	if cfg.Permissions.ReadOnly || len(cfg.Permissions.AllowedActions) > 0 || len(cfg.Permissions.AllowedDatabases) > 0 || len(cfg.Permissions.BlockedTables) > 0 {
		return true
	}
	// 检测 permissions_groups 是否为单一对象（空 key 标记）
	if _, ok := cfg.PermissionsGroup.Relational[""]; ok {
		return true
	}
	if _, ok := cfg.PermissionsGroup.Nosql[""]; ok {
		return true
	}
	return false
}

// NormalizeConfig 将旧格式迁移到新格式
func NormalizeConfig(cfg *AppConfig) {
	// 初始化分组 map
	if cfg.DatabaseGroups.Relational == nil {
		cfg.DatabaseGroups.Relational = make(map[string]DatabaseConfig)
	}
	if cfg.DatabaseGroups.Nosql == nil {
		cfg.DatabaseGroups.Nosql = make(map[string]DatabaseConfig)
	}
	if cfg.DatabaseGroups.Timeseries == nil {
		cfg.DatabaseGroups.Timeseries = make(map[string]DatabaseConfig)
	}
	if cfg.DatabaseGroups.Graph == nil {
		cfg.DatabaseGroups.Graph = make(map[string]DatabaseConfig)
	}

	// 如果旧格式有数据，迁移到新格式
	if len(cfg.Databases) > 0 {
		for name, db := range cfg.Databases {
			if db.Driver == "" {
				continue
			}
			switch db.Driver {
			case "mysql", "postgres", "postgresql", "sqlite", "sqlite3", "mssql", "sqlserver", "dm", "dmdbms", "dameng":
				cfg.DatabaseGroups.Relational[name] = db
			case "redis":
				cfg.DatabaseGroups.Nosql[name] = db
			default:
				log.Printf("[config] unknown driver '%s' for '%s', migrating to nosql", db.Driver, name)
				cfg.DatabaseGroups.Nosql[name] = db
			}
		}
	}

	// 初始化权限分组 map
	if cfg.PermissionsGroup.Relational == nil {
		cfg.PermissionsGroup.Relational = make(map[string]PermissionConfig)
	}
	if cfg.PermissionsGroup.Nosql == nil {
		cfg.PermissionsGroup.Nosql = make(map[string]NosqlPermissionConfig)
	}
	if cfg.PermissionsGroup.Timeseries == nil {
		cfg.PermissionsGroup.Timeseries = make(map[string]PermissionConfig)
	}
	if cfg.PermissionsGroup.Graph == nil {
		cfg.PermissionsGroup.Graph = make(map[string]PermissionConfig)
	}

	// 迁移旧 permissions 到 per-database 权限
	if len(cfg.Databases) > 0 && (cfg.Permissions.ReadOnly || len(cfg.Permissions.AllowedActions) > 0) {
		oldPerm := cfg.Permissions
		for name := range cfg.DatabaseGroups.Relational {
			if _, exists := cfg.PermissionsGroup.Relational[name]; !exists {
				cfg.PermissionsGroup.Relational[name] = PermissionConfig{
					ReadOnly:         oldPerm.ReadOnly,
					AllowedDatabases: oldPerm.AllowedDatabases,
					AllowedActions:   oldPerm.AllowedActions,
					BlockedTables:    oldPerm.BlockedTables,
				}
			}
		}
	}

	// 迁移 permissions_groups 中的单一对象（旧分组格式）到 per-database map
	if relSingle, ok := cfg.PermissionsGroup.Relational[""]; ok {
		delete(cfg.PermissionsGroup.Relational, "")
		if len(cfg.PermissionsGroup.Relational) == 0 {
			// 没有 per-database 配置，将单一对象分发给所有数据库
			for name := range cfg.DatabaseGroups.Relational {
				cfg.PermissionsGroup.Relational[name] = relSingle
			}
		}
		// 如果已有 per-database 配置，空 key 是冗余残留，直接删除即可
	}
	if nosqlSingle, ok := cfg.PermissionsGroup.Nosql[""]; ok {
		delete(cfg.PermissionsGroup.Nosql, "")
		if len(cfg.PermissionsGroup.Nosql) == 0 {
			for name := range cfg.DatabaseGroups.Nosql {
				cfg.PermissionsGroup.Nosql[name] = nosqlSingle
			}
		}
	}

	// 确保新格式 map 已初始化
	if len(cfg.DatabaseGroups.Relational) == 0 {
		cfg.DatabaseGroups.Relational = make(map[string]DatabaseConfig)
	}
	if len(cfg.DatabaseGroups.Nosql) == 0 {
		cfg.DatabaseGroups.Nosql = make(map[string]DatabaseConfig)
	}
	if len(cfg.DatabaseGroups.Timeseries) == 0 {
		cfg.DatabaseGroups.Timeseries = make(map[string]DatabaseConfig)
	}
	if len(cfg.DatabaseGroups.Graph) == 0 {
		cfg.DatabaseGroups.Graph = make(map[string]DatabaseConfig)
	}
}

// applyDefaults 应用默认权限配置
func applyDefaults(cfg *AppConfig) {
	// 初始化权限 map
	if cfg.PermissionsGroup.Relational == nil {
		cfg.PermissionsGroup.Relational = make(map[string]PermissionConfig)
	}
	if cfg.PermissionsGroup.Nosql == nil {
		cfg.PermissionsGroup.Nosql = make(map[string]NosqlPermissionConfig)
	}
	if cfg.PermissionsGroup.Timeseries == nil {
		cfg.PermissionsGroup.Timeseries = make(map[string]PermissionConfig)
	}
	if cfg.PermissionsGroup.Graph == nil {
		cfg.PermissionsGroup.Graph = make(map[string]PermissionConfig)
	}

	// 为每个 relational 数据库生成默认权限
	for name := range cfg.DatabaseGroups.Relational {
		if _, exists := cfg.PermissionsGroup.Relational[name]; !exists {
			cfg.PermissionsGroup.Relational[name] = PermissionConfig{
				ReadOnly:         false,
				AllowedDatabases: []string{"*"},
				AllowedActions:   []string{"SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "DROP"},
				BlockedTables:    []string{},
			}
		}
	}

	// 为每个 nosql 数据库生成默认权限
	for name := range cfg.DatabaseGroups.Nosql {
		if _, exists := cfg.PermissionsGroup.Nosql[name]; !exists {
			cfg.PermissionsGroup.Nosql[name] = NosqlPermissionConfig{
				ReadOnly:        false,
				AllowedCommands: []string{
					"GET", "SET", "HGET", "HGETALL", "HSET", "LPUSH", "LRANGE",
					"SADD", "SMEMBERS", "SCAN", "INFO", "DEL", "EXISTS", "TTL",
					"TYPE", "PING", "ECHO", "DBSIZE", "KEYS",
				},
				BlockedKeys: []string{},
			}
		}
	}
}

// ValidateConfig 校验配置合法性
func ValidateConfig(cfg *AppConfig) error {
	allDB := cfg.DatabaseGroups.AllDatabases()
	if len(allDB) == 0 {
		return &ConfigError{Message: "no valid databases configured"}
	}
	for name, db := range allDB {
		if db.Driver == "" {
			return &ConfigError{Message: "database '" + name + "' missing driver"}
		}
		if db.DSN == "" && db.Host == "" {
			return &ConfigError{Message: "database '" + name + "' missing dsn or host"}
		}
	}
	return nil
}

// ConfigError 配置错误
type ConfigError struct {
	Message string
}

func (e *ConfigError) Error() string {
	return e.Message
}

// ReloadContext 热重载状态
type ReloadContext struct {
	LastReload    time.Time
	ReloadSuccess bool
}

// AppState 运行时配置(线程安全)
type AppState struct {
	config     atomic.Pointer[AppConfig]
	reloadCtx  atomic.Pointer[ReloadContext]
	mu         sync.RWMutex
	configPath string
}

// NewAppState 创建运行时配置
func NewAppState(configPath string) (*AppState, error) {
	app := &AppState{configPath: configPath}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	app.config.Store(cfg)
	app.reloadCtx.Store(&ReloadContext{LastReload: time.Now(), ReloadSuccess: true})
	return app, nil
}

// Config 原子读取当前配置
func (a *AppState) Config() *AppConfig {
	return a.config.Load()
}

// ReloadCtx 原子读取热重载状态
func (a *AppState) ReloadCtx() *ReloadContext {
	return a.reloadCtx.Load()
}

// ConfigPath 获取配置文件路径
func (a *AppState) ConfigPath() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.configPath
}

// SetConfigPath 原子更新配置路径
func (a *AppState) SetConfigPath(path string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.configPath = path
}

// UpdateConfig 原子更新配置
func (a *AppState) UpdateConfig(cfg *AppConfig) {
	a.config.Store(cfg)
	a.reloadCtx.Store(&ReloadContext{LastReload: time.Now(), ReloadSuccess: true})
}

// UpdateReloadFailed 记录热重载失败
func (a *AppState) UpdateReloadFailed() {
	a.reloadCtx.Store(&ReloadContext{LastReload: time.Now(), ReloadSuccess: false})
}

// LoadConfig 从文件加载配置
func LoadConfig(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// 检测是否需要迁移
	needsMigration := cfg.HasOldFormat()
	if needsMigration {
		// 先迁移，再生成默认值（确保迁移后的权限不被默认值覆盖）
		NormalizeConfig(&cfg)
		applyDefaults(&cfg)
		// 序列化为新格式
		newData, err := yaml.Marshal(&cfg)
		if err != nil {
			return nil, fmt.Errorf("marshal migrated config: %w", err)
		}
		// 先备份
		if err := BackupConfig(path); err != nil {
			log.Printf("[config] backup failed before migration: %v", err)
		}
		// 写回新格式
		if err := os.WriteFile(path, newData, 0644); err != nil {
			return nil, fmt.Errorf("write migrated config: %w", err)
		}
		log.Println("[config] migrated config to per-database permissions")
	} else {
		NormalizeConfig(&cfg)
		applyDefaults(&cfg)
	}

	if err := ValidateConfig(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// BackupConfig 备份配置文件
func BackupConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".bak", data, 0600)
}
