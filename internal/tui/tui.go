package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The TUI is a focused APPROVAL CONSOLE for headless / SSH-only environments
// where there is no browser or tray. It connects to the daemon, shows the pending
// request queue (secret_request / execute_accept / clipboard_copy) and lets the
// operator provide a value, approve, or deny. Secret management lives in the web UI.

// Messages
type tickMsg time.Time
type healthCheckMsg struct{ err error }
type pendingMsg struct {
	requests []SecretRequest
	err      error
}
type sessionsMsg struct {
	sessions []ActiveSession
	err      error
}
type actionDoneMsg struct{ err error }

type Model struct {
	client *Client
	width  int
	height int
	err    error

	sessions []ActiveSession

	// Request console state
	pendingRequests  []SecretRequest
	requestGroups    []requestGroup
	groupIndex       int
	interruptInput   textinput.Model
	interruptPolicy  int
	interruptFocus   int // secret: 0=value 1=policy 2=buttons; exec: 0=buttons
	interruptButtons buttonBar
}

var policyOptions = []string{"always_allow", "ask_session", "ask_always"}
var policyLabels = []string{"Always allow", "Ask per session", "Ask every time"}

type requestGroup struct {
	reason   string
	isExec   bool
	requests []SecretRequest
}

// groupPendingRequests batches the credentials of a single exec command into one
// approval, while standalone secret requests stay individual.
func groupPendingRequests(requests []SecretRequest) []requestGroup {
	var groups []requestGroup
	execIndex := make(map[string]int) // reason -> index in groups
	for _, req := range requests {
		if strings.HasPrefix(req.Reason, "exec: ") {
			if idx, ok := execIndex[req.Reason]; ok {
				groups[idx].requests = append(groups[idx].requests, req)
			} else {
				execIndex[req.Reason] = len(groups)
				groups = append(groups, requestGroup{reason: req.Reason, isExec: true, requests: []SecretRequest{req}})
			}
		} else {
			groups = append(groups, requestGroup{reason: req.Reason, isExec: false, requests: []SecretRequest{req}})
		}
	}
	return groups
}

func NewModel(daemonAddr string) Model {
	ii := textinput.New()
	ii.Placeholder = "Enter secret value"
	ii.EchoMode = textinput.EchoPassword
	ii.EchoCharacter = '*'
	return Model{
		client:          NewClient(daemonAddr),
		interruptInput:  ii,
		interruptPolicy: 1, // default: ask_session
		interruptButtons: newButtonBar(
			buttonDef{label: "Approve", kind: btnKindNormal},
			buttonDef{label: "Deny", kind: btnKindDanger},
		),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.checkHealth, m.tickCmd())
}

// active reports whether a request is currently in front of the operator.
func (m Model) active() bool {
	return len(m.requestGroups) > 0 && m.groupIndex < len(m.requestGroups)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// When idle, q/esc quit. While a request is active these fall through to
		// the console so they can be typed into the value field / handled there.
		if !m.active() && (msg.String() == "q" || msg.String() == "esc") {
			return m, tea.Quit
		}

	case healthCheckMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Quit
		}
		return m, tea.Batch(m.pollRequests, m.loadSessions)

	case pendingMsg:
		if msg.err == nil {
			wasActive := len(m.requestGroups) > 0
			m.pendingRequests = msg.requests
			m.requestGroups = groupPendingRequests(msg.requests)
			if m.groupIndex >= len(m.requestGroups) {
				m.groupIndex = 0
			}
			if len(m.requestGroups) > 0 && !wasActive {
				m.enterRequests()
			}
		}
		return m, nil

	case sessionsMsg:
		if msg.err == nil {
			m.sessions = msg.sessions
		}
		return m, nil

	case wsEventMsg:
		switch msg.Type {
		case "request_created", "request_resolved":
			return m, m.pollRequests
		case "sessions_changed":
			return m, m.loadSessions
		}
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.pollRequests, m.loadSessions, m.tickCmd())

	case actionDoneMsg:
		return m, m.pollRequests
	}

	// Delegate to the request console when one is active.
	if m.active() {
		return m.updateInterrupt(msg)
	}
	return m, nil
}

func (m *Model) enterRequests() {
	m.groupIndex = 0
	m.interruptInput.Reset()
	m.interruptPolicy = 1
	m.interruptFocus = 0
	m.interruptButtons.blur()
	m.focusInterruptInput()
	fmt.Fprint(os.Stderr, "\a") // ring the bell — the operator may be in another pane
}

// focusInterruptInput sets focus based on whether the current group needs a value.
func (m *Model) focusInterruptInput() {
	if m.groupIndex < len(m.requestGroups) && m.requestGroups[m.groupIndex].isExec {
		m.interruptInput.Blur()
		m.interruptButtons.focus()
	} else {
		m.interruptInput.Focus()
	}
}

func (m *Model) advanceInterrupt() {
	m.interruptInput.Reset()
	m.interruptPolicy = 1
	m.interruptFocus = 0
	m.interruptButtons.blur()
	m.groupIndex++
	if m.groupIndex < len(m.requestGroups) {
		m.focusInterruptInput()
	}
}

func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("\n  Error: %s\n\n  Daemon not reachable. Start a session or run 'yucca daemon start'.\n", m.err)
	}

	width := m.width - 2
	if width < 24 {
		width = 24
	}
	bodyHeight := m.height - 4
	if bodyHeight < 6 {
		bodyHeight = 6
	}

	var body string
	if m.active() {
		body = m.renderInterrupt(width-2, bodyHeight)
	} else {
		idle := lipgloss.JoinVertical(lipgloss.Center,
			titleStyle.Render("Waiting for approval requests…"),
			"",
			labelStyle.Render("Keep this console open. Pending secret and exec"),
			labelStyle.Render("approvals from your agent appear here."),
		)
		body = lipgloss.Place(width-2, bodyHeight, lipgloss.Center, lipgloss.Center, idle)
	}
	pane := paneFocusedStyle.Width(width - 2).Height(bodyHeight).Render(body)

	header := titleStyle.Render("yucca · approval console")

	sess := fmt.Sprintf("%d active session(s)", len(m.sessions))
	help := "q / ctrl+c quit"
	if m.active() {
		help = "tab cycle · ←→ button · enter activate · ctrl+c quit"
	}
	status := statusBarStyle.Width(width).Render(labelStyle.Render(sess) + "   " + helpStyle.Render(help))

	return lipgloss.JoinVertical(lipgloss.Left, header, pane, status)
}

// Commands

func (m Model) checkHealth() tea.Msg { return healthCheckMsg{err: m.client.Health()} }

func (m Model) loadSessions() tea.Msg {
	sessions, err := m.client.FetchSessions()
	return sessionsMsg{sessions: sessions, err: err}
}

func (m Model) pollRequests() tea.Msg {
	reqs, err := m.client.FetchPendingRequests()
	return pendingMsg{requests: reqs, err: err}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Run starts the approval console.
func Run(daemonAddr string) error {
	m := NewModel(daemonAddr)
	p := tea.NewProgram(m, tea.WithAltScreen())
	go wsListener(daemonAddr, p)
	_, err := p.Run()
	return err
}
