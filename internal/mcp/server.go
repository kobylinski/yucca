package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kobylinski/yucca/internal/fuzzy"
	"github.com/kobylinski/yucca/internal/proxy"
	"github.com/kobylinski/yucca/internal/scanner"
	"github.com/kobylinski/yucca/internal/service"
	"github.com/kobylinski/yucca/internal/store"
)

type Server struct {
	DaemonAddr  string
	ProjectPath string
	ProjectName string
	Store       *store.Store
	projectSlug string
	temp        *tempStore
	writeMu     sync.Mutex
}

type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type secretRequestArgs struct {
	Alias  string `json:"alias"`
	Reason string `json:"reason"`
}

type fileArgs struct {
	Action   string `json:"action"` // "read" or "write"
	FilePath string `json:"file_path"`
	Content  string `json:"content,omitempty"`
}

type execArgs struct {
	Command string `json:"command"`
}

type clipboardArgs struct {
	Alias  string `json:"alias"`
	Reason string `json:"reason,omitempty"`
}

type secretSearchArgs struct {
	Query   string `json:"query"`
	Project string `json:"project,omitempty"`
}

type secretCopyArgs struct {
	FromProject string `json:"from_project"`
	Alias       string `json:"alias"`
}

type secretIndexArgs struct {
	Filter string `json:"filter"`
}

type secretContextArgs struct {
	Action  string `json:"action"` // "read" or "write"
	Alias   string `json:"alias"`
	Context string `json:"context,omitempty"`
}

type secretStoreArgs struct {
	Alias   string `json:"alias"`
	Value   string `json:"value"`
	Context string `json:"context,omitempty"`
	Persist *bool  `json:"persist,omitempty"` // nil/true = persist; false = session-only temporary
}

type noteStoreArgs struct {
	Alias   string `json:"alias"`
	Body    string `json:"body"`
	Persist *bool  `json:"persist,omitempty"` // nil/true = persist; false = session-only temporary
}

type syncSecret struct {
	File string `json:"file"`
	Key  string `json:"key"`
}

type syncPayload struct {
	ProjectPath string       `json:"project_path"`
	ProjectName string       `json:"project_name"`
	Secrets     []syncSecret `json:"secrets"`
}

func New(daemonAddr, projectPath string, s *store.Store) *Server {
	name := loadProjectName(projectPath)
	return &Server{
		DaemonAddr:  daemonAddr,
		ProjectPath: projectPath,
		ProjectName: name,
		Store:       s,
		projectSlug: projectSlug(projectPath),
		temp:        newTempStore(),
	}
}

// loadProjectName reads project_name from .yucca.yaml, falls back to directory name.
func loadProjectName(projectPath string) string {
	data, err := os.ReadFile(fmt.Sprintf("%s/.yucca.yaml", projectPath))
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "project_name:") {
				name := strings.TrimSpace(strings.TrimPrefix(trimmed, "project_name:"))
				if name != "" {
					return name
				}
			}
		}
	}
	return filepath.Base(projectPath)
}

func (s *Server) Run() error {
	s.ensureDaemon()
	s.registerSession()
	s.syncCredentials()
	go s.heartbeatLoop()

	// Ensure deregister on signal (SIGINT, SIGTERM)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		s.deregisterSession()
		os.Exit(0)
	}()

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var req jsonrpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeResponse(jsonrpcResponse{
				JSONRPC: "2.0",
				Error:   map[string]any{"code": -32700, "message": "Parse error"},
			})
			continue
		}

		// Handle tool calls concurrently so parallel requests don't block each other
		// (e.g. two exec calls waiting for approval simultaneously).
		// Non-tool requests are handled inline since they're fast.
		if req.Method == "tools/call" {
			go func() {
				resp := s.handleRequest(req)
				s.writeResponse(resp)
			}()
		} else {
			resp := s.handleRequest(req)
			s.writeResponse(resp)
		}
	}
	s.deregisterSession()
	return scanner.Err()
}

func (s *Server) handleRequest(req jsonrpcRequest) jsonrpcResponse {
	switch req.Method {
	case "initialize":
		return jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"serverInfo": map[string]string{
					"name":    "yucca",
					"version": "0.1.0",
				},
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
			},
		}

	case "notifications/initialized":
		return jsonrpcResponse{}

	case "tools/list":
		return jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"tools": s.toolDefinitions(),
			},
		}

	case "tools/call":
		return s.handleToolCall(req)

	default:
		return jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   map[string]any{"code": -32601, "message": "Method not found"},
		}
	}
}

