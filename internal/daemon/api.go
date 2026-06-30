package daemon

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kobylinski/yucca/internal/clipboard"
	yuccaExec "github.com/kobylinski/yucca/internal/exec"
	"github.com/kobylinski/yucca/internal/fuzzy"
	"github.com/kobylinski/yucca/internal/store"
)

// clipboardClearAfter is how long a secret stays on the clipboard before
// Yucca clears it — but only if the clipboard still holds that exact value,
// so a copy the user made in the meantime is never wiped.
const clipboardClearAfter = 30 * time.Second

func (d *Daemon) registerAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", d.handleHealth)
	mux.HandleFunc("POST /api/requests", d.handleCreateRequest)
	mux.HandleFunc("GET /api/requests/{id}", d.handleGetRequest)
	mux.HandleFunc("GET /api/requests", d.handleListPending)
	mux.HandleFunc("POST /api/requests/{id}/approve", d.handleApprove)
	mux.HandleFunc("POST /api/requests/{id}/deny", d.handleDeny)
	mux.HandleFunc("GET /api/projects", d.handleListProjects)
	mux.HandleFunc("GET /api/projects/{slug}/credentials", d.handleListCredentials)
	mux.HandleFunc("GET /api/projects/{slug}/credentials/search", d.handleSearchProjectCredentials)
	mux.HandleFunc("GET /api/projects/{slug}/credentials/{alias}/reveal", d.handleRevealCredential)
	mux.HandleFunc("GET /api/credentials/search", d.handleSearchCredentials)
	mux.HandleFunc("GET /api/credentials/discover", d.handleDiscoverCredentials)
	mux.HandleFunc("PUT /api/projects/{slug}/credentials/{alias}", d.handleUpdateCredential)
	mux.HandleFunc("PUT /api/projects/{slug}/credentials/{alias}/context", d.handleSetCredentialContext)
	mux.HandleFunc("DELETE /api/projects/{slug}/credentials/{alias}", d.handleDeleteCredential)
	mux.HandleFunc("POST /api/projects/{slug}/credentials", d.handleCreateCredential)
	mux.HandleFunc("POST /api/projects/{slug}/credentials/copy", d.handleCopyCredential)
	mux.HandleFunc("GET /api/projects/{slug}/credentials/values", d.handleGetCredentialValues)
	mux.HandleFunc("POST /api/projects/{slug}/exec", d.handleExec)
	mux.HandleFunc("POST /api/projects/{slug}/clipboard", d.handleClipboard)
	mux.HandleFunc("POST /api/projects/{slug}/sync", d.handleSyncCredentials)
	mux.HandleFunc("GET /api/projects/{slug}/notes", d.handleListNotes)
	mux.HandleFunc("POST /api/projects/{slug}/notes", d.handleSetNote)
	mux.HandleFunc("DELETE /api/projects/{slug}/notes/{alias}", d.handleDeleteNote)
	mux.HandleFunc("POST /api/sessions/register", d.handleSessionRegister)
	mux.HandleFunc("POST /api/sessions/heartbeat", d.handleSessionHeartbeat)
	mux.HandleFunc("POST /api/sessions/deregister", d.handleSessionDeregister)
	mux.HandleFunc("GET /api/sessions", d.handleListSessions)
	mux.HandleFunc("GET /api/ws/status", d.handleWSStatus)
	mux.HandleFunc("POST /api/shutdown", d.handleShutdown)
	mux.HandleFunc("GET /api/ws", d.WS.handleWS)
}

