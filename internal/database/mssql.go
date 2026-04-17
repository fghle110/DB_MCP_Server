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
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping mssql: %w", err)
	}
	d.db = db
	return nil
}

// Query 执行查询
func (d *MSSQLDriver) Query(ctx context.Context, sqlStr string) (*QueryResult, error) {
	var rows *sql.Rows
	var err error
	if d.tx != nil {
		rows, err = d.tx.QueryContext(ctx, sqlStr)
	} else {
		rows, err = d.db.QueryContext(ctx, sqlStr)
	}
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
		return 0, fmt.Errorf("exec in transaction failed: %w", err)
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

// Close 关闭连接
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

// BeginTx 开始事务
func (d *MSSQLDriver) BeginTx(ctx context.Context) error {
	if d.tx != nil {
		return fmt.Errorf("transaction already in progress")
	}
	tx, err := d.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	d.tx = tx
	return nil
}

// Commit 提交事务
func (d *MSSQLDriver) Commit() error {
	if d.tx == nil {
		return fmt.Errorf("no transaction in progress")
	}
	tx := d.tx
	d.tx = nil
	return tx.Commit()
}

// Rollback 回滚事务
func (d *MSSQLDriver) Rollback() error {
	if d.tx == nil {
		return fmt.Errorf("no transaction in progress")
	}
	err := d.tx.Rollback()
	d.tx = nil
	return err
}

// ListDatabases 列出数据库
func (d *MSSQLDriver) ListDatabases(ctx context.Context) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT name FROM sys.databases WHERE state_desc = 'ONLINE'")
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
func (d *MSSQLDriver) ListTables(ctx context.Context, database string) ([]string, error) {
	rows, err := d.db.QueryContext(ctx,
		"SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_TYPE = 'BASE TABLE'")
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
func (d *MSSQLDriver) DescribeTable(ctx context.Context, database, table string) ([]Column, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT c.COLUMN_NAME, c.DATA_TYPE, c.IS_NULLABLE,
		        CASE WHEN pk.COLUMN_NAME IS NOT NULL THEN 'PRI' ELSE '' END AS COLUMN_KEY
		 FROM INFORMATION_SCHEMA.COLUMNS c
		 LEFT JOIN (
		     SELECT ku.TABLE_CATALOG, ku.TABLE_NAME, ku.COLUMN_NAME
		     FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
		     JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE ku ON tc.CONSTRAINT_NAME = ku.CONSTRAINT_NAME
		     WHERE tc.CONSTRAINT_TYPE = 'PRIMARY KEY'
		 ) pk ON c.TABLE_CATALOG = pk.TABLE_CATALOG AND c.TABLE_NAME = pk.TABLE_NAME AND c.COLUMN_NAME = pk.COLUMN_NAME
		 WHERE c.TABLE_NAME = N'`+table+`'
		 ORDER BY c.ORDINAL_POSITION`)
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
