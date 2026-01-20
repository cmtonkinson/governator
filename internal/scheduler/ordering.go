// Package scheduler provides deterministic routing helpers for task dispatch.
package scheduler

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cmtonkinson/governator/internal/index"
)

// Priority constants describe how close a task is to completion.
const (
	statePriorityConflict = 0
	statePriorityReview   = 1
	statePriorityTest     = 2
	statePriorityWork     = 3
)

// OrderedEligibleTasks returns eligible tasks ordered deterministically by state, plan order, and id.
func OrderedEligibleTasks(idx index.Index) ([]index.Task, error) {
	if err := detectDependencyCycles(idx.Tasks); err != nil {
		return nil, err
	}

	stateByID := make(map[string]index.TaskState, len(idx.Tasks))
	for _, task := range idx.Tasks {
		stateByID[task.ID] = task.State
	}

	eligible := make([]index.Task, 0, len(idx.Tasks))
	for _, task := range idx.Tasks {
		if _, ok := statePriority(task.State); !ok {
			continue
		}
		if !dependenciesSatisfied(task, stateByID) {
			continue
		}
		eligible = append(eligible, task)
	}

	sort.SliceStable(eligible, func(i, j int) bool {
		left := eligible[i]
		right := eligible[j]
		leftPriority, _ := statePriority(left.State)
		rightPriority, _ := statePriority(right.State)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		if left.Order != right.Order {
			return left.Order < right.Order
		}
		return left.ID < right.ID
	})

	return eligible, nil
}

// statePriority ranks task states so closer-to-completion work is favored.
func statePriority(state index.TaskState) (int, bool) {
	switch state {
	case index.TaskStateConflict, index.TaskStateResolved:
		return statePriorityConflict, true
	case index.TaskStateTested:
		return statePriorityReview, true
	case index.TaskStateWorked:
		return statePriorityTest, true
	case index.TaskStateOpen:
		return statePriorityWork, true
	default:
		return 0, false
	}
}

// dependenciesSatisfied reports whether all dependencies are complete for the task.
func dependenciesSatisfied(task index.Task, stateByID map[string]index.TaskState) bool {
	if len(task.Dependencies) == 0 {
		return true
	}
	for _, dependency := range task.Dependencies {
		state, ok := stateByID[dependency]
		if !ok || state != index.TaskStateDone {
			return false
		}
	}
	return true
}

// detectDependencyCycles reports a cycle in the dependency graph when present.
func detectDependencyCycles(tasks []index.Task) error {
	if len(tasks) == 0 {
		return nil
	}
	taskByID := make(map[string]index.Task, len(tasks))
	for _, task := range tasks {
		if _, ok := taskByID[task.ID]; ok {
			continue
		}
		taskByID[task.ID] = task
	}

	visitState := make(map[string]int, len(taskByID))
	for id := range taskByID {
		if visitState[id] != 0 {
			continue
		}
		if err := visitDependencies(id, taskByID, visitState, nil); err != nil {
			return err
		}
	}
	return nil
}

// visitDependencies performs a DFS walk and stops on the first detected cycle.
func visitDependencies(id string, taskByID map[string]index.Task, visitState map[string]int, stack []string) error {
	if visitState[id] == 1 {
		return fmt.Errorf("circular dependency detected: %s", strings.Join(cyclePath(stack, id), " -> "))
	}
	if visitState[id] == 2 {
		return nil
	}
	visitState[id] = 1
	stack = append(stack, id)
	for _, dependency := range taskByID[id].Dependencies {
		if _, ok := taskByID[dependency]; !ok {
			continue
		}
		if err := visitDependencies(dependency, taskByID, visitState, stack); err != nil {
			return err
		}
	}
	visitState[id] = 2
	return nil
}

// cyclePath returns a cycle slice starting at the repeated id.
func cyclePath(stack []string, repeat string) []string {
	index := indexOf(stack, repeat)
	if index == -1 {
		return []string{repeat, repeat}
	}
	path := append([]string{}, stack[index:]...)
	return append(path, repeat)
}

// indexOf returns the index of the target string or -1 when missing.
func indexOf(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}
