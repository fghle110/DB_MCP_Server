package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// PostgresDriver PostgreSQL 数据库驱动
type PostgresDriver struct {
	db  *sql.DB
	tx  *sql.Tx
}

// NewPostgresDriver 创建 PostgreSQL 驱动实例
func NewPostgresDriver() *PostgresDriver {
	return &PostgresDriver{}
}

// Connect 连接 PostgreSQL
func (d *PostgresDriver) Connect(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		return err
	}
	d.db = db
	return nil
}

// Query 执行查询
func (d *PostgresDriver) Query(ctx context.Context, sqlStr string) (*QueryResult, error) {
	rows, err := d.db.QueryContext(ctx, sqlStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	result := &QueryResult{Columns: columns}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}
		result.Rows = append(result.Rows, convertBytes(values))
	}
	return result, rows.Err()
}

// Exec 执行写入（使用事务，失败自动回滚）
func (d *PostgresDriver) Exec(ctx context.Context, sqlStr string) (int64, error) {
	// 如果已有事务，直接使用
	if d.tx != nil {
		return d.execInTx(ctx, sqlStr)
	}
	// 否则使用单语句事务
	return d.execSingle(ctx, sqlStr)
}

func (d *PostgresDriver) execInTx(ctx context.Context, sqlStr string) (int64, error) {
	res, err := d.tx.ExecContext(ctx, sqlStr)
	if err != nil {
		_ = d.tx.Rollback()
		d.tx = nil
		return 0, fmt.Errorf("exec failed: %w (rolled back)", err)
	}
	affected, _ := res.RowsAffected()
	return affected, nil
}

func (d *PostgresDriver) execSingle(ctx context.Context, sqlStr string) (int64, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	res, execErr := tx.ExecContext(ctx, sqlStr)
	if execErr != nil {
		_ = tx.Rollback()
		return 0, execErr
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	affected, _ := res.RowsAffected()
	return affected, nil
}

// ListDatabases 列出数据库
func (d *PostgresDriver) ListDatabases(ctx context.Context) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT datname FROM pg_database WHERE datistemplate = false")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var databases []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		databases = append(databases, name)
	}
	return databases, nil
}

// ListTables 列出表
func (d *PostgresDriver) ListTables(ctx context.Context, database string) ([]string, error) {
	rows, err := d.db.QueryContext(ctx,
		"SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname = 'public'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, nil
}

// DescribeTable 查看表结构
func (d *PostgresDriver) DescribeTable(ctx context.Context, database, table string) ([]Column, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT column_name, data_type, is_nullable,
		        COALESCE(
		          (SELECT 'PRI' FROM information_schema.table_constraints tc
		           JOIN information_schema.key_column_usage kcu
		           ON tc.constraint_name = kcu.constraint_name
		           WHERE tc.constraint_type = 'PRIMARY KEY'
		           AND kcu.table_name = $1 AND kcu.column_name = c.column_name),
		        '') as key
		 FROM information_schema.columns c
		 WHERE table_name = $1
		 ORDER BY ordinal_position`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []Column
	for rows.Next() {
		var c Column
		if err := rows.Scan(&c.Name, &c.Type, &c.Nullable, &c.Key); err != nil {
			return nil, err
		}
		columns = append(columns, c)
	}
	return columns, nil
}

// Close 关闭连接
func (d *PostgresDriver) Close() error {
	if d.tx != nil {
		_ = d.tx.Rollback()
		d.tx = nil
	}
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// BeginTx 开始事务
func (d *PostgresDriver) BeginTx(ctx context.Context) error {
	if d.tx != nil {
		return fmt.Errorf("transaction already in progress")
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	d.tx = tx
	return nil
}

// Commit 提交事务
func (d *PostgresDriver) Commit() error {
	if d.tx == nil {
		return fmt.Errorf("no transaction in progress")
	}
	tx := d.tx
	d.tx = nil
	return tx.Commit()
}

// Rollback 回滚事务
func (d *PostgresDriver) Rollback() error {
	if d.tx == nil {
		return fmt.Errorf("no transaction in progress")
	}
	err := d.tx.Rollback()
	d.tx = nil
	return err
}
