package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"assistant/internal/config"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

const (
	AnthropicVersion = "2023-06-01"
	DefaultMaxTokens = 4096
)

var ErrTokenCountUnsupported = errors.New("anthropic token counting is unsupported")

type Model interface {
	CreateMessage(ctx context.Context, request Request) (Response, error)
}

type TokenCounter interface {
	CountTokens(ctx context.Context, request Request) (int, error)
}

type Request struct {
	System   string    `json:"system,omitempty"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
}

type Message struct {
	Role    string  `json:"role"`
	Content string  `json:"content,omitempty"`
	Blocks  []Block `json:"blocks,omitempty"`
}

type Tool struct {
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema,omitempty"`
	Name        string `json:"name"`
}

type Block struct {
	Type              string          `json:"type"`
	Text              string          `json:"text,omitempty"`
	Thinking          string          `json:"thinking,omitempty"`
	ThinkingSignature string          `json:"thinking_signature,omitempty"`
	ToolUseID         string          `json:"tool_use_id,omitempty"`
	ToolName          string          `json:"tool_name,omitempty"`
	ToolInput         json.RawMessage `json:"tool_input,omitempty"`
	IsError           bool            `json:"is_error,omitempty"`
}

type Response struct {
	Blocks       []Block `json:"blocks"`
	InputTokens  int     `json:"input_tokens,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`
	StopReason   string  `json:"stop_reason,omitempty"`
}

type AnthropicClient struct {
	BaseURL               string
	APIKey                string
	ModelName             string
	MaxTokens             int
	HTTPClient            *http.Client
	tokenCountingDisabled atomic.Bool
}

const (
	RoleAssistant = "assistant"
	RoleUser      = "user"

	BlockTypeText       = "text"
	BlockTypeThinking   = "thinking"
	BlockTypeToolUse    = "tool_use"
	BlockTypeToolResult = "tool_result"
)

func NewAnthropicClient(cfg config.LLMConfig) *AnthropicClient {
	return &AnthropicClient{
		BaseURL:   normalizeSDKBaseURL(cfg.BaseURL),
		APIKey:    strings.TrimSpace(cfg.APIKey),
		ModelName: strings.TrimSpace(cfg.ModelName),
		MaxTokens: DefaultMaxTokens,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *AnthropicClient) Generate(ctx context.Context, request Request) (string, error) {
	response, err := c.CreateMessage(ctx, request)
	if err != nil {
		return "", err
	}

	var parts []string
	for _, block := range response.Blocks {
		if block.Type == BlockTypeText && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("anthropic messages response contains no text content")
	}

	return strings.TrimSpace(strings.Join(parts, "\n")), nil
}

func (c *AnthropicClient) CreateMessage(ctx context.Context, request Request) (Response, error) {
	if strings.TrimSpace(c.BaseURL) == "" || strings.TrimSpace(c.APIKey) == "" || strings.TrimSpace(c.ModelName) == "" {
		return Response{}, fmt.Errorf("llm.base_url, llm.api_key, and llm.model_name are required")
	}
	if len(request.Messages) == 0 {
		return Response{}, fmt.Errorf("llm request messages are required")
	}

	params := anthropic.MessageNewParams{
		MaxTokens: int64(c.maxTokens()),
		Model:     anthropic.Model(c.ModelName),
		Messages:  make([]anthropic.MessageParam, 0, len(request.Messages)),
		Tools:     makeAnthropicTools(request.Tools),
	}
	if system := strings.TrimSpace(request.System); system != "" {
		params.System = []anthropic.TextBlockParam{{Text: system}}
	}
	for _, message := range request.Messages {
		paramMessage, err := makeAnthropicMessage(message)
		if err != nil {
			return Response{}, err
		}
		params.Messages = append(params.Messages, paramMessage)
	}

	client := c.sdkClient()
	response, err := client.Messages.New(ctx, params)
	if err != nil {
		return Response{}, err
	}

	return parseAnthropicResponse(response), nil
}

func (c *AnthropicClient) CountTokens(ctx context.Context, request Request) (int, error) {
	if c.tokenCountingDisabled.Load() {
		return 0, ErrTokenCountUnsupported
	}
	if strings.TrimSpace(c.BaseURL) == "" || strings.TrimSpace(c.APIKey) == "" || strings.TrimSpace(c.ModelName) == "" {
		return 0, fmt.Errorf("llm.base_url, llm.api_key, and llm.model_name are required")
	}
	if len(request.Messages) == 0 {
		return 0, fmt.Errorf("llm request messages are required")
	}

	params := anthropic.MessageCountTokensParams{
		Messages: make([]anthropic.MessageParam, 0, len(request.Messages)),
		Model:    anthropic.Model(c.ModelName),
		Tools:    makeAnthropicCountTokensTools(request.Tools),
	}
	if system := strings.TrimSpace(request.System); system != "" {
		params.System = anthropic.MessageCountTokensParamsSystemUnion{
			OfTextBlockArray: []anthropic.TextBlockParam{{Text: system}},
		}
	}
	for _, message := range request.Messages {
		paramMessage, err := makeAnthropicMessage(message)
		if err != nil {
			return 0, err
		}
		params.Messages = append(params.Messages, paramMessage)
	}

	client := c.sdkClient()
	count, err := client.Messages.CountTokens(ctx, params)
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusMethodNotAllowed || apiErr.StatusCode == http.StatusNotImplemented) {
			c.tokenCountingDisabled.Store(true)
			return 0, ErrTokenCountUnsupported
		}
		return 0, err
	}
	return int(count.InputTokens), nil
}

