// Package dag provides dependency graph visualization.
package dag

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/cmtonkinson/governator/internal/index"
)

const (
	idColumnWidth    = 6
	stateColumnWidth = 14
	depsColumnWidth  = 20
	blocksColumnWidth = 20
	titleColumnWidth = 40
)

var (
	headerStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12"))

	cellStyle = lipgloss.NewStyle()

	separatorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	summaryStyle = lipgloss.NewStyle().
		Bold(true)
)

// Summary represents the DAG visualization output.
type Summary struct {
	Tasks       []TaskRow
	TotalTasks  int
	InProgress  int
	Merged      int
	Backlog     int
}

// TaskRow represents a single row in the DAG display.
type TaskRow struct {
	ID         string
	State      string
	DependsOn  string
	Blocks     string
	Title      string
	Order      int
}

// String returns the formatted DAG output.
func (s Summary) String() string {
	var b strings.Builder

	// Summary header
	summary := summaryStyle.Render(fmt.Sprintf(
		"Tasks (%d total, %d backlog, %d in-progress, %d merged)",
		s.TotalTasks, s.Backlog, s.InProgress, s.Merged,
	))
	b.WriteString(summary)
	b.WriteString("\n\n")

	if len(s.Tasks) == 0 {
		b.WriteString("No tasks found.\n")
		return b.String()
	}

	// Column headers
	headers := []string{
		padRight("ID", idColumnWidth),
		padRight("State", stateColumnWidth),
		padRight("Depends On", depsColumnWidth),
		padRight("Blocks", blocksColumnWidth),
		"Title",
	}
	headerLine := headerStyle.Render(strings.Join(headers, "  "))
	b.WriteString(headerLine)
	b.WriteString("\n")

	// Separator
	totalWidth := idColumnWidth + stateColumnWidth + depsColumnWidth + blocksColumnWidth + titleColumnWidth + 8
	separator := separatorStyle.Render(strings.Repeat("─", totalWidth))
	b.WriteString(separator)
	b.WriteString("\n")

	// Task rows
	for _, row := range s.Tasks {
		line := fmt.Sprintf("%s  %s  %s  %s  %s",
			padRight(row.ID, idColumnWidth),
			padRight(row.State, stateColumnWidth),
			padRight(row.DependsOn, depsColumnWidth),
			padRight(row.Blocks, blocksColumnWidth),
			truncate(row.Title, titleColumnWidth),
		)
		b.WriteString(cellStyle.Render(line))
		b.WriteString("\n")
	}

	return b.String()
}

// GetSummary builds a DAG summary from the task index.
func GetSummary(idx index.Index) Summary {
	summary := Summary{
		TotalTasks: len(idx.Tasks),
	}

	// Build reverse dependency map (who blocks whom)
	blockedBy := make(map[string][]string)
	for _, task := range idx.Tasks {
		if task.Kind != index.TaskKindExecution {
			continue
		}
		for _, dep := range task.Dependencies {
			blockedBy[dep] = append(blockedBy[dep], extractNumericID(task.ID))
		}
	}

	// Sort blocked-by lists for consistent output
	for key := range blockedBy {
		sort.Strings(blockedBy[key])
	}

	var rows []TaskRow
	for _, task := range idx.Tasks {
		if task.Kind != index.TaskKindExecution {
			continue
		}

		// Count states
		switch task.State {
		case index.TaskStateBacklog:
			summary.Backlog++
		case index.TaskStateMerged:
			summary.Merged++
		default:
			summary.InProgress++
		}

		// Format dependencies
		var depsStr string
		if len(task.Dependencies) == 0 {
			depsStr = "-"
		} else {
			depIDs := make([]string, len(task.Dependencies))
			for i, dep := range task.Dependencies {
				depIDs[i] = extractNumericID(dep)
			}
			depsStr = strings.Join(depIDs, ",")
		}

		// Format blocks
		var blocksStr string
		if blocks, ok := blockedBy[task.ID]; ok && len(blocks) > 0 {
			blocksStr = strings.Join(blocks, ",")
		} else {
			blocksStr = "-"
		}

		rows = append(rows, TaskRow{
			ID:        extractNumericID(task.ID),
			State:     string(task.State),
			DependsOn: depsStr,
			Blocks:    blocksStr,
			Title:     task.Title,
			Order:     task.Order,
		})
	}

	// Sort by order, then by ID
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Order != rows[j].Order {
			return rows[i].Order < rows[j].Order
		}
		return rows[i].ID < rows[j].ID
	})

	summary.Tasks = rows
	return summary
}

// extractNumericID extracts the numeric prefix from a task ID.
// E.g., "001-implement-auth" -> "001"
func extractNumericID(taskID string) string {
	parts := strings.SplitN(taskID, "-", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return taskID
}

// padRight pads a string to the specified width.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

// truncate truncates a string to the specified width with ellipsis.
func truncate(s string, width int) string {
	if len(s) <= width {
		return s
	}
	if width <= 3 {
		return s[:width]
	}
	return s[:width-3] + "..."
}
