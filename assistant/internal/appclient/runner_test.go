package appclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"assistant/internal/agent"
	"assistant/internal/builtintools"
	"assistant/internal/llm"
	"assistant/internal/mcpclient"
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

func TestConversationAgentRunnerKeepsSessionAfterConversationEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := newConversationAgentRunner(ctx)
	registry, err := mcpclient.NewRegistry(ctx, []mcpclient.Source{builtintools.NewSource()})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	var mu sync.Mutex
	var requests []llm.Request
	firstCalled := make(chan struct{}, 1)
	assistantAgent := agent.New(llmModelFunc(func(_ context.Context, request llm.Request) (llm.Response, error) {
		mu.Lock()
		requests = append(requests, request)
		requestNumber := len(requests)
		mu.Unlock()
		if requestNumber == 1 {
			firstCalled <- struct{}{}
			return llm.Response{Blocks: []llm.Block{{
				Type:      llm.BlockTypeToolUse,
				ToolUseID: "toolu_end",
				ToolName:  "builtin__end_conversation",
				ToolInput: json.RawMessage(`{}`),
			}}}, nil
		}
		return llm.Response{Blocks: []llm.Block{{Type: llm.BlockTypeText, Text: "继续会话"}}}, nil
	}), agent.WithToolRegistry(registry))
	output := make(chan struct{}, 1)
	sink := agent.OutputSinkFunc(func(context.Context, string) error {
		output <- struct{}{}
		return nil
	})
	first := preparedTextRun("conversation-1", "message-1", 1, "第一条")
	runner.Start(ctx, "conversation-1", sink, assistantAgent, first)
	waitForSignal(t, firstCalled, "end tool request")

	deadline := time.Now().Add(time.Second)
	for {
		runner.mu.Lock()
		job, exists := runner.jobs["conversation-1"]
		idle := exists && !job.running
		runner.mu.Unlock()
		if idle {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("conversation session did not become idle after end: exists=%v", exists)
		}
		time.Sleep(time.Millisecond)
	}

	second := preparedTextRun("conversation-1", "message-2", 2, "第二条")
	runner.Start(ctx, "conversation-1", sink, assistantAgent, second)
	waitForSignal(t, output, "new session response")

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(requests))
	}
	secondJSON, err := json.Marshal(requests[1].Messages)
	if err != nil {
		t.Fatalf("marshal second request: %v", err)
	}
	if !strings.Contains(string(secondJSON), "第一条") || !strings.Contains(string(secondJSON), "第二条") {
		t.Fatalf("second request messages = %s, want preserved and new context", secondJSON)
	}
}

func TestConversationWaitRegistryClaimsOnlyMatchingReplies(t *testing.T) {
	registry := newConversationWaitRegistry()
	registration, err := registry.RegisterConversationWait("conversation-1", 10, "user", "user-1")
	if err != nil {
		t.Fatalf("RegisterConversationWait() error = %v", err)
	}
	if registry.Claim("conversation-1", 10, "user", "user-2") {
		t.Fatal("message at after_seq was claimed")
	}
	if registry.Claim("conversation-1", 11, "user", "user-1") {
		t.Fatal("runas identity's own message was claimed")
	}
	if registry.Claim("conversation-1", 11, "system", "") {
		t.Fatal("system message was claimed")
	}
	if !registry.Claim("conversation-1", 11, "user", "user-2") {
		t.Fatal("new reply was not claimed")
	}
	if !registry.Claim("conversation-1", 12, "app", "app-2") {
		t.Fatal("new app reply was not claimed")
	}
	registration.Close()
	if registry.Claim("conversation-1", 13, "user", "user-2") {
		t.Fatal("message was claimed after waiter closed")
	}
}

func TestConversationWaitRegistryRejectsConcurrentWaiter(t *testing.T) {
	registry := newConversationWaitRegistry()
	registration, err := registry.RegisterConversationWait("conversation-1", 10, "user", "user-1")
	if err != nil {
		t.Fatalf("RegisterConversationWait() error = %v", err)
	}
	defer registration.Close()
	if _, err := registry.RegisterConversationWait("conversation-1", 20, "user", "user-2"); err == nil {
		t.Fatal("second waiter registration error = nil")
	}
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

func TestConversationAgentRunnerUsesJobWatermarkAfterGlobalEviction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := newConversationAgentRunner(ctx)
	outputs := make(chan struct{}, 2)
	assistantAgent := agent.New(llmModelFunc(func(context.Context, llm.Request) (llm.Response, error) {
		return llm.Response{Blocks: []llm.Block{{Type: llm.BlockTypeText, Text: "完成"}}}, nil
	}))
	sink := agent.OutputSinkFunc(func(context.Context, string) error {
		outputs <- struct{}{}
		return nil
	})
	prepared := preparedTextRun("conversation-1", "message-1", 7, "第一条")
	runner.Start(ctx, "conversation-1", sink, assistantAgent, prepared)
	waitForSignal(t, outputs, "first response")

	runner.mu.Lock()
	for i := 0; i <= maxConversationSequenceWatermarks; i++ {
		runner.recordSequenceLocked(fmt.Sprintf("other-%d", i), 1)
	}
	_, stillCached := runner.lastSeenSeq["conversation-1"]
	runner.mu.Unlock()
	if stillCached {
		t.Fatal("conversation watermark was not evicted")
	}

	runner.Start(ctx, "conversation-1", sink, assistantAgent, prepared)
	select {
	case <-outputs:
		t.Fatal("duplicate sequence appended after global watermark eviction")
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
