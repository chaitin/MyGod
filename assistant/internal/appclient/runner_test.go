package appclient

import (
	"context"
	"encoding/json"
	"errors"
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

func TestConversationAgentRunnerIdleRetirementClosesTopic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requester := newRunnerTopicRequester()
	runner := newConversationAgentRunner(ctx, conversationAgentRunnerOptions{
		IdleTimeout: 15 * time.Millisecond,
		MaxSessions: 2,
	})
	defer runner.CancelAll()
	output := make(chan struct{}, 1)
	assistantAgent := agent.New(llmModelFunc(func(context.Context, llm.Request) (llm.Response, error) {
		return llm.Response{Blocks: []llm.Block{{Type: llm.BlockTypeText, Text: "完成"}}}, nil
	}))
	prepared := preparedTopicRun("topic-1", "topic-message-1", 1, "第一条", "user-1", "auth_1", requester)
	runner.Start(ctx, "topic-1", agent.OutputSinkFunc(func(context.Context, string) error {
		output <- struct{}{}
		return nil
	}), assistantAgent, prepared)
	waitForSignal(t, output, "topic response")

	select {
	case closed := <-requester.closed:
		if closed.ConversationID != "topic-1" || closed.ExpectedLastMessageSeq != 1 {
			t.Fatalf("closed topic request = %#v", closed)
		}
	case <-time.After(time.Second):
		t.Fatal("idle topic was not closed")
	}
	waitForRunnerJobRemoved(t, runner, "topic-1")
}

func TestConversationAgentRunnerNewMessageCancelsIdleRetirementWithoutReplacingSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requester := newCancelableRetirementRequester()
	runner := newConversationAgentRunner(ctx, conversationAgentRunnerOptions{
		IdleTimeout: 15 * time.Millisecond,
		MaxSessions: 2,
	})
	defer runner.CancelAll()
	outputs := make(chan string, 2)
	var modelCalls int
	var modelMu sync.Mutex
	assistantAgent := agent.New(llmModelFunc(func(context.Context, llm.Request) (llm.Response, error) {
		modelMu.Lock()
		modelCalls++
		call := modelCalls
		modelMu.Unlock()
		return llm.Response{Blocks: []llm.Block{{Type: llm.BlockTypeText, Text: fmt.Sprintf("完成-%d", call)}}}, nil
	}))
	sink := agent.OutputSinkFunc(func(_ context.Context, content string) error {
		outputs <- content
		return nil
	})
	first := preparedTopicRun("topic-1", "topic-message-1", 1, "第一条", "user-1", "auth_1", requester)
	runner.Start(ctx, "topic-1", sink, assistantAgent, first)
	if output := waitForString(t, outputs, "first topic response"); output != "完成-1" {
		t.Fatalf("first output = %q", output)
	}
	waitForSignal(t, requester.getStarted, "idle retirement request")

	runner.mu.Lock()
	originalJob := runner.jobs["topic-1"]
	runner.idleTimeout = time.Hour
	runner.mu.Unlock()
	second := preparedTopicRun("topic-1", "topic-message-2", 2, "第二条", "user-1", "auth_2", requester)
	runner.Start(ctx, "topic-1", sink, assistantAgent, second)
	waitForSignal(t, requester.getCanceled, "idle retirement cancellation")
	if output := waitForString(t, outputs, "second topic response"); output != "完成-2" {
		t.Fatalf("second output = %q", output)
	}

	runner.mu.Lock()
	currentJob := runner.jobs["topic-1"]
	retiring := currentJob != nil && currentJob.retiring
	runner.mu.Unlock()
	if currentJob != originalJob || retiring {
		t.Fatalf("topic job after recovery = %p retiring=%v, want original %p active", currentJob, retiring, originalJob)
	}
	select {
	case closed := <-requester.closed:
		t.Fatalf("resumed topic was closed: %#v", closed)
	default:
	}
}

