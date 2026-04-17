package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisDriver Redis 数据库驱动
type RedisDriver struct {
	client *redis.Client
	ctx    context.Context
}

// NewRedisDriver 创建 Redis 驱动实例
func NewRedisDriver() *RedisDriver {
	return &RedisDriver{ctx: context.Background()}
}

// Connect 连接 Redis
func (d *RedisDriver) Connect(dsn string) error {
	opts, err := redis.ParseURL(dsn)
	if err != nil {
		return fmt.Errorf("parse redis dsn: %w", err)
	}

	client := redis.NewClient(opts)
	d.client = client

	ctx, cancel := context.WithTimeout(d.ctx, 10*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis authentication failed: %w", err)
	}

	return nil
}

// Query Redis 不支持 SQL 查询
func (d *RedisDriver) Query(ctx context.Context, sql string) (*QueryResult, error) {
	return nil, fmt.Errorf("redis does not support SQL queries. Use redis_command tool")
}

// Exec 执行 Redis 命令
func (d *RedisDriver) Exec(ctx context.Context, cmd string) (int64, error) {
	if d.client == nil {
		return 0, fmt.Errorf("not connected to redis")
	}

	command, args := ParseRedisCommand(cmd)
	if command == "" {
		return 0, fmt.Errorf("empty command")
	}

	// 不支持的命令类型
	blockedCommands := []string{"EVAL", "EVALSHA", "SCRIPT", "MULTI", "EXEC", "DISCARD", "WATCH", "UNWATCH", "SUBSCRIBE", "PSUBSCRIBE", "MONITOR", "SYNC", "PSYNC", "DEBUG", "BGSAVE", "BGREWRITEAOF", "SAVE"}
	for _, b := range blockedCommands {
		if command == b {
			return 0, fmt.Errorf("command '%s' is not supported via redis_command tool", command)
		}
	}

	allArgs := append([]string{command}, args...)
	result := d.client.Do(ctx, interfaceSlice(allArgs)...)

	if err := result.Err(); err != nil {
		return 0, fmt.Errorf("redis command '%s' failed: %w", command, err)
	}

	// 返回固定值 1（Redis 命令不总是返回 affected rows）
	return 1, nil
}

// ListDatabases 列出数据库
func (d *RedisDriver) ListDatabases(ctx context.Context) ([]string, error) {
	return []string{"Redis (16 logical databases)"}, nil
}

// ListTables 列出 key（使用 SCAN）
func (d *RedisDriver) ListTables(ctx context.Context, database string) ([]string, error) {
	if d.client == nil {
		return nil, fmt.Errorf("not connected to redis")
	}

	var keys []string
	cursor := uint64(0)
	for {
		k, nextCursor, err := d.client.Scan(ctx, cursor, "*", 100).Result()
		if err != nil {
			return nil, err
		}
		keys = append(keys, k...)
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return keys, nil
}

// DescribeTable 查看 key 的信息
func (d *RedisDriver) DescribeTable(ctx context.Context, database, table string) ([]Column, error) {
	if d.client == nil {
		return nil, fmt.Errorf("not connected to redis")
	}

	keyType, err := d.client.Type(ctx, table).Result()
	if err != nil {
		return nil, err
	}

	ttl, err := d.client.TTL(ctx, table).Result()
	if err != nil {
		return nil, err
	}

	ttlStr := "-1"
	if ttl == -2 {
		ttlStr = "key does not exist"
	} else if ttl == -1 {
		ttlStr = "no expiry"
	} else {
		ttlStr = ttl.String()
	}

	columns := []Column{
		{Name: "key", Type: "string", Nullable: "NO", Key: ""},
		{Name: "type", Type: keyType, Nullable: "NO", Key: ""},
		{Name: "ttl", Type: ttlStr, Nullable: "YES", Key: ""},
	}

	return columns, nil
}

// Close 关闭连接
func (d *RedisDriver) Close() error {
	if d.client != nil {
		return d.client.Close()
	}
	return nil
}

// BeginTx Redis 不支持传统事务
func (d *RedisDriver) BeginTx(ctx context.Context) error {
	return fmt.Errorf("redis does not support SQL-style transactions. Use MULTI/EXEC directly via redis_command")
}

// Commit Redis 不支持传统事务
func (d *RedisDriver) Commit() error {
	return fmt.Errorf("redis does not support SQL-style transactions. Use MULTI/EXEC directly via redis_command")
}

// Rollback Redis 不支持传统事务
func (d *RedisDriver) Rollback() error {
	return fmt.Errorf("redis does not support SQL-style transactions. Use MULTI/EXEC directly via redis_command")
}

// ParseRedisCommand 解析 Redis 命令文本
func ParseRedisCommand(cmd string) (string, []string) {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return "", nil
	}
	return strings.ToUpper(fields[0]), fields[1:]
}

// interfaceSlice 转换 []string 为 []interface{}
func interfaceSlice(ss []string) []interface{} {
	result := make([]interface{}, len(ss))
	for i, s := range ss {
		result[i] = s
	}
	return result
}

// ClientInfo 获取 Redis INFO 信息
func (d *RedisDriver) ClientInfo(ctx context.Context, section string) (string, error) {
	if d.client == nil {
		return "", fmt.Errorf("not connected to redis")
	}
	if section != "" {
		return d.client.Info(ctx, section).Result()
	}
	return d.client.Info(ctx).Result()
}

// Scan 使用 SCAN 命令扫描 key
func (d *RedisDriver) Scan(ctx context.Context, cursor uint64, pattern string, count int64) ([]string, uint64, error) {
	if d.client == nil {
		return nil, 0, fmt.Errorf("not connected to redis")
	}
	return d.client.Scan(ctx, cursor, pattern, count).Result()
}
