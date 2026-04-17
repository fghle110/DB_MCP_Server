package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "dm"
)

// DmDriver 达梦数据库驱动
type DmDriver struct {
	db *sql.DB
	tx *sql.Tx
}

// NewDmDriver 创建达梦驱动实例
func NewDmDriver() *DmDriver {
	return &DmDriver{}
}

// Connect 连接达梦
func (d *DmDriver) Connect(dsn string) error {
	db, err := sql.Open("dm", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping dm: %w", err)
	}
	d.db = db
	return nil
}

// Query 执行查询
func (d *DmDriver) Query(ctx context.Context, sqlStr string) (*QueryResult, error) {
	if d.db == nil {
		return nil, fmt.Errorf("not connected to database")
	}
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
func (d *DmDriver) Exec(ctx context.Context, sqlStr string) (int64, error) {
	if d.db == nil {
		return 0, fmt.Errorf("not connected to database")
	}
	if d.tx != nil {
		return d.execInTx(ctx, sqlStr)
	}
	return d.execSingle(ctx, sqlStr)
}

func (d *DmDriver) execInTx(ctx context.Context, sqlStr string) (int64, error) {
	res, err := d.tx.ExecContext(ctx, sqlStr)
	if err != nil {
		_ = d.tx.Rollback()
		d.tx = nil
		return 0, fmt.Errorf("exec failed: %w (rolled back)", err)
	}
	affected, _ := res.RowsAffected()
	return affected, nil
}

func (d *DmDriver) execSingle(ctx context.Context, sqlStr string) (int64, error) {
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
func (d *DmDriver) ListDatabases(ctx context.Context) ([]string, error) {
	if d.db == nil {
		return nil, fmt.Errorf("not connected to database")
	}
	rows, err := d.db.QueryContext(ctx, "SELECT NAME FROM V$DATABASE")
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
func (d *DmDriver) ListTables(ctx context.Context, database string) ([]string, error) {
	if d.db == nil {
		return nil, fmt.Errorf("not connected to database")
	}
	rows, err := d.db.QueryContext(ctx, "SELECT TABLE_NAME FROM USER_TABLES")
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
func (d *DmDriver) DescribeTable(ctx context.Context, database, table string) ([]Column, error) {
	if d.db == nil {
		return nil, fmt.Errorf("not connected to database")
	}
	rows, err := d.db.QueryContext(ctx,
		"SELECT COLUMN_NAME, DATA_TYPE, NULLABLE FROM USER_TAB_COLUMNS WHERE TABLE_NAME = ? ORDER BY COLUMN_ID", table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []Column
	for rows.Next() {
		var c Column
		if err := rows.Scan(&c.Name, &c.Type, &c.Nullable); err != nil {
			return nil, err
		}
		columns = append(columns, c)
	}
	return columns, nil
}

// Close 关闭连接
func (d *DmDriver) Close() error {
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
func (d *DmDriver) BeginTx(ctx context.Context) error {
	if d.db == nil {
		return fmt.Errorf("not connected to database")
	}
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
func (d *DmDriver) Commit() error {
	if d.tx == nil {
		return fmt.Errorf("no transaction in progress")
	}
	tx := d.tx
	d.tx = nil
	return tx.Commit()
}

// Rollback 回滚事务
func (d *DmDriver) Rollback() error {
	if d.tx == nil {
		return fmt.Errorf("no transaction in progress")
	}
	err := d.tx.Rollback()
	d.tx = nil
	return err
}
