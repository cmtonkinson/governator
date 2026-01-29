// Package status provides task index status reporting.
package status

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/run"
	"github.com/cmtonkinson/governator/internal/supervisor"
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

var statusStateOrder = map[index.TaskState]int{
	index.TaskStateTriaged:     0,
	index.TaskStateImplemented: 1,
	index.TaskStateTested:      2,
	index.TaskStateReviewed:    3,
	index.TaskStateMergeable:   4,
}

// Summary represents task counts and the in-progress table.
type Summary struct {
	Supervisors   []SupervisorSummary
	PlanningSteps []PlanningStepSummary
	Total         int
	Backlog       int
	Merged        int
	InProgress    int
	Rows          []statusRow
}

type statusRow struct {
	id    string
	state string
	pid   string
	role  string
	attrs string
	title string
	order int
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

// PlanningStepSummary captures the status output for a planning step.
type PlanningStepSummary struct {
	ID     string
	Name   string
	Status string
	Order  int
}

// String returns the formatted status output per flow.md.
func (s Summary) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "supervisors=%d\n", len(s.Supervisors))
	for _, supervisor := range s.Supervisors {
		stepName := normalizeToken(supervisor.StepName)
		fmt.Fprintf(&b, "supervisor phase=%s state=%s pid=%s step_id=%s step_name=%s worker_pid=%s validation_pid=%s started_at=%s last_transition=%s log_path=%s\n",
			normalizeToken(supervisor.Phase),
			normalizeToken(supervisor.State),
			formatPID(supervisor.PID),
			normalizeToken(supervisor.StepID),
			strconv.Quote(stepName),
			formatPID(supervisor.WorkerPID),
			formatPID(supervisor.ValidationPID),
			formatTime(supervisor.StartedAt),
			formatTime(supervisor.LastTransition),
			normalizeToken(supervisor.LogPath),
		)
	}
	if len(s.PlanningSteps) > 0 {
		fmt.Fprintf(&b, "planning-steps=%d\n", len(s.PlanningSteps))
		fmt.Fprintf(&b, "%-*s %-*s %s\n",
			planningIDWidth, "id",
			planningStatusWidth, "status",
			"name",
		)
		for _, step := range s.PlanningSteps {
			fmt.Fprintf(&b, "%-*s %-*s %s\n",
				planningIDWidth, step.ID,
				planningStatusWidth, step.Status,
				step.Name,
			)
		}
	}
	fmt.Fprintf(&b, "tasks backlog=%d merged=%d in-progress=%d\n", s.Backlog, s.Merged, s.InProgress)
	if s.InProgress == 0 {
		return strings.TrimSpace(b.String())
	}
	fmt.Fprintf(&b, "%-*s %-*s %-*s %-*s %-*s %s\n",
		idColumnWidth, "id",
		stateColumnWidth, "state",
		pidColumnWidth, "pid",
		roleColumnWidth, "role",
		attrsColumnWidth, "attrs",
		"title",
	)
	for _, row := range s.Rows {
		fmt.Fprintf(&b, "%-*s %-*s %-*s %-*s %-*s %s\n",
			idColumnWidth, row.id,
			stateColumnWidth, row.state,
			pidColumnWidth, row.pid,
			roleColumnWidth, row.role,
			attrsColumnWidth, row.attrs,
			row.title,
		)
	}
	return strings.TrimSpace(b.String())
}

// GetSummary reads the task index and returns a detailed summary.
func GetSummary(repoRoot string) (Summary, error) {
	indexPath := filepath.Join(repoRoot, "_governator", "task-index.json")

	idx, err := index.Load(indexPath)
	if err != nil {
		return Summary{}, fmt.Errorf("load task index: %w", err)
	}

	var rows []statusRow
	summary := Summary{Total: len(idx.Tasks)}

	if supervisorState, ok, err := supervisor.LoadPlanningState(repoRoot); err != nil {
		return Summary{}, fmt.Errorf("load planning supervisor state: %w", err)
	} else if ok {
		summary.Supervisors = append(summary.Supervisors, SupervisorSummary{
			Phase:          strings.TrimSpace(supervisorState.Phase),
			State:          string(supervisorState.State),
			PID:            supervisorState.PID,
			WorkerPID:      supervisorState.WorkerPID,
			ValidationPID:  supervisorState.ValidationPID,
			StepID:         supervisorState.StepID,
			StepName:       supervisorState.StepName,
			StartedAt:      supervisorState.StartedAt,
			LastTransition: supervisorState.LastTransition,
			LogPath:        supervisorState.LogPath,
		})
	}

	for _, task := range idx.Tasks {
		if task.Kind != index.TaskKindExecution {
			continue
		}
		switch task.State {
		case index.TaskStateBacklog:
			summary.Backlog++
			continue
		case index.TaskStateMerged:
			summary.Merged++
			continue
		default:
			summary.InProgress++
		}

		row := statusRow{
			id:    task.ID,
			state: string(task.State),
			pid:   formatPID(task.PID),
			role:  resolveAssignedRole(task),
			attrs: formatAttrs(task),
			title: truncateTitle(task.Title, titleMaxWidth),
			order: statusOrder(task.State),
		}
		rows = append(rows, row)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].order != rows[j].order {
			return rows[i].order < rows[j].order
		}
		return rows[i].id < rows[j].id
	})

	summary.Rows = rows
	steps, err := planningStepSummary(repoRoot, idx)
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

func planningStepSummary(repoRoot string, idx index.Index) ([]PlanningStepSummary, error) {
	stateID, found := planningTaskStateID(idx)
	if !found {
		return nil, nil
	}
	if stateID == "" || stateID == run.PlanningCompleteState {
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
	steps := make([]PlanningStepSummary, 0, len(spec.Steps))
	for i, step := range spec.Steps {
		status := "open"
		switch {
		case i < currentIndex:
			status = "complete"
		case i == currentIndex:
			status = "in-progress"
		}
		name := strings.TrimSpace(step.Name)
		if name == "" {
			name = step.ID
		}
		steps = append(steps, PlanningStepSummary{
			ID:     step.ID,
			Name:   name,
			Status: status,
			Order:  (i + 1) * 10,
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
