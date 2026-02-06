// Package status provides task index status reporting.
package status

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/inflight"
	"github.com/cmtonkinson/governator/internal/run"
	"github.com/cmtonkinson/governator/internal/supervisor"
	"golang.org/x/term"
)

const (
	idColumnWidth       = 14
	stateColumnWidth    = 12
	pidColumnWidth      = 6
	roleColumnWidth     = 12
	attrsColumnWidth    = 18
	titleMaxWidth       = 40
	planningIDWidth     = 24
	planningStatusWidth = 12
)

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12")).
			Align(lipgloss.Left)

	cellStyle = lipgloss.NewStyle().
			Align(lipgloss.Left)

	tableStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	separatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	countsStyle = lipgloss.NewStyle().
			Bold(true)
)

// statusStateOrder prioritizes tasks closer to completion.
// Lower numbers = higher priority (sorted first).
var statusStateOrder = map[index.TaskState]int{
	index.TaskStateMergeable:   0, // Highest priority - ready to merge
	index.TaskStateReviewed:    1,
	index.TaskStateTested:      2,
	index.TaskStateImplemented: 3,
	index.TaskStateTriaged:     4, // Lowest priority - just started
}

// Summary represents task counts and the in-progress table.
type Summary struct {
	Supervisors   []SupervisorSummary
	Workers       []WorkerSummary
	PlanningSteps []PlanningStepSummary
	Total         int
	Backlog       int
	Merged        int
	InProgress    int
	Rows          []StatusRow // Active (non-merged) tasks
	MergedRows    []StatusRow // Merged tasks (kept separate)
	Aggregates    AggregateMetrics
}

// AggregateMetrics holds cumulative metrics across all tasks.
type AggregateMetrics struct {
	TotalDurationMs   int64 // Sum of all DurationMs + elapsed time for in-flight
	TotalTokensPrompt int   // Sum of TokensPrompt
	TotalTokensOutput int   // Sum of TokensResponse
	TotalTokens       int   // Sum of TokensTotal
}

// StatusRow represents a single task row in the status display.
type StatusRow struct {
	id      string
	state   string
	pid     string
	runtime string
	role    string
	attrs   string
	title   string
	order   int
}

// Accessor methods for StatusRow (used by TUI package)
func (r StatusRow) ID() string      { return r.id }
func (r StatusRow) State() string   { return r.state }
func (r StatusRow) PID() string     { return r.pid }
func (r StatusRow) Runtime() string { return r.runtime }
func (r StatusRow) Role() string    { return r.role }
func (r StatusRow) Attrs() string   { return r.attrs }
func (r StatusRow) Title() string   { return r.title }

// NewSeparatorRow creates an empty row used as a visual separator.
func NewSeparatorRow() StatusRow {
	return StatusRow{}
}

// SupervisorSummary captures the status output for a supervisor.
type SupervisorSummary struct {
	Phase          string
	State          string
	PID            int
	WorkerPID      int
	ValidationPID  int
	StepID         string
	StepName       string
	StartedAt      time.Time
	LastTransition time.Time
	LogPath        string
}

// WorkerSummary captures the status output for an active worker.
type WorkerSummary struct {
	PID       int
	Role      string
	StartedAt time.Time
}

// PlanningStepSummary captures the status output for a planning step.
type PlanningStepSummary struct {
	ID        string
	Name      string
	Status    string
	Order     int
	PID       int       // From supervisor if this step is currently running
	StartedAt time.Time // From supervisor if this step is currently running
}

// String returns the formatted status output per flow.md.
// Uses styled output for TTY, plain output for pipes/redirects.
func (s Summary) String() string {
	// Check if output is a TTY
	isTTY := term.IsTerminal(int(os.Stdout.Fd()))

	if !isTTY {
		return s.plainString()
	}

	return s.styledString()
}