func (d *Daemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type CreateRequestPayload struct {
	Alias       string `json:"alias"`
	Reason      string `json:"reason"`
	ProjectPath string `json:"project_path"`
	ProjectName string `json:"project_name"`
}

func (d *Daemon) handleCreateRequest(w http.ResponseWriter, r *http.Request) {
	var p CreateRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	slug := d.Store.ProjectSlug(p.ProjectPath)
	projectName := p.ProjectName
	if projectName == "" {
		projectName = d.Store.ProjectName(p.ProjectPath)
	}

	id := genID()
	req := &SecretRequest{
		ID:          id,
		Kind:        KindSecretRequest,
		Alias:       p.Alias,
		Reason:      p.Reason,
		ProjectPath: p.ProjectPath,
		ProjectName: projectName,
		ProjectSlug: slug,
		Status:      StatusPending,
		CreatedAt:   time.Now(),
	}

	// Check if credential exists and policy allows auto-approve
	val, meta, err := d.Store.GetCredential(p.ProjectPath, p.Alias)
	if err == nil && meta.Policy == store.PolicyAlwaysAllow {
		now := time.Now()
		req.Status = StatusApproved
		req.ResolvedAt = &now
		req.Policy = meta.Policy
		d.Queue.Add(req)
		writeJSON(w, http.StatusOK, map[string]any{
			"request":       req,
			"auto_approved": true,
		})
		_ = val
		return
	}

	d.Queue.Add(req)
	d.WS.Broadcast(WSEvent{
		Type:    EventRequestCreated,
		Project: slug,
		Data:    req,
	})

	// If no UI is connected, try to open the browser
	if !d.WS.HasClients(slug) {
		go openBrowser(fmt.Sprintf("http://127.0.0.1:%d/", d.Port))
	}

	writeJSON(w, http.StatusCreated, req)
}

func (d *Daemon) handleGetRequest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := d.Queue.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (d *Daemon) handleListPending(w http.ResponseWriter, r *http.Request) {
	pending := d.Queue.Pending()
	// Optional project filter
	if projectSlug := r.URL.Query().Get("project"); projectSlug != "" {
		var filtered []*SecretRequest
		for _, req := range pending {
			if req.ProjectSlug == projectSlug {
				filtered = append(filtered, req)
			}
		}
		writeJSON(w, http.StatusOK, filtered)
		return
	}
	writeJSON(w, http.StatusOK, pending)
}

type ApprovePayload struct {
	Value  string               `json:"value"`
	Policy store.ApprovalPolicy `json:"policy"`
}

// rememberSessionApprovals records ask_session aliases from an approved request
// so the same secret is not re-prompted for the rest of this agent session.
func (d *Daemon) rememberSessionApprovals(req *SecretRequest) {
	if req == nil {
		return
	}
	aliases := append([]string{}, req.Aliases...)
	if req.Alias != "" {
		aliases = append(aliases, req.Alias)
	}
	if len(aliases) == 0 {
		return
	}
	creds, err := d.Store.ListCredentials(req.ProjectPath)
	if err != nil {
		return
	}
	for _, alias := range aliases {
		if meta, ok := creds[alias]; ok && meta.Policy == store.PolicyAskSession {
			d.Approvals.Approve(req.ProjectSlug, alias)
		}
	}
}

func (d *Daemon) handleApprove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := d.Queue.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	var p ApprovePayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if req.Kind == KindSecretRequest {
		// secret_request: set credential if it doesn't already exist
		if !d.Store.HasCredential(req.ProjectPath, req.Alias) {
			if p.Policy == "" {
				p.Policy = store.PolicyAskSession
			}
			if err := d.Store.SetCredential(req.ProjectPath, req.Alias, p.Value, p.Policy); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}
	}
	// execute_accept: no credential storage needed, just approve

	d.Queue.Resolve(id, StatusApproved, p.Policy)
	resolved, ok := d.Queue.Get(id) // fresh copy with status+policy applied
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "request lost"})
		return
	}
	d.rememberSessionApprovals(resolved)
	projectSlug := resolved.ProjectSlug
	d.WS.Broadcast(WSEvent{Type: EventRequestResolved, Project: projectSlug, Data: resolved})
	d.WS.Broadcast(WSEvent{Type: EventCredentialsChanged, Project: projectSlug})
	pending := d.Queue.Pending()
	writeJSON(w, http.StatusOK, map[string]any{"result": resolved, "pending": pending})
}

func (d *Daemon) handleDeny(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := d.Queue.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	d.Queue.Resolve(id, StatusDenied, "")
	d.WS.Broadcast(WSEvent{
		Type:    EventRequestResolved,
		Project: req.ProjectSlug,
		Data:    map[string]string{"id": id, "status": "denied"},
	})
	pending := d.Queue.Pending()
	writeJSON(w, http.StatusOK, map[string]any{"result": map[string]string{"id": id, "status": "denied"}, "pending": pending})
}

