package networktools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"assistant/internal/mcpclient"
	"assistant/internal/publicnet"
)

const (
	databaseQueryTimeout  = 10 * time.Second
	maxDatabaseQueryBytes = 100 * 1024
	maxDatabaseResultRows = 100
)

type databaseConnectionInput struct {
	Database string `json:"database"`
	Host     string `json:"host"`
	Password string `json:"password"`
	Port     int    `json:"port"`
	TLSMode  string `json:"tls_mode"`
	Username string `json:"username"`
}

type databaseQueryInput struct {
	Connection databaseConnectionInput `json:"connection"`
	Query      string                  `json:"query"`
}

type databaseQueryResult struct {
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	RowCount  int      `json:"row_count"`
	Truncated bool     `json:"truncated"`
}

type databaseQueryFunc func(context.Context, *publicnet.Guard, databaseQueryInput) (databaseQueryResult, error)

func (s *Source) callDatabaseQuery(
	ctx context.Context,
	raw json.RawMessage,
	defaultPort int,
	run databaseQueryFunc,
) (mcpclient.ToolResult, error) {
	if run == nil {
		return mcpclient.ToolResult{}, fmt.Errorf("database query tool is not configured")
	}
	var input databaseQueryInput
	if err := decodeStrictJSON(raw, &input); err != nil {
		return mcpclient.ToolResult{}, fmt.Errorf("parse database query input: %w", err)
	}
	if err := normalizeDatabaseInput(&input, defaultPort); err != nil {
		return mcpclient.ToolResult{}, err
	}
	if err := s.guard.ValidateHost(ctx, input.Connection.Host); err != nil {
		return mcpclient.ToolResult{}, err
	}
	if err := validateReadOnlyQuery(input.Query); err != nil {
		return mcpclient.ToolResult{}, err
	}

	queryCtx, cancel := context.WithTimeout(ctx, databaseQueryTimeout)
	defer cancel()
	result, err := run(queryCtx, s.guard, input)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	return jsonResult(result)
}

func normalizeDatabaseInput(input *databaseQueryInput, defaultPort int) error {
	input.Connection.Host = strings.TrimSpace(input.Connection.Host)
	input.Connection.Database = strings.TrimSpace(input.Connection.Database)
	input.Connection.Username = strings.TrimSpace(input.Connection.Username)
	input.Connection.TLSMode = strings.ToLower(strings.TrimSpace(input.Connection.TLSMode))
	input.Query = strings.TrimSpace(input.Query)

	if input.Connection.Host == "" || strings.ContainsAny(input.Connection.Host, "/\\\x00\r\n") {
		return fmt.Errorf("database host is invalid")
	}
	if input.Connection.Port == 0 {
		input.Connection.Port = defaultPort
	}
	if input.Connection.Port < 1 || input.Connection.Port > 65535 {
		return fmt.Errorf("database port is invalid")
	}
	if input.Connection.Database == "" || strings.ContainsAny(input.Connection.Database, "\x00\r\n/") {
		return fmt.Errorf("database name is invalid")
	}
	if input.Connection.Username == "" || strings.ContainsAny(input.Connection.Username, "\x00\r\n") {
		return fmt.Errorf("database username is invalid")
	}
	if strings.ContainsAny(input.Connection.Password, "\x00\r\n") {
		return fmt.Errorf("database password is invalid")
	}
	if input.Connection.TLSMode == "" {
		input.Connection.TLSMode = "verify-full"
	}
	if input.Connection.TLSMode != "verify-full" && input.Connection.TLSMode != "disable" {
		return fmt.Errorf("database tls_mode must be verify-full or disable")
	}
	if input.Query == "" || len([]byte(input.Query)) > maxDatabaseQueryBytes || !utf8.ValidString(input.Query) || strings.ContainsRune(input.Query, '\x00') {
		return fmt.Errorf("database query is required, must be valid UTF-8, and must not exceed %d bytes", maxDatabaseQueryBytes)
	}
	return nil
}

func validateReadOnlyQuery(query string) error {
	if !isSingleSQLStatement(query) {
		return fmt.Errorf("database query must contain exactly one statement")
	}
	keyword := firstSQLKeyword(query)
	switch keyword {
	case "select", "with", "show", "describe", "desc", "explain":
		return nil
	default:
		return fmt.Errorf("database query must be read-only")
	}
}

func firstSQLKeyword(query string) string {
	index := skipSQLSpaceAndComments(query, 0)
	start := index
	for index < len(query) {
		character := rune(query[index])
		if character > unicode.MaxASCII || !unicode.IsLetter(character) {
			break
		}
		index++
	}
	return strings.ToLower(query[start:index])
}

func isSingleSQLStatement(query string) bool {
	semicolon := findSQLSemicolon(query)
	if semicolon < 0 {
		return firstSQLKeyword(query) != ""
	}
	return firstSQLKeyword(query) != "" && skipSQLSpaceAndComments(query, semicolon+1) == len(query)
}