// plainString returns plain text output for pipes/redirects.
func (s Summary) plainString() string {
	var b strings.Builder

	// Overall metrics
	fmt.Fprintln(&b, "overall")
	fmt.Fprintln(&b, formatAggregateMetrics(s.Aggregates))

	if len(s.Supervisors) > 0 {
		fmt.Fprintln(&b, "supervisor")
		for _, supervisor := range s.Supervisors {
			fmt.Fprintf(&b, "phase=%s\n", normalizeToken(supervisor.Phase))
			fmt.Fprintf(&b, "state=%s\n", normalizeToken(supervisor.State))
			fmt.Fprintf(&b, "pid=%s\n", formatPID(supervisor.PID))
			fmt.Fprintf(&b, "runtime=%s\n", formatSupervisorRuntime(supervisor.StartedAt))
			if supervisor.WorkerPID > 0 {
				fmt.Fprintf(&b, "worker_pid=%s\n", formatPID(supervisor.WorkerPID))
			}
			if supervisor.ValidationPID > 0 {
				fmt.Fprintf(&b, "validation_pid=%s\n", formatPID(supervisor.ValidationPID))
			}
			fmt.Fprintf(&b, "step_id=%s\n", normalizeToken(supervisor.StepID))
			fmt.Fprintf(&b, "step_name=%s\n", normalizeToken(supervisor.StepName))
			fmt.Fprintf(&b, "started_at=%s\n", formatTime(supervisor.StartedAt))
			fmt.Fprintf(&b, "last_transition=%s\n", formatTime(supervisor.LastTransition))
			fmt.Fprintf(&b, "log=%s\n", normalizeToken(supervisor.LogPath))
		}
	}
	if len(s.PlanningSteps) > 0 {
		fmt.Fprintf(&b, "planning-steps=%d\n", len(s.PlanningSteps))
		fmt.Fprintf(&b, "%-40s %-6s %-8s %-*s\n",
			"name",
			"pid",
			"runtime",
			planningStatusWidth, "status",
		)
		for _, step := range s.PlanningSteps {
			fmt.Fprintf(&b, "%-40s %-6s %-8s %-*s\n",
				step.Name,
				formatPID(step.PID),
				formatSupervisorRuntime(step.StartedAt),
				planningStatusWidth, step.Status,
			)
		}
	}
	fmt.Fprintln(&b, "tasks")
	fmt.Fprintf(&b, "backlog=%d merged=%d in-progress=%d\n", s.Backlog, s.Merged, s.InProgress)
	if s.InProgress == 0 {
		return strings.TrimSpace(b.String())
	}
	fmt.Fprintf(&b, "%-*s %-*s %-*s %-*s %-*s %-*s %s\n",
		idColumnWidth, "id",
		stateColumnWidth, "state",
		pidColumnWidth, "pid",
		8, "runtime",
		roleColumnWidth, "role",
		attrsColumnWidth, "attrs",
		"title",
	)
	for _, row := range s.Rows {
		fmt.Fprintf(&b, "%-*s %-*s %-*s %-*s %-*s %-*s %s\n",
			idColumnWidth, row.id,
			stateColumnWidth, row.state,
			pidColumnWidth, row.pid,
			8, row.runtime,
			roleColumnWidth, row.role,
			attrsColumnWidth, row.attrs,
			row.title,
		)
	}
	return strings.TrimSpace(b.String())
}

