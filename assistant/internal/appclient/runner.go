package appclient

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"assistant/internal/agent"
	"assistant/internal/builtintools"
)

const maxConversationSequenceWatermarks = 10_000

type agentRunner interface {
	Start(context.Context, string, agent.OutputSink, replyAgent, preparedAgentRun) bool
}

type preparedAgentRun struct {
	Authorization preparedAuthorization
	MessageSeq    int64
	Request       agent.Request
	Scope         builtintools.Scope
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
	if err := assistantAgent.Run(builtintools.WithScope(ctx, prepared.Scope), prepared.Request, sink); err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		log.Printf("agent reply failed: %v", err)
		return sendAgentFallback(ctx, sink) == nil
	}
	return true
}

type conversationAgentRunner struct {
	ctx              context.Context
	idleTimeout      time.Duration
	mu               sync.Mutex
	jobs             map[string]*conversationAgentJob
	lastSeenSeq      map[string]int64
	lastSeenSeqOrder []string
	waiters          *conversationWaitRegistry
}

type conversationAgentJob struct {
	authorizations *conversationAuthorizationStore
	cancel         context.CancelFunc
	ctx            context.Context
	lastSeenSeq    int64
	running        bool
	scope          builtintools.Scope
	session        *agent.Session
	sink           agent.OutputSink
	timer          *time.Timer
}

func newConversationAgentRunner(ctx context.Context) *conversationAgentRunner {
	if ctx == nil {
		ctx = context.Background()
	}
	return &conversationAgentRunner{
		ctx:         ctx,
		idleTimeout: time.Hour,
		jobs:        map[string]*conversationAgentJob{},
		lastSeenSeq: map[string]int64{},
		waiters:     newConversationWaitRegistry(),
	}
}

func (r *conversationAgentRunner) Start(ctx context.Context, key string, sink agent.OutputSink, assistantAgent replyAgent, prepared preparedAgentRun) bool {
	if key == "" {
		key = "unknown"
	}
	prepared.Scope.ConversationWaiter = r.waiters
	r.mu.Lock()
	if prepared.MessageSeq > 0 && prepared.MessageSeq <= r.lastSeenSeq[key] {
		r.mu.Unlock()
		return true
	}
	sessionAgent, ok := assistantAgent.(sessionReplyAgent)
	if !ok {
		r.mu.Unlock()
		accepted := directAgentRunner{}.Start(ctx, key, sink, assistantAgent, prepared)
		if accepted {
			r.mu.Lock()
			r.recordSequenceLocked(key, prepared.MessageSeq)
			r.mu.Unlock()
		}
		return accepted
	}

	if job, ok := r.jobs[key]; ok {
		if prepared.MessageSeq > 0 && prepared.MessageSeq <= job.lastSeenSeq {
			r.mu.Unlock()
			return true
		}
		if job.timer != nil {
			job.timer.Stop()
			job.timer = nil
		}
		request := prepared.Request
		request.History = filterHistoryAfterSeq(request.History, job.lastSeenSeq)
		request.AuthorizationCandidates = job.authorizations.Add(prepared.Authorization)
		if err := job.session.Append(request); err != nil {
			r.mu.Unlock()
			log.Printf("append agent instruction failed: %v", err)
			return sendAgentFallback(ctx, sink) == nil
		}
		if prepared.MessageSeq > job.lastSeenSeq {
			job.lastSeenSeq = prepared.MessageSeq
		}
		r.recordSequenceLocked(key, prepared.MessageSeq)
		if !job.running {
			job.running = true
			go r.runJob(key, job)
		}
		r.mu.Unlock()
		return true
	}

	jobCtx, cancel := context.WithCancel(r.ctx)
	authorizations := newConversationAuthorizationStore()
	prepared.Request.AuthorizationCandidates = authorizations.Add(prepared.Authorization)
	prepared.Scope.AuthorizationResolver = authorizations
	session, err := sessionAgent.NewSession(prepared.Request)
	if err != nil {
		r.mu.Unlock()
		cancel()
		log.Printf("create agent session failed: %v", err)
		return sendAgentFallback(ctx, sink) == nil
	}
	job := &conversationAgentJob{
		authorizations: authorizations,
		cancel:         cancel,
		ctx:            jobCtx,
		lastSeenSeq:    prepared.MessageSeq,
		running:        true,
		scope:          prepared.Scope,
		session:        session,
		sink:           sink,
	}
	r.jobs[key] = job
	r.recordSequenceLocked(key, prepared.MessageSeq)
	r.mu.Unlock()

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
		job.cancel()
	}
}

func (r *conversationAgentRunner) runJob(key string, job *conversationAgentJob) {
	for {
		err := job.session.RunCycle(
			builtintools.WithScope(job.ctx, job.scope),
			conversationAgentSink{
				delegate: job.sink,
				job:      job,
				key:      key,
				runner:   r,
			},
		)
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("agent reply failed: %v", err)
		}

		r.mu.Lock()
		current, ok := r.jobs[key]
		if !ok || current != job {
			r.mu.Unlock()
			return
		}
		if job.ctx.Err() == nil && job.session.HasPending() {
			r.mu.Unlock()
			continue
		}
		job.running = false
		job.timer = time.AfterFunc(r.idleTimeout, func() {
			r.clearIdleJob(key, job)
		})
		r.mu.Unlock()
		return
	}
}

func (r *conversationAgentRunner) clearIdleJob(key string, job *conversationAgentJob) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.jobs[key]
	if ok && current == job && !job.running && !job.session.HasPending() {
		delete(r.jobs, key)
		job.cancel()
	}
}

func (r *conversationAgentRunner) sendIfCurrent(ctx context.Context, key string, job *conversationAgentJob, delegate agent.OutputSink, content string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	current, ok := r.jobs[key]
	r.mu.Unlock()
	if !ok || current != job {
		return context.Canceled
	}

	return delegate.SendMarkdown(ctx, content)
}

type conversationAgentSink struct {
	delegate agent.OutputSink
	job      *conversationAgentJob
	key      string
	runner   *conversationAgentRunner
}

func (s conversationAgentSink) SendMarkdown(ctx context.Context, content string) error {
	return s.runner.sendIfCurrent(ctx, s.key, s.job, s.delegate, content)
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
