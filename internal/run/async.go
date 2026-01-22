// Package run provides helpers for async worker dispatch and collection.
package run

import (
	"fmt"
	"time"

	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/inflight"
	"github.com/cmtonkinson/governator/internal/scheduler"
)

// inFlightMap converts the in-flight set to a scheduler-friendly lookup.
func inFlightMap(set inflight.Set) map[string]struct{} {
	if len(set) == 0 {
		return nil
	}
	mapped := make(map[string]struct{}, len(set))
	for id := range set {
		mapped[id] = struct{}{}
	}
	return mapped
}

// adjustCapsForInFlight reduces concurrency caps based on current in-flight tasks.
func adjustCapsForInFlight(caps scheduler.RoleCaps, idx index.Index, set inflight.Set) scheduler.RoleCaps {
	if len(set) == 0 {
		return caps
	}
	adjusted := scheduler.RoleCaps{
		Global:      caps.Global,
		DefaultRole: caps.DefaultRole,
		Roles:       make(map[index.Role]int, len(caps.Roles)),
	}
	for role, cap := range caps.Roles {
		adjusted.Roles[role] = cap
	}
	inFlightCounts := buildInFlightRoleCounts(idx, set)
	totalInFlight := 0
	for _, count := range inFlightCounts {
		totalInFlight += count
	}
	if adjusted.Global > 0 {
		adjusted.Global = maxInt(adjusted.Global-totalInFlight, 0)
	}
	for role, count := range inFlightCounts {
		cap := adjusted.DefaultRole
		if roleCap, ok := adjusted.Roles[role]; ok {
			cap = roleCap
		}
		adjustedCap := maxInt(cap-count, 0)
		adjusted.Roles[role] = adjustedCap
	}
	return adjusted
}

// buildInFlightRoleCounts counts in-flight tasks by role.
func buildInFlightRoleCounts(idx index.Index, set inflight.Set) map[index.Role]int {
	counts := map[index.Role]int{}
	for _, task := range idx.Tasks {
		if !set.Contains(task.ID) {
			continue
		}
		role := task.Role
		if role == "" {
			continue
		}
		counts[role]++
	}
	return counts
}

// maxInt returns the larger of two ints.
func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

// timedOut reports whether the elapsed time exceeds the timeout.
func timedOut(startedAt time.Time, timeoutSecs int) bool {
	if timeoutSecs <= 0 {
		return false
	}
	if startedAt.IsZero() {
		return false
	}
	return time.Since(startedAt) > time.Duration(timeoutSecs)*time.Second
}

// startedAtForTask returns the recorded start time for an in-flight task.
func startedAtForTask(set inflight.Set, taskID string) (time.Time, bool) {
	startedAt, ok := set.StartedAt(taskID)
	if !ok {
		return time.Time{}, false
	}
	return startedAt, true
}

// worktreePathForTask returns the stored worktree path for an in-flight task.
func worktreePathForTask(set inflight.Set, taskID string) (string, bool) {
	path, ok := set.WorktreePath(taskID)
	if !ok {
		return "", false
	}
	return path, true
}

// formatTimeoutReason produces a consistent timeout message.
func formatTimeoutReason(timeoutSecs int) string {
	return fmt.Sprintf("worker timed out after %d seconds", timeoutSecs)
}
