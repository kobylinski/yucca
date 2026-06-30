package init

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kobylinski/yucca/internal/scanner"
)

var (
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))            // green
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))            // cyan
	categoryStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5")) // magenta
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))            // gray
)

type selectorModel struct {
	files      []scanner.DetectedFile
	selected   map[int]bool
	cursor     int
	done       bool
	aborted    bool
	openPicker bool // signal to launch file picker after exit
}

// totalItems returns file count + 1 for the "Other file..." row
func (m selectorModel) totalItems() int {
	return len(m.files) + 1
}

func (m selectorModel) isOtherRow() bool {
	return m.cursor == len(m.files)
}

func newSelectorModel(files []scanner.DetectedFile, preSelected []string) selectorModel {
	preSet := make(map[string]bool)
	for _, p := range preSelected {
		preSet[p] = true
	}

	selected := make(map[int]bool)
	if len(preSelected) > 0 {
		// Reinit: only pre-select files from existing config
		for i, f := range files {
			if preSet[f.Path] {
				selected[i] = true
			}
		}
	} else {
		// First init: pre-select all detected files
		for i := range files {
			selected[i] = true
		}
	}
	return selectorModel{
		files:    files,
		selected: selected,
	}
}

func (m selectorModel) Init() tea.Cmd {
	return nil
}

func (m selectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.aborted = true
			m.done = true
			return m, tea.Quit
		case "enter":
			if m.isOtherRow() {
				m.openPicker = true
				m.done = true
				return m, tea.Quit
			}
			m.done = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < m.totalItems()-1 {
				m.cursor++
			}
		case " ":
			if m.isOtherRow() {
				m.openPicker = true
				m.done = true
				return m, tea.Quit
			}
			m.selected[m.cursor] = !m.selected[m.cursor]
		case "a":
			// Toggle all (only detected files, not "Other")
			allSelected := true
			for i := range m.files {
				if !m.selected[i] {
					allSelected = false
					break
				}
			}
			for i := range m.files {
				m.selected[i] = !allSelected
			}
		}
	}
	return m, nil
}

func (m selectorModel) View() string {
	if m.done {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Select files to protect (secrets will be redacted):\n")
	sb.WriteString(dimStyle.Render("  ↑/↓ navigate • space toggle • a toggle all • enter confirm • q quit") + "\n\n")

	lastCategory := ""
	for i, f := range m.files {
		if f.Category != lastCategory {
			sb.WriteString(categoryStyle.Render("  "+f.Category) + "\n")
			lastCategory = f.Category
		}

		cursor := "  "
		if i == m.cursor {
			cursor = cursorStyle.Render("▸ ")
		}

		check := "[ ]"
		if m.selected[i] {
			check = selectedStyle.Render("[✓]")
		}

		line := fmt.Sprintf("%s%s %s", cursor, check, f.Path)
		if f.Description != "" {
			line += dimStyle.Render("  " + f.Description)
		}
		sb.WriteString(line + "\n")
	}

	// "Other file..." row
	sb.WriteString("\n")
	otherCursor := "  "
	if m.isOtherRow() {
		otherCursor = cursorStyle.Render("▸ ")
	}
	fmt.Fprintf(&sb, "%s%s\n", otherCursor, dimStyle.Render("+ Other file..."))

	count := 0
	for _, v := range m.selected {
		if v {
			count++
		}
	}
	fmt.Fprintf(&sb, "\n  %d of %d selected\n", count, len(m.files))

	return sb.String()
}

// RunSelector presents an interactive multi-select for detected files.
// If preSelected is provided (reinit), only those files start checked.
// Otherwise all detected files start checked (first init).
// projectPath is needed for the file picker when "Other file..." is chosen.
func RunSelector(files []scanner.DetectedFile, preSelected []string, projectPath string) ([]string, error) {
	for {
		m := newSelectorModel(files, preSelected)
		p := tea.NewProgram(m)
		finalModel, err := p.Run()
		if err != nil {
			return nil, fmt.Errorf("selector: %w", err)
		}

		result := finalModel.(selectorModel)
		if result.aborted {
			return nil, fmt.Errorf("aborted")
		}

		if result.openPicker {
			// Launch file picker
			picked, err := RunFilePicker(projectPath)
			if err != nil {
				return nil, err
			}
			if picked != "" {
				// Check if file is already in the list
				alreadyExists := false
				for _, f := range files {
					if f.Path == picked {
						alreadyExists = true
						break
					}
				}
				if !alreadyExists {
					files = append(files, scanner.DetectedFile{
						Path:        picked,
						Category:    "Custom",
						Description: "manually added",
					})
				}
				// Build preSelected from current selections + the new file
				preSelected = nil
				for i, f := range files {
					if result.selected[i] || f.Path == picked {
						preSelected = append(preSelected, f.Path)
					}
				}
			}
			// Loop back to show selector again with the new file
			continue
		}

		var selected []string
		for i, f := range result.files {
			if result.selected[i] {
				selected = append(selected, f.Path)
			}
		}
		return selected, nil
	}
}