// styledString returns lipgloss-styled output for interactive terminals.
func (s Summary) styledString() string {
	var b strings.Builder

	// Get terminal width for responsive layout
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		width = 120 // fallback
	}

	// Overall metrics section
	b.WriteString(headerStyle.Render("Overall Metrics"))
	b.WriteString("\n")
	aggregateStr := formatAggregateMetrics(s.Aggregates)
	b.WriteString(countsStyle.Render(aggregateStr))
	b.WriteString("\n\n")

	// Supervisor section
	if len(s.Supervisors) > 0 {
		b.WriteString(headerStyle.Render("Supervisor"))
		b.WriteString("\n")
		for _, supervisor := range s.Supervisors {
			b.WriteString(renderSupervisorKV(supervisor))
		}
		b.WriteString("\n")
	}

	// Workers section
	if len(s.Workers) > 0 {
		b.WriteString(headerStyle.Render(fmt.Sprintf("Workers (%d)", len(s.Workers))))
		b.WriteString("\n")
		workersTable := renderWorkersTable(s.Workers, width)
		b.WriteString(tableStyle.Render(workersTable))
		b.WriteString("\n\n")
	}

	// Planning steps section
	if len(s.PlanningSteps) > 0 {
		b.WriteString(headerStyle.Render(fmt.Sprintf("Planning Steps (%d)", len(s.PlanningSteps))))
		b.WriteString("\n")
		planningTable := renderPlanningTable(s.PlanningSteps, width)
		b.WriteString(tableStyle.Render(planningTable))
		b.WriteString("\n\n")
	}

	// Tasks section
	b.WriteString(headerStyle.Render("Tasks"))
	b.WriteString("\n")
	b.WriteString(countsStyle.Render(fmt.Sprintf("backlog=%d merged=%d in-progress=%d",
		s.Backlog, s.Merged, s.InProgress)))
	b.WriteString("\n")

	// Task table
	if s.InProgress > 0 {
		taskTable := renderTaskTable(s.Rows, width)
		b.WriteString(tableStyle.Render(taskTable))
	}

	return strings.TrimSpace(b.String())
}

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

func formatSupervisorRuntime(startedAt time.Time) string {
	if startedAt.IsZero() {
		return "-"
	}
	return formatDurationShort(time.Since(startedAt))
}

