// Package scheduler provides deterministic routing helpers for task dispatch.
package scheduler

import "github.com/cmtonkinson/governator/internal/index"

// RoutingDecision captures the selection outcome for a task and the reason.
type RoutingDecision struct {
	Task     index.Task
	Selected bool
	Reason   string
}

// RoutingResult summarizes routing decisions and selected tasks.
type RoutingResult struct {
	Decisions []RoutingDecision
	Selected  []index.Task
}

// Routing decision reasons describe why a task was selected or skipped.
const (
	reasonSelected        = "selected (global and role caps available)"
	reasonOverlapConflict = "skipped (overlap conflict)"
	reasonRoleCapReached  = "skipped (role cap reached)"
	reasonRoleCapDisabled = "skipped (role cap is zero)"
)

// RouteEligibleTasks orders eligible tasks and applies caps to select tasks with reasons.
func RouteEligibleTasks(idx index.Index, caps RoleCaps) (RoutingResult, error) {
	ordered, err := OrderedEligibleTasks(idx)
	if err != nil {
		return RoutingResult{}, err
	}
	return RouteOrderedTasks(ordered, caps), nil
}

// RouteOrderedTasks applies caps to ordered tasks and returns routing decisions.
func RouteOrderedTasks(ordered []index.Task, caps RoleCaps) RoutingResult {
	if caps.Global <= 0 || len(ordered) == 0 {
		return RoutingResult{}
	}

	result := RoutingResult{
		Decisions: make([]RoutingDecision, 0, len(ordered)),
		Selected:  make([]index.Task, 0, min(caps.Global, len(ordered))),
	}
	usage := map[index.Role]int{}
	activeOverlap := map[string]struct{}{}
	for _, task := range ordered {
		if len(result.Selected) >= caps.Global {
			break
		}
		if overlapConflict(task, activeOverlap) {
			result.Decisions = append(result.Decisions, RoutingDecision{
				Task:     task,
				Selected: false,
				Reason:   reasonOverlapConflict,
			})
			continue
		}
		roleCap := capForRole(task.Role, caps)
		if roleCap <= 0 {
			result.Decisions = append(result.Decisions, RoutingDecision{
				Task:     task,
				Selected: false,
				Reason:   reasonRoleCapDisabled,
			})
			continue
		}
		if usage[task.Role] >= roleCap {
			result.Decisions = append(result.Decisions, RoutingDecision{
				Task:     task,
				Selected: false,
				Reason:   reasonRoleCapReached,
			})
			continue
		}
		usage[task.Role]++
		result.Selected = append(result.Selected, task)
		recordOverlap(task, activeOverlap)
		result.Decisions = append(result.Decisions, RoutingDecision{
			Task:     task,
			Selected: true,
			Reason:   reasonSelected,
		})
	}
	return result
}

// overlapConflict reports whether the task shares any overlap tag with active tasks.
func overlapConflict(task index.Task, activeOverlap map[string]struct{}) bool {
	if len(task.Overlap) == 0 {
		return false
	}
	for _, overlap := range task.Overlap {
		if overlap == "" {
			continue
		}
		if _, ok := activeOverlap[overlap]; ok {
			return true
		}
	}
	return false
}

// recordOverlap adds task overlap tags to the active overlap set.
func recordOverlap(task index.Task, activeOverlap map[string]struct{}) {
	for _, overlap := range task.Overlap {
		if overlap == "" {
			continue
		}
		activeOverlap[overlap] = struct{}{}
	}
}
