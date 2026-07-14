package networktools

import (
	"context"
	"encoding/json"
	"fmt"

	"assistant/internal/mcpclient"
	"assistant/internal/publicnet"
)

const (
	sourceName            = "builtin"
	httpClientToolName    = "http_client"
	mysqlQueryToolName    = "mysql_query"
	postgresQueryToolName = "postgresql_query"
)

type Source struct {
	guard      *publicnet.Guard
	httpClient httpDoer
	mysql      databaseQueryFunc
	postgres   databaseQueryFunc
}

func NewSource() *Source {
	guard := publicnet.NewGuard()
	return &Source{
		guard:      guard,
		httpClient: newHTTPClient(guard),
		mysql:      queryMySQL,
		postgres:   queryPostgreSQL,
	}
}

func (s *Source) SourceName() string {
	return sourceName
}

func (s *Source) ListTools(context.Context) ([]mcpclient.Tool, error) {
	return []mcpclient.Tool{
		{
			Name:        httpClientToolName,
			Description: "发送自定义 HTTP/HTTPS 请求。method、url、headers 和原始字符串 body 均由调用者提供；工具会正规化请求头并重新计算 Content-Length。只允许连接解析结果全部为公网 IP 的目标，禁止内网、环回、链路本地和保留地址。",
			InputSchema: httpClientInputSchema(),
		},
		{
			Name:        mysqlQueryToolName,
			Description: "连接公网 MySQL 数据库并执行一条只读查询。连接信息和完整 query 由调用者提供；工具固定 10 秒超时、最多返回 100 行，不支持任何写操作。",
			InputSchema: databaseQueryInputSchema(3306),
		},
		{
			Name:        postgresQueryToolName,
			Description: "连接公网 PostgreSQL 数据库并执行一条只读查询。连接信息和完整 query 由调用者提供；工具固定 10 秒超时、最多返回 100 行，不支持任何写操作。",
			InputSchema: databaseQueryInputSchema(5432),
		},
	}, nil
}

func (s *Source) CallTool(ctx context.Context, name string, input json.RawMessage) (mcpclient.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return mcpclient.ToolResult{}, err
	}
	if s == nil || s.guard == nil {
		return mcpclient.ToolResult{}, fmt.Errorf("network tools are not configured")
	}

	switch name {
	case httpClientToolName:
		return s.callHTTPClient(ctx, input)
	case mysqlQueryToolName:
		return s.callDatabaseQuery(ctx, input, 3306, s.mysql)
	case postgresQueryToolName:
		return s.callDatabaseQuery(ctx, input, 5432, s.postgres)
	default:
		return mcpclient.ToolResult{}, fmt.Errorf("unknown network tool %q", name)
	}
}

func jsonResult(value any) (mcpclient.ToolResult, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	return mcpclient.ToolResult{Content: string(encoded)}, nil
}
