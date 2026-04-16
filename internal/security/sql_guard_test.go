package security

import (
	"testing"
)

func newTestGuard() *SQLGuard {
	return NewSQLGuard(MaxSQLLength)
}

func TestCheckSQL_ValidSelect(t *testing.T) {
	guard := newTestGuard()
	err := guard.CheckSQL("SELECT * FROM users WHERE id = 1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCheckSQL_MultiStatement(t *testing.T) {
	guard := newTestGuard()
	err := guard.CheckSQL("SELECT * FROM users; DROP TABLE users")
	if err == nil {
		t.Error("expected error for multiple statements")
	}
}

func TestCheckSQL_MultiStatement_AllowedTrailingSemicolon(t *testing.T) {
	guard := newTestGuard()
	err := guard.CheckSQL("SELECT * FROM users;")
	if err != nil {
		t.Errorf("trailing semicolon should be allowed: %v", err)
	}
}

func TestCheckSQL_DangerKeyword_GRANT(t *testing.T) {
	guard := newTestGuard()
	err := guard.CheckSQL("GRANT ALL ON *.* TO 'user'")
	if err == nil {
		t.Error("expected error for GRANT")
	}
}

func TestCheckSQL_TooLong(t *testing.T) {
	guard := NewSQLGuard(10)
	err := guard.CheckSQL("SELECT 12345678901234567890")
	if err == nil {
		t.Error("expected error for too long SQL")
	}
}

func TestCheckSQL_ControlCharacter(t *testing.T) {
	guard := newTestGuard()
	err := guard.CheckSQL("SELECT\x00 * FROM users")
	if err == nil {
		t.Error("expected error for control character")
	}
}

func TestCheckSQL_NonUTF8(t *testing.T) {
	guard := newTestGuard()
	invalid := "SELECT \xff\xfe * FROM users"
	err := guard.CheckSQL(invalid)
	if err == nil {
		t.Error("expected error for non-UTF-8")
	}
}

func TestExtractTableName_Select(t *testing.T) {
	tests := []struct {
		sql  string
		want string
	}{
		{"SELECT * FROM users WHERE id = 1", "users"},
		{"SELECT name FROM orders WHERE id = 1", "orders"},
		{"INSERT INTO users (name) VALUES ('test')", "users"},
		{"UPDATE users SET name = 'test'", "users"},
		{"DELETE FROM users WHERE id = 1", "users"},
		{"CREATE TABLE test_table (id INT)", "test_table"},
		{"DROP TABLE test_table", "test_table"},
		{"CREATE TABLE IF NOT EXISTS test_table (id INT)", "test_table"},
	}

	for _, tt := range tests {
		got := ExtractTableName(tt.sql)
		if got != tt.want {
			t.Errorf("ExtractTableName(%q) = %q, want %q", tt.sql, got, tt.want)
		}
	}
}

func TestExtractActionType(t *testing.T) {
	tests := []struct {
		sql  string
		want string
	}{
		{"SELECT * FROM users", "SELECT"},
		{"  SELECT * FROM users", "SELECT"},
		{"-- comment\nSELECT * FROM users", "SELECT"},
		{"INSERT INTO users VALUES (1)", "INSERT"},
		{"UPDATE users SET x = 1", "UPDATE"},
		{"DELETE FROM users", "DELETE"},
		{"CREATE TABLE test (id INT)", "CREATE"},
		{"DROP TABLE test", "DROP"},
		{"DESCRIBE users", "DESCRIBE"},
	}

	for _, tt := range tests {
		got := ExtractActionType(tt.sql)
		if got != tt.want {
			t.Errorf("ExtractActionType(%q) = %q, want %q", tt.sql, got, tt.want)
		}
	}
}

func TestHasMultipleStatements_InString(t *testing.T) {
	guard := newTestGuard()
	err := guard.CheckSQL("SELECT * FROM users WHERE name = 'test;value'")
	if err != nil {
		t.Errorf("semicolon in string should be allowed: %v", err)
	}
}
