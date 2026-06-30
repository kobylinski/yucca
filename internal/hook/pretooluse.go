package hook

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultProtectedPatterns are the default protected file patterns
var DefaultProtectedPatterns = []string{
	".env",
	".env.*",
	"secrets/**",
	"*.tfvars",
}

// HandlePreToolUse checks if a tool call targets a protected file.
// Returns JSON to deny if protected, empty string to allow.
func HandlePreToolUse(input *HookInput, protectedPatterns []string) string {
	deny := PreToolUseDeny(
		"This file is protected by Yucca. " +
			"Use the yucca_file MCP tool to read it (secret values are redacted), " +
			"or yucca_exec to run commands that need the real values.",
	)
	switch input.ToolName {
	case "Read", "Write", "Edit":
		if fp := input.ToolInput.FilePath; fp != "" && isProtectedPath(fp, input.Cwd, protectedPatterns) {
			return deny
		}
	case "Grep":
		// A content grep of a protected file would surface secret lines.
		if p := input.ToolInput.Path; p != "" && isProtectedPath(resolveHookPath(p, input.Cwd), input.Cwd, protectedPatterns) {
			return deny
		}
	case "Bash":
		// Best-effort: catch a casual `cat .env` / `grep KEY .env` that would
		// pull secret values into the model's context, bypassing the Read gate.
		// Not a hard boundary (a determined model can obfuscate), but it stops
		// the common accidental leak.
		if bashReadsProtected(input.ToolInput.Command, input.Cwd, protectedPatterns) {
			return deny
		}
	}
	return ""
}

// readCommands emit file contents to stdout, which the model then sees.
var readCommands = map[string]bool{
	"cat": true, "less": true, "more": true, "head": true, "tail": true,
	"bat": true, "nl": true, "tac": true, "xxd": true, "od": true, "hexdump": true,
	"strings": true, "grep": true, "egrep": true, "fgrep": true, "rg": true,
	"ag": true, "ack": true,
}

// bashReadsProtected reports whether a shell command appears to read a protected
// file: a read-style command with a protected-path argument, or an input
// redirection from a protected path.
func bashReadsProtected(command, cwd string, patterns []string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}
	// Skip leading VAR=val assignments and env/sudo/command wrappers to find the verb.
	i := 0
	for i < len(fields) && (strings.Contains(fields[i], "=") || fields[i] == "env" || fields[i] == "sudo" || fields[i] == "command" || fields[i] == "nice") {
		i++
	}
	isReader := i < len(fields) && readCommands[filepath.Base(strings.Trim(fields[i], `"'`))]

	for j, f := range fields {
		tok := strings.Trim(f, `"'`)
		// Input redirection: `< file` or `<file`.
		if tok == "<" && j+1 < len(fields) {
			tok = strings.Trim(fields[j+1], `"'`)
		} else if strings.HasPrefix(tok, "<") && tok != "<" {
			tok = strings.TrimPrefix(tok, "<")
		} else if !isReader {
			continue
		}
		if tok == "" || strings.HasPrefix(tok, "-") {
			continue
		}
		if isProtectedPath(resolveHookPath(tok, cwd), cwd, patterns) {
			return true
		}
	}
	return false
}

func resolveHookPath(p, cwd string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(cwd, p)
}

// LoadProtectedPatterns reads .yucca.yaml from the project directory
// and returns the protected file patterns. Falls back to defaults if
// the file doesn't exist.
func LoadProtectedPatterns(projectDir string) []string {
	data, err := os.ReadFile(filepath.Join(projectDir, ".yucca.yaml"))
	if err != nil {
		return DefaultProtectedPatterns
	}

	// Simple line-by-line parse — look for "  - <path>" lines under protected_files:
	var patterns []string
	inList := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "protected_files:" {
			inList = true
			continue
		}
		if inList && strings.HasPrefix(trimmed, "- ") {
			pattern := strings.TrimPrefix(trimmed, "- ")
			patterns = append(patterns, strings.TrimSpace(pattern))
		} else if inList && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			inList = false
		}
	}

	if len(patterns) == 0 {
		return DefaultProtectedPatterns
	}
	// Merge: always include defaults
	merged := make(map[string]bool)
	for _, p := range DefaultProtectedPatterns {
		merged[p] = true
	}
	for _, p := range patterns {
		merged[p] = true
	}
	result := make([]string, 0, len(merged))
	for p := range merged {
		result = append(result, p)
	}
	return result
}

// isProtectedPath checks if a file path matches any protected pattern.
// Patterns are matched relative to the project root (cwd).
func isProtectedPath(filePath, cwd string, patterns []string) bool {
	// Normalize first: filepath.Clean collapses "./" and "../" segments and
	// trailing slashes so they can't sidestep a pattern, and we lowercase for
	// case-insensitive filesystems (the macOS default) so ".ENV" can't bypass a
	// ".env" rule. Secret-file patterns are conventionally lowercase.
	filePath = filepath.Clean(filePath)
	cwd = filepath.Clean(cwd)

	// Use filepath.Rel so a sibling like /a/bc doesn't get treated as under /a/b
	// via a raw prefix; fall back to the absolute path when filePath is outside cwd.
	relPath := filePath
	if rel, relErr := filepath.Rel(cwd, filePath); relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		relPath = rel
	}
	relPath = strings.ToLower(relPath)
	base := strings.ToLower(filepath.Base(filePath))

	for _, pattern := range patterns {
		pattern = strings.ToLower(pattern)
		// Try matching the relative path against the pattern
		if matched, _ := filepath.Match(pattern, relPath); matched {
			return true
		}
		// Also try matching just the filename (for patterns like "*.tfvars")
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
		// Handle directory patterns like "secrets/**"
		if strings.HasSuffix(pattern, "/**") {
			dir := strings.TrimSuffix(pattern, "/**")
			if strings.HasPrefix(relPath, dir+"/") {
				return true
			}
		}
	}
	return false
}
