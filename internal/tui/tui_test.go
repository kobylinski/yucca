package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestModel_InitialStateIsIdle(t *testing.T) {
	m := NewModel("http://127.0.0.1:9777")
	assert.Empty(t, m.requestGroups)
	assert.Empty(t, m.pendingRequests)
	assert.False(t, m.active())
}

func TestModel_PendingBuildsGroups(t *testing.T) {
	m := NewModel("http://127.0.0.1:9777")
	updated, _ := m.Update(pendingMsg{requests: []SecretRequest{
		{ID: "r1", Alias: "DB_PASS", Reason: "exec: npm run dev", Status: "pending"},
		{ID: "r2", Alias: "API_KEY", Reason: "exec: npm run dev", Status: "pending"},
		{ID: "r3", Alias: "TOKEN", Reason: "manual request", Status: "pending"},
	}})
	m = updated.(Model)
	assert.True(t, m.active())
	assert.Equal(t, 2, len(m.requestGroups))
	assert.Equal(t, 0, m.groupIndex)
}

func TestModel_PendingClearsWhenEmpty(t *testing.T) {
	m := NewModel("http://127.0.0.1:9777")
	updated, _ := m.Update(pendingMsg{requests: []SecretRequest{
		{ID: "r1", Alias: "X", Reason: "manual request", Status: "pending"},
	}})
	m = updated.(Model)
	assert.True(t, m.active())

	updated, _ = m.Update(pendingMsg{requests: nil})
	m = updated.(Model)
	assert.False(t, m.active())
}

func TestModel_QuitWhenIdle(t *testing.T) {
	m := NewModel("http://127.0.0.1:9777")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	assert.NotNil(t, cmd) // idle → q quits
}

func TestModel_CtrlCQuits(t *testing.T) {
	m := NewModel("http://127.0.0.1:9777")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	assert.NotNil(t, cmd)
}

func TestGroupPendingRequests(t *testing.T) {
	requests := []SecretRequest{
		{ID: "r1", Alias: "DB_PASS", Reason: "exec: npm run dev", Status: "pending"},
		{ID: "r2", Alias: "API_KEY", Reason: "exec: npm run dev", Status: "pending"},
		{ID: "r3", Alias: "TOKEN", Reason: "manual request", Status: "pending"},
	}
	groups := groupPendingRequests(requests)
	assert.Equal(t, 2, len(groups))
	assert.True(t, groups[0].isExec)
	assert.Equal(t, "exec: npm run dev", groups[0].reason)
	assert.Equal(t, 2, len(groups[0].requests))
	assert.False(t, groups[1].isExec)
	assert.Equal(t, "manual request", groups[1].reason)
	assert.Equal(t, 1, len(groups[1].requests))
}

func TestGroupPendingRequests_DifferentExecCommands(t *testing.T) {
	requests := []SecretRequest{
		{ID: "r1", Alias: "DB_PASS", Reason: "exec: npm run dev", Status: "pending"},
		{ID: "r2", Alias: "API_KEY", Reason: "exec: go run .", Status: "pending"},
		{ID: "r3", Alias: "TOKEN", Reason: "exec: npm run dev", Status: "pending"},
	}
	groups := groupPendingRequests(requests)
	assert.Equal(t, 2, len(groups))
	assert.Equal(t, 2, len(groups[0].requests)) // npm run dev: r1 + r3
	assert.Equal(t, 1, len(groups[1].requests)) // go run .: r2
}

func TestGroupPendingRequests_Empty(t *testing.T) {
	groups := groupPendingRequests(nil)
	assert.Equal(t, 0, len(groups))
}
