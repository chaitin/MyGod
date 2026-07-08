package builtintools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestSleepToolClampsDuration(t *testing.T) {
	var durations []time.Duration
	source := newSourceWithSleeper(func(ctx context.Context, duration time.Duration) error {
		durations = append(durations, duration)
		return nil
	})

	if _, err := source.CallTool(context.Background(), "sleep", json.RawMessage(`{"seconds":0}`)); err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if _, err := source.CallTool(context.Background(), "sleep", json.RawMessage(`{"seconds":100}`)); err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}

	want := []time.Duration{time.Second, 60 * time.Second}
	if len(durations) != len(want) {
		t.Fatalf("duration count = %d, want %d", len(durations), len(want))
	}
	for index := range want {
		if durations[index] != want[index] {
			t.Fatalf("duration[%d] = %s, want %s", index, durations[index], want[index])
		}
	}
}

func TestSleepToolReturnsCanceledContext(t *testing.T) {
	source := newSourceWithSleeper(func(ctx context.Context, duration time.Duration) error {
		<-ctx.Done()
		return ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := source.CallTool(ctx, "sleep", json.RawMessage(`{"seconds":10}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CallTool() error = %v, want context.Canceled", err)
	}
}

func TestSleepToolListMetadata(t *testing.T) {
	source := NewSource()

	tools, err := source.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("tool count = %d, want 1", len(tools))
	}
	if tools[0].Name != "sleep" {
		t.Fatalf("tool name = %q, want sleep", tools[0].Name)
	}
	if tools[0].Description == "" {
		t.Fatal("tool description is empty")
	}
	if tools[0].InputSchema == nil {
		t.Fatal("tool input schema is nil")
	}
}
