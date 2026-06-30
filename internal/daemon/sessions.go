package daemon

import (
	"sync"
	"time"
)

// ActiveSession represents a running Claude MCP session for a project.
type ActiveSession struct {
	ProjectSlug string    `json:"project_slug"`
	ProjectPath string    `json:"project_path"`
	ProjectName string    `json:"project_name"`
	LastSeen    time.Time `json:"last_seen"`
}

// SessionTracker tracks active MCP sessions with heartbeat-based expiry.
type SessionTracker struct {
	mu       sync.RWMutex
	sessions map[string]*ActiveSession // keyed by project slug
	timeout  time.Duration
	onReap   func(reaped []string) // called with the slugs of reaped sessions
}

func NewSessionTracker(timeout time.Duration, onReap func(reaped []string)) *SessionTracker {
	st := &SessionTracker{
		sessions: make(map[string]*ActiveSession),
		timeout:  timeout,
		onReap:   onReap,
	}
	go st.reapLoop()
	return st
}

// SessionApprovals remembers which (project slug, alias) ask_session credentials
// the user approved during the current agent session, so yucca_exec /
// yucca_clipboard prompt once per session instead of on every call. Entries
// are cleared when the project's session is deregistered or reaped.
type SessionApprovals struct {
	mu     sync.Mutex
	bySlug map[string]map[string]bool
}

func NewSessionApprovals() *SessionApprovals {
	return &SessionApprovals{bySlug: make(map[string]map[string]bool)}
}

func (a *SessionApprovals) Approve(slug, alias string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.bySlug[slug] == nil {
		a.bySlug[slug] = make(map[string]bool)
	}
	a.bySlug[slug][alias] = true
}

func (a *SessionApprovals) Approved(slug, alias string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.bySlug[slug][alias]
}

func (a *SessionApprovals) ClearProject(slug string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.bySlug, slug)
}

// Register adds or refreshes a session.
// Register adds a new session.
func (st *SessionTracker) Register(slug, path, name string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.sessions[slug] = &ActiveSession{
		ProjectSlug: slug,
		ProjectPath: path,
		ProjectName: name,
		LastSeen:    time.Now(),
	}
}

// PathForSlug returns the project path of an active session, or "".
func (st *SessionTracker) PathForSlug(slug string) string {
	st.mu.RLock()
	defer st.mu.RUnlock()
	if s, ok := st.sessions[slug]; ok {
		return s.ProjectPath
	}
	return ""
}

// Heartbeat refreshes LastSeen for an existing session. Returns false if session not found.
func (st *SessionTracker) Heartbeat(slug string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	s, ok := st.sessions[slug]
	if ok {
		s.LastSeen = time.Now()
	}
	return ok
}

// Deregister removes a session.
func (st *SessionTracker) Deregister(slug string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.sessions, slug)
}

// Active returns all non-expired sessions. Always returns a non-nil slice
// so JSON marshaling produces [] instead of null.
func (st *SessionTracker) Active() []ActiveSession {
	st.mu.RLock()
	defer st.mu.RUnlock()
	cutoff := time.Now().Add(-st.timeout)
	out := make([]ActiveSession, 0)
	for _, s := range st.sessions {
		if s.LastSeen.After(cutoff) {
			out = append(out, *s)
		}
	}
	return out
}

func (st *SessionTracker) reapLoop() {
	ticker := time.NewTicker(st.timeout / 2)
	defer ticker.Stop()
	for range ticker.C {
		st.mu.Lock()
		var reaped []string
		cutoff := time.Now().Add(-st.timeout)
		for slug, s := range st.sessions {
			if s.LastSeen.Before(cutoff) {
				delete(st.sessions, slug)
				reaped = append(reaped, slug)
			}
		}
		st.mu.Unlock()
		if len(reaped) > 0 && st.onReap != nil {
			st.onReap(reaped)
		}
	}
}
