package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
)

const keychainService = "yucca"

// localAliasPattern validates aliases for local (user-defined) credentials.
// File-sourced aliases use "file:key" format and are validated separately.
var localAliasPattern = regexp.MustCompile(`^[A-Za-z0-9_.\-]+$`)

const maxAliasLength = 64

// ValidateAlias checks that a local credential alias meets naming rules.
func ValidateAlias(alias string) error {
	if alias == "" {
		return fmt.Errorf("alias is required")
	}
	if len(alias) > maxAliasLength {
		return fmt.Errorf("alias exceeds %d characters", maxAliasLength)
	}
	if !localAliasPattern.MatchString(alias) {
		return fmt.Errorf("alias contains invalid characters (allowed: A-Z a-z 0-9 _ - .)")
	}
	return nil
}

type Store struct {
	baseDir string
	mu      sync.RWMutex
}

type BorrowMatch struct {
	Project ProjectInfo
	Meta    CredentialMeta
}

func New(baseDir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(baseDir, "projects"), 0700); err != nil {
		return nil, fmt.Errorf("create store dir: %w", err)
	}
	s := &Store{baseDir: baseDir}
	s.migrateHashDirs()
	return s, nil
}

// migrateHashDirs renames old hash-based project directories to slug-based ones.
func (s *Store) migrateHashDirs() {
	projectsDir := filepath.Join(s.baseDir, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		oldDir := filepath.Join(projectsDir, e.Name())
		raw, err := os.ReadFile(filepath.Join(oldDir, "project.json"))
		if err != nil {
			continue
		}
		var info ProjectInfo
		if err := json.Unmarshal(raw, &info); err != nil || info.Path == "" {
			continue
		}
		newSlug := projectSlug(info.Path)
		if e.Name() == newSlug {
			continue // already migrated
		}
		newDir := filepath.Join(projectsDir, newSlug)
		if err := os.Rename(oldDir, newDir); err != nil {
			continue
		}
		// Migrate keychain entries from old hash key to new slug key
		credRaw, err := os.ReadFile(filepath.Join(newDir, "credentials.json"))
		if err == nil {
			var creds map[string]CredentialMeta
			if json.Unmarshal(credRaw, &creds) == nil {
				oldKey := e.Name() // the old hash-based dir name
				for alias := range creds {
					if val, err := keyring.Get(keychainService, keychainKey(oldKey, alias)); err == nil {
						_ = keyring.Set(keychainService, keychainKey(newSlug, alias), val)
						_ = keyring.Delete(keychainService, keychainKey(oldKey, alias))
					}
				}
			}
		}
		// Rewrite project.json with correct slug field
		info.Slug = newSlug
		data, _ := json.MarshalIndent(info, "", "  ")
		os.WriteFile(filepath.Join(newDir, "project.json"), data, 0600)
	}
}

// projectSlug converts a project path to a slug by replacing "/" with "-".
// e.g. "/Users/marek/Projects/foo" → "-Users-marek-Projects-foo"
func projectSlug(path string) string {
	return strings.ReplaceAll(path, "/", "-")
}

// ProjectSlug returns the slug identifier for a given project path.
func (s *Store) ProjectSlug(path string) string {
	return projectSlug(path)
}

// ProjectSlugFromPath returns the slug for a path without needing a Store instance.
func ProjectSlugFromPath(path string) string {
	return projectSlug(path)
}

func (s *Store) projectDir(projectPath string) string {
	return filepath.Join(s.baseDir, "projects", projectSlug(projectPath))
}

func keychainKey(slug, alias string) string {
	return fmt.Sprintf("%s:%s", slug, alias)
}

