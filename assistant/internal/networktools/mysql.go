package networktools

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"

	"assistant/internal/publicnet"

	mysql "github.com/go-sql-driver/mysql"
)

func queryMySQL(ctx context.Context, guard *publicnet.Guard, input databaseQueryInput) (databaseQueryResult, error) {
	connection := input.Connection
	config := mysql.NewConfig()
	config.User = connection.Username
	config.Passwd = connection.Password
	config.Net = "tcp"
	config.Addr = databaseAddress(connection)
	config.DBName = connection.Database
	config.DialFunc = guard.DialContext
	config.MultiStatements = false
	config.ParseTime = true
	config.Timeout = databaseQueryTimeout
	config.ReadTimeout = databaseQueryTimeout
	config.WriteTimeout = databaseQueryTimeout
	if connection.TLSMode == "verify-full" {
		config.TLS = &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: connection.Host,
		}
	}

	connector, err := mysql.NewConnector(config)
	if err != nil {
		return databaseQueryResult{}, fmt.Errorf("configure MySQL connection: %w", err)
	}
	database := sql.OpenDB(connector)
	database.SetMaxIdleConns(0)
	database.SetMaxOpenConns(1)
	defer database.Close()

	conn, err := database.Conn(ctx)
	if err != nil {
		return databaseQueryResult{}, fmt.Errorf("connect MySQL database: %w", err)
	}
	defer conn.Close()
	if err := conn.PingContext(ctx); err != nil {
		return databaseQueryResult{}, fmt.Errorf("ping MySQL database: %w", err)
	}
	transaction, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return databaseQueryResult{}, fmt.Errorf("begin read-only MySQL transaction: %w", err)
	}
	defer transaction.Rollback()

	rows, err := transaction.QueryContext(ctx, input.Query)
	if err != nil {
		return databaseQueryResult{}, fmt.Errorf("execute MySQL query: %w", err)
	}
	result, err := collectDatabaseRows(rows)
	if err != nil {
		return databaseQueryResult{}, fmt.Errorf("read MySQL query result: %w", err)
	}
	return result, nil
}
