package security

const MaxSQLLength = 64 * 1024 // 64KB

// SanitizeResultSize 检查结果集是否过大
func SanitizeResultSize(data []byte, maxBytes int) ([]byte, bool) {
	if len(data) <= maxBytes {
		return data, true
	}
	return data[:maxBytes], false
}

// FormatResultLimitError 生成结果集过大提示
func FormatResultLimitError(maxMB int) string {
	return "result set truncated: exceeded 10MB limit, add LIMIT to your query"
}