func (d *Daemon) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := d.Store.ListProjects()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

type CredentialWithLength struct {
	store.CredentialMeta
	ValueLength int `json:"value_length"`
}

func (d *Daemon) handleListCredentials(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	projects, _ := d.Store.ListProjects()
	for _, p := range projects {
		if p.Slug == slug {
			creds, err := d.Store.ListCredentials(p.Path)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			// Enrich with value length — resolve file sources at runtime
			result := make(map[string]CredentialWithLength, len(creds))
			for alias, meta := range creds {
				vl := 0
				if meta.Source.Type == "file" && meta.Source.FilePath != "" && meta.Source.FileKey != "" {
					if val, err := ResolveFileCredential(p.Path, meta.Source.FilePath, meta.Source.FileKey); err == nil {
						vl = len(val)
					}
				} else if val, _, err := d.Store.GetCredential(p.Path, alias); err == nil {
					vl = len(val)
				}
				result[alias] = CredentialWithLength{CredentialMeta: meta, ValueLength: vl}
			}
			writeJSON(w, http.StatusOK, result)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
}

func (d *Daemon) handleRevealCredential(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	alias := r.PathValue("alias")

	projectPath := d.projectPathFromSlug(slug)
	if projectPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	val, meta, err := d.Store.GetCredential(projectPath, alias)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	// If stored value is empty and source is a file, read from the source file
	if val == "" && meta.Source.Type == "file" {
		val, err = ResolveFileCredential(projectPath, meta.Source.FilePath, meta.Source.FileKey)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"value": val})
}

func (d *Daemon) handleSearchCredentials(w http.ResponseWriter, r *http.Request) {
	alias := r.URL.Query().Get("alias")
	excludeProject := r.URL.Query().Get("exclude_project")
	matches, err := d.Store.FindCredentialAcrossProjects(alias, excludeProject)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, matches)
}

// resolveCredentialValue returns the secret value for any source type.
func (d *Daemon) resolveCredentialValue(projectPath, alias string) (string, error) {
	creds, err := d.Store.ListCredentials(projectPath)
	if err != nil {
		return "", err
	}
	meta, ok := creds[alias]
	if !ok {
		return "", fmt.Errorf("credential %q not found", alias)
	}
	if meta.Source.Type == "file" && meta.Source.FilePath != "" && meta.Source.FileKey != "" {
		return ResolveFileCredential(projectPath, meta.Source.FilePath, meta.Source.FileKey)
	}
	val, _, err := d.Store.GetCredential(projectPath, alias)
	return val, err
}

func (d *Daemon) projectPathFromSlug(slug string) string {
	projects, _ := d.Store.ListProjects()
	for _, p := range projects {
		if p.Slug == slug {
			return p.Path
		}
	}
	// Fall back to a live MCP session — covers a project that has an active
	// session but no persisted project.json yet (e.g. only temp secrets so far).
	if d.Sessions != nil {
		return d.Sessions.PathForSlug(slug)
	}
	return ""
}

type UpdateCredentialPayload struct {
	Value         string               `json:"value,omitempty"`
	Policy        store.ApprovalPolicy `json:"policy,omitempty"`
	CopyValueFrom *CredentialRef       `json:"copy_value_from,omitempty"`
}

