package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteDriver SQLite 数据库驱动
type SQLiteDriver struct {
	db *sql.DB
	tx *sql.Tx
}

// NewSQLiteDriver 创建 SQLite 驱动实例
func NewSQLiteDriver() *SQLiteDriver {
	return &SQLiteDriver{}
}

// Connect 连接 SQLite
func (d *SQLiteDriver) Connect(dsn string) error {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		return err
	}
	d.db = db
	return nil
}

// Query 执行查询
func (d *SQLiteDriver) Query(ctx context.Context, sqlStr string) (*QueryResult, error) {
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
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}
		result.Rows = append(result.Rows, values)
	}
	return result, rows.Err()
}

// Exec 执行写入（使用事务，失败自动回滚）
func (d *SQLiteDriver) Exec(ctx context.Context, sqlStr string) (int64, error) {
	if d.tx != nil {
		return d.execInTx(ctx, sqlStr)
	}
	return d.execSingle(ctx, sqlStr)
}

func (d *SQLiteDriver) execInTx(ctx context.Context, sqlStr string) (int64, error) {
	res, err := d.tx.ExecContext(ctx, sqlStr)
	if err != nil {
		_ = d.tx.Rollback()
		d.tx = nil
		return 0, fmt.Errorf("exec failed: %w (rolled back)", err)
	}
	affected, _ := res.RowsAffected()
	return affected, nil
}

func (d *SQLiteDriver) execSingle(ctx context.Context, sqlStr string) (int64, error) {
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

// ListDatabases 列出数据库(SQLite 是文件数据库,返回文件名)
func (d *SQLiteDriver) ListDatabases(ctx context.Context) ([]string, error) {
	return []string{"main"}, nil
}

// ListTables 列出表
func (d *SQLiteDriver) ListTables(ctx context.Context, database string) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
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
func (d *SQLiteDriver) DescribeTable(ctx context.Context, database, table string) ([]Column, error) {
	rows, err := d.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []Column
	for rows.Next() {
		var cid int
		var notnull int
		var pk int
		var dfltValue any
		var c Column
		if err := rows.Scan(&cid, &c.Name, &c.Type, &notnull, &dfltValue, &pk); err != nil {
			return nil, err
		}
		if notnull == 0 {
			c.Nullable = "YES"
		} else {
			c.Nullable = "NO"
		}
		if pk == 1 {
			c.Key = "PRI"
		}
		columns = append(columns, c)
	}
	return columns, nil
}

// Close 关闭连接
func (d *SQLiteDriver) Close() error {
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
func (d *SQLiteDriver) BeginTx(ctx context.Context) error {
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
func (d *SQLiteDriver) Commit() error {
	if d.tx == nil {
		return fmt.Errorf("no transaction in progress")
	}
	tx := d.tx
	d.tx = nil
	return tx.Commit()
}

// Rollback 回滚事务
func (d *SQLiteDriver) Rollback() error {
	if d.tx == nil {
		return fmt.Errorf("no transaction in progress")
	}
	err := d.tx.Rollback()
	d.tx = nil
	return err
}
