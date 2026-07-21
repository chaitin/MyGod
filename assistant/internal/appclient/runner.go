package appclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"assistant/internal/agent"
	"assistant/internal/builtintools"
)

const (
	maxConversationSequenceWatermarks = 10_000
	retirementRetryDelay              = 5 * time.Second
)

type agentRunner interface {
	Start(context.Context, string, agent.OutputSink, replyAgent, preparedAgentRun) bool
}

type preparedAgentRun struct {
	Authorization              preparedAuthorization
	CloseTopicOnSessionFailure bool
	ErrorSink                  agent.OutputSink
	EventConversationID        string
	MessageSeq                 int64
	ReplySink                  agent.OutputSink
	Request                    agent.Request
	Scope                      builtintools.Scope
}

type preparedAuthorization struct {
	Authorization builtintools.Authorization
	Candidate     agent.AuthorizationCandidate
	Ref           string
}

type sessionReplyAgent interface {
	NewSession(agent.Request) (*agent.Session, error)
}

type directAgentRunner struct{}

func (directAgentRunner) Start(ctx context.Context, key string, sink agent.OutputSink, assistantAgent replyAgent, prepared preparedAgentRun) bool {
	store := newConversationAuthorizationStore()
	prepared.Request.AuthorizationCandidates = store.Add(prepared.Authorization)
	prepared.Scope.AuthorizationResolver = store
	taskSink := &taskOutputSink{delegate: sink, errorSink: prepared.ErrorSink}
	if err := assistantAgent.Run(builtintools.WithScope(ctx, prepared.Scope), prepared.Request, taskSink); err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		log.Printf("agent reply failed: %v", err)
		if taskSink.taskErrorSent {
			return true
		}
		return sendAgentFallback(ctx, taskSink) == nil
	}
	return true
}

type conversationAgentRunner struct {
	ctx              context.Context
	idleTimeout      time.Duration
	maxSessions      int
	mu               sync.Mutex
	jobs             map[string]*conversationAgentJob
	lastSeenSeq      map[string]int64
	lastSeenSeqOrder []string
	waiters          *conversationWaitRegistry
}

type conversationAgentJob struct {
	actorID              string
	actorType            string
	cancel               context.CancelFunc
	ctx                  context.Context
	errorSink            agent.OutputSink
	lastActiveAt         time.Time
	lastScope            builtintools.Scope
	lastSeenSeq          int64
	pending              []preparedAgentRun
	retireCancel         context.CancelFunc
	retirementGeneration uint64
	retirementReason     retirementReason
	retiring             bool
	running              bool
	session              *agent.Session
	sink                 agent.OutputSink
	scopeStore           *conversationScopeStore
	timer                *time.Timer
}

type retirementReason string

const (
	retirementReasonIdle     retirementReason = "idle"
	retirementReasonCapacity retirementReason = "capacity"
)

type conversationAgentRunnerOptions struct {
	IdleTimeout time.Duration
	MaxSessions int
}

func newConversationAgentRunner(ctx context.Context, options ...conversationAgentRunnerOptions) *conversationAgentRunner {
	if ctx == nil {
		ctx = context.Background()
	}
	configured := conversationAgentRunnerOptions{IdleTimeout: time.Hour, MaxSessions: 1000}
	if len(options) > 0 {
		if options[0].IdleTimeout > 0 {
			configured.IdleTimeout = options[0].IdleTimeout
		}
		if options[0].MaxSessions > 0 {
			configured.MaxSessions = options[0].MaxSessions
		}
	}
	return &conversationAgentRunner{
		ctx:         ctx,
		idleTimeout: configured.IdleTimeout,
		maxSessions: configured.MaxSessions,
		jobs:        map[string]*conversationAgentJob{},
		lastSeenSeq: map[string]int64{},
		waiters:     newConversationWaitRegistry(),
	}
}