func (d *Daemon) handleUpdateCredential(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	alias := r.PathValue("alias")

	var p UpdateCredentialPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	projectPath := d.projectPathFromSlug(slug)
	if projectPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	// Resolve value from another project if copy_value_from is set
	if p.CopyValueFrom != nil {
		fromPath := d.projectPathFromSlug(p.CopyValueFrom.ProjectSlug)
		if fromPath == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "source project not found"})
			return
		}
		val, err := d.resolveCredentialValue(fromPath, p.CopyValueFrom.Alias)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "source credential not found"})
			return
		}
		p.Value = val
	}

	if p.Value != "" {
		if err := d.Store.SetCredential(projectPath, alias, p.Value, p.Policy); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	} else if p.Policy != "" {
		if err := d.Store.UpdatePolicy(projectPath, alias, p.Policy); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	if r.Header.Get("No-Emit") != "true" {
		d.WS.Broadcast(WSEvent{Type: EventCredentialsChanged, Project: slug})
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (d *Daemon) handleDeleteCredential(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	alias := r.PathValue("alias")

	projectPath := d.projectPathFromSlug(slug)
	if projectPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	if err := d.Store.DeleteCredential(projectPath, alias); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if r.Header.Get("No-Emit") != "true" {
		d.WS.Broadcast(WSEvent{Type: EventCredentialsChanged, Project: slug})
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type CopyCredentialPayload struct {
	FromProjectSlug string `json:"from_project_slug"`
	Alias           string `json:"alias"`
}

func (d *Daemon) handleCopyCredential(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	var p CopyCredentialPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	toPath := d.projectPathFromSlug(slug)
	fromPath := d.projectPathFromSlug(p.FromProjectSlug)
	if toPath == "" || fromPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	if err := d.Store.CopyCredential(fromPath, toPath, p.Alias); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "copied"})
}

type CreateCredentialPayload struct {
	Alias    string               `json:"alias"`
	Value    string               `json:"value,omitempty"`
	Policy   store.ApprovalPolicy `json:"policy"`
	Context  string               `json:"context,omitempty"`
	CopyFrom *CredentialRef       `json:"copy_from,omitempty"`
}

type CredentialRef struct {
	ProjectSlug string `json:"project_slug"`
	Alias       string `json:"alias"`
}

func (d *Daemon) handleCreateCredential(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	var p CreateCredentialPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := store.ValidateAlias(p.Alias); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if p.Policy == "" {
		p.Policy = store.PolicyAskSession
	}

	projectPath := d.projectPathFromSlug(slug)
	if projectPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	if d.Store.HasCredential(projectPath, p.Alias) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "credential already exists"})
		return
	}

	var value string
	if p.CopyFrom != nil {
		fromPath := d.projectPathFromSlug(p.CopyFrom.ProjectSlug)
		if fromPath == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "source project not found"})
			return
		}
		var err error
		value, err = d.resolveCredentialValue(fromPath, p.CopyFrom.Alias)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "source credential not found"})
			return
		}
	} else {
		value = p.Value
	}

	if err := d.Store.SetCredential(projectPath, p.Alias, value, p.Policy); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if p.Context != "" {
		_ = d.Store.SetCredentialContext(projectPath, p.Alias, p.Context)
	}

	d.WS.Broadcast(WSEvent{Type: EventCredentialsChanged, Project: slug})
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

func (d *Daemon) handleGetCredentialValues(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	projectPath := d.projectPathFromSlug(slug)
	if projectPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	creds, err := d.Store.ListCredentials(projectPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	values := make(map[string]string)
	for alias, meta := range creds {
		if meta.Source.Type == "file" && meta.Source.FilePath != "" && meta.Source.FileKey != "" {
			val, err := ResolveFileCredential(projectPath, meta.Source.FilePath, meta.Source.FileKey)
			if err == nil {
				values[alias] = val
			}
		} else {
			val, _, err := d.Store.GetCredential(projectPath, alias)
			if err == nil {
				values[alias] = val
			}
		}
	}
	writeJSON(w, http.StatusOK, values)
}

type ExecPayload struct {
	Command string `json:"command"`
	// TempSecrets are session-scoped values supplied by the MCP server for this
	// single exec. They are never persisted to the store or keychain.
	TempSecrets map[string]string `json:"temp_secrets,omitempty"`
}

type ExecResponse struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error,omitempty"`
}

// findYuccaPlaceholders extracts all {{YUCCA:alias}} references from a command.
func findYuccaPlaceholders(command string) []string {
	var aliases []string
	s := command
	for {
		start := strings.Index(s, "{{YUCCA:")
		if start == -1 {
			break
		}
		rest := s[start+len("{{YUCCA:"):]
		end := strings.Index(rest, "}}")
		if end == -1 {
			break
		}
		aliases = append(aliases, rest[:end])
		s = rest[end+2:]
	}
	return aliases
}

