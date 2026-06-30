package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// notesFile is the per-project notes store, kept separate from credentials.json
// so note text (non-secret) never mingles with the credential metadata or the
// keychain-backed value paths.
func (s *Store) notesFile(projectPath string) string {
	return filepath.Join(s.projectDir(projectPath), "notes.json")
}

func (s *Store) loadNotes(projectPath string) (map[string]Note, error) {
	raw, err := os.ReadFile(s.notesFile(projectPath))
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]Note), nil
		}
		return nil, err
	}
	notes := make(map[string]Note)
	if err := json.Unmarshal(raw, &notes); err != nil {
		return nil, err
	}
	return notes, nil
}

func (s *Store) saveNotes(projectPath string, notes map[string]Note) error {
	dir := s.projectDir(projectPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(notes, "", "  ")
	return os.WriteFile(s.notesFile(projectPath), raw, 0600)
}

// SetNote creates or updates a standalone note (non-secret free text).
func (s *Store) SetNote(projectPath, alias, body string) error {
	if err := ValidateAlias(alias); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	notes, err := s.loadNotes(projectPath)
	if err != nil {
		return err
	}

	now := time.Now()
	n, exists := notes[alias]
	if !exists {
		n = Note{Alias: alias, CreatedAt: now}
	}
	n.Body = body
	n.UpdatedAt = now
	notes[alias] = n

	return s.saveNotes(projectPath, notes)
}

// GetNote returns a single note by alias.
func (s *Store) GetNote(projectPath, alias string) (Note, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	notes, err := s.loadNotes(projectPath)
	if err != nil {
		return Note{}, err
	}
	n, ok := notes[alias]
	if !ok {
		return Note{}, fmt.Errorf("note %q not found for project %q", alias, projectPath)
	}
	return n, nil
}

// ListNotes returns all notes for a project, most-recently-updated first.
func (s *Store) ListNotes(projectPath string) ([]Note, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	notes, err := s.loadNotes(projectPath)
	if err != nil {
		return nil, err
	}
	result := make([]Note, 0, len(notes))
	for _, n := range notes {
		result = append(result, n)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return result, nil
}

// DeleteNote removes a note by alias. Deleting a missing note is a no-op.
func (s *Store) DeleteNote(projectPath, alias string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	notes, err := s.loadNotes(projectPath)
	if err != nil {
		return err
	}
	delete(notes, alias)
	return s.saveNotes(projectPath, notes)
}