func (s *Server) toolDefinitions() []map[string]any {
	return []map[string]any{
		{
			"name":        "yucca_note_store",
			"description": "Save a non-secret NOTE scoped to this project — a fact, reminder, or scratch detail you want to recall later WITHOUT keeping it in your own context/memory (e.g. 'staging DB listens on port 5433', 'deploy script is ops/deploy.sh'). Notes are plain text: never masked, never treated as secrets. For secret VALUES use yucca_secret_store instead. Reusing an alias overwrites that note. Set persist:false for a session-only ephemeral note (erased when the connection ends).",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"alias": map[string]string{
						"type":        "string",
						"description": "Short key/title for the note (e.g. staging-db-port). Allowed: A-Z a-z 0-9 _ - . (max 64 chars)",
					},
					"body": map[string]string{
						"type":        "string",
						"description": "The note text to remember",
					},
					"persist": map[string]string{
						"type":        "boolean",
						"description": "Default true. Set false for a session-only TEMPORARY note (erased when the connection ends).",
					},
				},
				"required": []string{"alias", "body"},
			},
		},
		{
			"name":        "yucca_note_list",
			"description": "List this project's notes (key + text) to recall facts you saved earlier with yucca_note_store. Notes are non-secret free text. Returns all notes for the project.",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "yucca_secret_request",
			"description": "Request a secret you do not have. This is the ONLY correct way to ask the user for a secret — it opens a secure approval UI where the user enters the value. NEVER ask for secrets in chat. When the user says they need to add/store/create a password, key, or token and you do NOT already have the value in your context, use this tool.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"alias": map[string]string{
						"type":        "string",
						"description": "Name/alias for the secret (e.g. OPENAI_API_KEY)",
					},
					"reason": map[string]string{
						"type":        "string",
						"description": "Why this secret is needed",
					},
				},
				"required": []string{"alias", "reason"},
			},
		},
		{
			"name":        "yucca_file",
			"description": "Read or write a protected file. On read, secret values are redacted to {{YUCCA:alias}} placeholders; on write, placeholders are rehydrated to real values and raw secrets are sanitized. Local files must be registered during project init. REMOTE paths in scp form (user@host:path, e.g. deploy@server:~/app/.env) are also supported — read/written over SSH using your ssh config and agent, with the same redaction on read and in-memory rehydration on write (the rehydrated content streams over SSH and never touches local disk).",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]string{
						"type":        "string",
						"description": "read or write",
					},
					"file_path": map[string]string{
						"type":        "string",
						"description": "Absolute local path to a registered protected file, OR a remote scp-style path user@host:path (e.g. deploy@server:~/app/.env)",
					},
					"content": map[string]string{
						"type":        "string",
						"description": "File content with {{YUCCA:alias}} placeholders (required for write)",
					},
				},
				"required": []string{"action", "file_path"},
			},
		},
		{
			"name":        "yucca_exec",
			"description": "Execute a shell command with secret values substituted and output masked. IMPORTANT: You MUST reference secrets using {{YUCCA:alias}} placeholders — NOT $VARIABLE or any other format. Example: echo {{YUCCA:API_KEY}} or curl -H \"Authorization: Bearer {{YUCCA:API_KEY}}\" https://api.example.com. The placeholders are securely replaced at execution time. Without {{YUCCA:...}} placeholders, no secrets are injected.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]string{
						"type":        "string",
						"description": "Shell command with {{YUCCA:alias}} placeholders for secrets (passed to sh -c)",
					},
				},
				"required": []string{"command"},
			},
		},
		{
			"name":        "yucca_clipboard",
			"description": "Copy an existing secret's value straight to the user's system clipboard so they can paste it somewhere you cannot reach (a website login field, a desktop app, an OS dialog). The value goes RAM → clipboard and is NEVER shown to you. Requires user approval, and the clipboard is auto-cleared shortly after. Use the alias of an existing secret (see yucca_secret_index). Use this instead of yucca_secret_request when the secret already exists and the user just needs it on their clipboard.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"alias": map[string]string{
						"type":        "string",
						"description": "Alias of the existing secret to copy (e.g. STRIPE_API_KEY)",
					},
					"reason": map[string]string{
						"type":        "string",
						"description": "Why the user needs this value on their clipboard",
					},
				},
				"required": []string{"alias"},
			},
		},
		{
			"name":        "yucca_secret_search",
			"description": "Search for a secret across your OTHER projects (not this one) — use when the user says they already have a credential somewhere, e.g. 'I think I have the API key in the caddy-pay project'. Both arguments are fuzzy: 'query' matches secret alias/context, 'project' narrows to a project by name. Returns matching project + alias + context — NEVER values. Follow up with yucca_secret_copy to bring a match into this project.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]string{
						"type":        "string",
						"description": "Fuzzy term to match against secret alias/context (e.g. 'stripe', 'api key'). Leave empty to list all secrets in the matched project(s).",
					},
					"project": map[string]string{
						"type":        "string",
						"description": "Optional fuzzy project name to search within (e.g. 'caddy pay' matches 'caddy-pay'). Omit to search all other projects.",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			"name":        "yucca_secret_copy",
			"description": "Copy an existing secret from another project INTO this project so you can use it (e.g. with yucca_exec). The target is ALWAYS this project — you cannot copy into other projects. Find the source first with yucca_secret_search. After copying, reference it as {{YUCCA:alias}}.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"from_project": map[string]string{
						"type":        "string",
						"description": "Source project to copy from — its slug (from yucca_secret_search) or a fuzzy name like 'caddy-pay'.",
					},
					"alias": map[string]string{
						"type":        "string",
						"description": "Exact alias of the secret to copy (from yucca_secret_search).",
					},
				},
				"required": []string{"from_project", "alias"},
			},
		},
		{
			"name":        "yucca_secret_index",
			"description": "List available secrets with metadata (alias, source type, context notes). Returns all secrets, optionally filtered by alias or context. Does NOT return secret values.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"filter": map[string]string{
						"type":        "string",
						"description": "Optional filter term to match against alias or context notes. Omit to list all.",
					},
				},
			},
		},
		{
			"name":        "yucca_secret_context",
			"description": "Read or write context notes on a secret. Notes describe purpose, origin, expiry, etc. and are searchable via yucca_secret_index.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]string{
						"type":        "string",
						"description": "read or write",
					},
					"alias": map[string]string{
						"type":        "string",
						"description": "Secret alias (e.g. '.env:API_KEY' or 'MANUAL_TOKEN')",
					},
					"context": map[string]string{
						"type":        "string",
						"description": "Free-text notes about this secret (required for write)",
					},
				},
				"required": []string{"action", "alias"},
			},
		},
		{
			"name":        "yucca_secret_store",
			"description": "Store a secret that ALREADY EXISTS in your context — the user pasted it in chat, a command output generated it, or you found it in a file. Use this to capture and secure a value you can already see. Do NOT use this to ask the user for a secret — use yucca_secret_request instead. Returns the alias to use as a {{YUCCA:alias}} placeholder. Set persist:false for a THROWAWAY/test value that should live ONLY this session — usable in yucca_exec, never written to disk or keychain, erased when the connection ends (and it cannot be written to files).",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"alias": map[string]string{
						"type":        "string",
						"description": "Name for the secret (e.g. STRIPE_API_KEY). Allowed: A-Z a-z 0-9 _ - . (max 64 chars)",
					},
					"value": map[string]string{
						"type":        "string",
						"description": "The secret value to store",
					},
					"context": map[string]string{
						"type":        "string",
						"description": "Optional notes about this secret (e.g. where it was found, what service it belongs to)",
					},
					"persist": map[string]string{
						"type":        "boolean",
						"description": "Default true. Set false for a session-only TEMPORARY secret (throwaway/test token): usable in yucca_exec, never written to disk/keychain, erased when the connection ends.",
					},
				},
				"required": []string{"alias", "value"},
			},
		},
	}
}