// randomEnvName generates a random environment variable name like _YUCCA_A1B2C3D4.
func randomEnvName() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "_YUCCA_" + hex.EncodeToString(b)
}

// replaceInShellContext replaces all occurrences of placeholder in cmd with a
// reference to envName, respecting shell quoting. Inside a single-quoted string,
// variable expansion is suppressed, so it breaks out of the single-quote context:
// 'prefix{{YUCCA:K}}suffix' → 'prefix'"$envName"'suffix'
func replaceInShellContext(cmd, placeholder, envName string) string {
	const (
		stateUnquoted = 0
		stateSingle   = 1
		stateDouble   = 2
	)
	state := stateUnquoted
	var out strings.Builder
	i := 0
	for i < len(cmd) {
		if strings.HasPrefix(cmd[i:], placeholder) {
			switch state {
			case stateSingle:
				// Break out of single-quote context: close ', expand in double quotes, reopen '
				out.WriteString(`'"$` + envName + `"'`)
			case stateDouble:
				// Already inside double quotes — a bare $VAR is not word-split.
				out.WriteString("$" + envName)
			default: // stateUnquoted — quote so the value isn't word-split or glob-expanded
				out.WriteString(`"$` + envName + `"`)
			}
			i += len(placeholder)
			continue
		}
		c := cmd[i]
		out.WriteByte(c)
		switch state {
		case stateUnquoted:
			switch c {
			case '\'':
				state = stateSingle
			case '"':
				state = stateDouble
			}
		case stateSingle:
			if c == '\'' {
				state = stateUnquoted
			}
		case stateDouble:
			if c == '\\' && i+1 < len(cmd) {
				// Skip the escaped character (keeps \" from closing the string)
				i++
				out.WriteByte(cmd[i])
			} else if c == '"' {
				state = stateUnquoted
			}
		}
		i++
	}
	return out.String()
}

