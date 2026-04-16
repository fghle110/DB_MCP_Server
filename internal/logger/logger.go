package logger

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// AuditLogger 操作审计日志
type AuditLogger struct {
	db *sql.DB
}

// LogEntry 日志条目
type LogEntry struct {
	ID            int64
	Timestamp     time.Time
	Database      string
	DSN           string
	Action        string
	SQL           string
	Result        string
	ErrorMessage  string
	DurationMs    int64
	IsHighRisk    bool
}

// NewAuditLogger 创建日志记录器
func NewAuditLogger() (*AuditLogger, error) {
	dir := os.Getenv("HOME")
	if dir == "" {
		dir = os.Getenv("USERPROFILE")
	}
	if dir == "" {
		dir = "."
	}
	dbPath := filepath.Join(dir, ".dbmcp", "audit.db")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create audit log dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open audit log db: %w", err)
	}

	createTableSQL := `
	CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		database TEXT,
		dsn TEXT,
		action TEXT,
		sql TEXT,
		result TEXT,
		error_message TEXT,
		duration_ms INTEGER,
		is_high_risk INTEGER DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_audit_log_timestamp ON audit_log(timestamp);
	CREATE INDEX IF NOT EXISTS idx_audit_log_database ON audit_log(database);
	`
	if _, err := db.Exec(createTableSQL); err != nil {
		return nil, fmt.Errorf("create audit table: %w", err)
	}

	// 自动迁移：为旧库添加新列
	migrateSQL := []string{
		`ALTER TABLE audit_log ADD COLUMN dsn TEXT DEFAULT ''`,
		`ALTER TABLE audit_log ADD COLUMN is_high_risk INTEGER DEFAULT 0`,
	}
	for _, sql := range migrateSQL {
		_, _ = db.Exec(sql) // 列已存在时会失败，忽略
	}

	return &AuditLogger{db: db}, nil
}

// Log 记录操作日志（同时输出到 stderr 控制台）
func (al *AuditLogger) Log(entry LogEntry) error {
	if len(entry.SQL) > 4096 {
		entry.SQL = entry.SQL[:4096] + "..."
	}

	// 写入 SQLite（使用格式化后的时间字符串确保兼容）
	ts := entry.Timestamp.Format("2006-01-02 15:04:05")
	highRisk := 0
	if entry.IsHighRisk {
		highRisk = 1
	}
	_, err := al.db.Exec(
		`INSERT INTO audit_log (timestamp, database, dsn, action, sql, result, error_message, duration_ms, is_high_risk)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ts, entry.Database, entry.DSN, entry.Action, entry.SQL,
		entry.Result, entry.ErrorMessage, entry.DurationMs, highRisk,
	)

	// 控制台输出（stderr，不干扰 stdio MCP 协议）
	statusIcon := "✓"
	riskTag := ""
	if entry.Result == "error" {
		statusIcon = "✗"
	}
	if entry.IsHighRisk {
		riskTag = " [HIGH_RISK]"
	}
	log.Printf("[audit] %s %s/%s | %s %s | %dms%s",
		statusIcon,
		entry.Database,
		entry.Action,
		entry.Result,
		truncate(entry.SQL, 120),
		entry.DurationMs,
		riskTag,
	)
	if entry.ErrorMessage != "" {
		log.Printf("[audit]   error: %s", entry.ErrorMessage)
	}

	return err
}

// QueryLogs 查询操作日志
func (al *AuditLogger) QueryLogs(limit int, database string, actionType string) ([]LogEntry, error) {
	query := `SELECT id, timestamp, database, dsn, action, sql, result, error_message, duration_ms, is_high_risk
	          FROM audit_log WHERE 1=1`
	args := []interface{}{}

	if database != "" {
		query += " AND database = ?"
		args = append(args, database)
	}
	if actionType != "" {
		query += " AND action = ?"
		args = append(args, actionType)
	}

	query += " ORDER BY timestamp DESC LIMIT ?"
	args = append(args, limit)

	rows, err := al.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []LogEntry
	for rows.Next() {
		var e LogEntry
		var ts string
		var highRisk int
		if err := rows.Scan(&e.ID, &ts, &e.Database, &e.DSN, &e.Action, &e.SQL, &e.Result, &e.ErrorMessage, &e.DurationMs, &highRisk); err != nil {
			return nil, err
		}
		e.Timestamp = parseTimestamp(ts)
		e.IsHighRisk = highRisk == 1
		entries = append(entries, e)
	}
	return entries, nil
}

func parseTimestamp(s string) time.Time {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// Close 关闭日志数据库
func (al *AuditLogger) Close() error {
	if al.db != nil {
		return al.db.Close()
	}
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
