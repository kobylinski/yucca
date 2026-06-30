package store

import "time"

type ApprovalPolicy string

const (
	PolicyAlwaysAllow ApprovalPolicy = "always_allow"
	PolicyAskSession  ApprovalPolicy = "ask_session"
	PolicyAskAlways   ApprovalPolicy = "ask_always"
)

type CredentialSource struct {
	Type     string `json:"type"`                // "local", "file"
	FilePath string `json:"file_path,omitempty"` // relative path from project root
	FileKey  string `json:"file_key,omitempty"`  // dot-notation key within file
}

type CredentialMeta struct {
	Alias     string           `json:"alias"`
	Policy    ApprovalPolicy   `json:"policy"`
	Source    CredentialSource `json:"source"`
	Context   string           `json:"context,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// Note is a standalone, non-secret free-text entry scoped to a project.
// It is deliberately separate from CredentialMeta: notes never touch the
// keychain, masking, placeholders, or the approval flow.
type Note struct {
	Alias     string    `json:"alias"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ProjectInfo struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type ProjectData struct {
	Info        ProjectInfo               `json:"info"`
	Credentials map[string]CredentialMeta `json:"credentials"`
}