func (d *Daemon) handleExec(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	projectPath := d.projectPathFromSlug(slug)
	if projectPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	var p ExecPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// 1. Find all {{YUCCA:alias}} placeholders in the command
	referenced := findYuccaPlaceholders(p.Command)
	if len(referenced) == 0 {
		// Check if command uses $ALIAS patterns that match known credentials
		creds, _ := d.Store.ListCredentials(projectPath)
		var misused []string
		for alias := range creds {
			if strings.Contains(p.Command, "$"+alias) || strings.Contains(p.Command, "${"+alias+"}") {
				misused = append(misused, alias)
			}
		}
		if len(misused) > 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("Command references credentials as shell variables ($%s) but must use {{YUCCA:%s}} placeholder format. Example: echo {{YUCCA:%s}}", misused[0], misused[0], misused[0]),
			})
			return
		}

		// No secrets referenced — just run the command
		stdout, stderr, exitCode, execErr := yuccaExec.RunCapture(nil, projectPath, []string{"sh", "-c", p.Command})
		resp := ExecResponse{ExitCode: exitCode, Stdout: stdout, Stderr: stderr}
		if execErr != nil {
			resp.Error = execErr.Error()
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// 2. Verify all referenced credentials exist
	creds, err := d.Store.ListCredentials(projectPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for _, alias := range referenced {
		if _, ok := creds[alias]; ok {
			continue
		}
		if _, ok := p.TempSecrets[alias]; ok {
			continue // session-scoped temp secret supplied by the MCP server
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("credential %q not found", alias)})
		return
	}

	// 3. Request approval for credentials that need it
	var needsApproval []string
	for _, alias := range referenced {
		if _, ok := p.TempSecrets[alias]; ok {
			continue // temp secrets are session-scoped; no approval gate
		}
		meta := creds[alias]
		if meta.Policy == store.PolicyAlwaysAllow {
			continue
		}
		// ask_session: skip if the user already approved this secret this session.
		if meta.Policy == store.PolicyAskSession && d.Approvals.Approved(slug, alias) {
			continue
		}
		needsApproval = append(needsApproval, alias)
	}

	var requestID string
	if len(needsApproval) > 0 {
		projectName := d.Store.ProjectName(projectPath)
		id := genID()
		req := &SecretRequest{
			ID:          id,
			Kind:        KindExecuteAccept,
			Aliases:     needsApproval,
			Reason:      fmt.Sprintf("exec: %s", p.Command),
			ProjectPath: projectPath,
			ProjectName: projectName,
			ProjectSlug: slug,
			Status:      StatusPending,
			CreatedAt:   time.Now(),
		}
		ch := d.Queue.Add(req)
		d.WS.Broadcast(WSEvent{
			Type:    EventRequestCreated,
			Project: slug,
			Data:    req,
		})
		requestID = id

		// If no UI is connected, open browser
		if !d.WS.HasClients(slug) {
			go openBrowser(fmt.Sprintf("http://127.0.0.1:%d/", d.Port))
		}

		// 4. Wait for approval (up to 2 minutes)
		select {
		case <-ch:
			// resolved
		case <-time.After(2 * time.Minute):
			writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "timed out waiting for approval"})
			return
		}

		req, ok := d.Queue.Get(requestID)
		if !ok {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "request lost"})
			return
		}
		if req.Status == StatusDenied {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "execution denied by user"})
			return
		}
	}
	_ = requestID

	// 5. Resolve values for referenced credentials
	secrets := make(map[string]string)
	for _, alias := range referenced {
		if v, ok := p.TempSecrets[alias]; ok {
			secrets[alias] = v // session-scoped; used for this exec + masking, never stored
			continue
		}
		meta := creds[alias]
		if meta.Source.Type == "file" && meta.Source.FilePath != "" && meta.Source.FileKey != "" {
			if val, err := ResolveFileCredential(projectPath, meta.Source.FilePath, meta.Source.FileKey); err == nil {
				secrets[alias] = val
			}
		} else {
			if val, _, err := d.Store.GetCredential(projectPath, alias); err == nil {
				secrets[alias] = val
			}
		}
	}

	// 6. Replace {{YUCCA:alias}} with random env var names, set values in env.
	// Use shell-context-aware replacement so placeholders inside single-quoted
	// strings are handled correctly (single quotes suppress variable expansion).
	cmd := p.Command
	env := os.Environ()
	for alias, val := range secrets {
		envName := randomEnvName()
		cmd = replaceInShellContext(cmd, "{{YUCCA:"+alias+"}}", envName)
		env = append(env, envName+"="+val)
	}

	// 7. Execute and mask output
	stdout, stderr, exitCode, execErr := yuccaExec.RunCaptureWithEnv(secrets, projectPath, []string{"sh", "-c", cmd}, env)

	resp := ExecResponse{ExitCode: exitCode, Stdout: stdout, Stderr: stderr}
	if execErr != nil {
		resp.Error = execErr.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

type ClipboardPayload struct {
	Alias string `json:"alias"`
	// UI marks a copy initiated by the user from the trusted desktop client
	// (the tray menu). The click itself is the authorization, so the model
	// approval flow is skipped. The MCP tool never sets this.
	UI bool `json:"ui,omitempty"`
}

// handleClipboard copies a single secret's value to the user's clipboard.
// The value is resolved in the daemon and written via the clipboard package —
// it never crosses back over the API to the caller (the MCP model).
func (d *Daemon) handleClipboard(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	projectPath := d.projectPathFromSlug(slug)
	if projectPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	var p ClipboardPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if p.Alias == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "alias is required"})
		return
	}

	creds, err := d.Store.ListCredentials(projectPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	meta, ok := creds[p.Alias]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("credential %q not found", p.Alias)})
		return
	}

	// Request approval unless the user initiated this from the trusted desktop
	// client, the credential is always-allow, or it was already approved this
	// session.
	needsApproval := meta.Policy != store.PolicyAlwaysAllow
	if meta.Policy == store.PolicyAskSession && d.Approvals.Approved(slug, p.Alias) {
		needsApproval = false
	}
	if !p.UI && needsApproval {
		id := genID()
		req := &SecretRequest{
			ID:          id,
			Kind:        KindClipboardCopy,
			Alias:       p.Alias,
			Reason:      fmt.Sprintf("copy %s to clipboard", p.Alias),
			ProjectPath: projectPath,
			ProjectName: d.Store.ProjectName(projectPath),
			ProjectSlug: slug,
			Status:      StatusPending,
			CreatedAt:   time.Now(),
		}
		ch := d.Queue.Add(req)
		d.WS.Broadcast(WSEvent{Type: EventRequestCreated, Project: slug, Data: req})
		if !d.WS.HasClients(slug) {
			go openBrowser(fmt.Sprintf("http://127.0.0.1:%d/", d.Port))
		}

		select {
		case <-ch:
		case <-time.After(2 * time.Minute):
			writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "timed out waiting for approval"})
			return
		}

		if got, ok := d.Queue.Get(id); ok && got.Status == StatusDenied {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "clipboard copy denied by user"})
			return
		}
	}

	// Resolve the value (file-backed or stored) — same logic as exec injection.
	var value string
	if meta.Source.Type == "file" && meta.Source.FilePath != "" && meta.Source.FileKey != "" {
		value, err = ResolveFileCredential(projectPath, meta.Source.FilePath, meta.Source.FileKey)
	} else {
		value, _, err = d.Store.GetCredential(projectPath, p.Alias)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("resolve credential: %v", err)})
		return
	}

	if err := clipboard.CopyWithClear(value, clipboardClearAfter); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("clipboard: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "copied",
		"alias":          p.Alias,
		"clear_after_ms": clipboardClearAfter.Milliseconds(),
	})
}

