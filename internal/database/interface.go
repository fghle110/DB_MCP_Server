package database

import "context"

// Column 表字段信息
type Column struct {
	Name     string
	Type     string
	Nullable string
	Key      string
}

// QueryResult 查询结果
type QueryResult struct {
	Columns []string
	Rows    [][]interface{}
}

// DatabaseDriver 统一数据库驱动接口
type DatabaseDriver interface {
	Connect(dsn string) error
	Query(ctx context.Context, sql string) (*QueryResult, error)
	Exec(ctx context.Context, sql string) (int64, error)
	ListDatabases(ctx context.Context) ([]string, error)
	ListTables(ctx context.Context, database string) ([]string, error)
	DescribeTable(ctx context.Context, database, table string) ([]Column, error)
	Close() error
	// 事务支持
	BeginTx(ctx context.Context) error
	Commit() error
	Rollback() error
}
