package init

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kobylinski/yucca/internal/scanner"
)

// RunInit detects credential files, lets user select which to protect,
// then lets user pick specific secret fields within each file.
// Writes .yucca.yaml and installs Claude Code hooks.
// When agentMode is true, flags are routed to discovery/decision flow.
func RunInit(projectPath string, agentMode bool, files []string, projectName string) error {
	if agentMode {
		specs, err := ParseFileFlags(files)
		if err != nil {
			return err
		}
		// Decision mode: explicit key selections, or --name with no files (manual-only setup)
		if IsDecisionMode(specs) || (len(specs) == 0 && projectName != "") {
			output, err := RunAgentDecision(projectPath, projectName, specs)
			if err != nil {
				return err
			}
			fmt.Print(output)
			return nil
		}
		// Discovery mode
		output, err := RunAgentDiscovery(projectPath, specs)
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	}

	// Interactive mode below
	fmt.Println()

	// Step 1: Project name
	if projectName == "" {
		existingName := loadProjectNameField(projectPath)
		defaultName := existingName
		if defaultName == "" {
			defaultName = filepath.Base(projectPath)
		}

		var err error
		projectName, err = RunTextInput("Project name", defaultName)
		if err != nil {
			return err
		}
		if projectName == "" {
			projectName = defaultName
		}

		if existingName != "" && projectName == existingName {
			StepDone("Project name")
			StepBullet(fmt.Sprintf("Confirmed: %s", projectName))
		} else {
			StepDone("Project name")
			StepBullet(fmt.Sprintf("Name: %s", projectName))
		}
	} else {
		StepDone("Project name")
		StepBullet(fmt.Sprintf("Name: %s", projectName))
	}
	fmt.Println()

	// Step 2: Detect and select credential files
	detected, err := scanner.Detect(projectPath)
	if err != nil {
		return fmt.Errorf("detect: %w", err)
	}

	existingFiles := LoadProtectedFiles(projectPath)

	var protectedFiles []string
	var allSecrets []SelectedSecret

	if len(detected) == 0 && len(existingFiles) == 0 {
		fmt.Print("No credential files detected, skipping file selection.\n\n")
		StepDone("Credential files")
		StepBullet("0 files protected")
		fmt.Println()
	} else {
		if len(detected) == 0 {
			fmt.Print("No known credential files detected.\n\n")
		} else {
			fmt.Printf("Found %d potential credential files.\n\n", len(detected))
		}

		selected, err := RunSelector(detected, existingFiles, projectPath)
		if err != nil {
			return err
		}
		protectedFiles = selected

		{
			// Compute summary
			fromConfig := 0
			for _, sel := range protectedFiles {
				for _, ex := range existingFiles {
					if sel == ex {
						fromConfig++
						break
					}
				}
			}
			newlyAdded := len(protectedFiles) - fromConfig

			StepDone("Credential files")
			if fromConfig > 0 {
				StepBullet(fmt.Sprintf("%d from existing config", fromConfig))
			}
			if newlyAdded > 0 {
				StepBullet(fmt.Sprintf("%d newly selected", newlyAdded))
			}
			StepBullet(fmt.Sprintf("%d files protected", len(protectedFiles)))
		}
		fmt.Println()

		// Step 3: Select secret fields within files
		existingSecrets := LoadSecrets(projectPath)
		for _, file := range protectedFiles {
			absPath := filepath.Join(projectPath, file)
			fields, err := scanner.ParseFile(absPath)
			if err != nil {
				continue
			}
			if len(fields) == 0 {
				continue
			}

			var secrets []SelectedSecret
			var skipped bool
			if len(existingSecrets) > 0 {
				secrets, skipped, err = RunFieldSelectorWithPreselection(file, fields, existingSecrets)
			} else {
				secrets, skipped, err = RunFieldSelector(file, fields)
			}
			if err != nil {
				return err
			}
			if skipped {
				continue
			}
			allSecrets = append(allSecrets, secrets...)
		}

		StepDone("Secret fields")
		StepBullet(fmt.Sprintf("%d secrets identified", len(allSecrets)))
		fmt.Println()
	}

	// Write .yucca.yaml
	if err := WriteYuccaYAML(projectPath, projectName, protectedFiles); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	if len(allSecrets) > 0 {
		if err := WriteSecretsConfig(projectPath, allSecrets); err != nil {
			return fmt.Errorf("write secrets config: %w", err)
		}
	}

	// Step 4: Claude configuration
	changes, err := PrepareConfigChanges(projectPath)
	if err != nil {
		return fmt.Errorf("prepare config: %w", err)
	}

	hasChanges := false
	for _, c := range changes {
		if c.Changed {
			hasChanges = true
		}
	}

	if !hasChanges {
		StepDone("Claude configuration")
		StepBullet("Already up to date")
		fmt.Println()
		return nil
	}

	fmt.Print("Setting up Claude configuration...\n\n")
	for _, c := range changes {
		if c.Changed {
			fmt.Println(c.DiffView)
			fmt.Println()
		}
	}

	confirmed, err := confirmChanges()
	if err != nil {
		return err
	}

	if !confirmed {
		StepDone("Claude configuration")
		StepBullet("Skipped (no changes applied)")
	} else {
		if err := ApplyConfigChanges(changes); err != nil {
			return fmt.Errorf("apply config: %w", err)
		}
		StepDone("Claude configuration")
		StepBullet("Config files updated")
	}

	fmt.Println()
	return nil
}