func makeAnthropicMessage(message Message) (anthropic.MessageParam, error) {
	blocks, err := makeAnthropicBlocks(messageBlocks(message))
	if err != nil {
		return anthropic.MessageParam{}, err
	}

	switch message.Role {
	case RoleAssistant:
		return anthropic.NewAssistantMessage(blocks...), nil
	default:
		return anthropic.NewUserMessage(blocks...), nil
	}
}

func messageBlocks(message Message) []Block {
	if len(message.Blocks) > 0 {
		return message.Blocks
	}
	if message.Content != "" {
		return []Block{{Type: BlockTypeText, Text: message.Content}}
	}

	return nil
}

func makeAnthropicBlocks(blocks []Block) ([]anthropic.ContentBlockParamUnion, error) {
	result := make([]anthropic.ContentBlockParamUnion, 0, len(blocks))
	for _, block := range blocks {
		paramBlock, err := makeAnthropicBlock(block)
		if err != nil {
			return nil, err
		}
		result = append(result, paramBlock)
	}

	return result, nil
}

func makeAnthropicBlock(block Block) (anthropic.ContentBlockParamUnion, error) {
	switch block.Type {
	case "", BlockTypeText:
		return anthropic.NewTextBlock(block.Text), nil
	case BlockTypeThinking:
		return anthropic.NewThinkingBlock(block.ThinkingSignature, block.Thinking), nil
	case BlockTypeToolUse:
		return anthropic.NewToolUseBlock(block.ToolUseID, toolInputValue(block.ToolInput), block.ToolName), nil
	case BlockTypeToolResult:
		return anthropic.NewToolResultBlock(block.ToolUseID, block.Text, block.IsError), nil
	default:
		return anthropic.ContentBlockParamUnion{}, fmt.Errorf("unsupported llm content block type %q", block.Type)
	}
}

func toolInputValue(input json.RawMessage) any {
	if len(input) == 0 {
		return map[string]any{}
	}

	return input
}

func makeAnthropicTools(tools []Tool) []anthropic.ToolUnionParam {
	if len(tools) == 0 {
		return nil
	}

	result := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		inputSchema := makeAnthropicInputSchema(tool.InputSchema)
		paramTool := anthropic.ToolUnionParamOfTool(inputSchema, tool.Name)
		if strings.TrimSpace(tool.Description) != "" {
			paramTool.OfTool.Description = param.NewOpt(tool.Description)
		}
		result = append(result, paramTool)
	}

	return result
}

func makeAnthropicCountTokensTools(tools []Tool) []anthropic.MessageCountTokensToolUnionParam {
	if len(tools) == 0 {
		return nil
	}

	result := make([]anthropic.MessageCountTokensToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		inputSchema := makeAnthropicInputSchema(tool.InputSchema)
		paramTool := anthropic.MessageCountTokensToolParamOfTool(inputSchema, tool.Name)
		if strings.TrimSpace(tool.Description) != "" {
			paramTool.OfTool.Description = param.NewOpt(tool.Description)
		}
		result = append(result, paramTool)
	}
	return result
}

func makeAnthropicInputSchema(schema any) anthropic.ToolInputSchemaParam {
	if schema == nil {
		return anthropic.ToolInputSchemaParam{Properties: map[string]any{}}
	}

	data, err := json.Marshal(schema)
	if err != nil {
		return anthropic.ToolInputSchemaParam{Properties: map[string]any{}}
	}
	var result anthropic.ToolInputSchemaParam
	if err := json.Unmarshal(data, &result); err != nil {
		return anthropic.ToolInputSchemaParam{Properties: map[string]any{}}
	}

	return result
}

func parseAnthropicResponse(response *anthropic.Message) Response {
	blocks := make([]Block, 0, len(response.Content))
	for _, block := range response.Content {
		switch block.Type {
		case BlockTypeText:
			blocks = append(blocks, Block{
				Type: BlockTypeText,
				Text: block.Text,
			})
		case BlockTypeThinking:
			blocks = append(blocks, Block{
				Type:              BlockTypeThinking,
				Thinking:          block.Thinking,
				ThinkingSignature: block.Signature,
			})
		case BlockTypeToolUse:
			blocks = append(blocks, Block{
				Type:      BlockTypeToolUse,
				ToolUseID: block.ID,
				ToolName:  block.Name,
				ToolInput: cloneRawMessage(block.Input),
			})
		}
	}

	return Response{
		Blocks:       blocks,
		InputTokens:  int(response.Usage.InputTokens),
		OutputTokens: int(response.Usage.OutputTokens),
		StopReason:   string(response.StopReason),
	}
}

func cloneRawMessage(value json.RawMessage) json.RawMessage {
	if value == nil {
		return nil
	}

	cloned := make(json.RawMessage, len(value))
	copy(cloned, value)
	return cloned
}

func (c *AnthropicClient) sdkClient() anthropic.Client {
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}

	return anthropic.NewClient(
		option.WithAPIKey(c.APIKey),
		option.WithBaseURL(c.BaseURL),
		option.WithHTTPClient(httpClient),
		option.WithRequestTimeout(60*time.Second),
	)
}

func (c *AnthropicClient) maxTokens() int {
	if c.MaxTokens > 0 {
		return c.MaxTokens
	}

	return DefaultMaxTokens
}

func normalizeSDKBaseURL(value string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(value), "/")
	return strings.TrimSuffix(baseURL, "/v1")
}
