package security

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// 危险 SQL 关键字黑名单(不区分大小写)
var dangerousPatternStrings = []string{
	// 文件读写
	`(?i)\bLOAD_FILE\b`,
	`(?i)\bINTO\s+OUTFILE\b`,
	`(?i)\bINTO\s+DUMPFILE\b`,
	`(?i)\bLOAD\s+DATA\b`,
	`(?i)\bBULK\s+INSERT\b`,
	// 系统命令
	`(?i)\bxp_cmdshell\b`,
	`(?i)\bsys_exec\b`,
	// 提权操作
	`(?i)\bGRANT\b`,
	`(?i)\bREVOKE\b`,
	`(?i)\bALTER\s+USER\b`,
	`(?i)\bCREATE\s+USER\b`,
	// 存储过程执行
	`(?i)\bEXEC\b`,
	`(?i)\bEXECUTE\b`,
	// 数据库特定危险操作
	`(?i)\bSHOW\s+GRANTS\b`,
}

// SQLGuard SQL 安全检查
type SQLGuard struct {
	maxSQLLength int
	compiledRE   []*regexp.Regexp
}

// NewSQLGuard 创建安全检查器
func NewSQLGuard(maxSQLLength int) *SQLGuard {
	sg := &SQLGuard{maxSQLLength: maxSQLLength}
	for _, pat := range dangerousPatternStrings {
		sg.compiledRE = append(sg.compiledRE, regexp.MustCompile(pat))
	}
	return sg
}

// CheckSQL 执行完整 SQL 安全检查
func (sg *SQLGuard) CheckSQL(sql string) error {
	// 长度检查
	if len(sql) > sg.maxSQLLength {
		return fmt.Errorf("sql too long: %d bytes (max %d)", len(sql), sg.maxSQLLength)
	}

	// 编码检查: 拒绝非 UTF-8
	if !utf8.ValidString(sql) {
		return fmt.Errorf("sql contains invalid UTF-8")
	}

	// 控制字符检查
	for _, r := range sql {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			return fmt.Errorf("sql contains control character: %d", r)
		}
	}

	// 多语句检测
	if hasMultipleStatements(sql) {
		return fmt.Errorf("multiple statements not allowed, split into separate calls")
	}

	// 危险关键字拦截
	for _, re := range sg.compiledRE {
		if re.MatchString(sql) {
			return fmt.Errorf("sql contains blocked keyword")
		}
	}

	return nil
}

// hasMultipleStatements 检测是否包含多条 SQL(以 ; 分隔)
func hasMultipleStatements(sql string) bool {
	cleaned := removeStringLiterals(sql)
	cleaned = removeComments(cleaned)
	count := strings.Count(cleaned, ";")
	trimmed := strings.TrimSpace(cleaned)
	if strings.HasSuffix(trimmed, ";") {
		count--
	}
	return count > 0
}

// removeStringLiterals 去除 SQL 字符串字面量中的内容
func removeStringLiterals(sql string) string {
	result := make([]byte, 0, len(sql))
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false

	for i := 0; i < len(sql); i++ {
		c := sql[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			result = append(result, c)
			continue
		}
		if c == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			continue
		}
		if c == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			continue
		}
		if !inSingleQuote && !inDoubleQuote {
			result = append(result, c)
		}
	}
	return string(result)
}

// removeComments 去除 SQL 注释
func removeComments(sql string) string {
	re1 := regexp.MustCompile(`(?s)/\*.*?\*/`)
	sql = re1.ReplaceAllString(sql, "")
	re2 := regexp.MustCompile(`--[^\n]*`)
	sql = re2.ReplaceAllString(sql, "")
	re3 := regexp.MustCompile(`#[^\n]*`)
	sql = re3.ReplaceAllString(sql, "")
	return sql
}

// ExtractTableName 从 SQL 中提取表名(用于权限校验)
func ExtractTableName(sql string) string {
	sql = strings.TrimSpace(sql)
	upper := strings.ToUpper(sql)

	// SELECT ... FROM table
	if idx := strings.Index(upper, "FROM"); idx >= 0 {
		rest := strings.TrimSpace(sql[idx+4:])
		return firstWord(rest)
	}
	// INSERT INTO table
	if idx := strings.Index(upper, "INTO"); idx >= 0 {
		rest := strings.TrimSpace(sql[idx+4:])
		return firstWord(rest)
	}
	// UPDATE table
	if strings.HasPrefix(upper, "UPDATE") {
		rest := strings.TrimSpace(sql[6:])
		return firstWord(rest)
	}
	// DELETE FROM table
	if idx := strings.Index(upper, "DELETE"); idx >= 0 {
		rest := strings.TrimSpace(sql[idx+6:])
		if strings.HasPrefix(strings.ToUpper(rest), "FROM") {
			rest = strings.TrimSpace(rest[4:])
			return firstWord(rest)
		}
	}
	// CREATE TABLE table
	if idx := strings.Index(upper, "TABLE"); idx >= 0 {
		rest := strings.TrimSpace(sql[idx+5:])
		upperRest := strings.ToUpper(rest)
		if strings.HasPrefix(upperRest, "IF") {
			if idx2 := strings.Index(upperRest, "EXISTS"); idx2 >= 0 {
				rest = strings.TrimSpace(rest[idx2+6:])
			}
		}
		return firstWord(rest)
	}
	// DROP TABLE table
	if strings.HasPrefix(upper, "DROP TABLE") {
		rest := strings.TrimSpace(sql[10:])
		return firstWord(rest)
	}
	return ""
}

// firstWord 取第一个词(去除括号前的部分)
func firstWord(s string) string {
	s = strings.TrimSpace(s)
	for i, c := range s {
		if c == '(' || c == ' ' || c == '.' {
			if c == '(' && i > 0 {
				return strings.TrimSpace(s[:i])
			}
			if c == ' ' && i > 0 {
				return strings.TrimSpace(s[:i])
			}
		}
	}
	return s
}

// ExtractActionType 提取 SQL 操作类型
func ExtractActionType(sql string) string {
	sql = strings.TrimSpace(sql)
	if len(sql) == 0 {
		return ""
	}
	cleaned := removeComments(sql)
	cleaned = strings.TrimSpace(cleaned)
	upper := strings.ToUpper(cleaned)
	for _, kw := range []string{"SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "DROP", "ALTER", "TRUNCATE", "DESCRIBE", "SHOW", "USE"} {
		if strings.HasPrefix(upper, kw) {
			return kw
		}
	}
	return "OTHER"
}