// AddProtectedFile adds a file to .yucca.yaml
func AddProtectedFile(projectPath, file string) error {
	existing := LoadProtectedFiles(projectPath)

	// Check if already protected
	for _, f := range existing {
		if f == file {
			fmt.Printf("%s is already protected\n", file)
			return nil
		}
	}

	existing = append(existing, file)
	name := loadProjectNameField(projectPath)
	if name == "" {
		name = filepath.Base(projectPath)
	}
	if err := WriteYuccaYAML(projectPath, name, existing); err != nil {
		return err
	}
	fmt.Printf("Added %s to protected files\n", file)
	return nil
}

// LoadProtectedFiles reads the protected files list from .yucca.yaml
func LoadProtectedFiles(projectPath string) []string {
	data, err := os.ReadFile(filepath.Join(projectPath, ".yucca.yaml"))
	if err != nil {
		return nil
	}

	var files []string
	inList := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "protected_files:" {
			inList = true
			continue
		}
		if inList && strings.HasPrefix(trimmed, "- ") {
			pattern := strings.TrimPrefix(trimmed, "- ")
			files = append(files, strings.TrimSpace(pattern))
		} else if inList && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			inList = false
		}
	}
	return files
}

// WriteYuccaYAML writes the protected files list
func WriteYuccaYAML(projectPath, projectName string, protectedFiles []string) error {
	var sb strings.Builder
	sb.WriteString("# Yucca configuration\n")
	sb.WriteString("# Files listed here are protected — Claude cannot read/write them directly\n\n")

	fmt.Fprintf(&sb, "project_name: %s\n", projectName)
	sb.WriteString("\n")

	sb.WriteString("protected_files:\n")
	for _, f := range protectedFiles {
		fmt.Fprintf(&sb, "  - %s\n", f)
	}
	return os.WriteFile(filepath.Join(projectPath, ".yucca.yaml"), []byte(sb.String()), 0600)
}

func loadProjectNameField(projectPath string) string {
	return loadYAMLField(projectPath, "project_name:")
}

func loadYAMLField(projectPath, prefix string) string {
	data, err := os.ReadFile(filepath.Join(projectPath, ".yucca.yaml"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		}
	}
	return ""
}

// WriteSecretsConfig appends the secrets section to .yucca.yaml
func WriteSecretsConfig(projectPath string, secrets []SelectedSecret) error {
	configPath := filepath.Join(projectPath, ".yucca.yaml")

	existing, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	var sb strings.Builder
	sb.Write(existing)
	sb.WriteString("\n# Secret fields identified during init\n")
	sb.WriteString("# These keys within protected files contain sensitive values\n")
	sb.WriteString("secrets:\n")
	for _, s := range secrets {
		fmt.Fprintf(&sb, "  - file: %s\n    key: %s\n", s.File, s.Key)
	}

	return os.WriteFile(configPath, []byte(sb.String()), 0600)
}

