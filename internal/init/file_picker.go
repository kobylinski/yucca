package init

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kobylinski/yucca/internal/scanner"
)

type filePickerModel struct {
	picker      filepicker.Model
	projectPath string
	selected    string
	done        bool
	aborted     bool
	err         string
}

func newFilePickerModel(projectPath string) filePickerModel {
	fp := filepicker.New()
	fp.CurrentDirectory = projectPath
	fp.ShowHidden = true
	fp.ShowPermissions = false
	fp.ShowSize = false
	fp.SetHeight(15)
	// Remove esc from Back binding so it cancels instead
	fp.KeyMap.Back = key.NewBinding(key.WithKeys("h", "backspace", "left"), key.WithHelp("h", "back"))

	return filePickerModel{
		picker:      fp,
		projectPath: projectPath,
	}
}

func (m filePickerModel) Init() tea.Cmd {
	return m.picker.Init()
}

func (m filePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		m.err = "" // clear error on any key
		if msg.String() == "esc" || msg.String() == "q" || msg.String() == "ctrl+c" {
			m.aborted = true
			m.done = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)

	if didSelect, path := m.picker.DidSelectFile(msg); didSelect {
		// Try parsing the file to verify we can extract fields from it
		fields, err := scanner.ParseFile(path)
		if err != nil || len(fields) == 0 {
			m.err = "Cannot parse this file — unsupported format or no fields found"
			return m, cmd
		}
		rel, relErr := filepath.Rel(m.projectPath, path)
		if relErr != nil {
			rel = path
		}
		m.selected = rel
		m.done = true
		return m, tea.Quit
	}

	return m, cmd
}

func (m filePickerModel) View() string {
	if m.done {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Select a file to add:\n")
	sb.WriteString(dimStyle.Render("  navigate • enter to select • esc to cancel") + "\n\n")
	sb.WriteString(m.picker.View())
	if m.err != "" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		sb.WriteString("\n" + errStyle.Render("  "+m.err) + "\n")
	}
	return sb.String()
}

// RunFilePicker presents a file browser rooted at projectPath and returns
// the selected file path relative to the project root.
func RunFilePicker(projectPath string) (string, error) {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return "", fmt.Errorf("file picker: %w", err)
	}
	m := newFilePickerModel(absPath)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("file picker: %w", err)
	}
	result := finalModel.(filePickerModel)
	if result.aborted || result.selected == "" {
		return "", nil
	}
	return result.selected, nil
}