func TestConversationAgentRunnerRoutesModelFailureToParentConversation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := newConversationAgentRunner(ctx)
	defer runner.CancelAll()
	modelErr := errors.New("model unavailable")
	assistantAgent := agent.New(llmModelFunc(func(context.Context, llm.Request) (llm.Response, error) {
		return llm.Response{}, modelErr
	}))
	topicOutputs := make(chan string, 1)
	parentOutputs := make(chan string, 2)
	prepared := preparedTopicRun("topic-1", "topic-message-1", 1, "开始", "user-1", "auth_1", newRunnerTopicRequester())
	prepared.ErrorSink = agent.OutputSinkFunc(func(_ context.Context, content string) error {
		parentOutputs <- content
		return nil
	})
	runner.Start(ctx, "topic-1", agent.OutputSinkFunc(func(_ context.Context, content string) error {
		topicOutputs <- content
		return nil
	}), assistantAgent, prepared)
	if output := waitForString(t, parentOutputs, "parent task error"); output != agent.ModelErrorFallback {
		t.Fatalf("parent output = %q, want model fallback", output)
	}
	select {
	case output := <-topicOutputs:
		t.Fatalf("model failure was sent to topic: %q", output)
	case <-time.After(50 * time.Millisecond):
	}
	select {
	case output := <-parentOutputs:
		t.Fatalf("model failure was sent more than once: %q", output)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestConversationAgentRunnerCapacityFailureClosesNewTopicAndRepliesToParent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := newConversationAgentRunner(ctx, conversationAgentRunnerOptions{
		IdleTimeout: time.Hour,
		MaxSessions: 1,
	})
	defer runner.CancelAll()
	firstStarted := make(chan struct{})
	assistantAgent := agent.New(llmModelFunc(func(ctx context.Context, request llm.Request) (llm.Response, error) {
		close(firstStarted)
		<-ctx.Done()
		return llm.Response{}, ctx.Err()
	}))
	first := preparedTopicRun("topic-busy", "busy-message", 1, "持续任务", "user-1", "auth_1", newRunnerTopicRequester())
	runner.Start(ctx, "topic-busy", agent.OutputSinkFunc(func(context.Context, string) error { return nil }), assistantAgent, first)
	waitForSignal(t, firstStarted, "busy topic session")

	requester := newRunnerTopicRequester()
	topicOutputs := make(chan string, 1)
	parentOutputs := make(chan string, 1)
	second := preparedTopicRun("topic-rejected", "rejected-message", 1, "新任务", "user-2", "auth_2", requester)
	second.CloseTopicOnSessionFailure = true
	second.ErrorSink = agent.OutputSinkFunc(func(_ context.Context, content string) error {
		parentOutputs <- content
		return nil
	})
	accepted := runner.Start(ctx, "topic-rejected", agent.OutputSinkFunc(func(_ context.Context, content string) error {
		topicOutputs <- content
		return nil
	}), assistantAgent, second)
	if !accepted {
		t.Fatal("capacity rejection was not handled after notifying the parent and closing the topic")
	}
	if output := waitForString(t, parentOutputs, "capacity error in parent"); output != agent.ModelErrorFallback {
		t.Fatalf("parent output = %q, want model fallback", output)
	}
	select {
	case output := <-topicOutputs:
		t.Fatalf("capacity error was sent to topic: %q", output)
	default:
	}
	select {
	case closed := <-requester.closed:
		if closed.ConversationID != "topic-rejected" || closed.ExpectedLastMessageSeq != 1 {
			t.Fatalf("closed rejected topic = %#v", closed)
		}
	case <-time.After(time.Second):
		t.Fatal("capacity-rejected topic was left open")
	}
	runner.mu.Lock()
	_, busyExists := runner.jobs["topic-busy"]
	_, rejectedExists := runner.jobs["topic-rejected"]
	activeCount := runner.activeSessionCountLocked()
	runner.mu.Unlock()
	if !busyExists || rejectedExists || activeCount != 1 {
		t.Fatalf("jobs after capacity rejection: busy=%v rejected=%v active=%d", busyExists, rejectedExists, activeCount)
	}
}