// LoadSecrets reads the secrets section from .yucca.yaml
func LoadSecrets(projectPath string) []SelectedSecret {
	data, err := os.ReadFile(filepath.Join(projectPath, ".yucca.yaml"))
	if err != nil {
		return nil
	}

	var secrets []SelectedSecret
	lines := strings.Split(string(data), "\n")
	inSecrets := false
	var current SelectedSecret
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
			if current.File != "" && current.Key != "" {
				secrets = append(secrets, current)
			}
			current = SelectedSecret{File: strings.TrimPrefix(trimmed, "- file: ")}
		} else if strings.HasPrefix(trimmed, "key: ") {
			current.Key = strings.TrimPrefix(trimmed, "key: ")
		} else if trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "- ") && !strings.HasPrefix(trimmed, "file:") && !strings.HasPrefix(trimmed, "key:") {
			// End of secrets section
			break
		}
	}
	if current.File != "" && current.Key != "" {
		secrets = append(secrets, current)
	}
	return secrets
}

// BuildAliases creates deterministic alias names from selected secrets.
// Format: filepath:inner_key (e.g. ".env:API_KEY", "config/db.yml:production.password")
func BuildAliases(secrets []SelectedSecret) []string {
	aliases := make([]string, len(secrets))
	for i, s := range secrets {
		aliases[i] = s.File + ":" + s.Key
	}
	return aliases
}

// ConfigChange represents a pending change to a config file.
type ConfigChange struct {
	FilePath   string
	FileName   string
	OldContent []byte
	NewContent []byte
	Changed    bool
	DiffView   string
}

// PrepareConfigChanges computes what changes need to be made to config files
// without actually writing them.
func PrepareConfigChanges(projectPath string) ([]ConfigChange, error) {
	var changes []ConfigChange

	settingsPath := filepath.Join(projectPath, ".claude", "settings.json")
	settingsChange, err := prepareSettingsChange(projectPath, settingsPath)
	if err != nil {
		return nil, err
	}
	changes = append(changes, settingsChange)

	mcpPath := filepath.Join(projectPath, ".mcp.json")
	mcpChange, err := prepareMCPChange(mcpPath)
	if err != nil {
		return nil, err
	}
	changes = append(changes, mcpChange)

	return changes, nil
}

func prepareSettingsChange(projectPath, settingsPath string) (ConfigChange, error) {
	change := ConfigChange{
		FilePath: settingsPath,
		FileName: ".claude/settings.json",
	}

	settings := make(map[string]any)
	if data, err := os.ReadFile(settingsPath); err == nil {
		change.OldContent = data
		json.Unmarshal(data, &settings)
	}

	hooks := getMapOrNew(settings, "hooks")
	mergeHookEvent(hooks, "SessionStart", map[string]any{
		"matcher": "startup",
		"hooks": []map[string]any{
			{"type": "command", "command": "yucca hook session-start"},
		},
	})
	mergeHookEvent(hooks, "SessionEnd", map[string]any{
		"hooks": []map[string]any{
			{"type": "command", "command": "yucca hook session-end"},
		},
	})
	mergeHookEvent(hooks, "PreToolUse", map[string]any{
		"matcher": "Read|Write|Edit|Bash|Grep",
		"hooks": []map[string]any{
			{"type": "command", "command": "yucca hook pre-tool-use"},
		},
	})
	settings["hooks"] = hooks

	newData, _ := json.MarshalIndent(settings, "", "  ")
	change.NewContent = newData

	changed, oldFmt, newFmt := ComputeJSONDiff(change.OldContent, newData)
	change.Changed = changed
	if changed {
		change.DiffView = FormatDiffView(change.FileName, oldFmt, newFmt)
	}

	return change, nil
}