func (r *conversationAgentRunner) Start(ctx context.Context, key string, sink agent.OutputSink, assistantAgent replyAgent, prepared preparedAgentRun) bool {
	if key == "" {
		key = "unknown"
	}
	if prepared.ErrorSink == nil {
		prepared.ErrorSink = sink
	}
	prepared.ReplySink = sink
	prepared.Scope.ConversationWaiter = r.waiters
	eventKey := strings.TrimSpace(prepared.EventConversationID)
	if eventKey == "" {
		eventKey = key
	}
	r.mu.Lock()
	if prepared.MessageSeq > 0 && prepared.MessageSeq <= r.lastSeenSeq[eventKey] {
		r.mu.Unlock()
		return true
	}
	sessionAgent, ok := assistantAgent.(sessionReplyAgent)
	if !ok {
		r.mu.Unlock()
		accepted := directAgentRunner{}.Start(ctx, key, sink, assistantAgent, prepared)
		if accepted {
			r.mu.Lock()
			r.recordSequenceLocked(eventKey, prepared.MessageSeq)
			r.mu.Unlock()
		}
		return accepted
	}

	if job, ok := r.jobs[key]; ok {
		if eventKey == key && prepared.MessageSeq > 0 && prepared.MessageSeq <= job.lastSeenSeq {
			r.mu.Unlock()
			return true
		}
		if job.retiring {
			if job.retirementReason == retirementReasonCapacity {
				r.mu.Unlock()
				log.Printf("agent topic is being retired for session capacity: conversation_id=%s", key)
				return r.rejectPreparedRun(ctx, key, sink, prepared, false)
			}
			r.cancelRetirementLocked(job)
		}
		if job.timer != nil {
			job.timer.Stop()
			job.timer = nil
		}
		request := prepared.Request
		request.History = filterHistoryAfterSeq(request.History, job.lastSeenSeq)
		request.AuthorizationCandidates = authorizationCandidatesForTrigger(prepared.Authorization)
		prepared.Request = request
		if job.running && (len(job.pending) > 0 || !job.sameActor(prepared.Authorization)) {
			job.pending = append(job.pending, prepared)
			job.session.RequestYield()
			job.lastActiveAt = time.Now().UTC()
			if eventKey == key && prepared.MessageSeq > job.lastSeenSeq {
				job.lastSeenSeq = prepared.MessageSeq
			}
			r.recordSequenceLocked(eventKey, prepared.MessageSeq)
			r.mu.Unlock()
			return true
		}
		if err := job.session.AppendWithActivation(request, func() {
			job.scopeStore.Activate(prepared.Scope, prepared.Authorization)
		}); err != nil {
			r.mu.Unlock()
			log.Printf("append agent instruction failed: %v", err)
			return sendAgentFallback(ctx, prepared.ErrorSink) == nil
		}
		job.actorType, job.actorID = authorizationActor(prepared.Authorization)
		job.lastScope = prepared.Scope
		job.lastActiveAt = time.Now().UTC()
		job.sink = sink
		job.errorSink = prepared.ErrorSink
		if eventKey == key && prepared.MessageSeq > job.lastSeenSeq {
			job.lastSeenSeq = prepared.MessageSeq
		}
		r.recordSequenceLocked(eventKey, prepared.MessageSeq)
		if !job.running {
			job.running = true
			go r.runJob(key, job)
		}
		r.mu.Unlock()
		return true
	}

	var retiredKey string
	var retiredJob *conversationAgentJob
	var retirementGeneration uint64
	var retirementCtx context.Context
	if r.activeSessionCountLocked() >= r.maxSessions {
		retiredKey, retiredJob = r.selectOldestIdleJobLocked()
		if retiredJob == nil {
			r.mu.Unlock()
			log.Printf("agent session capacity reached: max=%d", r.maxSessions)
			return r.rejectPreparedRun(ctx, key, sink, prepared, prepared.CloseTopicOnSessionFailure)
		}
		retirementGeneration, retirementCtx = r.beginRetirementLocked(retiredJob, retirementReasonCapacity)
		retiredJob.cancel()
		retiredJob.session = nil
	}
	jobCtx, cancel := context.WithCancel(r.ctx)
	prepared.Request.AuthorizationCandidates = authorizationCandidatesForTrigger(prepared.Authorization)
	scopeStore := newConversationScopeStore(prepared.Scope, prepared.Authorization)
	session, err := sessionAgent.NewSession(prepared.Request)
	if err != nil {
		r.mu.Unlock()
		cancel()
		if retiredJob != nil {
			go r.retireJob(retiredKey, retiredJob, retirementGeneration, retirementCtx)
		}
		log.Printf("create agent session failed: %v", err)
		return r.rejectPreparedRun(ctx, key, sink, prepared, prepared.CloseTopicOnSessionFailure)
	}
	job := &conversationAgentJob{
		actorID:      strings.TrimSpace(prepared.Authorization.Authorization.ActorID),
		actorType:    strings.ToLower(strings.TrimSpace(prepared.Authorization.Authorization.ActorType)),
		cancel:       cancel,
		ctx:          jobCtx,
		errorSink:    prepared.ErrorSink,
		lastActiveAt: time.Now().UTC(),
		lastScope:    prepared.Scope,
		running:      true,
		session:      session,
		sink:         sink,
		scopeStore:   scopeStore,
	}
	if eventKey == key {
		job.lastSeenSeq = prepared.MessageSeq
	}
	r.jobs[key] = job
	r.recordSequenceLocked(eventKey, prepared.MessageSeq)
	r.mu.Unlock()

	if retiredJob != nil {
		go r.retireJob(retiredKey, retiredJob, retirementGeneration, retirementCtx)
	}
	go r.runJob(key, job)
	return true
}

