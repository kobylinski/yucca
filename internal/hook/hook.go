package hook

import (
	"encoding/json"
	"fmt"
	"io"
)

// HookInput represents the JSON received from Claude Code on stdin
type HookInput struct {
	SessionID      string    `json:"session_id"`
	TranscriptPath string    `json:"transcript_path"`
	Cwd            string    `json:"cwd"`
	PermissionMode string    `json:"permission_mode"`
	HookEventName  string    `json:"hook_event_name"`
	Source         string    `json:"source,omitempty"`
	Model          string    `json:"model,omitempty"`
	ToolName       string    `json:"tool_name,omitempty"`
	ToolInput      ToolInput `json:"tool_input,omitempty"`
	ToolUseID      string    `json:"tool_use_id,omitempty"`
}

type ToolInput struct {
	Command  string `json:"command,omitempty"`
	FilePath string `json:"file_path,omitempty"`
	Content  string `json:"content,omitempty"`
	Pattern  string `json:"pattern,omitempty"`
	Path     string `json:"path,omitempty"`
}

func ParseHookInput(r io.Reader) (*HookInput, error) {
	var hi HookInput
	if err := json.NewDecoder(r).Decode(&hi); err != nil {
		return nil, fmt.Errorf("parse hook input: %w", err)
	}
	return &hi, nil
}

// SessionStartOutput returns JSON that sets additionalContext
// telling Claude about the Yucca session
func SessionStartOutput(sessionID, daemonAddr string) string {
	output := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName": "SessionStart",
			"additionalContext": fmt.Sprintf(
				"Yucca secret manager is active for this session (session %s, daemon %s).\n"+
					"Secrets stay out of your context — work with them by name:\n"+
					"  - Need a secret you don't have? yucca_secret_request opens an approval UI for the user to enter it — never ask for secrets in chat.\n"+
					"  - In yucca_exec and yucca_file, reference secrets as {{YUCCA:alias}} placeholders — never raw values or $ENV vars.\n"+
					"  - Protected files (.env, *.tfvars, …) can ONLY be read/written via yucca_file (values are redacted on read); reading them directly is blocked.\n"+
					"See the MCP tools/list for the full yucca_* toolset (store, capture, search, notes, clipboard).",
				sessionID, daemonAddr),
		},
	}
	b, _ := json.Marshal(output)
	return string(b)
}

// PreToolUseDeny returns JSON to deny a tool call
func PreToolUseDeny(reason string) string {
	output := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "deny",
			"permissionDecisionReason": reason,
		},
	}
	b, _ := json.Marshal(output)
	return string(b)
}

// PreToolUseAllow returns JSON to allow a tool call
func PreToolUseAllow() string {
	output := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":      "PreToolUse",
			"permissionDecision": "allow",
		},
	}
	b, _ := json.Marshal(output)
	return string(b)
}