func (s *Server) handleToolCall(req jsonrpcRequest) jsonrpcResponse {
	var params toolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   map[string]any{"code": -32602, "message": "Invalid params"},
		}
	}

	switch params.Name {
	case "yucca_note_store":
		return s.handleNoteStore(req.ID, params.Arguments)
	case "yucca_note_list":
		return s.handleNoteList(req.ID, params.Arguments)
	case "yucca_secret_request":
		return s.handleSecretRequest(req.ID, params.Arguments)
	case "yucca_file":
		return s.handleFile(req.ID, params.Arguments)
	case "yucca_exec":
		return s.handleExec(req.ID, params.Arguments)
	case "yucca_clipboard":
		return s.handleClipboardCopy(req.ID, params.Arguments)
	case "yucca_secret_search":
		return s.handleSecretSearch(req.ID, params.Arguments)
	case "yucca_secret_copy":
		return s.handleSecretCopy(req.ID, params.Arguments)
	case "yucca_secret_index":
		return s.handleSecretIndex(req.ID, params.Arguments)
	case "yucca_secret_context":
		return s.handleSecretContext(req.ID, params.Arguments)
	case "yucca_secret_store":
		return s.handleSecretStore(req.ID, params.Arguments)
	default:
		return jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   map[string]any{"code": -32602, "message": fmt.Sprintf("Unknown tool: %s", params.Name)},
		}
	}
}

// registeredFiles returns the set of absolute file paths that have file-sourced credentials.
func (s *Server) registeredFiles() map[string]bool {
	slug := projectSlug(s.ProjectPath)
	resp, err := http.Get(fmt.Sprintf("%s/api/projects/%s/credentials", s.DaemonAddr, slug))
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var creds map[string]store.CredentialMeta
	if err := json.NewDecoder(resp.Body).Decode(&creds); err != nil {
		return nil
	}
	files := make(map[string]bool)
	for _, meta := range creds {
		if meta.Source.Type == "file" && meta.Source.FilePath != "" {
			abs := filepath.Join(s.ProjectPath, meta.Source.FilePath)
			files[abs] = true
		}
	}
	return files
}

