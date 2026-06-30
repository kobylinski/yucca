package init

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kobylinski/yucca/internal/scanner"
)

// FileSpec represents a parsed -f flag value.
// Bare file (discovery): Path="file.json", Keys=nil
// With selections (decision): Path="file.json", Keys=["key1","key2"]
type FileSpec struct {
	Path string
	Keys []string
}

// ParseFileFlags parses -f flag values into FileSpec entries.
// Returns an error if bare files and keyed files are mixed.
func ParseFileFlags(flags []string) ([]FileSpec, error) {
	if len(flags) == 0 {
		return nil, nil
	}

	var specs []FileSpec
	hasBare, hasKeyed := false, false

	for _, f := range flags {
		parts := strings.Split(f, ",")
		spec := FileSpec{Path: parts[0]}
		if len(parts) > 1 {
			spec.Keys = parts[1:]
			hasKeyed = true
		} else {
			hasBare = true
		}
		specs = append(specs, spec)
	}

	if hasBare && hasKeyed {
		return nil, fmt.Errorf("cannot mix bare files and field selections in -f flags")
	}

	return specs, nil
}

// discoveredFile groups a file path with its parsed fields for output.
type discoveredFile struct {
	path        string
	description string // from scanner detection, or empty for extra files
	fields      []scanner.ParsedField
}

// RunAgentDiscovery scans for credential files and prints their contents as markdown.
// extraFiles are additional files provided via -f flags (bare, no keys).
// Returns the markdown string. Does not write any files.
func RunAgentDiscovery(projectPath string, extraFiles []FileSpec) (string, error) {
	var files []discoveredFile

	// Auto-detect known patterns
	detected, err := scanner.Detect(projectPath)
	if err != nil {
		return "", fmt.Errorf("detect: %w", err)
	}
	seen := make(map[string]bool)
	for _, d := range detected {
		absPath := filepath.Join(projectPath, d.Path)
		fields, err := scanner.ParseFile(absPath)
		if err != nil {
			continue
		}
		files = append(files, discoveredFile{
			path:        d.Path,
			description: d.Description,
			fields:      fields,
		})
		seen[d.Path] = true
	}

	// Add extra files from -f flags
	for _, ef := range extraFiles {
		if seen[ef.Path] {
			continue
		}
		absPath := filepath.Join(projectPath, ef.Path)
		fields, err := scanner.ParseFile(absPath)
		if err != nil {
			return "", fmt.Errorf("cannot parse %s: %w", ef.Path, err)
		}
		files = append(files, discoveredFile{
			path:   ef.Path,
			fields: fields,
		})
		seen[ef.Path] = true
	}

	var sb strings.Builder
	sb.WriteString("# Yucca Discovery\n\n")

	if len(files) == 0 {
		sb.WriteString("No credential files detected in this project.\n\n")
	} else {
		sb.WriteString("## Detected Secrets\n\n")
		for _, f := range files {
			if f.description != "" {
				fmt.Fprintf(&sb, "### %s (%s)\n", f.path, f.description)
			} else {
				fmt.Fprintf(&sb, "### %s\n", f.path)
			}
			if len(f.fields) == 0 {
				sb.WriteString("No fields found.\n")
			} else {
				// Model-facing output: emit key names only, never any part of
				// the value (MaskValue still reveals a prefix/suffix preview,
				// which belongs only in the local human TUI).
				for _, field := range f.fields {
					fmt.Fprintf(&sb, "- %s\n", field.Key)
				}
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("## Next Steps\n\n")
	sb.WriteString("To initialize yucca for this project, run the decision phase:\n\n")
	if len(files) > 0 {
		sb.WriteString("Select which files and keys to protect with `-f` flags:\n")
		sb.WriteString("```\nyucca init --agent")
		for _, f := range files {
			if len(f.fields) > 0 {
				keys := make([]string, 0, len(f.fields))
				for _, field := range f.fields {
					keys = append(keys, field.Key)
				}
				fmt.Fprintf(&sb, " -f \"%s,%s\"", f.path, strings.Join(keys, ","))
			}
		}
		sb.WriteString("\n```\n\n")
	}
	sb.WriteString("You can also initialize with no file-sourced credentials:\n")
	sb.WriteString("```\nyucca init --agent --name <project-name>\n```\n")
	sb.WriteString("This sets up yucca for manual credentials managed through the UI.\n")

	return sb.String(), nil
}

// RunAgentDecision writes .yucca.yaml with the specified files and secrets,
// and returns markdown with setup instructions for the agent.
// Does NOT write .claude/settings.json or .mcp.json.
func RunAgentDecision(projectPath, projectName string, specs []FileSpec) (string, error) {
	if projectName == "" {
		projectName = filepath.Base(projectPath)
	}

	// Collect protected files and secrets
	var protectedFiles []string
	var secrets []SelectedSecret
	for _, spec := range specs {
		protectedFiles = append(protectedFiles, spec.Path)
		for _, key := range spec.Keys {
			secrets = append(secrets, SelectedSecret{
				File: spec.Path,
				Key:  key,
			})
		}
	}

	// Write .yucca.yaml
	if err := WriteYuccaYAML(projectPath, projectName, protectedFiles); err != nil {
		return "", fmt.Errorf("write config: %w", err)
	}
	if len(secrets) > 0 {
		if err := WriteSecretsConfig(projectPath, secrets); err != nil {
			return "", fmt.Errorf("write secrets: %w", err)
		}
	}

	// Build output
	var sb strings.Builder
	sb.WriteString("# Yucca Initialized\n\n")
	fmt.Fprintf(&sb, "Project: %s\n", projectName)
	fmt.Fprintf(&sb, "Protected files: %d\n", len(protectedFiles))
	fmt.Fprintf(&sb, "Secrets: %d\n\n", len(secrets))
	if len(protectedFiles) == 0 {
		sb.WriteString("No file-sourced credentials configured. You can add credentials manually through the yucca UI.\n")
	}

	sb.WriteString("\n## Setup Instructions\n\n")
	sb.WriteString("Add to `.claude/settings.json` under `hooks`:\n\n")
	sb.WriteString("```json\n")
	sb.WriteString(`{
  "hooks": {
    "SessionStart": [{"matcher": "startup", "hooks": [{"type": "command", "command": "yucca hook session-start"}]}],
    "SessionEnd": [{"hooks": [{"type": "command", "command": "yucca hook session-end"}]}],
    "PreToolUse": [{"matcher": "Read|Write|Edit|Bash|Grep", "hooks": [{"type": "command", "command": "yucca hook pre-tool-use"}]}]
  }
}`)
	sb.WriteString("\n```\n\n")
	sb.WriteString("Add to `.mcp.json`:\n\n")
	sb.WriteString("```json\n")
	sb.WriteString(`{
  "mcpServers": {
    "yucca": {"type": "stdio", "command": "yucca", "args": ["mcp", "serve"]}
  }
}`)
	sb.WriteString("\n```\n")

	return sb.String(), nil
}

// IsDecisionMode returns true when all file specs include explicit key selections.
func IsDecisionMode(specs []FileSpec) bool {
	if len(specs) == 0 {
		return false
	}
	for _, s := range specs {
		if len(s.Keys) == 0 {
			return false
		}
	}
	return true
}
