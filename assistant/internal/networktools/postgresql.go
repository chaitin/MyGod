package networktools

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"

	"assistant/internal/publicnet"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func queryPostgreSQL(ctx context.Context, guard *publicnet.Guard, input databaseQueryInput) (databaseQueryResult, error) {
	connection := input.Connection
	connectionURL := &url.URL{
		Scheme: "postgresql",
		Host:   net.JoinHostPort(connection.Host, fmt.Sprintf("%d", connection.Port)),
		Path:   "/" + connection.Database,
		User:   url.UserPassword(connection.Username, connection.Password),
	}
	query := connectionURL.Query()
	if connection.TLSMode == "disable" {
		query.Set("sslmode", "disable")
	} else {
		query.Set("sslmode", "verify-full")
	}
	connectionURL.RawQuery = query.Encode()

	config, err := pgx.ParseConfig(connectionURL.String())
	if err != nil {
		return databaseQueryResult{}, fmt.Errorf("configure PostgreSQL connection: %w", err)
	}
	config.DialFunc = guard.DialContext
	config.DefaultQueryExecMode = pgx.QueryExecModeExec
	database := stdlib.OpenDB(*config)
	database.SetMaxIdleConns(0)
	database.SetMaxOpenConns(1)
	defer database.Close()

	conn, err := database.Conn(ctx)
	if err != nil {
		return databaseQueryResult{}, fmt.Errorf("connect PostgreSQL database: %w", err)
	}
	defer conn.Close()
	if err := conn.PingContext(ctx); err != nil {
		return databaseQueryResult{}, fmt.Errorf("ping PostgreSQL database: %w", err)
	}
	transaction, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return databaseQueryResult{}, fmt.Errorf("begin read-only PostgreSQL transaction: %w", err)
	}
	defer transaction.Rollback()
	if _, err := transaction.ExecContext(ctx, "SET LOCAL statement_timeout = '10s'"); err != nil {
		return databaseQueryResult{}, fmt.Errorf("set PostgreSQL query timeout: %w", err)
	}

	rows, err := transaction.QueryContext(ctx, input.Query)
	if err != nil {
		return databaseQueryResult{}, fmt.Errorf("execute PostgreSQL query: %w", err)
	}
	result, err := collectDatabaseRows(rows)
	if err != nil {
		return databaseQueryResult{}, fmt.Errorf("read PostgreSQL query result: %w", err)
	}
	return result, nil
}