func (s *Server) handleFile(id any, rawArgs json.RawMessage) jsonrpcResponse {
	var args fileArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return s.toolError(id, "Invalid arguments")
	}

	// Remote scp-style paths (user@host:path) transfer over SSH with the same
	// redact/rehydrate protection, bypassing the local registered-file gate.
	if host, rpath, remote := parseRemotePath(args.FilePath); remote {
		return s.handleRemoteFile(id, host, rpath, args)
	}

	// Verify file is registered (has file-sourced credentials)
	registered := s.registeredFiles()
	if !registered[args.FilePath] {
		return s.toolResult(id, fmt.Sprintf("Access denied: %s is not a registered protected file. Only files with secrets detected during project init can be accessed.", args.FilePath), true)
	}

	secrets, err := s.fetchCredentialValues()
	if err != nil {
		return s.toolResult(id, fmt.Sprintf("Error fetching credentials: %v", err), true)
	}

	switch args.Action {
	case "read":
		data, err := os.ReadFile(args.FilePath)
		if err != nil {
			return s.toolResult(id, fmt.Sprintf("Error reading file: %v", err), true)
		}
		content := string(data)
		redactMap := s.withTempRedaction(secrets)
		if len(redactMap) > 0 {
			content = proxy.Redact(content, redactMap)
		}
		// Defense in depth: a protected file may contain secrets that were never
		// registered at init (a second key, a DB password). Redact every parsed
		// field value so an unregistered secret doesn't reach the model verbatim.
		if fields, perr := scanner.ParseFile(args.FilePath); perr == nil {
			content = redactUnregisteredValues(content, fields)
		}
		return s.toolResult(id, content, false)

	case "write":
		if args.Content == "" {
			return s.toolResult(id, "Content is required for write action", true)
		}
		if a := s.tempPlaceholderIn(args.Content); a != "" {
			return s.toolResult(id, fmt.Sprintf("Refusing to write: %q is a TEMPORARY secret and must not be persisted to a file (it lives only in this session). Use yucca_secret_store for a value that should be written to disk.", a), true)
		}
		content := args.Content
		if len(secrets) > 0 {
			content = proxy.Sanitize(content, secrets)
			content = proxy.Rehydrate(content, secrets)
		}
		if err := os.WriteFile(args.FilePath, []byte(content), 0600); err != nil {
			return s.toolResult(id, fmt.Sprintf("Error writing file: %v", err), true)
		}
		return s.toolResult(id, fmt.Sprintf("File written: %s", args.FilePath), false)

	default:
		return s.toolResult(id, "Action must be 'read' or 'write'", true)
	}
}

