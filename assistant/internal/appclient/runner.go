package appclient

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"assistant/internal/agent"
	"assistant/internal/builtintools"
)

type agentRunner interface {
	Start(context.Context, string, agent.OutputSink, replyAgent, preparedAgentRun)
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

func (directAgentRunner) Start(ctx context.Context, key string, sink agent.OutputSink, assistantAgent replyAgent, prepared preparedAgentRun) {
	store := newConversationAuthorizationStore()
	prepared.Request.AuthorizationCandidates = store.Add(prepared.Authorization)
	prepared.Scope.AuthorizationResolver = store
	if err := assistantAgent.Run(builtintools.WithScope(ctx, prepared.Scope), prepared.Request, sink); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("agent reply failed: %v", err)
	}
}

type conversationAgentRunner struct {
	ctx         context.Context
	idleTimeout time.Duration
	mu          sync.Mutex
	jobs        map[string]*conversationAgentJob
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
	}
}

func (r *conversationAgentRunner) Start(ctx context.Context, key string, sink agent.OutputSink, assistantAgent replyAgent, prepared preparedAgentRun) {
	if key == "" {
		key = "unknown"
	}
	sessionAgent, ok := assistantAgent.(sessionReplyAgent)
	if !ok {
		directAgentRunner{}.Start(ctx, key, sink, assistantAgent, prepared)
		return
	}

	r.mu.Lock()
	if job, ok := r.jobs[key]; ok {
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
			sendAgentFallback(ctx, sink)
			return
		}
		if prepared.MessageSeq > job.lastSeenSeq {
			job.lastSeenSeq = prepared.MessageSeq
		}
		if !job.running {
			job.running = true
			go r.runJob(key, job)
		}
		r.mu.Unlock()
		return
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
		sendAgentFallback(ctx, sink)
		return
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
	r.mu.Unlock()

	go r.runJob(key, job)
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

func sendAgentFallback(ctx context.Context, sink agent.OutputSink) {
	if sink == nil {
		return
	}
	if err := sink.SendMarkdown(ctx, agent.ModelErrorFallback); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("send agent fallback failed: %v", err)
	}
}