func formatTaskRuntime(startedAt time.Time) string {
	if startedAt.IsZero() {
		return "-"
	}
	return formatDurationShort(time.Since(startedAt))
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
func formatAggregateMetrics(agg AggregateMetrics) string {
	duration := formatDurationShort(time.Duration(agg.TotalDurationMs) * time.Millisecond)
	totalTokens := formatTokens(agg.TotalTokens)
	inputTokens := formatTokens(agg.TotalTokensPrompt)
	outputTokens := formatTokens(agg.TotalTokensOutput)

	return fmt.Sprintf("Total Runtime: %s | Total Tokens: %s (in: %s | out: %s)",
		duration, totalTokens, inputTokens, outputTokens)
}

// GetSummary reads the task index and returns a detailed summary.
func GetSummary(repoRoot string) (Summary, error) {
	indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")

	idx, err := index.Load(indexPath)
	if err != nil {
		return Summary{}, fmt.Errorf("load task index: %w", err)
	}

	// Load in-flight store to get task start times
	inflightStore, err := inflight.NewStore(repoRoot)
	if err != nil {
		return Summary{}, fmt.Errorf("create in-flight store: %w", err)
	}
	inflightSet, err := inflightStore.Load()
	if err != nil {
		return Summary{}, fmt.Errorf("load in-flight data: %w", err)
	}

	var rows []StatusRow
	summary := Summary{Total: len(idx.Tasks)}

	// Calculate aggregate metrics
	var aggregates AggregateMetrics
	for _, task := range idx.Tasks {
		if task.Kind != index.TaskKindExecution {
			continue
		}
		// Add completed task metrics
		aggregates.TotalDurationMs += task.Metrics.DurationMs
		aggregates.TotalTokensPrompt += task.Metrics.TokensPrompt
		aggregates.TotalTokensOutput += task.Metrics.TokensResponse
		aggregates.TotalTokens += task.Metrics.TokensTotal

		// Add elapsed time for in-flight tasks
		if task.State != index.TaskStateBacklog && task.State != index.TaskStateMerged {
			if startedAt, ok := inflightSet.StartedAt(task.ID); ok {
				elapsed := time.Since(startedAt)
				aggregates.TotalDurationMs += int64(elapsed / time.Millisecond)
			}
		}
	}
	summary.Aggregates = aggregates

	if supervisorState, ok, err := supervisor.LoadPlanningState(repoRoot); err != nil {
		return Summary{}, fmt.Errorf("load planning supervisor state: %w", err)
	} else if ok {
		state := supervisorState
		if supervisorState.State == supervisor.SupervisorStateRunning {
			runningState, running, err := supervisor.PlanningSupervisorRunning(repoRoot)
			if err != nil {
				return Summary{}, fmt.Errorf("check planning supervisor: %w", err)
			}
			if running {
				state = runningState
			} else {
				state.State = supervisor.SupervisorStateFailed
			}
		}
		if state.State == supervisor.SupervisorStateRunning || state.State == supervisor.SupervisorStateFailed {
			summary.Supervisors = append(summary.Supervisors, SupervisorSummary{
				Phase:          strings.TrimSpace(state.Phase),
				State:          string(state.State),
				PID:            state.PID,
				WorkerPID:      state.WorkerPID,
				ValidationPID:  state.ValidationPID,
				StepID:         state.StepID,
				StepName:       state.StepName,
				StartedAt:      state.StartedAt,
				LastTransition: state.LastTransition,
				LogPath:        state.LogPath,
			})
		}
	}
	if supervisorState, ok, err := supervisor.LoadExecutionState(repoRoot); err != nil {
		return Summary{}, fmt.Errorf("load execution supervisor state: %w", err)
	} else if ok {
		state := supervisorState
		if supervisorState.State == supervisor.SupervisorStateRunning {
			runningState, running, err := supervisor.ExecutionSupervisorRunning(repoRoot)
			if err != nil {
				return Summary{}, fmt.Errorf("check execution supervisor: %w", err)
			}
			if running {
				state = runningState
			} else {
				state.State = supervisor.SupervisorStateFailed
			}
		}
		if state.State == supervisor.SupervisorStateRunning || state.State == supervisor.SupervisorStateFailed {
			summary.Supervisors = append(summary.Supervisors, SupervisorSummary{
				Phase:          strings.TrimSpace(state.Phase),
				State:          string(state.State),
				PID:            state.PID,
				WorkerPID:      state.WorkerPID,
				ValidationPID:  state.ValidationPID,
				StepID:         state.StepID,
				StepName:       state.StepName,
				StartedAt:      state.StartedAt,
				LastTransition: state.LastTransition,
				LogPath:        state.LogPath,
			})
			// Collect active workers from execution supervisor (during triage step)
			if state.WorkerPID > 0 && state.StepID == "triage" {
				summary.Workers = append(summary.Workers, WorkerSummary{
					PID:       state.WorkerPID,
					Role:      "triage",
					StartedAt: state.LastTransition,
				})
			}
		}
	}

	var mergedRows []StatusRow
	for _, task := range idx.Tasks {
		if task.Kind != index.TaskKindExecution {
			continue
		}

		// Track state counts and skip backlog tasks
		if task.State == index.TaskStateBacklog {
			summary.Backlog++
			continue
		}

		// Count merged vs in-progress
		if task.State == index.TaskStateMerged {
			summary.Merged++
		} else {
			summary.InProgress++
		}

		// Calculate runtime from in-flight store
		// Only show runtime if there's an active worker PID
		var runtime string
		if task.PID > 0 {
			if startedAt, ok := inflightSet.StartedAt(task.ID); ok {
				runtime = formatTaskRuntime(startedAt)
			} else {
				runtime = "-"
			}
		} else {
			runtime = "-"
		}

		row := StatusRow{
			id:      extractNumericPrefix(task.ID),
			state:   currentStatus(task),
			pid:     formatPID(task.PID),
			runtime: runtime,
			role:    resolveAssignedRole(task),
			attrs:   formatAttrs(task),
			title:   truncateTitle(task.Title, titleMaxWidth),
			order:   statusOrder(task.State),
		}

		// Separate merged tasks from active tasks
		if task.State == index.TaskStateMerged {
			mergedRows = append(mergedRows, row)
		} else {
			rows = append(rows, row)
		}
	}

	// Sort active tasks by priority (closer to done = higher)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].order != rows[j].order {
			return rows[i].order < rows[j].order
		}
		return rows[i].id < rows[j].id
	})

	// Sort merged tasks by ID
	sort.Slice(mergedRows, func(i, j int) bool {
		return mergedRows[i].id < mergedRows[j].id
	})

	summary.Rows = rows
	summary.MergedRows = mergedRows
	steps, err := planningStepSummary(repoRoot, idx, summary.Supervisors)
	if err != nil {
		return Summary{}, err
	}
	if len(steps) > 0 {
		summary.PlanningSteps = steps
	}
	return summary, nil
}

