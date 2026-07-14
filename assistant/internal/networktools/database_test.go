package networktools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"assistant/internal/publicnet"
)

func TestDatabaseQueryToolNormalizesInputAndUsesFixedTimeout(t *testing.T) {
	var captured databaseQueryInput
	source := &Source{
		guard: publicnet.NewGuard(),
		mysql: func(ctx context.Context, _ *publicnet.Guard, input databaseQueryInput) (databaseQueryResult, error) {
			captured = input
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("query context has no deadline")
			}
			remaining := time.Until(deadline)
			if remaining <= 0 || remaining > databaseQueryTimeout {
				t.Fatalf("query timeout = %v, want <= %v", remaining, databaseQueryTimeout)
			}
			return databaseQueryResult{
				Columns:  []string{"status", "count"},
				Rows:     [][]any{{"todo", int64(2)}},
				RowCount: 1,
			}, nil
		},
	}
	result, err := source.CallTool(context.Background(), mysqlQueryToolName, json.RawMessage(`{
		"connection": {
			"host": "8.8.8.8",
			"database": " analytics ",
			"username": " reader ",
			"password": "secret"
		},
		"query": " SELECT status, COUNT(*) AS count FROM tasks GROUP BY status "
	}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if captured.Connection.Port != 3306 || captured.Connection.TLSMode != "verify-full" || captured.Connection.Database != "analytics" || captured.Connection.Username != "reader" {
		t.Fatalf("captured connection = %#v", captured.Connection)
	}
	if captured.Query != "SELECT status, COUNT(*) AS count FROM tasks GROUP BY status" {
		t.Fatalf("captured query = %q", captured.Query)
	}
	var response databaseQueryResult
	if err := json.Unmarshal([]byte(result.Content), &response); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if response.RowCount != 1 || len(response.Rows) != 1 || response.Truncated {
		t.Fatalf("response = %#v", response)
	}
}

func TestDatabaseQueryToolRejectsPrivateHostBeforeConnecting(t *testing.T) {
	called := false
	source := &Source{
		guard: publicnet.NewGuard(),
		postgres: func(context.Context, *publicnet.Guard, databaseQueryInput) (databaseQueryResult, error) {
			called = true
			return databaseQueryResult{}, nil
		},
	}
	_, err := source.CallTool(context.Background(), postgresQueryToolName, json.RawMessage(`{
		"connection": {
			"host": "192.168.1.10",
			"port": 5432,
			"database": "analytics",
			"username": "reader",
			"password": "secret"
		},
		"query": "SELECT 1"
	}`))
	if err == nil || !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("CallTool() error = %v, want non-public rejection", err)
	}
	if called {
		t.Fatal("database runner was called for private host")
	}
}

func TestValidateReadOnlyQuery(t *testing.T) {
	allowed := []string{
		"SELECT * FROM users",
		"/* context */ WITH totals AS (SELECT COUNT(*) AS count FROM tasks) SELECT * FROM totals",
		"SHOW TABLES",
		"DESCRIBE users",
		"EXPLAIN SELECT * FROM users",
		"SELECT ';' AS separator; -- trailing comment",
		"SELECT $$a;b$$ AS value",
	}
	for _, query := range allowed {
		if err := validateReadOnlyQuery(query); err != nil {
			t.Errorf("validateReadOnlyQuery(%q) error = %v", query, err)
		}
	}

	rejected := []string{
		"",
		"UPDATE users SET status = 'disabled'",
		"DELETE FROM users",
		"INSERT INTO users(id) VALUES (1)",
		"CREATE TABLE demo(id int)",
		"SET transaction_read_only = off",
		"SELECT 1; SELECT 2",
	}
	for _, query := range rejected {
		if err := validateReadOnlyQuery(query); err == nil {
			t.Errorf("validateReadOnlyQuery(%q) error = nil", query)
		}
	}
}

func TestDatabaseQuerySchemasDoNotExposeAuthorizationOrLimits(t *testing.T) {
	tools, err := NewSource().ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	for _, tool := range tools {
		if tool.Name != mysqlQueryToolName && tool.Name != postgresQueryToolName {
			continue
		}
		schema := tool.InputSchema.(map[string]any)
		properties := schema["properties"].(map[string]any)
		for _, forbidden := range []string{"authorization_ref", "params", "max_rows", "timeout_seconds"} {
			if _, ok := properties[forbidden]; ok {
				t.Fatalf("%s schema exposes %s: %#v", tool.Name, forbidden, schema)
			}
		}
	}
}
