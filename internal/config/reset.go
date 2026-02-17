// Package config provides workspace configuration and reset helpers.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cmtonkinson/governator/internal/index"
)

// ResetExecutionTasksOptions controls reset behavior for execution task state.
type ResetExecutionTasksOptions struct {
	ClearTriageState bool
}

// ResetExecutionTasks resets all non-backlog/non-merged execution tasks to backlog and clears transient runtime state.
func ResetExecutionTasks(repoRoot string, opts ResetExecutionTasksOptions) error {
	indexPath := filepath.Join(repoRoot, ".governator", ".local-state", "index.json")
	openTaskIDs := map[string]struct{}{}

	if idx, err := index.Load(indexPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("load task index %s: %w", indexPath, err)
		}
	} else {
		for i := range idx.Tasks {
			task := &idx.Tasks[i]
			if task.Kind != index.TaskKindExecution {
				continue
			}
			if task.State == index.TaskStateBacklog || task.State == index.TaskStateMerged {
				continue
			}
			openTaskIDs[task.ID] = struct{}{}
			task.State = index.TaskStateBacklog
			task.Role = ""
			task.AssignedRole = ""
			task.BlockedReason = ""
			task.MergeConflict = false
			task.PID = 0
			task.Attempts = index.AttemptCounters{}
		}

		if err := index.Save(indexPath, idx); err != nil {
			return fmt.Errorf("save task index %s: %w", indexPath, err)
		}
	}

	return finalizeExecutionTaskReset(repoRoot, openTaskIDs, opts.ClearTriageState)
}

// finalizeExecutionTaskReset clears transient state and purges worktrees/branches for reset task IDs.
func finalizeExecutionTaskReset(repoRoot string, resetTaskIDs map[string]struct{}, clearTriageState bool) error {
	if err := resetInFlightState(repoRoot); err != nil {
		return err
	}
	if clearTriageState {
		if err := clearTriageLocalState(repoRoot); err != nil {
			return err
		}
	}
	if err := purgeTaskWorktreesAndBranches(repoRoot, resetTaskIDs); err != nil {
		return err
	}
	return nil
}

// clearTriageLocalState removes persisted triage execution artifacts.
func clearTriageLocalState(repoRoot string) error {
	triageDir := filepath.Join(repoRoot, ".governator", ".local-state", "triage")
	if err := os.RemoveAll(triageDir); err != nil {
		return fmt.Errorf("remove triage local state %s: %w", triageDir, err)
	}
	triageDAG := filepath.Join(repoRoot, ".governator", ".local-state", "dag.json")
	if err := os.Remove(triageDAG); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove triage dag %s: %w", triageDAG, err)
	}
	return nil
}