func statusOrder(state index.TaskState) int {
	if rank, ok := statusStateOrder[state]; ok {
		return rank
	}
	return len(statusStateOrder)
}

func formatPID(pid int) string {
	if pid <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", pid)
}

func resolveAssignedRole(task index.Task) string {
	if role := strings.TrimSpace(task.AssignedRole); role != "" {
		return role
	}
	return string(task.Role)
}

// currentStatus returns the current activity status of a task.
// If a worker is actively working on the task (PID > 0), it shows what the worker is doing.
// Otherwise, it shows the task's current state.
func currentStatus(task index.Task) string {
	// If no active worker, just show the state
	if task.PID <= 0 {
		return string(task.State)
	}

	// Active worker - derive status from state
	switch task.State {
	case index.TaskStateTriaged:
		return "implementing"
	case index.TaskStateImplemented:
		return "testing"
	case index.TaskStateTested:
		return "reviewing"
	case index.TaskStateReviewed:
		// Could be resolving conflicts or ready for merge
		if task.MergeConflict {
			return "resolving"
		}
		return "reviewed" // Keep as-is when ready for merge
	case index.TaskStateMergeable:
		return "merging"
	default:
		return string(task.State)
	}
}

func formatAttrs(task index.Task) string {
	var attrs []string
	if task.BlockedReason != "" {
		attrs = append(attrs, "blocked")
	}
	if task.MergeConflict {
		attrs = append(attrs, "merge_conflict")
	}
	return strings.Join(attrs, ",")
}

func truncateTitle(title string, maxLen int) string {
	title = strings.TrimSpace(title)
	if title == "" || len(title) <= maxLen {
		return title
	}
	if maxLen <= 3 {
		return title[:maxLen]
	}
	return title[:maxLen-3] + "..."
}

