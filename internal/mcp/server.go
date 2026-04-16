package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dbmcp/dbmcp/internal/config"
	"github.com/dbmcp/dbmcp/internal/database"
	"github.com/dbmcp/dbmcp/internal/logger"
	"github.com/dbmcp/dbmcp/internal/permission"
	"github.com/dbmcp/dbmcp/internal/security"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// DBMCPServer 数据库 MCP Server
type DBMCPServer struct {
	srv      *server.MCPServer
	app      *config.AppState
	dm       *database.DriverManager
	perm     *permission.Checker
	guard    *security.SQLGuard
	auditLog *logger.AuditLogger
}

// New 创建并配置 MCP Server
func New(app *config.AppState, dm *database.DriverManager, perm *permission.Checker, guard *security.SQLGuard, auditLog *logger.AuditLogger) *DBMCPServer {
	d := &DBMCPServer{
		srv:      server.NewMCPServer("dbmcp", "1.0.0"),
		app:      app,
		dm:       dm,
		perm:     perm,
		guard:    guard,
		auditLog: auditLog,
	}
	d.registerTools()
	return d
}

// Server 返回底层 MCP Server
func (d *DBMCPServer) Server() *server.MCPServer {
	return d.srv
}

// registerTools 注册所有 Tool
func (d *DBMCPServer) registerTools() {
	d.srv.AddTool(
		mcp.NewTool("execute_query",
			mcp.WithDescription("Execute a SELECT SQL query"),
			mcp.WithString("sql", mcp.Required(), mcp.Description("SQL query")),
			mcp.WithString("database", mcp.Required(), mcp.Description("Database name")),
		),
		d.handleExecuteQuery,
	)

	d.srv.AddTool(
		mcp.NewTool("execute_update",
			mcp.WithDescription("Execute INSERT/UPDATE/DELETE/DDL SQL statement"),
			mcp.WithString("sql", mcp.Required(), mcp.Description("SQL statement")),
			mcp.WithString("database", mcp.Required(), mcp.Description("Database name")),
		),
		d.handleExecuteUpdate,
	)

	d.srv.AddTool(
		mcp.NewTool("execute_param_query",
			mcp.WithDescription("Execute a parameterized query (prevents SQL injection)"),
			mcp.WithString("sql", mcp.Required(), mcp.Description("SQL with ? placeholders")),
			mcp.WithString("database", mcp.Required(), mcp.Description("Database name")),
			mcp.WithArray("params", mcp.Description("Parameter values")),
		),
		d.handleExecuteParamQuery,
	)

	d.srv.AddTool(
		mcp.NewTool("list_databases",
			mcp.WithDescription("List all connected databases"),
		),
		d.handleListDatabases,
	)

	d.srv.AddTool(
		mcp.NewTool("list_tables",
			mcp.WithDescription("List tables in a database"),
			mcp.WithString("database", mcp.Required(), mcp.Description("Database name")),
		),
		d.handleListTables,
	)

	d.srv.AddTool(
		mcp.NewTool("describe_table",
			mcp.WithDescription("Show table structure"),
			mcp.WithString("database", mcp.Required(), mcp.Description("Database name")),
			mcp.WithString("table", mcp.Required(), mcp.Description("Table name")),
		),
		d.handleDescribeTable,
	)

	d.srv.AddTool(
		mcp.NewTool("query_logs",
			mcp.WithDescription("Query AI operation audit logs"),
			mcp.WithNumber("limit", mcp.Description("Max log entries (default 50)")),
			mcp.WithString("database", mcp.Description("Filter by database")),
			mcp.WithString("action_type", mcp.Description("Filter by action type")),
		),
		d.handleQueryLogs,
	)

	d.srv.AddTool(
		mcp.NewTool("config_status",
			mcp.WithDescription("Show current configuration status"),
		),
		d.handleConfigStatus,
	)

	d.srv.AddTool(
		mcp.NewTool("begin_tx",
			mcp.WithDescription("Start a database transaction"),
			mcp.WithString("database", mcp.Required(), mcp.Description("Database name")),
		),
		d.handleBeginTx,
	)

	d.srv.AddTool(
		mcp.NewTool("commit",
			mcp.WithDescription("Commit the current transaction"),
			mcp.WithString("database", mcp.Required(), mcp.Description("Database name")),
		),
		d.handleCommit,
	)

	d.srv.AddTool(
		mcp.NewTool("rollback",
			mcp.WithDescription("Rollback the current transaction"),
			mcp.WithString("database", mcp.Required(), mcp.Description("Database name")),
		),
		d.handleRollback,
	)
}

// getArgs 从 CallToolRequest 中提取参数 map
func getArgs(req mcp.CallToolRequest) map[string]any {
	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return args
}

// strArg 从参数 map 中获取字符串参数
func strArg(args map[string]any, name string) string {
	v, _ := args[name].(string)
	return v
}

// numArg 从参数 map 中获取数字参数
func numArg(args map[string]any, name string) float64 {
	v, _ := args[name].(float64)
	return v
}