func (r *conversationAgentRunner) ClaimIncomingConversationMessage(conversationID string, seq int64, senderType string, senderID string) bool {
	if r == nil || r.waiters == nil {
		return false
	}
	return r.waiters.Claim(conversationID, seq, senderType, senderID)
}

type conversationWaitRegistry struct {
	mu      sync.Mutex
	waiters map[string]*conversationWaitRegistration
}

type conversationWaitRegistration struct {
	actorID        string
	actorType      string
	afterSeq       int64
	conversationID string
	registry       *conversationWaitRegistry
	closed         bool
}

func newConversationWaitRegistry() *conversationWaitRegistry {
	return &conversationWaitRegistry{waiters: map[string]*conversationWaitRegistration{}}
}

func (r *conversationWaitRegistry) RegisterConversationWait(conversationID string, afterSeq int64, actorType string, actorID string) (builtintools.ConversationWaitRegistration, error) {
	conversationID = strings.TrimSpace(conversationID)
	actorType = strings.ToLower(strings.TrimSpace(actorType))
	actorID = strings.TrimSpace(actorID)
	if conversationID == "" || afterSeq <= 0 {
		return nil, fmt.Errorf("conversation waiter requires conversation_id and after_seq")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.waiters[conversationID]; exists {
		return nil, fmt.Errorf("conversation %q already has an active reply waiter", conversationID)
	}
	registration := &conversationWaitRegistration{
		actorID:        actorID,
		actorType:      actorType,
		afterSeq:       afterSeq,
		conversationID: conversationID,
		registry:       r,
	}
	r.waiters[conversationID] = registration
	return registration, nil
}

func (r *conversationWaitRegistry) Claim(conversationID string, seq int64, senderType string, senderID string) bool {
	if r == nil || seq <= 0 {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	waiter := r.waiters[strings.TrimSpace(conversationID)]
	if waiter == nil || seq <= waiter.afterSeq {
		return false
	}
	senderType = strings.ToLower(strings.TrimSpace(senderType))
	senderID = strings.TrimSpace(senderID)
	if senderType != "user" && senderType != "app" {
		return false
	}
	if waiter.actorType != "" && waiter.actorID != "" && senderType == waiter.actorType && senderID == waiter.actorID {
		return false
	}
	return true
}

func (r *conversationWaitRegistration) Close() {
	if r == nil || r.registry == nil {
		return
	}
	r.registry.mu.Lock()
	defer r.registry.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	if r.registry.waiters[r.conversationID] == r {
		delete(r.registry.waiters, r.conversationID)
	}
}

func (r *conversationAgentRunner) recordSequenceLocked(key string, seq int64) {
	if seq <= 0 || seq <= r.lastSeenSeq[key] {
		return
	}
	if _, exists := r.lastSeenSeq[key]; !exists {
		r.lastSeenSeqOrder = append(r.lastSeenSeqOrder, key)
	}
	r.lastSeenSeq[key] = seq
	for len(r.lastSeenSeqOrder) > maxConversationSequenceWatermarks {
		oldest := r.lastSeenSeqOrder[0]
		r.lastSeenSeqOrder = r.lastSeenSeqOrder[1:]
		delete(r.lastSeenSeq, oldest)
	}
}

func (r *conversationAgentRunner) CancelAll() {
	r.mu.Lock()
	jobs := make([]*conversationAgentJob, 0, len(r.jobs))
	for _, job := range r.jobs {
		jobs = append(jobs, job)
	}
	r.jobs = map[string]*conversationAgentJob{}
	r.mu.Unlock()

	for _, job := range jobs {
		if job.timer != nil {
			job.timer.Stop()
		}
		if job.retireCancel != nil {
			job.retireCancel()
		}
		job.cancel()
	}
}

func (r *conversationAgentRunner) runJob(key string, job *conversationAgentJob) {
	for {
		r.mu.Lock()
		current, ok := r.jobs[key]
		if !ok || current != job || job.retiring {
			r.mu.Unlock()
			return
		}
		replySink := job.sink
		errorSink := job.errorSink
		r.mu.Unlock()

		taskSink := &conversationAgentSink{
			delegate:  replySink,
			errorSink: errorSink,
			job:       job,
			key:       key,
			runner:    r,
		}
		err := job.session.RunCycle(
			builtintools.WithScopeProvider(job.ctx, job.scopeStore),
			taskSink,
		)
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("agent reply failed: %v", err)
			if !taskSink.taskErrorSent {
				_ = sendAgentFallback(job.ctx, taskSink)
			}
		}

		r.mu.Lock()
		current, ok = r.jobs[key]
		if !ok || current != job {
			r.mu.Unlock()
			return
		}
		if job.ctx.Err() == nil && job.session.HasPending() {
			r.mu.Unlock()
			continue
		}
		var failedAppends []preparedAgentRun
		if job.ctx.Err() == nil && len(job.pending) > 0 {
			batchEnd := 1
			for batchEnd < len(job.pending) && sameAuthorizationActor(job.pending[0].Authorization, job.pending[batchEnd].Authorization) {
				batchEnd++
			}
			batch := append([]preparedAgentRun(nil), job.pending[:batchEnd]...)
			job.pending = job.pending[batchEnd:]
			job.session.ClearYield()
			appended := false
			for _, next := range batch {
				next := next
				if err := job.session.AppendWithActivation(next.Request, func() {
					job.scopeStore.Activate(next.Scope, next.Authorization)
				}); err != nil {
					log.Printf("append queued agent instruction failed: %v", err)
					failedAppends = append(failedAppends, next)
					continue
				}
				appended = true
				job.actorType, job.actorID = authorizationActor(next.Authorization)
				job.lastScope = next.Scope
				job.sink = next.ReplySink
				job.errorSink = next.ErrorSink
			}
			if len(job.pending) > 0 {
				job.session.RequestYield()
			}
			if appended || len(job.pending) > 0 {
				r.mu.Unlock()
				for _, failed := range failedAppends {
					_ = sendAgentFallback(job.ctx, failed.ErrorSink)
				}
				continue
			}
		}
		job.running = false
		job.lastActiveAt = time.Now().UTC()
		job.timer = time.AfterFunc(r.idleTimeout, func() {
			r.retireIdleJob(key, job)
		})
		r.mu.Unlock()
		for _, failed := range failedAppends {
			_ = sendAgentFallback(job.ctx, failed.ErrorSink)
		}
		return
	}
}

func (r *conversationAgentRunner) retireIdleJob(key string, job *conversationAgentJob) {
	r.mu.Lock()
	current, ok := r.jobs[key]
	if !ok || current != job || job.retiring || job.running || job.session.HasPending() || len(job.pending) > 0 {
		r.mu.Unlock()
		return
	}
	generation, retireCtx := r.beginRetirementLocked(job, retirementReasonIdle)
	r.mu.Unlock()
	r.retireJob(key, job, generation, retireCtx)
}

func (r *conversationAgentRunner) activeSessionCountLocked() int {
	count := 0
	for _, job := range r.jobs {
		if !job.retiring {
			count++
		}
	}
	return count
}

func (r *conversationAgentRunner) selectOldestIdleJobLocked() (string, *conversationAgentJob) {
	var selectedKey string
	var selected *conversationAgentJob
	for key, job := range r.jobs {
		if job.retiring || job.running || job.session.HasPending() || len(job.pending) > 0 {
			continue
		}
		if selected == nil || job.lastActiveAt.Before(selected.lastActiveAt) {
			selectedKey, selected = key, job
		}
	}
	if selected == nil {
		return "", nil
	}
	return selectedKey, selected
}

func (r *conversationAgentRunner) beginRetirementLocked(job *conversationAgentJob, reason retirementReason) (uint64, context.Context) {
	if job.timer != nil {
		job.timer.Stop()
		job.timer = nil
	}
	if job.retireCancel != nil {
		job.retireCancel()
	}
	job.retiring = true
	job.retirementReason = reason
	job.retirementGeneration++
	retireCtx, cancel := context.WithCancel(r.ctx)
	job.retireCancel = cancel
	return job.retirementGeneration, retireCtx
}

func (r *conversationAgentRunner) cancelRetirementLocked(job *conversationAgentJob) {
	if job.retireCancel != nil {
		job.retireCancel()
		job.retireCancel = nil
	}
	if job.timer != nil {
		job.timer.Stop()
		job.timer = nil
	}
	job.retirementGeneration++
	job.retirementReason = ""
	job.retiring = false
}

func (r *conversationAgentRunner) retireJob(key string, job *conversationAgentJob, generation uint64, retireCtx context.Context) {
	r.mu.Lock()
	current, ok := r.jobs[key]
	if !ok || current != job || !job.retiring || job.retirementGeneration != generation {
		r.mu.Unlock()
		return
	}
	scope := job.lastScope
	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(retireCtx, 30*time.Second)
	defer cancel()
	err := closeRetiringConversation(ctx, key, scope)

	r.mu.Lock()
	current, ok = r.jobs[key]
	if !ok || current != job || !job.retiring || job.retirementGeneration != generation {
		r.mu.Unlock()
		return
	}
	if err == nil {
		delete(r.jobs, key)
		job.retireCancel()
		job.retireCancel = nil
		r.mu.Unlock()
		cancel()
		job.cancel()
		return
	}
	if errors.Is(err, context.Canceled) && retireCtx.Err() != nil {
		r.mu.Unlock()
		return
	}
	log.Printf("close retired agent topic failed: conversation_id=%s error=%v", key, err)
	job.retireCancel()
	job.retireCancel = nil
	if job.retirementReason == retirementReasonIdle {
		job.retiring = false
		job.retirementReason = ""
		job.lastActiveAt = time.Now().UTC()
		job.timer = time.AfterFunc(retirementRetryDelay, func() {
			r.retireIdleJob(key, job)
		})
		r.mu.Unlock()
		return
	}
	job.timer = time.AfterFunc(retirementRetryDelay, func() {
		r.retryCapacityRetirement(key, job)
	})
	r.mu.Unlock()
}

func (r *conversationAgentRunner) retryCapacityRetirement(key string, job *conversationAgentJob) {
	r.mu.Lock()
	current, ok := r.jobs[key]
	if !ok || current != job || !job.retiring || job.retirementReason != retirementReasonCapacity || job.retireCancel != nil {
		r.mu.Unlock()
		return
	}
	generation, retireCtx := r.beginRetirementLocked(job, retirementReasonCapacity)
	r.mu.Unlock()
	r.retireJob(key, job, generation, retireCtx)
}

func closeRetiringConversation(ctx context.Context, key string, scope builtintools.Scope) error {
	if scope.ConversationType != "topic" || scope.Requester == nil {
		return nil
	}
	return closeConversationTopic(ctx, scope.Requester, key)
}

func closeConversationTopic(ctx context.Context, requester builtintools.AppRequester, conversationID string) error {
	raw, err := requester.Request(ctx, methodConversationTopicGet, topicMutationRequestPayload{ConversationID: conversationID})
	if err != nil {
		return err
	}
	var topic topicMutationResponsePayload
	if err := json.Unmarshal(raw, &topic); err != nil {
		return err
	}
	if topic.Archived {
		return nil
	}
	_, err = requester.Request(ctx, methodConversationTopicClose, topicMutationRequestPayload{
		ConversationID: conversationID, ExpectedLastMessageSeq: topic.LastMessageSeq,
	})
	return err
}

func (r *conversationAgentRunner) CloseConversationSession(conversationID string) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return
	}
	r.mu.Lock()
	job := r.jobs[conversationID]
	if job != nil {
		delete(r.jobs, conversationID)
		if job.timer != nil {
			job.timer.Stop()
		}
		if job.retireCancel != nil {
			job.retireCancel()
			job.retireCancel = nil
		}
	}
	r.mu.Unlock()
	if job != nil {
		job.cancel()
	}
}