func extractNumericPrefix(taskID string) string {
	// Extract numeric prefix from task ID (e.g., "001-task-name" -> "001")
	parts := strings.SplitN(taskID, "-", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return taskID
}

func planningStepSummary(repoRoot string, idx index.Index, supervisors []SupervisorSummary) ([]PlanningStepSummary, error) {
	stateID, found := planningTaskStateID(idx)
	if !found {
		return nil, nil
	}
	if stateID == "" || stateID == run.PlanningCompleteState || stateID == run.PlanningNotStartedState {
		return nil, nil
	}
	spec, err := run.LoadPlanningSpec(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("load planning spec: %w", err)
	}
	currentIndex := -1
	for i, step := range spec.Steps {
		if step.ID == stateID {
			currentIndex = i
			break
		}
	}
	if currentIndex == -1 {
		return nil, fmt.Errorf("planning state id %q not found in planning spec", stateID)
	}

	// Find the planning supervisor to get PID and StartedAt
	var planningSupervisor *SupervisorSummary
	for i := range supervisors {
		if supervisors[i].Phase == "planning" {
			planningSupervisor = &supervisors[i]
			break
		}
	}

	steps := make([]PlanningStepSummary, 0, len(spec.Steps))
	for i, step := range spec.Steps {
		status := "open"
		var pid int
		var startedAt time.Time

		switch {
		case i < currentIndex:
			status = "complete"
		case i == currentIndex:
			status = "in-progress"
			// For the current step, get PID and StartedAt from supervisor
			// Use LastTransition as it's updated when the worker starts
			if planningSupervisor != nil && planningSupervisor.StepID == step.ID {
				pid = planningSupervisor.WorkerPID
				startedAt = planningSupervisor.LastTransition
			}
		}
		name := strings.TrimSpace(step.Name)
		if name == "" {
			name = step.ID
		}
		steps = append(steps, PlanningStepSummary{
			ID:        step.ID,
			Name:      name,
			Status:    status,
			Order:     (i + 1) * 10,
			PID:       pid,
			StartedAt: startedAt,
		})
	}
	return steps, nil
}

func planningTaskStateID(idx index.Index) (string, bool) {
	for _, task := range idx.Tasks {
		if task.Kind != index.TaskKindPlanning {
			continue
		}
		if task.ID != "planning" {
			continue
		}
		return strings.TrimSpace(string(task.State)), true
	}
	return "", false
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func normalizeToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}

// renderTaskTable renders the task table with lipgloss styling.
func renderTaskTable(rows []StatusRow, maxWidth int) string {
	if len(rows) == 0 {
		return ""
	}

	var buf strings.Builder

	// Calculate column widths
	// Start with minimum widths for fixed columns
	minWidths := []int{
		6,  // ID - minimum to fit "ID" header and "001"
		12, // State - minimum to fit "implemented"
		6,  // PID - minimum to fit "12345"
		8,  // Runtime - minimum to fit "1h23m45s"
		12, // Role - minimum to fit role names
		18, // Attrs - minimum to fit "blocked,merge_conflict"
		20, // Title - minimum viable
	}

	// Calculate space used by fixed columns
	totalFixed := minWidths[0] + minWidths[1] + minWidths[2] + minWidths[3] + minWidths[4] + minWidths[5]

	// Account for table borders and padding (lipgloss rounded border + padding)
	// Border adds ~4 chars (left/right), padding adds 2*2=4 chars
	overhead := 8

	// Calculate available space for title column
	availableForTitle := maxWidth - totalFixed - overhead

	// Set title width between minimum and maximum
	titleWidth := minWidths[6] // start with minimum
	if availableForTitle > titleWidth {
		titleWidth = availableForTitle
		if titleWidth > 80 { // cap at reasonable maximum
			titleWidth = 80
		}
	}

	widths := []int{
		minWidths[0],
		minWidths[1],
		minWidths[2],
		minWidths[3],
		minWidths[4],
		minWidths[5],
		titleWidth,
	}

	// Header row
	headers := []string{"ID", "State", "PID", "Runtime", "Role", "Attrs", "Title"}
	headerCells := make([]string, len(headers))
	for i, h := range headers {
		headerCells[i] = headerStyle.Width(widths[i]).Render(h)
	}
	buf.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, headerCells...))
	buf.WriteString("\n")

	// Separator
	totalWidth := 0
	for _, w := range widths {
		totalWidth += w
	}
	separator := separatorStyle.Render(strings.Repeat("─", totalWidth))
	buf.WriteString(separator)
	buf.WriteString("\n")

	// Data rows
	for _, row := range rows {
		cells := []string{
			row.id,
			row.state,
			row.pid,
			row.runtime,
			row.role,
			row.attrs,
			row.title,
		}
		renderedCells := make([]string, len(cells))
		for i, cell := range cells {
			// Use MaxWidth to truncate with ellipsis if needed
			style := cellStyle.Width(widths[i]).MaxWidth(widths[i])
			renderedCells[i] = style.Render(cell)
		}
		buf.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, renderedCells...))
		buf.WriteString("\n")
	}

	return strings.TrimRight(buf.String(), "\n")
}

