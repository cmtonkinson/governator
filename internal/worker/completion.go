// Package worker provides worker completion detection helpers.
package worker

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/roles"
)

// StageCompletion captures marker and commit checks for a stage.
type StageCompletion struct {
	Completed   bool
	HasCommit   bool
	HasMarker   bool
	MarkerPath  string
	MarkerFound bool
}

// CheckStageCompletion inspects the worktree to determine whether a stage completed.
func CheckStageCompletion(worktreePath string, workerStateDir string, stage roles.Stage) (StageCompletion, error) {
	if strings.TrimSpace(worktreePath) == "" {
		return StageCompletion{}, errors.New("worktree path is required")
	}
	if strings.TrimSpace(workerStateDir) == "" {
		return StageCompletion{}, errors.New("worker state dir is required")
	}
	if !stage.Valid() {
		return StageCompletion{}, fmt.Errorf("invalid stage %q", stage)
	}

	hasCommit, err := checkForCommit(worktreePath)
	if err != nil {
		return StageCompletion{}, fmt.Errorf("check for commit: %w", err)
	}

	markerPath := filepath.Join(workerStateDir, markerFileName(stage))
	hasMarker, err := checkForMarkerFile(markerPath)
	if err != nil {
		return StageCompletion{}, fmt.Errorf("check for marker file: %w", err)
	}

	relMarker := repoRelativePath(worktreePath, markerPath)
	completed := hasCommit && hasMarker
	return StageCompletion{
		Completed:   completed,
		HasCommit:   hasCommit,
		HasMarker:   hasMarker,
		MarkerPath:  relMarker,
		MarkerFound: hasMarker,
	}, nil
}

// CompletionResultToIngest builds an ingest result from completion data.
func CompletionResultToIngest(taskID string, stage roles.Stage, completion StageCompletion) (IngestResult, error) {
	if strings.TrimSpace(taskID) == "" {
		return IngestResult{}, errors.New("task id is required")
	}
	if !stage.Valid() {
		return IngestResult{}, fmt.Errorf("invalid stage %q", stage)
	}
	if completion.Completed {
		return IngestResult{
			Success:      true,
			NewState:     stageToSuccessState(stage),
			HasCommit:    completion.HasCommit,
			HasMarker:    completion.HasMarker,
			MarkerPath:   completion.MarkerPath,
			MarkerExists: completion.MarkerFound,
		}, nil
	}
	return IngestResult{
		Success:      false,
		NewState:     index.TaskStateBlocked,
		BlockReason:  buildBlockReason(completion.HasCommit, completion.HasMarker, stage),
		HasCommit:    completion.HasCommit,
		HasMarker:    completion.HasMarker,
		MarkerPath:   completion.MarkerPath,
		MarkerExists: completion.MarkerFound,
	}, nil
}
