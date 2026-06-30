package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// interruptFocus for secret requests: 0=value input, 1=policy, 2=buttons
// interruptFocus for exec requests:   0=buttons only (no value, no policy)

func (m Model) interruptNeedsValue() bool {
	if m.groupIndex >= len(m.requestGroups) {
		return false
	}
	return !m.requestGroups[m.groupIndex].isExec
}

func (m Model) updateInterrupt(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.groupIndex >= len(m.requestGroups) {
		return m, nil
	}

	needsValue := m.interruptNeedsValue()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		buttonsFocus := 2
		if !needsValue {
			buttonsFocus = 0
		}

		// Buttons focused
		if m.interruptFocus == buttonsFocus {
			switch msg.String() {
			case "tab":
				m.interruptButtons.blur()
				if needsValue {
					m.interruptFocus = 0
					m.interruptInput.Focus()
				} else {
					m.interruptFocus = 0
					m.interruptButtons.focus()
				}
				return m, nil
			case "shift+tab":
				m.interruptButtons.blur()
				if needsValue {
					m.interruptFocus = 1 // policy
				} else {
					m.interruptFocus = 0
					m.interruptButtons.focus()
				}
				return m, nil
			}
			action := m.interruptButtons.update(msg)
			switch action {
			case "Approve":
				group := m.requestGroups[m.groupIndex]
				value := ""
				policy := "ask_session"
				if needsValue {
					value = m.interruptInput.Value()
					policy = policyOptions[m.interruptPolicy]
				}
				m.advanceInterrupt()
				return m, func() tea.Msg {
					var lastErr error
					for _, req := range group.requests {
						if err := m.client.ApproveRequest(req.ID, value, policy); err != nil {
							lastErr = err
						}
					}
					return actionDoneMsg{err: lastErr}
				}
			case "Deny":
				group := m.requestGroups[m.groupIndex]
				m.advanceInterrupt()
				return m, func() tea.Msg {
					var lastErr error
					for _, req := range group.requests {
						if err := m.client.DenyRequest(req.ID); err != nil {
							lastErr = err
						}
					}
					return actionDoneMsg{err: lastErr}
				}
			}
			return m, nil
		}

		// Value input focused (only when needsValue)
		if needsValue && m.interruptFocus == 0 {
			switch msg.String() {
			case "tab":
				m.interruptInput.Blur()
				m.interruptFocus = 1 // policy
				return m, nil
			case "shift+tab":
				m.interruptInput.Blur()
				m.interruptFocus = 2 // buttons
				m.interruptButtons.focus()
				return m, nil
			}
			var cmd tea.Cmd
			m.interruptInput, cmd = m.interruptInput.Update(msg)
			return m, cmd
		}

		// Policy focused (only when needsValue)
		if needsValue && m.interruptFocus == 1 {
			switch msg.String() {
			case "tab":
				m.interruptFocus = 2 // buttons
				m.interruptButtons.focus()
				return m, nil
			case "shift+tab":
				m.interruptFocus = 0 // value
				m.interruptInput.Focus()
				return m, nil
			case "up", "k":
				if m.interruptPolicy > 0 {
					m.interruptPolicy--
				}
				return m, nil
			case "down", "j":
				if m.interruptPolicy < len(policyOptions)-1 {
					m.interruptPolicy++
				}
				return m, nil
			}
		}
	}

	return m, nil
}

func (m Model) renderInterrupt(width, height int) string {
	if m.groupIndex >= len(m.requestGroups) {
		return labelStyle.Render("No pending requests")
	}

	group := m.requestGroups[m.groupIndex]

	var b strings.Builder

	if group.isExec {
		b.WriteString(titleStyle.Render("● Exec Approval") + "\n\n")

		command := strings.TrimPrefix(group.reason, "exec: ")
		b.WriteString(labelStyle.Render("Command") + "\n")
		b.WriteString(valueStyle.Render(command) + "\n\n")

		b.WriteString(labelStyle.Render(fmt.Sprintf("Credentials (%d)", len(group.requests))) + "\n")
		for _, req := range group.requests {
			b.WriteString("  " + aliasStyle.Render(req.Alias) + "\n")
		}
		b.WriteString("\n")

		if len(group.requests) > 0 {
			b.WriteString(labelStyle.Render("Project") + "\n")
			b.WriteString(valueStyle.Render(group.requests[0].ProjectPath) + "\n\n")
		}
	} else {
		req := group.requests[0]

		b.WriteString(titleStyle.Render("● Secret Request") + "\n\n")

		b.WriteString(labelStyle.Render("Alias") + "\n")
		b.WriteString(aliasStyle.Render(req.Alias) + "\n\n")

		b.WriteString(labelStyle.Render("Reason") + "\n")
		b.WriteString(valueStyle.Render("\""+req.Reason+"\"") + "\n\n")

		b.WriteString(labelStyle.Render("Project") + "\n")
		b.WriteString(valueStyle.Render(req.ProjectPath) + "\n\n")
	}

	// Secret requests: show value input + policy selector
	if !group.isExec {
		b.WriteString(titleStyle.Render("Value") + "\n")
		inputWidth := width - 4
		if inputWidth < 10 {
			inputWidth = 10
		}
		m.interruptInput.Width = inputWidth
		b.WriteString(m.interruptInput.View() + "\n\n")

		b.WriteString(titleStyle.Render("Policy") + "\n")
		for i, label := range policyLabels {
			marker := "  ( ) "
			if i == m.interruptPolicy {
				marker = "  (•) "
			}
			if m.interruptFocus == 1 && i == m.interruptPolicy {
				marker = "> (•) "
			}
			style := normalItemStyle
			if i == m.interruptPolicy {
				style = selectedItemStyle
			}
			b.WriteString(style.Render(marker+label) + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(m.interruptButtons.view())
	b.WriteString("\n\n")

	remaining := len(m.requestGroups) - m.groupIndex
	helpText := "tab cycle fields  ←→ switch button  enter activate"
	if remaining > 1 {
		helpText = fmt.Sprintf("%d remaining  %s", remaining, helpText)
	}
	b.WriteString(helpStyle.Render(helpText))

	return b.String()
}
