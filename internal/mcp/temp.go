package mcp

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// tempEntry is a session-scoped value held ONLY in this MCP process's memory.
// It never reaches disk, the keychain, or the daemon's store. When the process
// exits (the Claude session/connection ends) the OS reclaims the memory and the
// value is gone — that process lifetime IS the "session" the entry is scoped to.
type tempEntry struct {
	value     string
	isSecret  bool
	createdAt time.Time
}

type tempStore struct {
	mu      sync.Mutex
	entries map[string]*tempEntry
}

func newTempStore() *tempStore {
	return &tempStore{entries: make(map[string]*tempEntry)}
}

// Put creates or overwrites a temporary entry. The process is the namespace, so
// entries are keyed by alias alone.
func (t *tempStore) Put(alias, value string, isSecret bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries[alias] = &tempEntry{value: value, isSecret: isSecret, createdAt: time.Now()}
}

// SecretValues returns all temporary SECRET alias→value pairs.
func (t *tempStore) SecretValues() map[string]string {
	t.mu.Lock()
	defer t.mu.Unlock()
	m := make(map[string]string)
	for a, e := range t.entries {
		if e.isSecret {
			m[a] = e.value
		}
	}
	return m
}

// SecretsReferencedIn returns the temp secrets whose {{YUCCA:alias}} placeholder
// appears in cmd — the only ones that need to be ferried to the daemon for one exec.
func (t *tempStore) SecretsReferencedIn(cmd string) map[string]string {
	t.mu.Lock()
	defer t.mu.Unlock()
	m := make(map[string]string)
	for a, e := range t.entries {
		if e.isSecret && strings.Contains(cmd, "{{YUCCA:"+a+"}}") {
			m[a] = e.value
		}
	}
	return m
}

// SecretAliases lists temp secret aliases (no values), for the index display.
func (t *tempStore) SecretAliases() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []string
	for a, e := range t.entries {
		if e.isSecret {
			out = append(out, a)
		}
	}
	sort.Strings(out)
	return out
}

type tempNote struct{ Alias, Body string }

// Notes lists temp notes (non-secret) with their text.
func (t *tempStore) Notes() []tempNote {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []tempNote
	for a, e := range t.entries {
		if !e.isSecret {
			out = append(out, tempNote{Alias: a, Body: e.value})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })
	return out
}

// withTempRedaction merges temp secret values into a redaction map so a temp
// secret appearing in file content is scrubbed before the model ever sees it.
func (s *Server) withTempRedaction(secrets map[string]string) map[string]string {
	tv := s.temp.SecretValues()
	if len(tv) == 0 {
		return secrets
	}
	m := make(map[string]string, len(secrets)+len(tv))
	for k, v := range secrets {
		m[k] = v
	}
	for k, v := range tv {
		m[k] = v
	}
	return m
}

// tempPlaceholderIn returns the first temp-secret alias whose placeholder appears
// in content, or "". Temp secrets must NEVER be rehydrated into a file — that
// would persist them to disk and outlive the session — so such writes are refused.
func (s *Server) tempPlaceholderIn(content string) string {
	for _, alias := range s.temp.SecretAliases() {
		if strings.Contains(content, "{{YUCCA:"+alias+"}}") {
			return alias
		}
	}
	return ""
}
