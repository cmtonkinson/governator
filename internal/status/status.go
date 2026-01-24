// Package status provides task index status reporting.
package status

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cmtonkinson/governator/internal/index"
)

const (
	idColumnWidth    = 14
	stateColumnWidth = 12
	pidColumnWidth   = 6
	roleColumnWidth  = 12
	attrsColumnWidth = 18
	titleMaxWidth    = 40
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
	Total      int
	Backlog    int
	Merged     int
	InProgress int
	Rows       []statusRow
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

// String returns the formatted status output per flow.md.
func (s Summary) String() string {
	var b strings.Builder
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

	for _, task := range idx.Tasks {
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
