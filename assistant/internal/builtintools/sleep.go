package builtintools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"assistant/internal/mcpclient"
)

const (
	sourceName       = "builtin"
	sleepToolName    = "sleep"
	minSleepSeconds  = 1
	maxSleepSeconds  = 60
	defaultSleepUnit = time.Second
)

type sleepFunc func(context.Context, time.Duration) error

type Source struct {
	sleep sleepFunc
}

type sleepInput struct {
	Seconds float64 `json:"seconds"`
}

func NewSource() *Source {
	return newSourceWithSleeper(realSleep)
}

func newSourceWithSleeper(sleep sleepFunc) *Source {
	if sleep == nil {
		sleep = realSleep
	}

	return &Source{sleep: sleep}
}

func (s *Source) SourceName() string {
	return sourceName
}

func (s *Source) ListTools(ctx context.Context) ([]mcpclient.Tool, error) {
	return []mcpclient.Tool{
		{
			Name:        sleepToolName,
			Description: "等待指定秒数，常用于等待异步任务完成或协调多个工具调用。seconds 小于 1 时按 1 秒处理，大于 60 时按 60 秒处理。",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"seconds"},
				"properties": map[string]any{
					"seconds": map[string]any{
						"type":        "number",
						"description": "等待秒数。最小 1，最大 60，超出范围会被自动截断。",
						"minimum":     minSleepSeconds,
						"maximum":     maxSleepSeconds,
					},
				},
			},
		},
	}, nil
}

func (s *Source) CallTool(ctx context.Context, name string, input json.RawMessage) (mcpclient.ToolResult, error) {
	if name != sleepToolName {
		return mcpclient.ToolResult{}, fmt.Errorf("unknown builtin tool %q", name)
	}
	if err := ctx.Err(); err != nil {
		return mcpclient.ToolResult{}, err
	}

	duration, seconds, err := sleepDuration(input)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	if err := s.sleep(ctx, duration); err != nil {
		return mcpclient.ToolResult{}, err
	}

	return mcpclient.ToolResult{Content: fmt.Sprintf("slept %s", formatSeconds(seconds))}, nil
}

func sleepDuration(input json.RawMessage) (time.Duration, float64, error) {
	var parsed sleepInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &parsed); err != nil {
			return 0, 0, fmt.Errorf("parse sleep input: %w", err)
		}
	}

	seconds := clampSeconds(parsed.Seconds)
	duration := time.Duration(seconds * float64(defaultSleepUnit))
	return duration, seconds, nil
}

func clampSeconds(seconds float64) float64 {
	if math.IsNaN(seconds) || seconds < minSleepSeconds {
		return minSleepSeconds
	}
	if seconds > maxSleepSeconds {
		return maxSleepSeconds
	}

	return seconds
}

func realSleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func formatSeconds(seconds float64) string {
	if seconds == math.Trunc(seconds) {
		if seconds == 1 {
			return "1 second"
		}
		return fmt.Sprintf("%.0f seconds", seconds)
	}

	return fmt.Sprintf("%g seconds", seconds)
}
