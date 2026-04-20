//go:build !dm

package database

import (
	"context"
	"fmt"
)

// DmDriver 达梦数据库驱动（未编译，需要 -tags dm）
type DmDriver struct{}

// NewDmDriver 创建达梦驱动实例（需要 -tags dm 编译）
func NewDmDriver() (*DmDriver, error) {
	return nil, fmt.Errorf("DM driver not available: build with -tags dm to enable Dameng support")
}

func (d *DmDriver) Connect(dsn string) error                          { return fmt.Errorf("DM driver not available") }
func (d *DmDriver) Query(ctx context.Context, sqlStr string) (*QueryResult, error) {
	return nil, fmt.Errorf("DM driver not available")
}
func (d *DmDriver) Exec(ctx context.Context, sqlStr string) (int64, error) {
	return 0, fmt.Errorf("DM driver not available")
}
func (d *DmDriver) QueryWithParams(ctx context.Context, sqlStr string, params []any) (*QueryResult, error) {
	return nil, fmt.Errorf("DM driver not available")
}
func (d *DmDriver) ListDatabases(ctx context.Context) ([]string, error) {
	return nil, fmt.Errorf("DM driver not available")
}
func (d *DmDriver) ListTables(ctx context.Context, database string) ([]string, error) {
	return nil, fmt.Errorf("DM driver not available")
}
func (d *DmDriver) DescribeTable(ctx context.Context, database, table string) ([]Column, error) {
	return nil, fmt.Errorf("DM driver not available")
}
func (d *DmDriver) Close() error           { return nil }
func (d *DmDriver) BeginTx(ctx context.Context) error { return fmt.Errorf("DM driver not available") }
func (d *DmDriver) Commit() error          { return fmt.Errorf("DM driver not available") }
func (d *DmDriver) Rollback() error        { return fmt.Errorf("DM driver not available") }