func (s *Server) handleExec(id any, rawArgs json.RawMessage) jsonrpcResponse {
	var args execArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return s.toolError(id, "Invalid arguments")
	}

	slug := projectSlug(s.ProjectPath)
	payload := map[string]any{"command": args.Command}
	// Ferry any temp secrets the command references — they live only in this
	// process, so the daemon needs them transiently for this single exec.
	if temp := s.temp.SecretsReferencedIn(args.Command); len(temp) > 0 {
		payload["temp_secrets"] = temp
	}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(
		fmt.Sprintf("%s/api/projects/%s/exec", s.DaemonAddr, slug),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return s.toolResult(id, fmt.Sprintf("Error contacting daemon: %v", err), true)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		var errResp map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return s.toolResult(id, fmt.Sprintf("Denied: %s", errResp["error"]), true)
	}
	if resp.StatusCode == http.StatusGatewayTimeout {
		return s.toolResult(id, "Timed out waiting for credential approval", true)
	}
	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return s.toolResult(id, fmt.Sprintf("Error: %s", errResp["error"]), true)
	}

	var result struct {
		ExitCode int    `json:"exit_code"`
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		Error    string `json:"error,omitempty"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)

	var output strings.Builder
	if result.Stdout != "" {
		output.WriteString(result.Stdout)
	}
	if result.Stderr != "" {
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		output.WriteString("STDERR:\n")
		output.WriteString(result.Stderr)
	}
	if output.Len() == 0 {
		output.WriteString("(no output)")
	}

	return s.toolResult(id, fmt.Sprintf("Exit code: %d\n%s", result.ExitCode, output.String()), false)
}

func (s *Server) handleSecretIndex(id any, rawArgs json.RawMessage) jsonrpcResponse {
	var args secretIndexArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return s.toolError(id, "Invalid arguments")
	}
	slug := projectSlug(s.ProjectPath)
	url := fmt.Sprintf("%s/api/projects/%s/credentials/search?q=%s", s.DaemonAddr, slug, args.Filter)
	resp, err := http.Get(url)
	if err != nil {
		return s.toolResult(id, fmt.Sprintf("Error listing secrets: %v", err), true)
	}
	defer resp.Body.Close()
	var results []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return s.toolResult(id, "Error parsing results", true)
	}
	tempSecrets := s.temp.SecretAliases()
	if len(results) == 0 && len(tempSecrets) == 0 {
		if args.Filter != "" {
			return s.toolResult(id, fmt.Sprintf("No secrets found matching %q", args.Filter), false)
		}
		return s.toolResult(id, "No secrets registered for this project.", false)
	}
	var sb strings.Builder
	if len(results) > 0 {
		fmt.Fprintf(&sb, "%d secret(s):\n", len(results))
		for _, r := range results {
			alias, _ := r["alias"].(string)
			sourceMap, _ := r["source"].(map[string]any)
			srcType, _ := sourceMap["type"].(string)
			ctx, _ := r["context"].(string)
			fmt.Fprintf(&sb, "\n- %s (source: %s)", alias, srcType)
			if ctx != "" {
				fmt.Fprintf(&sb, "\n  Context: %s", ctx)
			}
		}
	}
	if len(tempSecrets) > 0 {
		fmt.Fprintf(&sb, "\n\n%d temporary secret(s) — this session only, values hidden:", len(tempSecrets))
		for _, a := range tempSecrets {
			fmt.Fprintf(&sb, "\n- %s", a)
		}
	}
	return s.toolResult(id, sb.String(), false)
}

func (s *Server) handleSecretContext(id any, rawArgs json.RawMessage) jsonrpcResponse {
	var args secretContextArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return s.toolError(id, "Invalid arguments")
	}
	slug := projectSlug(s.ProjectPath)

	switch args.Action {
	case "read":
		// Use search endpoint filtered by exact alias to get context
		resp, err := http.Get(fmt.Sprintf("%s/api/projects/%s/credentials/search?q=%s", s.DaemonAddr, slug, args.Alias))
		if err != nil {
			return s.toolResult(id, fmt.Sprintf("Error reading context: %v", err), true)
		}
		defer resp.Body.Close()
		var results []map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
			return s.toolResult(id, "Error parsing results", true)
		}
		for _, r := range results {
			if alias, _ := r["alias"].(string); alias == args.Alias {
				ctx, _ := r["context"].(string)
				if ctx == "" {
					return s.toolResult(id, fmt.Sprintf("No context set for %q.", args.Alias), false)
				}
				return s.toolResult(id, ctx, false)
			}
		}
		return s.toolResult(id, fmt.Sprintf("Secret %q not found.", args.Alias), true)

	case "write":
		if args.Context == "" {
			return s.toolResult(id, "Context is required for write action", true)
		}
		body, _ := json.Marshal(map[string]string{"context": args.Context})
		req, _ := http.NewRequest("PUT",
			fmt.Sprintf("%s/api/projects/%s/credentials/%s/context", s.DaemonAddr, slug, args.Alias),
			bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return s.toolResult(id, fmt.Sprintf("Error setting context: %v", err), true)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return s.toolResult(id, fmt.Sprintf("Error: daemon returned status %d", resp.StatusCode), true)
		}
		return s.toolResult(id, fmt.Sprintf("Context updated for %q.", args.Alias), false)

	default:
		return s.toolResult(id, "Action must be 'read' or 'write'", true)
	}
}

func (s *Server) handleSecretStore(id any, rawArgs json.RawMessage) jsonrpcResponse {
	var args secretStoreArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return s.toolError(id, "Invalid arguments")
	}

	if err := store.ValidateAlias(args.Alias); err != nil {
		return s.toolResult(id, fmt.Sprintf("Invalid alias: %v", err), true)
	}

	// persist defaults to true; persist:false makes this a session-only temporary
	// secret held in this MCP process — never written to disk or the keychain.
	if args.Persist != nil && !*args.Persist {
		if vals, err := s.fetchCredentialValues(); err == nil {
			if _, exists := vals[args.Alias]; exists {
				return s.toolResult(id, fmt.Sprintf("Alias %q already exists as a persisted secret. Use a different alias for a temporary entry.", args.Alias), true)
			}
		}
		s.temp.Put(args.Alias, args.Value, true)
		return s.toolResult(id, fmt.Sprintf("Temporary secret %q stored for THIS session only (erased when the connection ends; never written to disk). Reference it as {{YUCCA:%s}} in yucca_exec. It cannot be written to files.", args.Alias, args.Alias), false)
	}

	payload := map[string]string{
		"alias":  args.Alias,
		"value":  args.Value,
		"policy": "ask_session",
	}
	if args.Context != "" {
		payload["context"] = args.Context
	}
	body, _ := json.Marshal(payload)

	slug := projectSlug(s.ProjectPath)
	resp, err := http.Post(
		fmt.Sprintf("%s/api/projects/%s/credentials", s.DaemonAddr, slug),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return s.toolResult(id, fmt.Sprintf("Error contacting daemon: %v", err), true)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return s.toolResult(id, fmt.Sprintf("Credential %q already exists. Use a different alias.", args.Alias), true)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return s.toolResult(id, fmt.Sprintf("Error: daemon returned status %d", resp.StatusCode), true)
	}

	return s.toolResult(id, fmt.Sprintf("Secret stored as %q. Use {{YUCCA:%s}} placeholder instead of the raw value.", args.Alias, args.Alias), false)
}

func (s *Server) handleNoteStore(id any, rawArgs json.RawMessage) jsonrpcResponse {
	var args noteStoreArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return s.toolError(id, "Invalid arguments")
	}
	if err := store.ValidateAlias(args.Alias); err != nil {
		return s.toolResult(id, fmt.Sprintf("Invalid alias: %v", err), true)
	}

	// persist defaults to true; persist:false makes this a session-only temporary
	// note held in this MCP process — readable back via yucca_note_list this session.
	if args.Persist != nil && !*args.Persist {
		s.temp.Put(args.Alias, args.Body, false)
		return s.toolResult(id, fmt.Sprintf("Temporary note %q saved for THIS session only (erased when the connection ends).", args.Alias), false)
	}

	body, _ := json.Marshal(map[string]string{"alias": args.Alias, "body": args.Body})
	slug := projectSlug(s.ProjectPath)
	resp, err := http.Post(
		fmt.Sprintf("%s/api/projects/%s/notes", s.DaemonAddr, slug),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return s.toolResult(id, fmt.Sprintf("Error contacting daemon: %v", err), true)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return s.toolResult(id, fmt.Sprintf("Error: daemon returned status %d", resp.StatusCode), true)
	}
	return s.toolResult(id, fmt.Sprintf("Note %q saved.", args.Alias), false)
}

func (s *Server) handleNoteList(id any, _ json.RawMessage) jsonrpcResponse {
	slug := projectSlug(s.ProjectPath)
	resp, err := http.Get(fmt.Sprintf("%s/api/projects/%s/notes", s.DaemonAddr, slug))
	if err != nil {
		return s.toolResult(id, fmt.Sprintf("Error listing notes: %v", err), true)
	}
	defer resp.Body.Close()
	var notes []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&notes); err != nil {
		return s.toolResult(id, "Error parsing notes", true)
	}
	tempNotes := s.temp.Notes()
	if len(notes) == 0 && len(tempNotes) == 0 {
		return s.toolResult(id, "No notes saved for this project.", false)
	}
	var sb strings.Builder
	if len(notes) > 0 {
		fmt.Fprintf(&sb, "%d note(s):\n", len(notes))
		for _, n := range notes {
			alias, _ := n["alias"].(string)
			body, _ := n["body"].(string)
			fmt.Fprintf(&sb, "\n- %s\n  %s", alias, body)
		}
	}
	if len(tempNotes) > 0 {
		fmt.Fprintf(&sb, "\n%d temporary note(s) — this session only:", len(tempNotes))
		for _, n := range tempNotes {
			fmt.Fprintf(&sb, "\n- %s\n  %s", n.Alias, n.Body)
		}
	}
	return s.toolResult(id, sb.String(), false)
}

func (s *Server) fetchCredentialValues() (map[string]string, error) {
	hash := projectSlug(s.ProjectPath)
	resp, err := http.Get(fmt.Sprintf("%s/api/projects/%s/credentials/values", s.DaemonAddr, hash))
	if err != nil {
		return nil, fmt.Errorf("daemon request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var values map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&values); err != nil {
		return nil, err
	}
	return values, nil
}

func projectSlug(path string) string {
	return strings.ReplaceAll(path, "/", "-")
}

func (s *Server) handleSecretRequest(id any, rawArgs json.RawMessage) jsonrpcResponse {
	var args secretRequestArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return s.toolError(id, "Invalid arguments")
	}

	// POST to daemon with project path from server context
	reqBody := map[string]string{
		"alias":        args.Alias,
		"reason":       args.Reason,
		"project_path": s.ProjectPath,
		"project_name": s.ProjectName,
	}
	body, _ := json.Marshal(reqBody)
	resp, err := http.Post(s.DaemonAddr+"/api/requests", "application/json", bytes.NewReader(body))
	if err != nil {
		return s.toolResult(id, fmt.Sprintf("Error contacting daemon: %v", err), true)
	}
	defer func() { _ = resp.Body.Close() }()

	var createResp map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&createResp)

	// Check if auto-approved
	if autoApproved, ok := createResp["auto_approved"].(bool); ok && autoApproved {
		return s.toolResult(id, fmt.Sprintf("Secret %q auto-approved (policy: always_allow). Credential is available.", args.Alias), false)
	}

	// Get request ID from response
	var requestID string
	if r, ok := createResp["id"].(string); ok {
		requestID = r
	} else {
		return s.toolResult(id, "Error: could not get request ID from daemon", true)
	}

	// Poll for resolution (daemon handles browser-open if no UI connected)
	for i := 0; i < 120; i++ {
		time.Sleep(1 * time.Second)

		pollResp, err := http.Get(fmt.Sprintf("%s/api/requests/%s", s.DaemonAddr, requestID))
		if err != nil {
			continue
		}

		var status map[string]any
		_ = json.NewDecoder(pollResp.Body).Decode(&status)
		_ = pollResp.Body.Close()

		if st, ok := status["status"].(string); ok && st != "pending" {
			if st == "approved" {
				return s.toolResult(id, fmt.Sprintf("Secret %q approved and stored. Credential is available.", args.Alias), false)
			}
			return s.toolResult(id, fmt.Sprintf("Secret %q was denied by user.", args.Alias), false)
		}
	}

	return s.toolResult(id, "Timed out waiting for user approval (2 minutes)", true)
}

func (s *Server) handleClipboardCopy(id any, rawArgs json.RawMessage) jsonrpcResponse {
	var args clipboardArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return s.toolError(id, "Invalid arguments")
	}
	if args.Alias == "" {
		return s.toolResult(id, "Error: alias is required", true)
	}

	slug := projectSlug(s.ProjectPath)
	body, _ := json.Marshal(map[string]string{"alias": args.Alias})
	resp, err := http.Post(
		fmt.Sprintf("%s/api/projects/%s/clipboard", s.DaemonAddr, slug),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return s.toolResult(id, fmt.Sprintf("Error contacting daemon: %v", err), true)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return s.toolResult(id, fmt.Sprintf("Clipboard copy of %q was denied by the user.", args.Alias), false)
	}
	if resp.StatusCode == http.StatusGatewayTimeout {
		return s.toolResult(id, "Timed out waiting for user approval", true)
	}
	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return s.toolResult(id, fmt.Sprintf("Error: %s", errResp["error"]), true)
	}

	var result struct {
		ClearAfterMs int64 `json:"clear_after_ms"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)

	msg := fmt.Sprintf("Copied %q to the clipboard — the user can paste it now. The value was not shown to you.", args.Alias)
	if result.ClearAfterMs > 0 {
		msg += fmt.Sprintf(" It will auto-clear in %ds.", result.ClearAfterMs/1000)
	}
	return s.toolResult(id, msg, false)
}