// handleDiscoverCredentials fuzzy-searches secrets across all projects (except
// the caller's own), optionally narrowed to a fuzzy-matched project. Returns
// metadata only — never values.
func (d *Daemon) handleDiscoverCredentials(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	projectQuery := r.URL.Query().Get("project")
	excludePath := ""
	if ex := r.URL.Query().Get("exclude"); ex != "" {
		excludePath = d.projectPathFromSlug(ex)
	}

	projects, err := d.Store.ListProjects()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	type match struct {
		ProjectSlug string `json:"project_slug"`
		ProjectName string `json:"project_name"`
		Alias       string `json:"alias"`
		Context     string `json:"context,omitempty"`
		Source      string `json:"source,omitempty"`
	}
	results := []match{}
	for _, p := range projects {
		if p.Path == excludePath {
			continue
		}
		slug := d.Store.ProjectSlug(p.Path)
		if projectQuery != "" && !fuzzy.Match(projectQuery, p.Name, slug) {
			continue
		}
		for _, meta := range d.Store.SearchCredentials(p.Path, q) {
			results = append(results, match{
				ProjectSlug: slug,
				ProjectName: p.Name,
				Alias:       meta.Alias,
				Context:     meta.Context,
				Source:      meta.Source.Type,
			})
		}
	}
	writeJSON(w, http.StatusOK, results)
}

func (d *Daemon) handleSearchProjectCredentials(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	query := r.URL.Query().Get("q")
	projectPath := d.projectPathFromSlug(slug)
	if projectPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	results := d.Store.SearchCredentials(projectPath, query)
	if results == nil {
		results = []store.CredentialMeta{}
	}
	writeJSON(w, http.StatusOK, results)
}

type SetContextPayload struct {
	Context string `json:"context"`
}