func (r *conversationAgentRunner) sendIfCurrent(ctx context.Context, key string, job *conversationAgentJob, delegate agent.OutputSink, content string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if delegate == nil {
		return errors.New("agent output sink unavailable")
	}

	r.mu.Lock()
	current, ok := r.jobs[key]
	r.mu.Unlock()
	if !ok || current != job {
		return context.Canceled
	}

	return delegate.SendMarkdown(ctx, content)
}

func (r *conversationAgentRunner) rejectPreparedRun(
	ctx context.Context,
	key string,
	sink agent.OutputSink,
	prepared preparedAgentRun,
	closeTopic bool,
) bool {
	errorSink := prepared.ErrorSink
	if errorSink == nil {
		errorSink = sink
	}
	sendErr := sendAgentFallback(ctx, errorSink)
	var closeErr error
	if closeTopic && prepared.Scope.ConversationType == "topic" && prepared.Scope.Requester != nil {
		closeCtx, cancel := context.WithTimeout(r.ctx, 30*time.Second)
		closeErr = closeConversationTopic(closeCtx, prepared.Scope.Requester, key)
		cancel()
		if closeErr != nil && !errors.Is(closeErr, context.Canceled) {
			log.Printf("close rejected agent topic failed: conversation_id=%s error=%v", key, closeErr)
		}
	}
	return sendErr == nil && closeErr == nil
}

