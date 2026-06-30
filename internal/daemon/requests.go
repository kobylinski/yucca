package daemon

import (
	"sync"
	"time"

	"github.com/kobylinski/yucca/internal/store"
)

type RequestStatus string

const (
	StatusPending  RequestStatus = "pending"
	StatusApproved RequestStatus = "approved"
	StatusDenied   RequestStatus = "denied"
)

type RequestKind string

const (
	KindExecuteAccept RequestKind = "execute_accept"
	KindSecretRequest RequestKind = "secret_request"
	KindClipboardCopy RequestKind = "clipboard_copy"
)

type SecretRequest struct {
	ID          string               `json:"id"`
	Kind        RequestKind          `json:"kind"`
	Alias       string               `json:"alias,omitempty"`   // set for secret_request
	Aliases     []string             `json:"aliases,omitempty"` // set for execute_accept
	Reason      string               `json:"reason"`
	ProjectPath string               `json:"project_path"`
	ProjectName string               `json:"project_name"`
	ProjectSlug string               `json:"project_slug"`
	Status      RequestStatus        `json:"status"`
	Policy      store.ApprovalPolicy `json:"policy,omitempty"`
	CreatedAt   time.Time            `json:"created_at"`
	ResolvedAt  *time.Time           `json:"resolved_at,omitempty"`
}

type RequestQueue struct {
	mu       sync.RWMutex
	requests map[string]*SecretRequest
	notify   map[string]chan struct{}
}

func NewRequestQueue() *RequestQueue {
	return &RequestQueue{
		requests: make(map[string]*SecretRequest),
		notify:   make(map[string]chan struct{}),
	}
}

func (q *RequestQueue) Add(req *SecretRequest) chan struct{} {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.requests[req.ID] = req
	ch := make(chan struct{}, 1)
	q.notify[req.ID] = ch
	return ch
}

// Get returns a copy of the request so callers can read/marshal it without
// racing the in-place mutations Resolve performs under the lock.
func (q *RequestQueue) Get(id string) (*SecretRequest, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	r, ok := q.requests[id]
	if !ok {
		return nil, false
	}
	cp := *r
	return &cp, true
}

// Resolve sets the request's terminal status and policy under the lock and
// wakes any waiter. Folding the policy write in here (rather than mutating a
// returned pointer in the handler) keeps all writes serialized.
func (q *RequestQueue) Resolve(id string, status RequestStatus, policy store.ApprovalPolicy) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if r, ok := q.requests[id]; ok {
		now := time.Now()
		r.Status = status
		r.Policy = policy
		r.ResolvedAt = &now
		if ch, ok := q.notify[id]; ok {
			close(ch)
			delete(q.notify, id)
		}
	}
}

// Pending returns copies of all pending requests.
func (q *RequestQueue) Pending() []*SecretRequest {
	q.mu.RLock()
	defer q.mu.RUnlock()
	out := make([]*SecretRequest, 0)
	for _, r := range q.requests {
		if r.Status == StatusPending {
			cp := *r
			out = append(out, &cp)
		}
	}
	return out
}