// renderSupervisorKV renders supervisor info as key-value pairs with lipgloss styling.
func renderSupervisorKV(supervisor SupervisorSummary) string {
	keyStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12")).
		Width(16).
		Align(lipgloss.Right)

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15"))

	var buf strings.Builder

	renderKV := func(key, value string) {
		buf.WriteString(keyStyle.Render(key + ":"))
		buf.WriteString(" ")
		buf.WriteString(valueStyle.Render(value))
		buf.WriteString("\n")
	}

	renderKV("Phase", normalizeToken(supervisor.Phase))
	renderKV("State", normalizeToken(supervisor.State))
	renderKV("PID", formatPID(supervisor.PID))
	renderKV("Runtime", formatSupervisorRuntime(supervisor.StartedAt))

	if supervisor.WorkerPID > 0 {
		renderKV("Worker PID", formatPID(supervisor.WorkerPID))
	}
	if supervisor.ValidationPID > 0 {
		renderKV("Validation PID", formatPID(supervisor.ValidationPID))
	}

	renderKV("Step ID", normalizeToken(supervisor.StepID))
	renderKV("Step Name", normalizeToken(supervisor.StepName))
	renderKV("Started At", formatTime(supervisor.StartedAt))
	renderKV("Last Transition", formatTime(supervisor.LastTransition))
	renderKV("Log", normalizeToken(supervisor.LogPath))

	return buf.String()
}

// renderPlanningTable renders the planning steps table with lipgloss styling.
func renderPlanningTable(steps []PlanningStepSummary, maxWidth int) string {
	if len(steps) == 0 {
		return ""
	}

	var buf strings.Builder

	// Column widths - calculate name dynamically
	minWidths := []int{30, 6, 8, planningStatusWidth}
	totalFixed := minWidths[1] + minWidths[2] + minWidths[3]
	overhead := 8
	availableForName := maxWidth - totalFixed - overhead
	nameWidth := minWidths[0]
	if availableForName > nameWidth {
		nameWidth = availableForName
		if nameWidth > 60 {
			nameWidth = 60
		}
	}

	widths := []int{
		nameWidth,
		minWidths[1],
		minWidths[2],
		minWidths[3],
	}

	// Header row
	headers := []string{"Name", "PID", "Runtime", "Status"}
	headerCells := make([]string, len(headers))
	for i, h := range headers {
		headerCells[i] = headerStyle.Width(widths[i]).Render(h)
	}
	buf.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, headerCells...))
	buf.WriteString("\n")

	// Separator
	totalWidth := 0
	for _, w := range widths {
		totalWidth += w
	}
	separator := separatorStyle.Render(strings.Repeat("─", totalWidth))
	buf.WriteString(separator)
	buf.WriteString("\n")

	// Data rows
	for _, step := range steps {
		cells := []string{
			step.Name,
			formatPID(step.PID),
			formatSupervisorRuntime(step.StartedAt),
			step.Status,
		}
		renderedCells := make([]string, len(cells))
		for i, cell := range cells {
			style := cellStyle.Width(widths[i]).MaxWidth(widths[i])
			renderedCells[i] = style.Render(cell)
		}
		buf.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, renderedCells...))
		buf.WriteString("\n")
	}

	return strings.TrimRight(buf.String(), "\n")
}

// renderWorkersTable renders the workers table with lipgloss styling.
func renderWorkersTable(workers []WorkerSummary, maxWidth int) string {
	if len(workers) == 0 {
		return ""
	}

	var buf strings.Builder

	// Column widths
	widths := []int{6, 12, 8} // PID, Role, Runtime

	// Header row
	headers := []string{"PID", "Role", "Runtime"}
	headerCells := make([]string, len(headers))
	for i, h := range headers {
		headerCells[i] = headerStyle.Width(widths[i]).Render(h)
	}
	buf.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, headerCells...))
	buf.WriteString("\n")

	// Separator
	totalWidth := 0
	for _, w := range widths {
		totalWidth += w
	}
	separator := separatorStyle.Render(strings.Repeat("─", totalWidth))
	buf.WriteString(separator)
	buf.WriteString("\n")

	// Data rows
	for _, worker := range workers {
		cells := []string{
			formatPID(worker.PID),
			worker.Role,
			formatSupervisorRuntime(worker.StartedAt),
		}
		renderedCells := make([]string, len(cells))
		for i, cell := range cells {
			style := cellStyle.Width(widths[i]).MaxWidth(widths[i])
			renderedCells[i] = style.Render(cell)
		}
		buf.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, renderedCells...))
		buf.WriteString("\n")
	}

	return strings.TrimRight(buf.String(), "\n")
}
