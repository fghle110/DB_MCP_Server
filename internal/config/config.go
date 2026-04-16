package config

import (
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

// PermissionConfig 权限配置
type PermissionConfig struct {
	ReadOnly         bool     `yaml:"read_only"`
	AllowedDatabases []string `yaml:"allowed_databases"`
	AllowedActions   []string `yaml:"allowed_actions"`
	BlockedTables    []string `yaml:"blocked_tables"`
}

// AppConfig 完整配置
type AppConfig struct {
	Databases   map[string]DatabaseConfig `yaml:"databases"`
	Permissions PermissionConfig          `yaml:"permissions"`
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
	applyDefaults(&cfg)
	if err := ValidateConfig(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// applyDefaults 应用默认权限配置
func applyDefaults(cfg *AppConfig) {
	if cfg.Permissions.AllowedActions == nil {
		cfg.Permissions.AllowedActions = []string{"select", "insert", "update", "delete", "create", "drop"}
	}
	if cfg.Permissions.AllowedDatabases == nil {
		cfg.Permissions.AllowedDatabases = []string{"*"}
	}
	if cfg.Permissions.BlockedTables == nil {
		cfg.Permissions.BlockedTables = []string{}
	}
	if cfg.Databases == nil {
		cfg.Databases = make(map[string]DatabaseConfig)
	}
}

// ValidateConfig 校验配置合法性
func ValidateConfig(cfg *AppConfig) error {
	if len(cfg.Databases) == 0 {
		return &ConfigError{Message: "no databases configured"}
	}
	for name, db := range cfg.Databases {
		if db.Driver == "" {
			return &ConfigError{Message: "database '" + name + "' missing driver"}
		}
		if db.DSN == "" {
			return &ConfigError{Message: "database '" + name + "' missing dsn"}
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

// BackupConfig 备份配置文件
func BackupConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".bak", data, 0600)
}
