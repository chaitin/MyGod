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
