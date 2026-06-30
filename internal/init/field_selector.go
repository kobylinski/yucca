package init

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kobylinski/yucca/internal/scanner"
)

var (
	fieldHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4")) // blue
	fieldValueStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))            // yellow
	searchStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))            // cyan
)

// SelectedSecret represents a field the user chose to protect
type SelectedSecret struct {
	File  string // relative file path
	Key   string // dot-notation key within the file
	Value string // the secret value
}

type fieldModel struct {
	file     string
	fields   []scanner.ParsedField
	filtered []int // indices into fields
	selected map[int]bool
	cursor   int
	search   string
	done     bool
	skipped  bool
}

func newFieldModel(file string, fields []scanner.ParsedField) fieldModel {
	indices := make([]int, len(fields))
	for i := range fields {
		indices[i] = i
	}
	return fieldModel{
		file:     file,
		fields:   fields,
		filtered: indices,
		selected: make(map[int]bool),
	}
}

func (m *fieldModel) updateFilter() {
	if m.search == "" {
		m.filtered = make([]int, len(m.fields))
		for i := range m.fields {
			m.filtered[i] = i
		}
	} else {
		q := strings.ToLower(m.search)
		m.filtered = nil
		for i, f := range m.fields {
			if strings.Contains(strings.ToLower(f.Key), q) ||
				strings.Contains(strings.ToLower(f.Value), q) {
				m.filtered = append(m.filtered, i)
			}
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func (m fieldModel) Init() tea.Cmd {
	return nil
}

func (m fieldModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.skipped = true
			m.done = true
			return m, tea.Quit
		case "enter":
			m.done = true
			return m, tea.Quit
		case "tab":
			// Skip this file
			m.skipped = true
			m.done = true
			return m, tea.Quit
		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "ctrl+n":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
		case " ":
			if len(m.filtered) > 0 {
				realIdx := m.filtered[m.cursor]
				m.selected[realIdx] = !m.selected[realIdx]
				// Move down after toggling
				if m.cursor < len(m.filtered)-1 {
					m.cursor++
				}
			}
		case "backspace":
			if len(m.search) > 0 {
				m.search = m.search[:len(m.search)-1]
				m.updateFilter()
			}
		default:
			if len(msg.String()) == 1 && msg.String() != " " {
				m.search += msg.String()
				m.updateFilter()
			}
		}
	}
	return m, nil
}

func (m fieldModel) View() string {
	if m.done {
		return ""
	}

	var sb strings.Builder

	sb.WriteString(fieldHeaderStyle.Render(fmt.Sprintf("  %s", m.file)))
	fmt.Fprintf(&sb, "  (%d fields)\n", len(m.fields))

	// Search bar
	searchLabel := searchStyle.Render("  search: ")
	if m.search != "" {
		sb.WriteString(searchLabel + m.search + "▎")
	} else {
		sb.WriteString(searchLabel + dimStyle.Render("type to filter..."))
	}
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  ↑/↓ navigate • space toggle • enter confirm • tab skip file • esc quit") + "\n\n")

	// Visible window — show max 15 items centered on cursor
	windowSize := 15
	start := 0
	if len(m.filtered) > windowSize {
		start = m.cursor - windowSize/2
		if start < 0 {
			start = 0
		}
		if start+windowSize > len(m.filtered) {
			start = len(m.filtered) - windowSize
		}
	}
	end := start + windowSize
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	if start > 0 {
		sb.WriteString(dimStyle.Render(fmt.Sprintf("  ↑ %d more...", start)) + "\n")
	}

	for vi := start; vi < end; vi++ {
		realIdx := m.filtered[vi]
		f := m.fields[realIdx]

		cursor := "  "
		if vi == m.cursor {
			cursor = cursorStyle.Render("▸ ")
		}

		check := "[ ]"
		if m.selected[realIdx] {
			check = selectedStyle.Render("[✓]")
		}

		masked := fieldValueStyle.Render(scanner.MaskValue(f.Value))
		line := fmt.Sprintf("%s%s %-35s = %s", cursor, check, f.Key, masked)
		sb.WriteString(line + "\n")
	}

	if end < len(m.filtered) {
		sb.WriteString(dimStyle.Render(fmt.Sprintf("  ↓ %d more...", len(m.filtered)-end)) + "\n")
	}

	if len(m.filtered) == 0 && m.search != "" {
		sb.WriteString(dimStyle.Render("  no matches") + "\n")
	}

	count := 0
	for _, v := range m.selected {
		if v {
			count++
		}
	}
	fmt.Fprintf(&sb, "\n  %d selected\n", count)

	return sb.String()
}

// NewFieldModelWithPreselection creates a field model with pre-selected fields
// based on previously configured secrets.
func NewFieldModelWithPreselection(file string, fields []scanner.ParsedField, existing []SelectedSecret) fieldModel {
	m := newFieldModel(file, fields)

	existingKeys := make(map[string]bool)
	for _, s := range existing {
		if s.File == file {
			existingKeys[s.Key] = true
		}
	}

	for i, f := range fields {
		if existingKeys[f.Key] {
			m.selected[i] = true
		}
	}

	return m
}

// IsSelected returns whether the field at index i is selected.
func (m fieldModel) IsSelected(i int) bool {
	return m.selected[i]
}

// RunFieldSelectorWithPreselection shows an interactive field picker with pre-selected fields.
func RunFieldSelectorWithPreselection(file string, fields []scanner.ParsedField, existing []SelectedSecret) ([]SelectedSecret, bool, error) {
	if len(fields) == 0 {
		return nil, false, nil
	}

	m := NewFieldModelWithPreselection(file, fields, existing)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return nil, false, fmt.Errorf("field selector: %w", err)
	}

	result := finalModel.(fieldModel)
	if result.skipped {
		return nil, true, nil
	}

	var secrets []SelectedSecret
	for i, f := range result.fields {
		if result.selected[i] {
			secrets = append(secrets, SelectedSecret{
				File:  file,
				Key:   f.Key,
				Value: f.Value,
			})
		}
	}
	return secrets, false, nil
}

// RunFieldSelector shows an interactive field picker for a single file.
// Returns selected secrets, or nil if skipped.
func RunFieldSelector(file string, fields []scanner.ParsedField) ([]SelectedSecret, bool, error) {
	if len(fields) == 0 {
		return nil, false, nil
	}

	m := newFieldModel(file, fields)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return nil, false, fmt.Errorf("field selector: %w", err)
	}

	result := finalModel.(fieldModel)
	if result.skipped {
		return nil, true, nil
	}

	var secrets []SelectedSecret
	for i, f := range result.fields {
		if result.selected[i] {
			secrets = append(secrets, SelectedSecret{
				File:  file,
				Key:   f.Key,
				Value: f.Value,
			})
		}
	}
	return secrets, false, nil
}
