package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/microsoft/go-mssqldb"
)

// MSSQLDriver MSSQL Server 数据库驱动
type MSSQLDriver struct {
	db *sql.DB
	tx *sql.Tx
}

// NewMSSQLDriver 创建 MSSQL 驱动实例
func NewMSSQLDriver() *MSSQLDriver {
	return &MSSQLDriver{}
}

// Connect 连接 MSSQL Server
func (d *MSSQLDriver) Connect(dsn string) error {
	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping mssql: %w", err)
	}
	d.db = db
	return nil
}

// Query 执行查询
func (d *MSSQLDriver) Query(ctx context.Context, sqlStr string) (*QueryResult, error) {
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
		result.Rows = append(result.Rows, values)
	}
	return result, rows.Err()
}

// Exec 执行写入（使用事务，失败自动回滚）
func (d *MSSQLDriver) Exec(ctx context.Context, sqlStr string) (int64, error) {
	if d.tx != nil {
		return d.execInTx(ctx, sqlStr)
	}
	return d.execSingle(ctx, sqlStr)
}

func (d *MSSQLDriver) execInTx(ctx context.Context, sqlStr string) (int64, error) {
	res, err := d.tx.ExecContext(ctx, sqlStr)
	if err != nil {
		_ = d.tx.Rollback()
		d.tx = nil
		return 0, fmt.Errorf("exec failed: %w (rolled back)", err)
	}
	affected, _ := res.RowsAffected()
	return affected, nil
}

func (d *MSSQLDriver) execSingle(ctx context.Context, sqlStr string) (int64, error) {
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

// Close 关闭连接（TODO: Tasks 4-5）
func (d *MSSQLDriver) Close() error {
	if d.tx != nil {
		_ = d.tx.Rollback()
		d.tx = nil
	}
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// BeginTx 开始事务（TODO: Tasks 4-5）
func (d *MSSQLDriver) BeginTx(ctx context.Context) error {
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

// Commit 提交事务（TODO: Tasks 4-5）
func (d *MSSQLDriver) Commit() error {
	if d.tx == nil {
		return fmt.Errorf("no transaction in progress")
	}
	tx := d.tx
	d.tx = nil
	return tx.Commit()
}

// Rollback 回滚事务（TODO: Tasks 4-5）
func (d *MSSQLDriver) Rollback() error {
	if d.tx == nil {
		return fmt.Errorf("no transaction in progress")
	}
	err := d.tx.Rollback()
	d.tx = nil
	return err
}

// ListDatabases 列出数据库（TODO: Tasks 4-5）
func (d *MSSQLDriver) ListDatabases(ctx context.Context) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT name FROM sys.databases WHERE state = 0")
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

// ListTables 列出表（TODO: Tasks 4-5）
func (d *MSSQLDriver) ListTables(ctx context.Context, database string) ([]string, error) {
	query := fmt.Sprintf(`
		SELECT TABLE_NAME FROM %s.INFORMATION_SCHEMA.TABLES
		WHERE TABLE_TYPE = 'BASE TABLE'`, database)
	rows, err := d.db.QueryContext(ctx, query)
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

// DescribeTable 查看表结构（TODO: Tasks 4-5）
func (d *MSSQLDriver) DescribeTable(ctx context.Context, database, table string) ([]Column, error) {
	query := fmt.Sprintf(`
		SELECT COLUMN_NAME, DATA_TYPE,
			CASE WHEN IS_NULLABLE = 'YES' THEN 'YES' ELSE 'NO' END AS IS_NULLABLE,
			COALESCE((
				SELECT 'PRI' FROM %s.INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
				JOIN %s.INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu
					ON tc.CONSTRAINT_NAME = kcu.CONSTRAINT_NAME
				WHERE tc.TABLE_NAME = '%s'
					AND tc.CONSTRAINT_TYPE = 'PRIMARY KEY'
					AND kcu.COLUMN_NAME = c.COLUMN_NAME
			), '') AS KEY
		FROM %s.INFORMATION_SCHEMA.COLUMNS c
		WHERE TABLE_NAME = '%s'
		ORDER BY ORDINAL_POSITION`, database, database, table, database, table)
	rows, err := d.db.QueryContext(ctx, query)
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
