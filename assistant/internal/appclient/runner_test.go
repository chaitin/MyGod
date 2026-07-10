package appclient

import (
	"context"
	"testing"
	"time"

	"assistant/internal/agent"
	"assistant/internal/llm"
)

func TestConversationAgentRunnerSessionOutlivesTriggerContext(t *testing.T) {
	rootCtx, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()
	runner := newConversationAgentRunner(rootCtx)

	started := make(chan struct{})
	canceled := make(chan struct{})
	assistantAgent := agent.New(llmModelFunc(func(ctx context.Context, request llm.Request) (llm.Response, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return llm.Response{}, ctx.Err()
	}))
	triggerCtx, cancelTrigger := context.WithCancel(context.Background())
	runner.Start(
		triggerCtx,
		"conversation-1",
		agent.OutputSinkFunc(func(context.Context, string) error { return nil }),
		assistantAgent,
		preparedTextRun("conversation-1", "message-1", 1, "第一条"),
	)
	waitForSignal(t, started, "agent session to start")

	cancelTrigger()
	select {
	case <-canceled:
		t.Fatal("agent session canceled with trigger context")
	case <-time.After(50 * time.Millisecond):
	}

	cancelRoot()
	waitForSignal(t, canceled, "agent session to stop with process context")
}

func TestConversationAgentRunnerIgnoresDuplicateSequence(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := newConversationAgentRunner(ctx)
	outputs := make(chan struct{}, 2)
	assistantAgent := agent.New(llmModelFunc(func(ctx context.Context, request llm.Request) (llm.Response, error) {
		return llm.Response{Blocks: []llm.Block{{Type: llm.BlockTypeText, Text: "完成"}}}, nil
	}))
	sink := agent.OutputSinkFunc(func(context.Context, string) error {
		outputs <- struct{}{}
		return nil
	})
	prepared := preparedTextRun("conversation-1", "message-1", 7, "第一条")
	runner.Start(ctx, "conversation-1", sink, assistantAgent, prepared)
	waitForSignal(t, outputs, "first response")

	runner.Start(ctx, "conversation-1", sink, assistantAgent, prepared)
	select {
	case <-outputs:
		t.Fatal("duplicate sequence executed a second time")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestConversationAgentRunnerKeepsSequenceWatermarkAfterIdleCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := newConversationAgentRunner(ctx)
	runner.idleTimeout = 10 * time.Millisecond
	outputs := make(chan struct{}, 2)
	assistantAgent := agent.New(llmModelFunc(func(ctx context.Context, request llm.Request) (llm.Response, error) {
		return llm.Response{Blocks: []llm.Block{{Type: llm.BlockTypeText, Text: "完成"}}}, nil
	}))
	sink := agent.OutputSinkFunc(func(context.Context, string) error {
		outputs <- struct{}{}
		return nil
	})
	prepared := preparedTextRun("conversation-1", "message-1", 7, "第一条")
	runner.Start(ctx, "conversation-1", sink, assistantAgent, prepared)
	waitForSignal(t, outputs, "first response")

	deadline := time.Now().Add(time.Second)
	for {
		runner.mu.Lock()
		_, exists := runner.jobs["conversation-1"]
		runner.mu.Unlock()
		if !exists {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("conversation job was not removed after idle timeout")
		}
		time.Sleep(time.Millisecond)
	}

	runner.Start(ctx, "conversation-1", sink, assistantAgent, prepared)
	select {
	case <-outputs:
		t.Fatal("duplicate sequence executed after idle session cleanup")
	case <-time.After(50 * time.Millisecond):
	}
}
