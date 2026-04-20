# Per-Database Permission Control Design

> **Goal:** 将当前按数据库类型（relational/nosql）全局共享的权限模型，改为按配置文件中的数据库名称独立控制权限。每个数据库有独立的 `read_only`、`allowed_actions`、`blocked_tables` 等配置。

**Tech:** Go 1.26, yaml.v3

---

## Problem

当前 `PermissionsGroup` 中 `relational` 和 `nosql` 各只有一份全局配置：

```yaml
permissions_groups:
  relational:
    read_only: false        # 对所有关系型数据库生效
    allowed_actions: [SELECT, INSERT, UPDATE, DELETE]
  nosql:
    allowed_commands: [GET, SET, ...]
```

无法实现以下场景：
- `mysql_prod` 只读，`mysql_dev` 完全开放
- `pg_analytics` 仅允许 SELECT，`mysql_app` 允许所有操作
- 对特定数据库设置特定的 `blocked_tables`

---

## Design

### 1. Config Structure Change

`PermissionsGroup` 中的 `relational` 和 `nosql` 从单一对象改为 `map[string]`，key 为数据库名称：

```go
// Before
type PermissionsGroup struct {
    Relational PermissionConfig          `yaml:"relational"`
    Nosql      NosqlPermissionConfig     `yaml:"nosql"`
}

// After
type PermissionsGroup struct {
    Relational map[string]PermissionConfig      `yaml:"relational"`
    Nosql      map[string]NosqlPermissionConfig `yaml:"nosql"`
    Timeseries map[string]PermissionConfig      `yaml:"timeseries"`
    Graph      map[string]PermissionConfig      `yaml:"graph"`
}
```

新配置格式示例：

```yaml
permissions_groups:
  relational:
    mysql_prod:
      read_only: true
      allowed_actions: [SELECT]
      blocked_tables: [secret_data]
    mysql_dev:
      read_only: false
      allowed_actions: [SELECT, INSERT, UPDATE, DELETE]
      blocked_tables: []
  nosql:
    local_redis:
      read_only: false
      allowed_commands: [GET, SET, SCAN]
      blocked_keys: ["*session*"]
```

### 2. Config Auto-Migration

旧配置（全局对象或扁平 `permissions`）检测后自动迁移并**写回文件**：

**迁移触发条件：**
- `permissions` 字段有值（旧扁平格式）
- `permissions_groups.relational` 是单一对象而非 map（旧分组格式）

**迁移行为：**
1. 备份原文件为 `config.yaml.bak`
2. 将旧权限复制给所有已存在的对应类型数据库
3. 未配置的数据库使用 `applyDefaults` 生成的默认值
4. 将新格式序列化为 YAML 写回配置文件
5. 日志记录：`[config] migrated config to per-database permissions`

**迁移示例：**

```yaml
# 输入（旧格式）
permissions:
  read_only: true
  allowed_actions: [SELECT]

# 假设已配置了 mysql_prod 和 mysql_dev（均为 relational 类型）
# 输出（新格式，写回 config.yaml）
permissions_groups:
  relational:
    mysql_prod:
      read_only: true
      allowed_actions: [SELECT]
      blocked_tables: []
    mysql_dev:
      read_only: true
      allowed_actions: [SELECT]
      blocked_tables: []
  nosql:
    {}  # 默认值，无 nosql 数据库时为空
```

### 3. Permission Checker Refactor

`Checker` 存储从全局配置改为 per-database map：

```go
type Checker struct {
    dbGroups       atomic.Pointer[config.DatabaseGroups]              // 数据库列表（存在性校验）
    relationalPerms atomic.Pointer[map[string]config.PermissionConfig]      // per-database 权限
    nosqlPerms     atomic.Pointer[map[string]config.NosqlPermissionConfig]   // per-database 权限
}
```

**初始化：**

```go
func NewChecker(groups config.DatabaseGroups, perms config.PermissionsGroup) *Checker {
    c := &Checker{}
    c.dbGroups.Store(&groups)
    c.relationalPerms.Store(&perms.Relational)
    c.nosqlPerms.Store(&perms.Nosql)
    return c
}
```

**查找逻辑：**

```go
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
```

**Redis 权限同理：**

```go
func (c *Checker) CheckRedisCommand(database string, cmd string, key string) error {
    perms := c.nosqlPerms.Load()
    cfg, ok := (*perms)[database]
    if !ok {
        return fmt.Errorf("database '%s' has no nosql permission config", database)
    }
    // 原有校验逻辑不变
    ...
}
```

### 4. Server Initialization Update

`main.go` 和热重载部分的权限初始化需要适配新签名：

```go
// Before
perm := permission.NewChecker(cfg.PermissionsGroup.Relational)

// After
perm := permission.NewChecker(cfg.DatabaseGroups, cfg.PermissionsGroup)
```

热重载部分同理：

```go
// In config watcher callback
perm.UpdateFromConfig(newCfg.DatabaseGroups, newCfg.PermissionsGroup)
```

### 5. Files to Modify

| File | Change |
|------|--------|
| `internal/config/config.go` | `PermissionsGroup` 结构体变更，新增 `HasOldFormat()` / `HasLegacyGroupedPermissions()` 检测函数，`NormalizeConfig` 处理 per-database 迁移，`LoadConfig` 增加写回逻辑 |
| `internal/config/config_test.go` | 新增 per-database 权限测试、旧格式迁移测试、文件写回测试 |
| `internal/permission/permission.go` | `Checker` 存储结构变更，`CheckSelect`/`CheckWrite`/`CheckRedisCommand` 改为 per-database 查找，`Update` 方法改为按数据库名更新 |
| `internal/mcp/server.go` | `CheckRedisCommand` 调用增加 `database` 参数 |
| `cmd/dbmcp/main.go` | `NewChecker` 调用适配新签名 |

### 6. Default Values

`applyDefaults` 为每个已存在的数据库生成默认权限配置：

```go
func applyDefaults(cfg *AppConfig) {
    // 为每个 relational 数据库生成默认权限
    if len(cfg.PermissionsGroup.Relational) == 0 {
        cfg.PermissionsGroup.Relational = make(map[string]config.PermissionConfig)
    }
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
    // nosql 同理
    ...
}
```

---

## Migration Safety

- 配置文件迁移前先备份为 `config.yaml.bak`
- 迁移失败时保留旧配置，不写回文件
- 日志记录迁移过程和结果
- 热重载时如果新格式解析失败，保留旧权限配置

---

## Self-Review

- [x] No TBD/TODO placeholders
- [x] Architecture matches feature descriptions (PermissionsGroup map structure, Checker lookup logic)
- [x] Scope is focused (per-database permissions only, no other features)
- [x] Requirements are unambiguous (specific struct changes, specific migration behavior, specific error messages)