func findSQLSemicolon(query string) int {
	for index := 0; index < len(query); {
		switch query[index] {
		case '\'', '"', '`':
			index = skipSQLQuoted(query, index, query[index])
		case '-':
			if index+1 < len(query) && query[index+1] == '-' {
				index = skipSQLLineComment(query, index+2)
			} else {
				index++
			}
		case '#':
			index = skipSQLLineComment(query, index+1)
		case '/':
			if index+1 < len(query) && query[index+1] == '*' {
				index = skipSQLBlockComment(query, index+2)
			} else {
				index++
			}
		case '$':
			if end := skipPostgreSQLDollarQuote(query, index); end > index {
				index = end
			} else {
				index++
			}
		case ';':
			return index
		default:
			index++
		}
	}
	return -1
}

func skipSQLSpaceAndComments(query string, start int) int {
	index := start
	for index < len(query) {
		if query[index] == ' ' || query[index] == '\t' || query[index] == '\r' || query[index] == '\n' {
			index++
			continue
		}
		if index+1 < len(query) && query[index] == '-' && query[index+1] == '-' {
			index = skipSQLLineComment(query, index+2)
			continue
		}
		if query[index] == '#' {
			index = skipSQLLineComment(query, index+1)
			continue
		}
		if index+1 < len(query) && query[index] == '/' && query[index+1] == '*' {
			index = skipSQLBlockComment(query, index+2)
			continue
		}
		break
	}
	return index
}

func skipSQLQuoted(query string, start int, quote byte) int {
	for index := start + 1; index < len(query); index++ {
		if query[index] == '\\' {
			index++
			continue
		}
		if query[index] != quote {
			continue
		}
		if index+1 < len(query) && query[index+1] == quote {
			index++
			continue
		}
		return index + 1
	}
	return len(query)
}

func skipSQLLineComment(query string, start int) int {
	if end := strings.IndexByte(query[start:], '\n'); end >= 0 {
		return start + end + 1
	}
	return len(query)
}

func skipSQLBlockComment(query string, start int) int {
	depth := 1
	for index := start; index < len(query)-1; index++ {
		if query[index] == '/' && query[index+1] == '*' {
			depth++
			index++
			continue
		}
		if query[index] == '*' && query[index+1] == '/' {
			depth--
			index++
			if depth == 0 {
				return index + 1
			}
		}
	}
	return len(query)
}

func skipPostgreSQLDollarQuote(query string, start int) int {
	endTag := start + 1
	for endTag < len(query) && (query[endTag] == '_' || query[endTag] >= 'a' && query[endTag] <= 'z' ||
		query[endTag] >= 'A' && query[endTag] <= 'Z' || query[endTag] >= '0' && query[endTag] <= '9') {
		endTag++
	}
	if endTag >= len(query) || query[endTag] != '$' {
		return start
	}
	tag := query[start : endTag+1]
	if closing := strings.Index(query[endTag+1:], tag); closing >= 0 {
		return endTag + 1 + closing + len(tag)
	}
	return len(query)
}

func collectDatabaseRows(rows *sql.Rows) (databaseQueryResult, error) {
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return databaseQueryResult{}, err
	}
	result := databaseQueryResult{
		Columns: columns,
		Rows:    make([][]any, 0, min(maxDatabaseResultRows, 16)),
	}
	for rows.Next() {
		values := make([]any, len(columns))
		destinations := make([]any, len(columns))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return databaseQueryResult{}, err
		}
		if len(result.Rows) == maxDatabaseResultRows {
			result.Truncated = true
			break
		}
		for index, value := range values {
			values[index] = normalizeDatabaseValue(value)
		}
		result.Rows = append(result.Rows, values)
	}
	if err := rows.Err(); err != nil {
		return databaseQueryResult{}, err
	}
	result.RowCount = len(result.Rows)
	return result, nil
}

func normalizeDatabaseValue(value any) any {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano)
	default:
		return value
	}
}

func databaseAddress(connection databaseConnectionInput) string {
	return net.JoinHostPort(connection.Host, fmt.Sprintf("%d", connection.Port))
}

func databaseQueryInputSchema(defaultPort int) map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"connection", "query"},
		"properties": map[string]any{
			"connection": map[string]any{
				"type":     "object",
				"required": []string{"host", "database", "username", "password"},
				"properties": map[string]any{
					"host":     map[string]any{"type": "string", "minLength": 1, "maxLength": 253},
					"port":     map[string]any{"type": "integer", "minimum": 1, "maximum": 65535, "default": defaultPort},
					"database": map[string]any{"type": "string", "minLength": 1, "maxLength": 255},
					"username": map[string]any{"type": "string", "minLength": 1, "maxLength": 255},
					"password": map[string]any{"type": "string", "maxLength": 4096},
					"tls_mode": map[string]any{"type": "string", "enum": []string{"verify-full", "disable"}, "default": "verify-full"},
				},
				"additionalProperties": false,
			},
			"query": map[string]any{"type": "string", "minLength": 1, "maxLength": maxDatabaseQueryBytes},
		},
		"additionalProperties": false,
	}
}