func (d *DBMCPServer) handleExecuteQuery(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(req)
	sqlStr := strArg(args, "sql")
	dbName := strArg(args, "database")

	result, err := d.executeSQL(ctx, sqlStr, dbName, "SELECT")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return result, nil
}

func (d *DBMCPServer) handleExecuteUpdate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(req)
	sqlStr := strArg(args, "sql")
	dbName := strArg(args, "database")

	actionType := security.ExtractActionType(sqlStr)
	result, err := d.executeSQL(ctx, sqlStr, dbName, actionType)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return result, nil
}

func (d *DBMCPServer) handleExecuteParamQuery(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(req)
	sqlStr := strArg(args, "sql")
	dbName := strArg(args, "database")

	if err := d.guard.CheckSQL(sqlStr); err != nil {
		d.logAudit(dbName, security.ExtractActionType(sqlStr), sqlStr, "error", err.Error(), 0)
		return mcp.NewToolResultError(fmt.Sprintf("security check failed: %v", err)), nil
	}

	tableName := security.ExtractTableName(sqlStr)
	if err := d.perm.CheckSelect(dbName, tableName); err != nil {
		d.logAudit(dbName, security.ExtractActionType(sqlStr), sqlStr, "error", err.Error(), 0)
		return mcp.NewToolResultError(fmt.Sprintf("permission denied: %v", err)), nil
	}

	drv, err := d.dm.Get(dbName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	start := time.Now()
	rows, err := drv.Query(ctx, sqlStr)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		d.logAudit(dbName, "SELECT", sqlStr, "error", err.Error(), duration)
		return mcp.NewToolResultError(err.Error()), nil
	}

	d.logAudit(dbName, "SELECT", sqlStr, "success", "", duration)
	return queryResultToText(rows), nil
}

func (d *DBMCPServer) executeSQL(ctx context.Context, sqlStr, dbName, actionType string) (*mcp.CallToolResult, error) {
	if err := d.guard.CheckSQL(sqlStr); err != nil {
		d.logAudit(dbName, actionType, sqlStr, "error", "security_block: "+err.Error(), 0)
		return nil, fmt.Errorf("security check: %w", err)
	}

	tableName := security.ExtractTableName(sqlStr)
	if actionType == "SELECT" {
		if err := d.perm.CheckSelect(dbName, tableName); err != nil {
			d.logAudit(dbName, actionType, sqlStr, "error", err.Error(), 0)
			return nil, fmt.Errorf("permission: %w", err)
		}
	} else {
		if err := d.perm.CheckWrite(dbName, tableName, actionType); err != nil {
			d.logAudit(dbName, actionType, sqlStr, "error", err.Error(), 0)
			return nil, fmt.Errorf("permission: %w", err)
		}
	}

	drv, err := d.dm.Get(dbName)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var result *mcp.CallToolResult
	var execErr error

	if actionType == "SELECT" {
		var rows *database.QueryResult
		rows, execErr = drv.Query(ctx, sqlStr)
		if execErr == nil {
			result = queryResultToText(rows)
		}
	} else {
		var affected int64
		affected, execErr = drv.Exec(ctx, sqlStr)
		if execErr == nil {
			result = mcp.NewToolResultText(fmt.Sprintf("OK. Rows affected: %d", affected))
		}
	}

	duration := time.Since(start).Milliseconds()

	if execErr != nil {
		d.logAudit(dbName, actionType, sqlStr, "error", execErr.Error(), duration)
		return nil, execErr
	}
	d.logAudit(dbName, actionType, sqlStr, "success", "", duration)

	return result, nil
}

func (d *DBMCPServer) handleListDatabases(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	names := d.dm.List()
	if len(names) == 0 {
		return mcp.NewToolResultText("No databases configured."), nil
	}
	return mcp.NewToolResultText("Connected databases:\n- " + joinStrings(names, "\n- ")), nil
}

func (d *DBMCPServer) handleListTables(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(req)
	dbName := strArg(args, "database")
	drv, err := d.dm.Get(dbName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	tables, err := drv.ListTables(ctx, dbName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if len(tables) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No tables in database '%s'.", dbName)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Tables in '%s':\n- %s", dbName, joinStrings(tables, "\n- "))), nil
}

func (d *DBMCPServer) handleDescribeTable(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(req)
	dbName := strArg(args, "database")
	table := strArg(args, "table")
	drv, err := d.dm.Get(dbName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	columns, err := drv.DescribeTable(ctx, dbName, table)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(formatColumns(columns)), nil
}

func (d *DBMCPServer) handleQueryLogs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(req)
	limit := numArg(args, "limit")
	if limit == 0 {
		limit = 50
	}
	dbName := strArg(args, "database")
	actionType := strArg(args, "action_type")

	entries, err := d.auditLog.QueryLogs(int(limit), dbName, actionType)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if len(entries) == 0 {
		return mcp.NewToolResultText("No audit logs found."), nil
	}
	return mcp.NewToolResultText(formatLogEntries(entries)), nil
}

