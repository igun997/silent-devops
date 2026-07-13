package registry

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
)

var (
	ErrDuplicateStream  = errors.New("duplicate agent stream")
	ErrIdentityMismatch = errors.New("certificate and hello identity mismatch")
	ErrVersionMismatch  = errors.New("protocol version mismatch")
	ErrLimitsExceeded   = errors.New("protocol limits exceeded")
	ErrAgentOffline     = errors.New("agent offline")
)

type Snapshot struct {
	AgentID  string
	Online   bool
	LastSeen time.Time
	Hello    *devopsv1.AgentHello
}
type entry struct {
	generation uint64
	online     bool
	lastSeen   time.Time
	hello      *devopsv1.AgentHello
	session    *Session
}
type Registry struct {
	mu         sync.Mutex
	entries    map[string]*entry
	generation uint64
	version    uint32
	limits     *devopsv1.ProtocolLimits
	timeout    time.Duration
}
type Session struct {
	registry   *Registry
	agentID    string
	generation uint64
	queue      chan *devopsv1.ValidatorMessage
	once       sync.Once
}

func New(version uint32, limits *devopsv1.ProtocolLimits, timeout time.Duration) *Registry {
	if limits == nil {
		limits = devopsv1.DefaultLimits()
	}
	return &Registry{entries: make(map[string]*entry), version: version, limits: limits, timeout: timeout}
}
func (r *Registry) Acquire(certID, helloID string, h *devopsv1.AgentHello, now time.Time) (*Session, *devopsv1.ConnectionState, error) {
	if h == nil || certID == "" || certID != helloID || helloID != h.AgentId {
		return nil, nil, ErrIdentityMismatch
	}
	if h.Protocol == nil || r.version < h.Protocol.Minimum || r.version > h.Protocol.Maximum {
		return nil, nil, ErrVersionMismatch
	}
	if uint32(len(h.Capabilities)) > r.limits.MaxCapabilities {
		return nil, nil, ErrLimitsExceeded
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if current := r.entries[certID]; current != nil && current.online {
		return nil, nil, ErrDuplicateStream
	}
	r.generation++
	e := &entry{generation: r.generation, online: true, lastSeen: now, hello: h}
	r.entries[certID] = e
	depth := r.limits.MaxQueueDepth
	if depth == 0 {
		depth = 1
	}
	s := &Session{registry: r, agentID: certID, generation: e.generation, queue: make(chan *devopsv1.ValidatorMessage, depth)}
	e.session = s
	return s, &devopsv1.ConnectionState{Accepted: true, ProtocolVersion: r.version, DuplicateStreamPolicy: devopsv1.DuplicateStreamPolicy_DUPLICATE_STREAM_POLICY_REJECT_NEW, DuplicateJobPolicy: devopsv1.DuplicateJobPolicy_DUPLICATE_JOB_POLICY_REJECT, NegotiatedLimits: negotiate(r.limits, h.Limits)}, nil
}
func negotiate(server, agent *devopsv1.ProtocolLimits) *devopsv1.ProtocolLimits {
	if agent == nil {
		return server
	}
	out := &devopsv1.ProtocolLimits{
		MaxMessageBytes: server.MaxMessageBytes, MaxOutputBytes: server.MaxOutputBytes,
		MaxOutputChunkBytes: server.MaxOutputChunkBytes, MaxMetrics: server.MaxMetrics,
		MaxLabelsPerMetric: server.MaxLabelsPerMetric, MaxCapabilities: server.MaxCapabilities,
		MaxQueueDepth: server.MaxQueueDepth, MaxReasonBytes: server.MaxReasonBytes,
		MaxCommandBytes: server.MaxCommandBytes, MaxUnitBytes: server.MaxUnitBytes,
		MaxPathBytes: server.MaxPathBytes, MaxJobTimeoutSeconds: server.MaxJobTimeoutSeconds,
		MaxSshTtlSeconds: server.MaxSshTtlSeconds, MaxPageSize: server.MaxPageSize,
	}
	if agent.MaxMessageBytes > 0 && agent.MaxMessageBytes < out.MaxMessageBytes {
		out.MaxMessageBytes = agent.MaxMessageBytes
	}
	if agent.MaxQueueDepth > 0 && agent.MaxQueueDepth < out.MaxQueueDepth {
		out.MaxQueueDepth = agent.MaxQueueDepth
	}
	if agent.MaxCapabilities > 0 && agent.MaxCapabilities < out.MaxCapabilities {
		out.MaxCapabilities = agent.MaxCapabilities
	}
	return out
}
func (s *Session) Enqueue(ctx context.Context, m *devopsv1.ValidatorMessage) error {
	select {
	case s.queue <- m:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Session) Messages() <-chan *devopsv1.ValidatorMessage { return s.queue }
func (s *Session) Heartbeat(now time.Time)                     { s.registry.touch(s.agentID, s.generation, now) }
func (s *Session) Release(now time.Time) {
	s.once.Do(func() { s.registry.release(s.agentID, s.generation, now); close(s.queue) })
}
func (r *Registry) touch(id string, g uint64, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.entries[id]; e != nil && e.generation == g && e.online {
		e.lastSeen = now
	}
}
func (r *Registry) release(id string, g uint64, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.entries[id]; e != nil && e.generation == g {
		e.online = false
		e.lastSeen = now
	}
}
func (r *Registry) Dispatch(ctx context.Context, id string, message *devopsv1.ValidatorMessage) error {
	r.mu.Lock()
	e := r.entries[id]
	if e == nil || !e.online || e.session == nil {
		r.mu.Unlock()
		return ErrAgentOffline
	}
	session := e.session
	r.mu.Unlock()
	return session.Enqueue(ctx, message)
}

func (r *Registry) Snapshot(id string) Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.entries[id]
	if e == nil {
		return Snapshot{AgentID: id}
	}
	return Snapshot{AgentID: id, Online: e.online, LastSeen: e.lastSeen, Hello: e.hello}
}
func (r *Registry) Expire(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.entries {
		if e.online && now.Sub(e.lastSeen) > r.timeout {
			e.online = false
		}
	}
}

type Backoff struct {
	min, max, current time.Duration
	rng               *rand.Rand
}

func NewBackoff(min, max time.Duration, seed int64) *Backoff {
	return &Backoff{min: min, max: max, rng: rand.New(rand.NewSource(seed))}
}
func (b *Backoff) Next() time.Duration {
	limit := b.current
	if limit == 0 {
		limit = b.min
	} else {
		limit *= 2
		if limit > b.max {
			limit = b.max
		}
	}
	b.current = limit
	if limit <= 0 {
		return 0
	}
	return time.Duration(b.rng.Int63n(int64(limit) + 1))
}
func (b *Backoff) Reset() { b.current = 0 }