type conversationAgentSink struct {
	delegate      agent.OutputSink
	errorSink     agent.OutputSink
	job           *conversationAgentJob
	key           string
	runner        *conversationAgentRunner
	taskErrorSent bool
}

func (s *conversationAgentSink) SendMarkdown(ctx context.Context, content string) error {
	target := s.delegate
	if isAgentTaskError(content) && s.errorSink != nil {
		target = s.errorSink
	}
	if err := s.runner.sendIfCurrent(ctx, s.key, s.job, target, content); err != nil {
		return err
	}
	if isAgentTaskError(content) {
		s.taskErrorSent = true
	}
	return nil
}

type taskOutputSink struct {
	delegate      agent.OutputSink
	errorSink     agent.OutputSink
	taskErrorSent bool
}

func (s *taskOutputSink) SendMarkdown(ctx context.Context, content string) error {
	target := s.delegate
	if isAgentTaskError(content) && s.errorSink != nil {
		target = s.errorSink
	}
	if target == nil {
		return errors.New("agent output sink unavailable")
	}
	if err := target.SendMarkdown(ctx, content); err != nil {
		return err
	}
	if isAgentTaskError(content) {
		s.taskErrorSent = true
	}
	return nil
}

func isAgentTaskError(content string) bool {
	return content == agent.ModelErrorFallback || content == agent.LoopLimitFallback
}

