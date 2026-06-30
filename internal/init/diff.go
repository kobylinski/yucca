package init

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	addedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	removedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	headerStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4"))
	unchangedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	stepDoneStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	bulletStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// ComputeJSONDiff compares old and new JSON content.
func ComputeJSONDiff(old, new []byte) (changed bool, oldFormatted, newFormatted string) {
	newFormatted = prettyJSON(new)

	if len(old) == 0 {
		return true, "", newFormatted
	}

	oldFormatted = prettyJSON(old)

	var oldParsed, newParsed any
	json.Unmarshal(old, &oldParsed)
	json.Unmarshal(new, &newParsed)

	oldNorm, _ := json.Marshal(oldParsed)
	newNorm, _ := json.Marshal(newParsed)

	if string(oldNorm) == string(newNorm) {
		return false, oldFormatted, newFormatted
	}

	return true, oldFormatted, newFormatted
}

// FormatDiffView returns a unified diff display.
func FormatDiffView(filename, oldContent, newContent string) string {
	var sb strings.Builder

	if oldContent == "" {
		sb.WriteString(headerStyle.Render("  "+filename) + "  " + addedStyle.Render("(new file)") + "\n\n")
		for _, line := range strings.Split(newContent, "\n") {
			sb.WriteString(addedStyle.Render("  + "+line) + "\n")
		}
		return sb.String()
	}

	sb.WriteString(headerStyle.Render("  "+filename) + "  " + unchangedStyle.Render("(modified)") + "\n\n")

	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	edits := diffLines(oldLines, newLines)
	for _, e := range edits {
		switch e.op {
		case opEqual:
			sb.WriteString(unchangedStyle.Render("    "+e.text) + "\n")
		case opDelete:
			sb.WriteString(removedStyle.Render("  - "+e.text) + "\n")
		case opInsert:
			sb.WriteString(addedStyle.Render("  + "+e.text) + "\n")
		}
	}

	return sb.String()
}

type editOp int

const (
	opEqual editOp = iota
	opDelete
	opInsert
)

type edit struct {
	op   editOp
	text string
}

// diffLines computes a unified diff between two slices of lines using LCS.
func diffLines(old, new []string) []edit {
	// Build LCS table
	m, n := len(old), len(new)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if old[i] == new[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	// Walk the table to produce edits
	var edits []edit
	i, j := 0, 0
	for i < m && j < n {
		if old[i] == new[j] {
			edits = append(edits, edit{opEqual, old[i]})
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			edits = append(edits, edit{opDelete, old[i]})
			i++
		} else {
			edits = append(edits, edit{opInsert, new[j]})
			j++
		}
	}
	for ; i < m; i++ {
		edits = append(edits, edit{opDelete, old[i]})
	}
	for ; j < n; j++ {
		edits = append(edits, edit{opInsert, new[j]})
	}

	return edits
}

func prettyJSON(data []byte) string {
	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return string(data)
	}
	pretty, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return string(data)
	}
	return string(pretty)
}

// StepDone prints a completed step with a checkmark.
func StepDone(label string) {
	dots := strings.Repeat(".", max(2, 70-len(label)))
	fmt.Println(label + " " + unchangedStyle.Render(dots) + " " + stepDoneStyle.Render("✓"))
}

// StepBullet prints an indented bullet point detail.
func StepBullet(text string) {
	fmt.Println(bulletStyle.Render("  • ") + text)
}
