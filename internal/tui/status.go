// Package tui provides interactive terminal UI components.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cmtonkinson/governator/internal/status"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12")).
			MarginLeft(1)

	timestampStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			MarginLeft(1).
			MarginTop(1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			MarginLeft(1)

	countsStyle = lipgloss.NewStyle().
			Bold(true).
			MarginLeft(1).
			MarginBottom(1)
)

// Model represents the interactive status TUI state.
type Model struct {
	table        table.Model
	repoRoot     string
	lastUpdate   time.Time
	err          error
	quitting     bool
	backlog      int
	merged       int
	inProgress   int
	supervisors  int
	planningSteps int
}

type tickMsg time.Time
type statusMsg struct {
	summary status.Summary
}
type errMsg error

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// New creates a new interactive status TUI model.
func New(repoRoot string) Model {
	columns := []table.Column{
		{Title: "ID", Width: 6},
		{Title: "State", Width: 12},
		{Title: "PID", Width: 6},
		{Title: "Role", Width: 12},
		{Title: "Attrs", Width: 18},
		{Title: "Title", Width: 50},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(20),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true).
		Foreground(lipgloss.Color("12"))
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	return Model{
		table:    t,
		repoRoot: repoRoot,
	}
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		m.updateStatus(),
	)
}

// Update handles messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		case "r":
			// Manual refresh
			return m, m.updateStatus()
		}

	case tea.WindowSizeMsg:
		// Adjust table height based on window size
		// Reserve space for header, footer, counts
		m.table.SetHeight(msg.Height - 10)
		return m, nil

	case tickMsg:
		return m, tea.Batch(
			tickCmd(),
			m.updateStatus(),
		)

	case statusMsg:
		m.lastUpdate = time.Now()
		m.backlog = msg.summary.Backlog
		m.merged = msg.summary.Merged
		m.inProgress = msg.summary.InProgress
		m.supervisors = len(msg.summary.Supervisors)
		m.planningSteps = len(msg.summary.PlanningSteps)

		// Convert summary rows to table rows
		rows := make([]table.Row, len(msg.summary.Rows))
		for i, row := range msg.summary.Rows {
			rows[i] = table.Row{
				row.ID(),
				row.State(),
				row.PID(),
				row.Role(),
				row.Attrs(),
				row.Title(),
			}
		}
		m.table.SetRows(rows)
		return m, nil

	case errMsg:
		m.err = msg
		return m, nil
	}

	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// View renders the TUI.
func (m Model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	var b strings.Builder

	// Header with title and timestamp
	title := titleStyle.Render("Governator Status")
	timestamp := timestampStyle.Render(fmt.Sprintf("Last update: %s", m.lastUpdate.Format("15:04:05")))

	header := lipgloss.JoinHorizontal(
		lipgloss.Top,
		title,
		strings.Repeat(" ", 5),
		timestamp,
	)
	b.WriteString(header)
	b.WriteString("\n\n")

	// Counts summary
	counts := countsStyle.Render(fmt.Sprintf(
		"Tasks: backlog=%d merged=%d in-progress=%d | Supervisors: %d | Planning steps: %d",
		m.backlog, m.merged, m.inProgress, m.supervisors, m.planningSteps,
	))
	b.WriteString(counts)
	b.WriteString("\n")

	// Table
	b.WriteString(m.table.View())
	b.WriteString("\n")

	// Help footer
	help := helpStyle.Render("↑/↓: navigate • r: refresh • q/esc: quit")
	b.WriteString(help)

	// Error display
	if m.err != nil {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
	}

	return b.String()
}

func (m Model) updateStatus() tea.Cmd {
	return func() tea.Msg {
		summary, err := status.GetSummary(m.repoRoot)
		if err != nil {
			return errMsg(err)
		}
		return statusMsg{summary: summary}
	}
}

// Run starts the interactive TUI.
func Run(repoRoot string) error {
	p := tea.NewProgram(
		New(repoRoot),
		tea.WithAltScreen(),
	)

	_, err := p.Run()
	return err
}