func TestConversationAgentRunnerEvictsLeastRecentlyUsedIdleTopicAtCapacity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requester := newRunnerTopicRequester()
	runner := newConversationAgentRunner(ctx, conversationAgentRunnerOptions{
		IdleTimeout: time.Hour,
		MaxSessions: 2,
	})
	defer runner.CancelAll()
	outputs := make(chan struct{}, 3)
	assistantAgent := agent.New(llmModelFunc(func(context.Context, llm.Request) (llm.Response, error) {
		return llm.Response{Blocks: []llm.Block{{Type: llm.BlockTypeText, Text: "完成"}}}, nil
	}))
	sink := agent.OutputSinkFunc(func(context.Context, string) error {
		outputs <- struct{}{}
		return nil
	})

	for _, topicID := range []string{"topic-old", "topic-new"} {
		runner.Start(ctx, topicID, sink, assistantAgent, preparedTopicRun(topicID, topicID+"-message", 1, "开始", "user-1", "auth_1", requester))
		waitForSignal(t, outputs, topicID+" response")
		waitForRunnerJobIdle(t, runner, topicID)
	}
	runner.mu.Lock()
	runner.jobs["topic-old"].lastActiveAt = time.Now().Add(-2 * time.Hour)
	runner.jobs["topic-new"].lastActiveAt = time.Now().Add(-time.Hour)
	runner.mu.Unlock()

	runner.Start(ctx, "topic-third", sink, assistantAgent, preparedTopicRun("topic-third", "topic-third-message", 1, "开始", "user-1", "auth_1", requester))
	waitForSignal(t, outputs, "third topic response")
	select {
	case closed := <-requester.closed:
		if closed.ConversationID != "topic-old" {
			t.Fatalf("evicted topic = %q, want topic-old", closed.ConversationID)
		}
	case <-time.After(time.Second):
		t.Fatal("capacity eviction did not close an idle topic")
	}
	waitForRunnerJobRemoved(t, runner, "topic-old")
	runner.mu.Lock()
	_, oldExists := runner.jobs["topic-old"]
	_, newExists := runner.jobs["topic-new"]
	_, thirdExists := runner.jobs["topic-third"]
	jobCount := len(runner.jobs)
	runner.mu.Unlock()
	if oldExists || !newExists || !thirdExists || jobCount != 2 {
		t.Fatalf("jobs after eviction: old=%v new=%v third=%v count=%d", oldExists, newExists, thirdExists, jobCount)
	}
}