func (s *Server) handleSecretSearch(id any, rawArgs json.RawMessage) jsonrpcResponse {
	var args secretSearchArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return s.toolError(id, "Invalid arguments")
	}
	own := projectSlug(s.ProjectPath)
	u := fmt.Sprintf("%s/api/credentials/discover?q=%s&project=%s&exclude=%s",
		s.DaemonAddr, url.QueryEscape(args.Query), url.QueryEscape(args.Project), url.QueryEscape(own))
	resp, err := http.Get(u)
	if err != nil {
		return s.toolResult(id, fmt.Sprintf("Error searching: %v", err), true)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return s.toolResult(id, fmt.Sprintf("Error searching: %s", errResp["error"]), true)
	}

	var matches []struct {
		ProjectSlug string `json:"project_slug"`
		ProjectName string `json:"project_name"`
		Alias       string `json:"alias"`
		Context     string `json:"context"`
		Source      string `json:"source"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&matches); err != nil {
		return s.toolResult(id, "Error parsing results", true)
	}
	if len(matches) == 0 {
		return s.toolResult(id, "No matching secrets found in your other projects.", false)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d secret(s) in other projects:\n", len(matches))
	for _, m := range matches {
		fmt.Fprintf(&sb, "\n- %s in project %q (slug: %s, source: %s)", m.Alias, m.ProjectName, m.ProjectSlug, m.Source)
		if m.Context != "" {
			fmt.Fprintf(&sb, "\n  Context: %s", m.Context)
		}
	}
	fmt.Fprintf(&sb, "\n\nTo use one here, call yucca_secret_copy with from_project (the slug) and alias.")
	return s.toolResult(id, sb.String(), false)
}

func (s *Server) handleSecretCopy(id any, rawArgs json.RawMessage) jsonrpcResponse {
	var args secretCopyArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return s.toolError(id, "Invalid arguments")
	}
	if args.FromProject == "" || args.Alias == "" {
		return s.toolResult(id, "Error: from_project and alias are required", true)
	}
	fromSlug, name, ok := s.resolveProjectSlug(args.FromProject)
	if !ok {
		return s.toolResult(id, fmt.Sprintf("No project found matching %q. Use yucca_secret_search to find the source.", args.FromProject), true)
	}

	own := projectSlug(s.ProjectPath)
	body, _ := json.Marshal(map[string]string{"from_project_slug": fromSlug, "alias": args.Alias})
	resp, err := http.Post(
		fmt.Sprintf("%s/api/projects/%s/credentials/copy", s.DaemonAddr, own),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return s.toolResult(id, fmt.Sprintf("Error contacting daemon: %v", err), true)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return s.toolResult(id, fmt.Sprintf("Error copying %q from %q: %s", args.Alias, name, errResp["error"]), true)
	}
	return s.toolResult(id, fmt.Sprintf("Copied %q from project %q into this project. Reference it as {{YUCCA:%s}}.", args.Alias, name, args.Alias), false)
}

// resolveProjectSlug fuzzily resolves a project name/slug to its slug.
// Returns (slug, displayName, found).
func (s *Server) resolveProjectSlug(query string) (string, string, bool) {
	resp, err := http.Get(s.DaemonAddr + "/api/projects")
	if err != nil {
		return "", "", false
	}
	defer resp.Body.Close()
	var projects []struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		return "", "", false
	}
	for _, p := range projects { // exact slug wins
		if p.Slug == query {
			return p.Slug, p.Name, true
		}
	}
	if fuzzy.Normalize(query) == "" {
		return "", "", false
	}
	for _, p := range projects {
		if fuzzy.Match(query, p.Slug, p.Name) {
			return p.Slug, p.Name, true
		}
	}
	return "", "", false
}

// redactUnregisteredValues replaces each parsed field value with a
// {{YUCCA:key}} placeholder when it still appears verbatim in content (i.e.
// it wasn't already redacted as a registered secret). A protected file is a
// secrets file, so redacting all of its values is the safe default. Values
// shorter than 4 chars are skipped to avoid over-redacting trivial tokens.
func redactUnregisteredValues(content string, fields []scanner.ParsedField) string {
	for _, f := range fields {
		v := strings.TrimSpace(f.Value)
		if len(v) < 4 || strings.HasPrefix(v, "{{YUCCA:") {
			continue
		}
		if !strings.Contains(content, v) {
			continue
		}
		content = strings.ReplaceAll(content, v, "{{YUCCA:"+f.Key+"}}")
	}
	return content
}

func (s *Server) toolResult(id any, text string, isError bool) jsonrpcResponse {
	result := map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
	}
	if isError {
		result["isError"] = true
	}
	return jsonrpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func (s *Server) toolError(id any, message string) jsonrpcResponse {
	return jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   map[string]any{"code": -32602, "message": message},
	}
}

func (s *Server) writeResponse(resp jsonrpcResponse) {
	if resp.JSONRPC == "" && resp.ID == nil && resp.Result == nil && resp.Error == nil {
		return // notification response, don't write
	}
	data, _ := json.Marshal(resp)
	s.writeMu.Lock()
	_, _ = fmt.Fprintf(os.Stdout, "%s\n", data)
	s.writeMu.Unlock()
}

func (s *Server) syncCredentials() {
	secrets := loadSecretsFromConfig(s.ProjectPath)
	if len(secrets) == 0 {
		return
	}
	payload := syncPayload{
		ProjectPath: s.ProjectPath,
		ProjectName: s.ProjectName,
		Secrets:     secrets,
	}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(
		fmt.Sprintf("%s/api/projects/%s/sync", s.DaemonAddr, s.projectSlug),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return
	}
	resp.Body.Close()
}

func loadSecretsFromConfig(projectPath string) []syncSecret {
	data, err := os.ReadFile(filepath.Join(projectPath, ".yucca.yaml"))
	if err != nil {
		return nil
	}
	var secrets []syncSecret
	lines := strings.Split(string(data), "\n")
	inSecrets := false
	var currentFile string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "secrets:" {
			inSecrets = true
			continue
		}
		if !inSecrets {
			continue
		}
		if strings.HasPrefix(trimmed, "- file: ") {
			currentFile = strings.TrimPrefix(trimmed, "- file: ")
		} else if strings.HasPrefix(trimmed, "key: ") {
			key := strings.TrimPrefix(trimmed, "key: ")
			if currentFile != "" && key != "" {
				secrets = append(secrets, syncSecret{File: currentFile, Key: key})
			}
		} else if trimmed != "" && !strings.HasPrefix(trimmed, "#") &&
			!strings.HasPrefix(trimmed, "file:") && !strings.HasPrefix(trimmed, "key:") {
			break
		}
	}
	return secrets
}

// ensureDaemon makes sure the daemon is running and managed. This is the
// automation point: whenever an agent starts its MCP server, Yucca installs
// (idempotently) the OS service that keeps the daemon alive — so the daemon no
// longer depends on this transient process staying up. The ephemeral spawn
// remains only as a fallback for systems without a usable service manager.
func (s *Server) ensureDaemon() {
	client := &http.Client{Timeout: 2 * time.Second}
	healthy := func() bool {
		resp, err := client.Get(s.DaemonAddr + "/api/health")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}
	waitHealthy := func() bool {
		for i := 0; i < 20; i++ {
			time.Sleep(250 * time.Millisecond)
			if healthy() {
				return true
			}
		}
		return false
	}

	// Persist the service descriptor pointing at our own binary. Harmless and
	// idempotent — it does not disturb an already-running daemon, it just makes
	// the daemon launchd/systemd-managed from now on (incl. next login).
	descriptorChanged := false
	if binPath, err := os.Executable(); err == nil {
		changed, werr := service.WriteDescriptor(binPath)
		if werr != nil {
			fmt.Fprintf(os.Stderr, "yucca: could not write service descriptor: %v\n", werr)
		}
		descriptorChanged = changed
	}

	if healthy() {
		return
	}

	// Not running — start it through the OS service manager so it stays up and
	// restarts on crash, independent of this MCP process.
	if service.Available() {
		if err := service.Start(descriptorChanged); err == nil && waitHealthy() {
			return
		}
	}

	// Fallback: spawn a detached daemon directly (no service manager available).
	cmd := exec.Command(os.Args[0], "daemon", "--idle-timeout", "0")
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "yucca: failed to start daemon: %v\n", err)
		return
	}
	cmd.Process.Release()
	if !waitHealthy() {
		fmt.Fprintf(os.Stderr, "yucca: daemon failed to start at %s after 5s\n", s.DaemonAddr)
	}
}

func (s *Server) registerSession() {
	body, _ := json.Marshal(map[string]string{
		"project_slug": s.projectSlug,
		"project_path": s.ProjectPath,
		"project_name": s.ProjectName,
	})
	resp, err := http.Post(s.DaemonAddr+"/api/sessions/register", "application/json", bytes.NewReader(body))
	if err == nil {
		resp.Body.Close()
	}
}

func (s *Server) deregisterSession() {
	body, _ := json.Marshal(map[string]string{
		"project_slug": s.projectSlug,
	})
	resp, err := http.Post(s.DaemonAddr+"/api/sessions/deregister", "application/json", bytes.NewReader(body))
	if err == nil {
		resp.Body.Close()
	}
}

func (s *Server) heartbeatLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		body, _ := json.Marshal(map[string]string{
			"project_slug": s.projectSlug,
			"project_path": s.ProjectPath,
			"project_name": s.ProjectName,
		})
		resp, err := http.Post(s.DaemonAddr+"/api/sessions/heartbeat", "application/json", bytes.NewReader(body))
		if err == nil {
			resp.Body.Close()
		}
	}
}

func ReadStdin() ([]byte, error) {
	return io.ReadAll(os.Stdin)
}