func (s *Store) loadProject(projectPath string) (*ProjectData, error) {
	dir := s.projectDir(projectPath)
	data := &ProjectData{
		Info: ProjectInfo{
			Path: projectPath,
			Name: filepath.Base(projectPath),
			Slug: projectSlug(projectPath),
		},
		Credentials: make(map[string]CredentialMeta),
	}

	// Read project.json to get the saved name
	if infoRaw, err := os.ReadFile(filepath.Join(dir, "project.json")); err == nil {
		var saved ProjectInfo
		if json.Unmarshal(infoRaw, &saved) == nil && saved.Name != "" {
			data.Info.Name = saved.Name
		}
	}

	metaFile := filepath.Join(dir, "credentials.json")
	raw, err := os.ReadFile(metaFile)
	if err != nil {
		if os.IsNotExist(err) {
			return data, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(raw, &data.Credentials); err != nil {
		return nil, err
	}
	return data, nil
}

// SetProjectName updates the project's display name.
func (s *Store) SetProjectName(projectPath, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadProject(projectPath)
	if err != nil {
		return err
	}
	data.Info.Name = name
	return s.saveProject(projectPath, data)
}

// ProjectName returns the display name for a project.
func (s *Store) ProjectName(projectPath string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := s.loadProject(projectPath)
	if err != nil {
		return filepath.Base(projectPath)
	}
	return data.Info.Name
}

func (s *Store) saveProject(projectPath string, data *ProjectData) error {
	dir := s.projectDir(projectPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	infoRaw, _ := json.MarshalIndent(data.Info, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "project.json"), infoRaw, 0600); err != nil {
		return err
	}

	metaRaw, _ := json.MarshalIndent(data.Credentials, "", "  ")
	return os.WriteFile(filepath.Join(dir, "credentials.json"), metaRaw, 0600)
}

func (s *Store) SetCredential(projectPath, alias, value string, policy ApprovalPolicy) error {
	return s.SetCredentialWithSource(projectPath, alias, value, policy, CredentialSource{Type: "local"})
}

func (s *Store) SetCredentialWithSource(projectPath, alias, value string, policy ApprovalPolicy, source CredentialSource) error {
	// Validate alias for local (user-defined) credentials only
	if source.Type == "local" {
		if err := ValidateAlias(alias); err != nil {
			return err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadProject(projectPath)
	if err != nil {
		return err
	}

	now := time.Now()
	meta, exists := data.Credentials[alias]
	if !exists {
		meta = CredentialMeta{
			Alias:     alias,
			CreatedAt: now,
		}
	}
	meta.Policy = policy
	meta.Source = source
	meta.UpdatedAt = now
	data.Credentials[alias] = meta

	if err := s.saveProject(projectPath, data); err != nil {
		return err
	}

	slug := projectSlug(projectPath)
	return keyring.Set(keychainService, keychainKey(slug, alias), value)
}

func (s *Store) GetCredential(projectPath, alias string) (string, CredentialMeta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := s.loadProject(projectPath)
	if err != nil {
		return "", CredentialMeta{}, err
	}

	meta, ok := data.Credentials[alias]
	if !ok {
		return "", CredentialMeta{}, fmt.Errorf("credential %q not found for project %q", alias, projectPath)
	}

	slug := projectSlug(projectPath)
	val, err := keyring.Get(keychainService, keychainKey(slug, alias))
	if err != nil {
		return "", CredentialMeta{}, fmt.Errorf("keychain get: %w", err)
	}

	return val, meta, nil
}

func (s *Store) DeleteCredential(projectPath, alias string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadProject(projectPath)
	if err != nil {
		return err
	}

	delete(data.Credentials, alias)
	if err := s.saveProject(projectPath, data); err != nil {
		return err
	}

	slug := projectSlug(projectPath)
	return keyring.Delete(keychainService, keychainKey(slug, alias))
}

func (s *Store) SetCredentialContext(projectPath, alias, context string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadProject(projectPath)
	if err != nil {
		return err
	}

	meta, ok := data.Credentials[alias]
	if !ok {
		return fmt.Errorf("credential %q not found for project %q", alias, projectPath)
	}

	meta.Context = context
	meta.UpdatedAt = time.Now()
	data.Credentials[alias] = meta

	return s.saveProject(projectPath, data)
}

func (s *Store) HasCredential(projectPath, alias string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := s.loadProject(projectPath)
	if err != nil {
		return false
	}
	_, ok := data.Credentials[alias]
	return ok
}

func (s *Store) UpdatePolicy(projectPath, alias string, policy ApprovalPolicy) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadProject(projectPath)
	if err != nil {
		return err
	}

	meta, ok := data.Credentials[alias]
	if !ok {
		return fmt.Errorf("credential %q not found for project %q", alias, projectPath)
	}

	meta.Policy = policy
	meta.UpdatedAt = time.Now()
	data.Credentials[alias] = meta

	return s.saveProject(projectPath, data)
}

func (s *Store) CopyCredential(fromProjectPath, toProjectPath, alias string) error {
	val, meta, err := s.GetCredential(fromProjectPath, alias)
	if err != nil {
		return err
	}
	return s.SetCredential(toProjectPath, alias, val, meta.Policy)
}

func (s *Store) ListProjects() ([]ProjectInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listProjectsLocked()
}

// listProjectsLocked is the body of ListProjects without acquiring the mutex,
// so callers that already hold s.mu (e.g. FindCredentialAcrossProjects) can
// reuse it without a re-entrant RLock — which would deadlock against a pending
// writer.
func (s *Store) listProjectsLocked() ([]ProjectInfo, error) {
	projectsDir := filepath.Join(s.baseDir, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	seen := make(map[string]bool)
	var projects []ProjectInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(projectsDir, e.Name(), "project.json"))
		if err != nil {
			continue
		}
		var info ProjectInfo
		if err := json.Unmarshal(raw, &info); err != nil {
			continue
		}
		// Always recompute slug from path (handles migration from old hash format)
		info.Slug = projectSlug(info.Path)
		if seen[info.Path] {
			continue
		}
		seen[info.Path] = true
		projects = append(projects, info)
	}
	return projects, nil
}

func (s *Store) ListCredentials(projectPath string) (map[string]CredentialMeta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := s.loadProject(projectPath)
	if err != nil {
		return nil, err
	}
	return data.Credentials, nil
}

// DeleteProject removes a project's metadata directory and all keychain entries.
func (s *Store) DeleteProject(projectPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadProject(projectPath)
	if err != nil {
		return err
	}

	slug := projectSlug(projectPath)
	for alias := range data.Credentials {
		_ = keyring.Delete(keychainService, keychainKey(slug, alias))
	}

	dir := s.projectDir(projectPath)
	return os.RemoveAll(dir)
}

func (s *Store) SearchCredentials(projectPath, query string) []CredentialMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := s.loadProject(projectPath)
	if err != nil {
		return nil
	}

	q := strings.ToLower(query)
	var results []CredentialMeta
	for _, meta := range data.Credentials {
		if strings.Contains(strings.ToLower(meta.Alias), q) ||
			strings.Contains(strings.ToLower(meta.Context), q) {
			results = append(results, meta)
		}
	}
	return results
}

func (s *Store) FindCredentialAcrossProjects(alias, excludeProjectPath string) ([]BorrowMatch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	projects, err := s.listProjectsLocked()
	if err != nil {
		return nil, err
	}

	var matches []BorrowMatch
	for _, p := range projects {
		if p.Path == excludeProjectPath {
			continue
		}
		data, err := s.loadProject(p.Path)
		if err != nil {
			continue
		}
		if meta, ok := data.Credentials[alias]; ok {
			matches = append(matches, BorrowMatch{Project: p, Meta: meta})
		}
	}
	return matches, nil
}