func TestConversationAgentRunnerIsolatesAuthorizationPerTopicTrigger(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requester := newRunnerTopicRequester()
	runner := newConversationAgentRunner(ctx)
	defer runner.CancelAll()
	source := builtintools.NewSource()
	var modelCalls int
	var modelMu sync.Mutex
	errorsSeen := make(chan error, 3)
	assistantAgent := agent.New(llmModelFunc(func(ctx context.Context, _ llm.Request) (llm.Response, error) {
		modelMu.Lock()
		modelCalls++
		call := modelCalls
		modelMu.Unlock()
		if call == 1 {
			_, err := source.CallTool(ctx, "projects", json.RawMessage(`{"operation":"search_projects","runas":{"type":"user","id":"user-a","authorization_ref":"auth_a"},"arguments":{}}`))
			errorsSeen <- err
		} else if call == 2 {
			_, oldErr := source.CallTool(ctx, "projects", json.RawMessage(`{"operation":"search_projects","runas":{"type":"user","id":"user-a","authorization_ref":"auth_a"},"arguments":{}}`))
			if oldErr == nil {
				errorsSeen <- errors.New("previous trigger authorization still resolves")
			} else {
				errorsSeen <- nil
			}
			_, currentErr := source.CallTool(ctx, "projects", json.RawMessage(`{"operation":"search_projects","runas":{"type":"user","id":"user-b","authorization_ref":"auth_b"},"arguments":{}}`))
			errorsSeen <- currentErr
		}
		return llm.Response{Blocks: []llm.Block{{Type: llm.BlockTypeText, Text: "完成"}}}, nil
	}))
	outputs := make(chan struct{}, 2)
	sink := agent.OutputSinkFunc(func(context.Context, string) error {
		outputs <- struct{}{}
		return nil
	})

	first := preparedTopicRun("topic-1", "parent-message", 42, "第一条", "user-a", "auth_a", requester)
	first.EventConversationID = "parent-group"
	first.Scope.AuthorizationConversationID = "parent-group"
	runner.Start(ctx, "topic-1", sink, assistantAgent, first)
	waitForSignal(t, outputs, "first trigger response")
	waitForRunnerJobIdle(t, runner, "topic-1")

	second := preparedTopicRun("topic-1", "topic-message-1", 1, "第二条", "user-b", "auth_b", requester)
	runner.Start(ctx, "topic-1", sink, assistantAgent, second)
	waitForSignal(t, outputs, "second trigger response")
	for index := 0; index < 3; index++ {
		if err := <-errorsSeen; err != nil {
			t.Fatalf("authorization check %d: %v", index+1, err)
		}
	}

	projectCalls := requester.projectCalls()
	if len(projectCalls) != 2 {
		t.Fatalf("project requester calls = %d, want 2", len(projectCalls))
	}
	if projectCalls[0].AuthorizationConversationID != "parent-group" || projectCalls[0].ID != "user-a" || projectCalls[0].TriggerMessageID != "parent-message" {
		t.Fatalf("first trigger runas = %#v", projectCalls[0])
	}
	if projectCalls[1].AuthorizationConversationID != "topic-1" || projectCalls[1].ID != "user-b" || projectCalls[1].TriggerMessageID != "topic-message-1" {
		t.Fatalf("second trigger runas = %#v", projectCalls[1])
	}
}