func (d *DBMCPServer) handleConfigStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cfg := d.app.Config()
	rc := d.app.ReloadCtx()
	dbs := d.dm.List()
	status := fmt.Sprintf("Configuration Status:\n"+
		"- Databases: %d connected (%s)\n"+
		"- Read-only: %v\n"+
		"- Allowed actions: %s\n"+
		"- Last reload: %s (success: %v)",
		len(dbs), joinStrings(dbs, ", "),
		cfg.Permissions.ReadOnly,
		joinStrings(cfg.Permissions.AllowedActions, ", "),
		rc.LastReload.Format("2006-01-02 15:04:05"),
		rc.ReloadSuccess,
	)
	return mcp.NewToolResultText(status), nil
}

func (d *DBMCPServer) handleBeginTx(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(req)
	dbName := strArg(args, "database")
	drv, err := d.dm.Get(dbName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := drv.BeginTx(ctx); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Transaction started on '%s'.", dbName)), nil
}

func (d *DBMCPServer) handleCommit(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(req)
	dbName := strArg(args, "database")
	drv, err := d.dm.Get(dbName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := drv.Commit(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Transaction committed on '%s'.", dbName)), nil
}

func (d *DBMCPServer) handleRollback(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(req)
	dbName := strArg(args, "database")
	drv, err := d.dm.Get(dbName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := drv.Rollback(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Transaction rolled back on '%s'.", dbName)), nil
}

func queryResultToText(result *database.QueryResult) *mcp.CallToolResult {
	if len(result.Rows) == 0 {
		return mcp.NewToolResultText("Query executed successfully. 0 rows returned.")
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data))
}

func formatColumns(columns []database.Column) string {
	result := "Columns:\n"
	for _, c := range columns {
		result += fmt.Sprintf("  %-30s %-20s Nullable: %-3s Key: %s\n", c.Name, c.Type, c.Nullable, c.Key)
	}
	return result
}

func formatLogEntries(entries []logger.LogEntry) string {
	result := "Audit Logs:\n"
	for _, e := range entries {
		riskTag := ""
		if e.IsHighRisk {
			riskTag = " [HIGH_RISK]"
		}
		result += fmt.Sprintf("[%s] %s/%s %s | %s | %dms | conn: %s%s\n",
			e.Timestamp.Format("2006-01-02 15:04:05"),
			e.Database, e.Action, e.Result,
			truncateString(e.SQL, 80),
			e.DurationMs,
			e.DSN,
			riskTag,
		)
	}
	return result
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func (d *DBMCPServer) logAudit(dbName, action, sql, result, errMsg string, durationMs int64) {
	cfg := d.app.Config()
	dsn := ""
	if dbCfg, ok := cfg.Databases[dbName]; ok {
		dsn = maskDSN(dbCfg.DSN)
		if dsn == "" && dbCfg.Host != "" {
			dsn = fmt.Sprintf("%s:%d", dbCfg.Host, dbCfg.Port)
		}
	}
	_ = d.auditLog.Log(logger.LogEntry{
		Timestamp:    time.Now(),
		Database:     dbName,
		DSN:          dsn,
		Action:       action,
		SQL:          sql,
		Result:       result,
		ErrorMessage: errMsg,
		DurationMs:   durationMs,
		IsHighRisk:   isHighRiskAction(action, sql),
	})
}

// maskDSN 脱敏 DSN 中的密码
func maskDSN(dsn string) string {
	if dsn == "" {
		return ""
	}
	// MySQL: user:pass@tcp(host:port)/db
	if idx := strings.Index(dsn, "@"); idx >= 0 {
		userPass := dsn[:idx]
		if colon := strings.Index(userPass, ":"); colon >= 0 {
			return dsn[:colon] + ":***" + dsn[idx:]
		}
	}
	// PostgreSQL: postgres://user:pass@host:port/db
	if strings.HasPrefix(dsn, "postgres://") {
		if atIdx := strings.Index(dsn, "@"); atIdx >= 0 {
			userPass := dsn[len("postgres://"):atIdx]
			if colon := strings.Index(userPass, ":"); colon >= 0 {
				return dsn[:len("postgres://")+colon] + "***" + dsn[atIdx:]
			}
		}
	}
	return dsn
}

// isHighRiskAction 判断是否为高危操作
func isHighRiskAction(action string, sql string) bool {
	highRiskActions := []string{"DROP", "TRUNCATE", "ALTER"}
	for _, a := range highRiskActions {
		if strings.HasPrefix(action, a) {
			return true
		}
	}
	// 高危 SQL 模式
	highRiskPatterns := []string{
		`(?i)\bDROP\s+(TABLE|DATABASE)\b`,
		`(?i)\bTRUNCATE\b`,
		`(?i)\bALTER\s+TABLE\b.*\bDROP\b`,
		`(?i)\bDELETE\b.*\bWHERE\b.*\b1\s*=\s*1`,
		`(?i)\bUPDATE\b.*\bWHERE\b.*\b1\s*=\s*1`,
	}
	for _, pat := range highRiskPatterns {
		if matched, _ := regexp.MatchString(pat, sql); matched {
			return true
		}
	}
	return false
}
