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
	table         table.Model
	planningTable table.Model
	repoRoot      string
	lastUpdate    time.Time
	err           error
	quitting      bool
	backlog       int
	merged        int
	inProgress    int
	supervisors   []status.SupervisorSummary
	planningSteps []status.PlanningStepSummary
	aggregates    status.AggregateMetrics
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
	// Task table
	taskColumns := []table.Column{
		{Title: "ID", Width: 6},
		{Title: "State", Width: 12},
		{Title: "PID", Width: 6},
		{Title: "Runtime", Width: 8},
		{Title: "Role", Width: 12},
		{Title: "Attrs", Width: 18},
		{Title: "Title", Width: 50},
	}

	taskTable := table.New(
		table.WithColumns(taskColumns),
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
	taskTable.SetStyles(s)

	// Planning steps table
	planningColumns := []table.Column{
		{Title: "Name", Width: 40},
		{Title: "PID", Width: 6},
		{Title: "Runtime", Width: 8},
		{Title: "Status", Width: 12},
	}

	planningTable := table.New(
		table.WithColumns(planningColumns),
		table.WithFocused(false),
		table.WithHeight(5),
	)
	planningTable.SetStyles(s)


	return Model{
		table:         taskTable,
		planningTable: planningTable,
		repoRoot:      repoRoot,
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
		m.supervisors = msg.summary.Supervisors
		m.planningSteps = msg.summary.PlanningSteps
		m.aggregates = msg.summary.Aggregates


		// Convert summary rows to table rows
		rows := make([]table.Row, len(msg.summary.Rows))
		for i, row := range msg.summary.Rows {
			rows[i] = table.Row{
				row.ID(),
				row.State(),
				row.PID(),
				row.Runtime(),
				row.Role(),
				row.Attrs(),
				row.Title(),
			}
		}
		m.table.SetRows(rows)

		// Convert planning steps to table rows
		planningRows := make([]table.Row, len(msg.summary.PlanningSteps))
		for i, step := range msg.summary.PlanningSteps {
			planningRows[i] = table.Row{
				step.Name,
				formatPID(step.PID),
				formatRuntime(step.StartedAt),
				step.Status,
			}
		}
		m.planningTable.SetRows(planningRows)

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

	// Overall metrics section
	overallTitle := titleStyle.Render("Overall Metrics")
	b.WriteString(overallTitle)
	b.WriteString("\n")
	aggregateStr := formatAggregateMetrics(m.aggregates)
	b.WriteString(countsStyle.Render(aggregateStr))
	b.WriteString("\n\n")

	// Supervisor section (if any)
	if len(m.supervisors) > 0 {
		supervisorTitle := titleStyle.Render("Supervisor")
		b.WriteString(supervisorTitle)
		b.WriteString("\n")
		for _, sup := range m.supervisors {
			b.WriteString(renderSupervisorKV(sup))
		}
		b.WriteString("\n")
	}

	// Planning steps table (if in planning phase)
	if len(m.planningSteps) > 0 {
		planningTitle := titleStyle.Render(fmt.Sprintf("Planning Steps (%d)", len(m.planningSteps)))
		b.WriteString(planningTitle)
		b.WriteString("\n")
		b.WriteString(m.planningTable.View())
		b.WriteString("\n\n")
	}

	// Tasks section header
	tasksTitle := titleStyle.Render("Tasks")
	b.WriteString(tasksTitle)
	b.WriteString("\n")

	// Counts summary
	counts := countsStyle.Render(fmt.Sprintf(
		"backlog=%d merged=%d in-progress=%d",
		m.backlog, m.merged, m.inProgress,
	))
	b.WriteString(counts)
	b.WriteString("\n")

	// Task table
	if m.inProgress > 0 {
		b.WriteString(m.table.View())
		b.WriteString("\n")
	}

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

// formatPID formats a PID for display, returning empty string for zero/negative values.
func formatPID(pid int) string {
	if pid <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", pid)
}

// formatRuntime formats the runtime duration since the given start time.
func formatRuntime(startedAt time.Time) string {
	if startedAt.IsZero() {
		return ""
	}
	d := time.Since(startedAt)
	if d < 0 {
		d = 0
	}
	totalSeconds := int64(d.Seconds())
	if totalSeconds < 60 {
		return fmt.Sprintf("%ds", totalSeconds)
	}
	minutes := totalSeconds / 60
	seconds := totalSeconds % 60
	if minutes < 60 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	hours := minutes / 60
	minutes = minutes % 60
	return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
}

// formatDurationShort formats a duration in a compact form.
func formatDurationShort(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalSeconds := int64(d.Seconds())
	if totalSeconds < 60 {
		return fmt.Sprintf("%ds", totalSeconds)
	}
	minutes := totalSeconds / 60
	seconds := totalSeconds % 60
	if minutes < 60 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	hours := minutes / 60
	minutes = minutes % 60
	return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
}

// formatTokens formats token count with thousand separators.
func formatTokens(n int) string {
	if n < 0 {
		n = 0
	}
	s := fmt.Sprintf("%d", n)
	var result strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(c)
	}
	return result.String()
}

// formatAggregateMetrics formats the aggregate metrics section.
func formatAggregateMetrics(agg status.AggregateMetrics) string {
	duration := formatDurationShort(time.Duration(agg.TotalDurationMs) * time.Millisecond)
	totalTokens := formatTokens(agg.TotalTokens)
	inputTokens := formatTokens(agg.TotalTokensPrompt)
	outputTokens := formatTokens(agg.TotalTokensOutput)

	return fmt.Sprintf("Total Runtime: %s | Total Tokens: %s (in: %s | out: %s)",
		duration, totalTokens, inputTokens, outputTokens)
}

// renderSupervisorKV renders supervisor info as key-value pairs.
func renderSupervisorKV(sup status.SupervisorSummary) string {
	keyStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12")).
		Width(12).
		Align(lipgloss.Right).
		MarginLeft(1)

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		MarginLeft(1)

	var b strings.Builder

	renderKV := func(key, value string) {
		b.WriteString(keyStyle.Render(key + ":"))
		b.WriteString(valueStyle.Render(value))
		b.WriteString("\n")
	}

	renderKV("Phase", sup.Phase)
	renderKV("State", sup.State)
	renderKV("PID", formatPID(sup.PID))
	renderKV("Runtime", formatRuntime(sup.StartedAt))

	if sup.WorkerPID > 0 {
		renderKV("Worker PID", formatPID(sup.WorkerPID))
	}
	if sup.ValidationPID > 0 {
		renderKV("Valid PID", formatPID(sup.ValidationPID))
	}

	return b.String()
}