func TestConversationAgentRunnerActivatesInterveningTriggerAtNextModelTurn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requester := newRunnerTopicRequester()
	runner := newConversationAgentRunner(ctx)
	defer runner.CancelAll()
	source := builtintools.NewSource()
	registry := &blockingToolRegistry{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	checks := make(chan error, 3)
	var modelCalls int
	var modelMu sync.Mutex
	assistantAgent := agent.New(llmModelFunc(func(ctx context.Context, _ llm.Request) (llm.Response, error) {
		modelMu.Lock()
		modelCalls++
		call := modelCalls
		modelMu.Unlock()
		if call == 1 {
			_, err := source.CallTool(ctx, "projects", json.RawMessage(`{"operation":"search_projects","runas":{"type":"user","id":"user-a","authorization_ref":"auth_a"},"arguments":{}}`))
			checks <- err
			return llm.Response{Blocks: []llm.Block{{
				Type:      llm.BlockTypeToolUse,
				ToolUseID: "toolu_wait",
				ToolName:  "test__wait",
				ToolInput: json.RawMessage(`{}`),
			}}}, nil
		}
		_, oldErr := source.CallTool(ctx, "projects", json.RawMessage(`{"operation":"search_projects","runas":{"type":"user","id":"user-a","authorization_ref":"auth_a"},"arguments":{}}`))
		if oldErr == nil {
			checks <- errors.New("previous trigger authorization still resolves")
		} else {
			checks <- nil
		}
		_, currentErr := source.CallTool(ctx, "projects", json.RawMessage(`{"operation":"search_projects","runas":{"type":"user","id":"user-b","authorization_ref":"auth_b"},"arguments":{}}`))
		checks <- currentErr
		return llm.Response{Blocks: []llm.Block{{Type: llm.BlockTypeText, Text: "已按补充继续"}}}, nil
	}), agent.WithToolRegistry(registry))
	outputs := make(chan struct{}, 1)
	sink := agent.OutputSinkFunc(func(context.Context, string) error {
		outputs <- struct{}{}
		return nil
	})

	first := preparedTopicRun("topic-1", "parent-message", 42, "第一条", "user-a", "auth_a", requester)
	first.EventConversationID = "parent-group"
	first.Scope.AuthorizationConversationID = "parent-group"
	runner.Start(ctx, "topic-1", sink, assistantAgent, first)
	waitForSignal(t, registry.started, "first tool call to start")

	second := preparedTopicRun("topic-1", "topic-message-1", 1, "方向不对，按第二条处理", "user-b", "auth_b", requester)
	runner.Start(ctx, "topic-1", sink, assistantAgent, second)
	runner.mu.Lock()
	job := runner.jobs["topic-1"]
	runner.mu.Unlock()
	if _, ok := job.scopeStore.ResolveAuthorization("auth_a"); !ok {
		t.Fatal("current tool-chain authorization changed before the next model turn")
	}
	if _, ok := job.scopeStore.ResolveAuthorization("auth_b"); ok {
		t.Fatal("intervening trigger authorization activated before the next model turn")
	}

	close(registry.release)
	waitForSignal(t, outputs, "intervening trigger response")
	for index := 0; index < 3; index++ {
		if err := <-checks; err != nil {
			t.Fatalf("authorization check %d: %v", index+1, err)
		}
	}

	projectCalls := requester.projectCalls()
	if len(projectCalls) != 2 {
		t.Fatalf("project requester calls = %d, want 2", len(projectCalls))
	}
	if projectCalls[0].AuthorizationConversationID != "parent-group" || projectCalls[0].ID != "user-a" {
		t.Fatalf("first trigger runas = %#v", projectCalls[0])
	}
	if projectCalls[1].AuthorizationConversationID != "topic-1" || projectCalls[1].ID != "user-b" {
		t.Fatalf("intervening trigger runas = %#v", projectCalls[1])
	}
}

func TestConversationAgentRunnerPreservesSenderOrderAcrossInterventions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := newConversationAgentRunner(ctx)
	defer runner.CancelAll()
	registry := &blockingToolRegistry{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	secondTurn := make(chan struct{})
	thirdTurn := make(chan struct{})
	var requests []llm.Request
	var requestsMu sync.Mutex
	assistantAgent := agent.New(llmModelFunc(func(_ context.Context, request llm.Request) (llm.Response, error) {
		requestsMu.Lock()
		requests = append(requests, request)
		call := len(requests)
		requestsMu.Unlock()
		switch call {
		case 1:
			return llm.Response{Blocks: []llm.Block{
				{Type: llm.BlockTypeToolUse, ToolUseID: "toolu_wait", ToolName: "test__wait", ToolInput: json.RawMessage(`{}`)},
				{Type: llm.BlockTypeToolUse, ToolUseID: "toolu_must_skip", ToolName: "test__must_skip", ToolInput: json.RawMessage(`{}`)},
			}}, nil
		case 2:
			close(secondTurn)
			return llm.Response{Blocks: []llm.Block{{Type: llm.BlockTypeText, Text: "处理 Bob 的消息"}}}, nil
		default:
			close(thirdTurn)
			return llm.Response{Blocks: []llm.Block{{Type: llm.BlockTypeText, Text: "处理 Alice 的后续消息"}}}, nil
		}
	}), agent.WithToolRegistry(registry))
	sink := agent.OutputSinkFunc(func(context.Context, string) error { return nil })
	requester := newRunnerTopicRequester()

	runner.Start(ctx, "topic-1", sink, assistantAgent,
		preparedTopicRun("topic-1", "message-a1", 1, "Alice 第一条", "user-a", "auth_a1", requester))
	waitForSignal(t, registry.started, "first sender tool call")
	runner.Start(ctx, "topic-1", sink, assistantAgent,
		preparedTopicRun("topic-1", "message-b1", 2, "Bob 插话", "user-b", "auth_b1", requester))
	runner.Start(ctx, "topic-1", sink, assistantAgent,
		preparedTopicRun("topic-1", "message-a2", 3, "Alice 再补充", "user-a", "auth_a2", requester))
	close(registry.release)
	waitForSignal(t, secondTurn, "second sender turn")
	waitForSignal(t, thirdTurn, "third sender turn")

	requestsMu.Lock()
	defer requestsMu.Unlock()
	if len(requests) != 3 {
		t.Fatalf("model request count = %d, want 3", len(requests))
	}
	secondJSON, _ := json.Marshal(requests[1].Messages)
	if !strings.Contains(string(secondJSON), "Bob 插话") || strings.Contains(string(secondJSON), "Alice 再补充") {
		t.Fatalf("second sender request = %s, want Bob only", secondJSON)
	}
	for _, snippet := range []string{"toolu_must_skip", "用户发送了新的消息，本工具尚未执行。"} {
		if !strings.Contains(string(secondJSON), snippet) {
			t.Fatalf("second sender request = %s, want interrupted tool result %q", secondJSON, snippet)
		}
	}
	thirdJSON, _ := json.Marshal(requests[2].Messages)
	if !strings.Contains(string(thirdJSON), "Alice 再补充") {
		t.Fatalf("third sender request = %s, want Alice follow-up", thirdJSON)
	}
}