func prepareMCPChange(mcpPath string) (ConfigChange, error) {
	change := ConfigChange{
		FilePath: mcpPath,
		FileName: ".mcp.json",
	}

	existing := make(map[string]any)
	if data, err := os.ReadFile(mcpPath); err == nil {
		change.OldContent = data
		json.Unmarshal(data, &existing)
	}

	servers := getMapOrNew(existing, "mcpServers")
	if _, hasYucca := servers["yucca"]; !hasYucca {
		servers["yucca"] = map[string]any{
			"type":    "stdio",
			"command": "yucca",
			"args":    []string{"mcp", "serve"},
			"env":     map[string]any{},
		}
		existing["mcpServers"] = servers
	}

	newData, _ := json.MarshalIndent(existing, "", "  ")
	change.NewContent = newData

	changed, oldFmt, newFmt := ComputeJSONDiff(change.OldContent, newData)
	change.Changed = changed
	if changed {
		change.DiffView = FormatDiffView(change.FileName, oldFmt, newFmt)
	}

	return change, nil
}

// ApplyConfigChanges writes the prepared changes to disk.
func ApplyConfigChanges(changes []ConfigChange) error {
	for _, c := range changes {
		if !c.Changed {
			continue
		}
		dir := filepath.Dir(c.FilePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		if err := os.WriteFile(c.FilePath, c.NewContent, 0600); err != nil {
			return err
		}
	}
	return nil
}

// InstallClaudeHooks writes or merges hooks into .claude/settings.json
func InstallClaudeHooks(projectPath string) error {
	changes, err := PrepareConfigChanges(projectPath)
	if err != nil {
		return err
	}
	return ApplyConfigChanges(changes)
}

func confirmChanges() (bool, error) {
	m := newConfirmModel()
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return false, err
	}
	return finalModel.(confirmModel).confirmed, nil
}

type confirmModel struct {
	confirmed bool
	done      bool
}

func newConfirmModel() confirmModel {
	return confirmModel{}
}

func (m confirmModel) Init() tea.Cmd { return nil }

func (m confirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y", "enter":
			m.confirmed = true
			m.done = true
			return m, tea.Quit
		case "n", "N", "q", "esc", "ctrl+c":
			m.confirmed = false
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m confirmModel) View() string {
	if m.done {
		return ""
	}
	return "Apply these changes? [Y/n] "
}

// getMapOrNew gets an existing map from settings, or creates a new one
func getMapOrNew(settings map[string]any, key string) map[string]any {
	if v, ok := settings[key]; ok {
		if m, ok := v.(map[string]any); ok {
			return m
		}
	}
	return make(map[string]any)
}

// mergeHookEvent adds a yucca hook entry to an event's hook array,
// replacing any existing yucca entry for that event.
func mergeHookEvent(hooks map[string]any, event string, yuccaEntry map[string]any) {
	existing, _ := hooks[event].([]any)

	// Remove any existing yucca hooks for this event
	var kept []any
	for _, entry := range existing {
		if m, ok := entry.(map[string]any); ok {
			if hasYuccaHook(m) {
				continue // skip existing yucca entry
			}
		}
		kept = append(kept, entry)
	}

	// Append the new yucca entry
	kept = append(kept, yuccaEntry)
	hooks[event] = kept
}

// hasYuccaHook checks if a hook entry contains a yucca command
func hasYuccaHook(entry map[string]any) bool {
	hooksVal, ok := entry["hooks"]
	if !ok {
		return false
	}
	hooksArr, ok := hooksVal.([]any)
	if !ok {
		// Could be []map[string]any from our own code
		if typedArr, ok := hooksVal.([]map[string]any); ok {
			for _, h := range typedArr {
				if cmd, ok := h["command"].(string); ok && strings.Contains(cmd, "yucca") {
					return true
				}
			}
		}
		return false
	}
	for _, h := range hooksArr {
		if hm, ok := h.(map[string]any); ok {
			if cmd, ok := hm["command"].(string); ok && strings.Contains(cmd, "yucca") {
				return true
			}
		}
	}
	return false
}