func (j *conversationAgentJob) sameActor(authorization preparedAuthorization) bool {
	actorType, actorID := authorizationActor(authorization)
	return actorType != "" && actorID != "" && actorType == j.actorType && actorID == j.actorID
}

func sameAuthorizationActor(left preparedAuthorization, right preparedAuthorization) bool {
	leftType, leftID := authorizationActor(left)
	rightType, rightID := authorizationActor(right)
	return leftType != "" && leftID != "" && leftType == rightType && leftID == rightID
}

func authorizationActor(authorization preparedAuthorization) (string, string) {
	return strings.ToLower(strings.TrimSpace(authorization.Authorization.ActorType)), strings.TrimSpace(authorization.Authorization.ActorID)
}

type conversationScopeStore struct {
	mu            sync.RWMutex
	authorization preparedAuthorization
	scope         builtintools.Scope
}

func newConversationScopeStore(scope builtintools.Scope, authorization preparedAuthorization) *conversationScopeStore {
	return &conversationScopeStore{authorization: authorization, scope: scope}
}

func (s *conversationScopeStore) Activate(scope builtintools.Scope, authorization preparedAuthorization) {
	s.mu.Lock()
	s.scope = scope
	s.authorization = authorization
	s.mu.Unlock()
}

func (s *conversationScopeStore) CurrentScope() builtintools.Scope {
	s.mu.RLock()
	scope := s.scope
	s.mu.RUnlock()
	scope.AuthorizationResolver = s
	return scope
}