func TestConversationAgentRunnerTopicClosedEventCancelsSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := newConversationAgentRunner(ctx)
	started := make(chan struct{})
	canceled := make(chan struct{})
	assistantAgent := agent.New(llmModelFunc(func(ctx context.Context, _ llm.Request) (llm.Response, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return llm.Response{}, ctx.Err()
	}))
	runner.Start(ctx, "topic-1", agent.OutputSinkFunc(func(context.Context, string) error { return nil }), assistantAgent,
		preparedTopicRun("topic-1", "topic-message-1", 1, "开始", "user-1", "auth_1", newRunnerTopicRequester()))
	waitForSignal(t, started, "topic session start")
	runner.CloseConversationSession("topic-1")
	waitForSignal(t, canceled, "topic session cancellation")
	runner.mu.Lock()
	_, exists := runner.jobs["topic-1"]
	runner.mu.Unlock()
	if exists {
		t.Fatal("closed topic session remains registered")
	}
}

type runnerTopicCloseRequest struct {
	ConversationID         string
	ExpectedLastMessageSeq int64
}

type runnerProjectRunAs struct {
	AuthorizationConversationID string `json:"authorization_conversation_id"`
	ID                          string `json:"id"`
	TriggerMessageID            string `json:"trigger_message_id"`
}

type runnerTopicRequester struct {
	mu       sync.Mutex
	closed   chan runnerTopicCloseRequest
	projects []runnerProjectRunAs
}

type cancelableRetirementRequester struct {
	closed      chan runnerTopicCloseRequest
	getCanceled chan struct{}
	getStarted  chan struct{}
	cancelOnce  sync.Once
	startOnce   sync.Once
}

func newCancelableRetirementRequester() *cancelableRetirementRequester {
	return &cancelableRetirementRequester{
		closed: make(chan runnerTopicCloseRequest, 1), getCanceled: make(chan struct{}), getStarted: make(chan struct{}),
	}
}

func (r *cancelableRetirementRequester) Request(ctx context.Context, method string, payload any) (json.RawMessage, error) {
	switch method {
	case methodConversationTopicGet:
		r.startOnce.Do(func() { close(r.getStarted) })
		<-ctx.Done()
		r.cancelOnce.Do(func() { close(r.getCanceled) })
		return nil, ctx.Err()
	case methodConversationTopicClose:
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		var request topicMutationRequestPayload
		if err := json.Unmarshal(raw, &request); err != nil {
			return nil, err
		}
		r.closed <- runnerTopicCloseRequest{
			ConversationID: request.ConversationID, ExpectedLastMessageSeq: request.ExpectedLastMessageSeq,
		}
		return json.RawMessage(`{"archived":true}`), nil
	default:
		return nil, fmt.Errorf("unexpected requester method %q", method)
	}
}

