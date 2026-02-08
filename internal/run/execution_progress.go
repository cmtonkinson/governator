// Package run provides execution-progress helpers used by supervisor orchestration.
package run

import "github.com/cmtonkinson/governator/internal/index"

// countBacklog returns the number of execution tasks awaiting triage.
func countBacklog(idx index.Index) int {
	count := 0
	for _, task := range idx.Tasks {
		if task.Kind != index.TaskKindExecution {
			continue
		}
		if task.State == index.TaskStateBacklog {
			count++
		}
	}
	return count
}

// executionComplete reports whether all execution tasks are in terminal states.
func executionComplete(idx index.Index) (bool, error) {
	for _, task := range idx.Tasks {
		if task.Kind != index.TaskKindExecution {
			continue
		}
		if !executionTerminalState(task.State) {
			return false, nil
		}
	}
	return true, nil
}

// executionTerminalState reports whether a task state is terminal for execution.
func executionTerminalState(state index.TaskState) bool {
	switch state {
	case index.TaskStateMerged, index.TaskStateBlocked, index.TaskStateConflict:
		return true
	default:
		return false
	}
}