func (d *Daemon) handleSetCredentialContext(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	alias := r.PathValue("alias")
	var p SetContextPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	projectPath := d.projectPathFromSlug(slug)
	if projectPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	if err := d.Store.SetCredentialContext(projectPath, alias, p.Context); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (d *Daemon) handleListNotes(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	projectPath := d.projectPathFromSlug(slug)
	if projectPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	notes, err := d.Store.ListNotes(projectPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if notes == nil {
		notes = []store.Note{}
	}
	writeJSON(w, http.StatusOK, notes)
}

type SetNotePayload struct {
	Alias string `json:"alias"`
	Body  string `json:"body"`
}

func (d *Daemon) handleSetNote(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	var p SetNotePayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	projectPath := d.projectPathFromSlug(slug)
	if projectPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	if err := d.Store.SetNote(projectPath, p.Alias, p.Body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	d.WS.Broadcast(WSEvent{Type: EventNotesChanged, Project: slug})
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (d *Daemon) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	alias := r.PathValue("alias")
	projectPath := d.projectPathFromSlug(slug)
	if projectPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	if err := d.Store.DeleteNote(projectPath, alias); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	d.WS.Broadcast(WSEvent{Type: EventNotesChanged, Project: slug})
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type SyncSecret struct {
	File string `json:"file"`
	Key  string `json:"key"`
}

type SyncPayload struct {
	ProjectPath string       `json:"project_path"`
	ProjectName string       `json:"project_name"`
	Secrets     []SyncSecret `json:"secrets"`
}

func (d *Daemon) handleSyncCredentials(w http.ResponseWriter, r *http.Request) {
	var p SyncPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if p.ProjectName != "" {
		_ = d.Store.SetProjectName(p.ProjectPath, p.ProjectName)
	}

	synced := 0
	for _, secret := range p.Secrets {
		alias := secret.File + ":" + secret.Key
		source := store.CredentialSource{
			Type:     "file",
			FilePath: secret.File,
			FileKey:  secret.Key,
		}

		// Check if already exists with same source — skip if unchanged
		existing, err := d.Store.ListCredentials(p.ProjectPath)
		if err == nil {
			if meta, ok := existing[alias]; ok &&
				meta.Source.Type == "file" &&
				meta.Source.FilePath == secret.File &&
				meta.Source.FileKey == secret.Key {
				continue
			}
		}

		// Default to ask_session (prompt once per session), not always_allow —
		// auto-detected secrets shouldn't be injected into exec with zero consent.
		if err := d.Store.SetCredentialWithSource(p.ProjectPath, alias, "", store.PolicyAskSession, source); err != nil {
			log.Printf("sync credential %s: %v", alias, err)
			continue
		}
		synced++
	}

	if synced > 0 {
		slug := r.PathValue("slug")
		d.WS.Broadcast(WSEvent{Type: "credentials_changed", Project: slug})
	}

	writeJSON(w, http.StatusOK, map[string]any{"synced": synced})
}

type SessionPayload struct {
	ProjectSlug string `json:"project_slug"`
	ProjectPath string `json:"project_path"`
	ProjectName string `json:"project_name"`
}

func (d *Daemon) handleSessionRegister(w http.ResponseWriter, r *http.Request) {
	var p SessionPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	// A new session starts with a clean slate: forget any ask_session approvals
	// left over from a prior session for this project (e.g. after a crash that
	// skipped deregister, re-registering within the reap window).
	d.Approvals.ClearProject(p.ProjectSlug)
	d.Sessions.Register(p.ProjectSlug, p.ProjectPath, p.ProjectName)
	d.WS.Broadcast(WSEvent{Type: "sessions_changed"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "registered"})
}

func (d *Daemon) handleSessionHeartbeat(w http.ResponseWriter, r *http.Request) {
	var p SessionPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !d.Sessions.Heartbeat(p.ProjectSlug) {
		// Session was reaped or never registered — re-register it
		d.Sessions.Register(p.ProjectSlug, p.ProjectPath, p.ProjectName)
		d.WS.Broadcast(WSEvent{Type: "sessions_changed"})
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (d *Daemon) handleSessionDeregister(w http.ResponseWriter, r *http.Request) {
	var p SessionPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	d.Sessions.Deregister(p.ProjectSlug)
	d.Approvals.ClearProject(p.ProjectSlug)
	d.WS.Broadcast(WSEvent{Type: "sessions_changed"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "deregistered"})
}

func (d *Daemon) handleListSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, d.Sessions.Active())
}

func (d *Daemon) handleWSStatus(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	writeJSON(w, http.StatusOK, map[string]bool{
		"connected": d.WS.HasClients(project),
	})
}

func (d *Daemon) handleShutdown(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "shutting down"})
	go func() {
		if d.cancel != nil {
			d.cancel()
		}
	}()
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("writeJSON encode error: %v", err)
	}
}

func genID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
