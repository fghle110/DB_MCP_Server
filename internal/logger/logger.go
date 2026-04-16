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
	ID           int64
	Timestamp    time.Time
	Database     string
	Action       string
	SQL          string
	Result       string
	ErrorMessage string
	DurationMs   int64
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
		action TEXT,
		sql TEXT,
		result TEXT,
		error_message TEXT,
		duration_ms INTEGER
	);
	CREATE INDEX IF NOT EXISTS idx_audit_log_timestamp ON audit_log(timestamp);
	CREATE INDEX IF NOT EXISTS idx_audit_log_database ON audit_log(database);
	`
	if _, err := db.Exec(createTableSQL); err != nil {
		return nil, fmt.Errorf("create audit table: %w", err)
	}

	return &AuditLogger{db: db}, nil
}

// Log 记录操作日志（同时输出到 stderr 控制台）
func (al *AuditLogger) Log(entry LogEntry) error {
	if len(entry.SQL) > 4096 {
		entry.SQL = entry.SQL[:4096] + "..."
	}

	// 写入 SQLite
	_, err := al.db.Exec(
		`INSERT INTO audit_log (timestamp, database, action, sql, result, error_message, duration_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.Timestamp, entry.Database, entry.Action, entry.SQL,
		entry.Result, entry.ErrorMessage, entry.DurationMs,
	)

	// 控制台输出（stderr，不干扰 stdio MCP 协议）
	statusIcon := "✓"
	if entry.Result == "error" {
		statusIcon = "✗"
	}
	log.Printf("[audit] %s %s/%s | %s %s | %dms",
		statusIcon,
		entry.Database,
		entry.Action,
		entry.Result,
		truncate(entry.SQL, 120),
		entry.DurationMs,
	)
	if entry.ErrorMessage != "" {
		log.Printf("[audit]   error: %s", entry.ErrorMessage)
	}

	return err
}

// QueryLogs 查询操作日志
func (al *AuditLogger) QueryLogs(limit int, database string, actionType string) ([]LogEntry, error) {
	query := `SELECT id, timestamp, database, action, sql, result, error_message, duration_ms
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
		if err := rows.Scan(&e.ID, &ts, &e.Database, &e.Action, &e.SQL, &e.Result, &e.ErrorMessage, &e.DurationMs); err != nil {
			return nil, err
		}
		e.Timestamp, _ = time.Parse("2006-01-02 15:04:05", ts)
		entries = append(entries, e)
	}
	return entries, nil
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