func (s *conversationScopeStore) ResolveAuthorization(ref string) (builtintools.Authorization, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.authorization.Ref == "" || strings.TrimSpace(ref) != s.authorization.Ref {
		return builtintools.Authorization{}, false
	}
	return s.authorization.Authorization, true
}

func authorizationCandidatesForTrigger(value preparedAuthorization) []agent.AuthorizationCandidate {
	if value.Ref == "" {
		return nil
	}
	return []agent.AuthorizationCandidate{value.Candidate}
}

type conversationAuthorizationStore struct {
	mu      sync.RWMutex
	entries []conversationAuthorizationEntry
}

type conversationAuthorizationEntry struct {
	authorization builtintools.Authorization
	candidate     agent.AuthorizationCandidate
	ref           string
}

func newConversationAuthorizationStore() *conversationAuthorizationStore {
	return &conversationAuthorizationStore{}
}

func (s *conversationAuthorizationStore) Add(authorization preparedAuthorization) []agent.AuthorizationCandidate {
	if authorization.Ref == "" {
		return s.Candidates()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	entries := s.entries[:0]
	for _, entry := range s.entries {
		if entry.ref != authorization.Ref {
			entries = append(entries, entry)
		}
	}
	entries = append(entries, conversationAuthorizationEntry{
		authorization: authorization.Authorization,
		candidate:     authorization.Candidate,
		ref:           authorization.Ref,
	})
	if len(entries) > 5 {
		entries = entries[len(entries)-5:]
	}
	s.entries = entries

	return authorizationCandidatesFromEntries(s.entries)
}

func (s *conversationAuthorizationStore) Candidates() []agent.AuthorizationCandidate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return authorizationCandidatesFromEntries(s.entries)
}

func (s *conversationAuthorizationStore) ResolveAuthorization(ref string) (builtintools.Authorization, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.entries) - 1; i >= 0; i-- {
		entry := s.entries[i]
		if entry.ref == ref {
			return entry.authorization, true
		}
	}
	return builtintools.Authorization{}, false
}

func authorizationCandidatesFromEntries(entries []conversationAuthorizationEntry) []agent.AuthorizationCandidate {
	candidates := make([]agent.AuthorizationCandidate, 0, len(entries))
	for _, entry := range entries {
		candidates = append(candidates, entry.candidate)
	}
	return candidates
}

func filterHistoryAfterSeq(history []agent.HistoryMessage, afterSeq int64) []agent.HistoryMessage {
	if afterSeq <= 0 || len(history) == 0 {
		return history
	}
	filtered := make([]agent.HistoryMessage, 0, len(history))
	for _, message := range history {
		if message.Seq > afterSeq {
			filtered = append(filtered, message)
		}
	}
	return filtered
}

func sendAgentFallback(ctx context.Context, sink agent.OutputSink) error {
	if sink == nil {
		return errors.New("agent output sink unavailable")
	}
	if err := sink.SendMarkdown(ctx, agent.ModelErrorFallback); err != nil {
		if !errors.Is(err, context.Canceled) {
			log.Printf("send agent fallback failed: %v", err)
		}
		return err
	}
	return nil
}