func newRunnerTopicRequester() *runnerTopicRequester {
	return &runnerTopicRequester{closed: make(chan runnerTopicCloseRequest, 8)}
}

func (r *runnerTopicRequester) Request(_ context.Context, method string, payload any) (json.RawMessage, error) {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	switch method {
	case methodConversationTopicGet:
		var request topicMutationRequestPayload
		if err := json.Unmarshal(rawPayload, &request); err != nil {
			return nil, err
		}
		return json.Marshal(topicMutationResponsePayload{
			Conversation:   conversationPayload{ID: request.ConversationID, Type: "topic"},
			LastMessageSeq: 1,
		})
	case methodConversationTopicClose:
		var request topicMutationRequestPayload
		if err := json.Unmarshal(rawPayload, &request); err != nil {
			return nil, err
		}
		r.closed <- runnerTopicCloseRequest{
			ConversationID: request.ConversationID, ExpectedLastMessageSeq: request.ExpectedLastMessageSeq,
		}
		return json.RawMessage(`{"archived":true}`), nil
	case "projects.list":
		var request struct {
			RunAs runnerProjectRunAs `json:"runas"`
		}
		if err := json.Unmarshal(rawPayload, &request); err != nil {
			return nil, err
		}
		r.mu.Lock()
		r.projects = append(r.projects, request.RunAs)
		r.mu.Unlock()
		return json.RawMessage(`{"ok":true}`), nil
	default:
		return nil, fmt.Errorf("unexpected requester method %q", method)
	}
}

func (r *runnerTopicRequester) projectCalls() []runnerProjectRunAs {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]runnerProjectRunAs(nil), r.projects...)
}

func preparedTopicRun(topicID, messageID string, seq int64, content, userID, authorizationRef string, requester builtintools.AppRequester) preparedAgentRun {
	prepared := preparedTextRun(topicID, messageID, seq, content)
	prepared.EventConversationID = topicID
	prepared.Authorization = preparedAuthorization{
		Authorization: builtintools.Authorization{ActorID: userID, ActorType: "user", TriggerMessageID: messageID},
		Candidate: agent.AuthorizationCandidate{
			Ref: authorizationRef, SenderID: userID, SenderName: userID,
			SenderType: "user", MessageSeq: seq, MessageSummary: content,
		},
		Ref: authorizationRef,
	}
	prepared.Request.AuthorizationRef = authorizationRef
	prepared.Request.Conversation = agent.Conversation{ID: topicID, Name: "话题", Type: "topic"}
	prepared.Request.Sender = agent.Sender{ID: userID, Name: userID, Type: "user"}
	prepared.Scope = builtintools.Scope{
		AuthorizationConversationID: topicID,
		ConversationID:              topicID,
		ConversationType:            "topic",
		Requester:                   requester,
	}
	return prepared
}

func waitForRunnerJobIdle(t *testing.T, runner *conversationAgentRunner, key string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		runner.mu.Lock()
		job := runner.jobs[key]
		idle := job != nil && !job.running && !job.session.HasPending() && len(job.pending) == 0
		runner.mu.Unlock()
		if idle {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("runner job %q did not become idle", key)
		}
		time.Sleep(time.Millisecond)
	}
}

func waitForRunnerJobRemoved(t *testing.T, runner *conversationAgentRunner, key string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		runner.mu.Lock()
		_, exists := runner.jobs[key]
		runner.mu.Unlock()
		if !exists {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("runner job %q was not removed", key)
		}
		time.Sleep(time.Millisecond)
	}
}

func waitForString(t *testing.T, ch <-chan string, label string) string {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", label)
		return ""
	}
}
