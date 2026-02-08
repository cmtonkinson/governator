// Package run provides execute orchestration helpers.
package run

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cmtonkinson/governator/internal/index"
)

// Execute runs execution orchestration when planning is complete.
func Execute(repoRoot string, opts Options) (Result, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return Result{}, fmt.Errorf("repo root is required")
	}
	if opts.Stdout == nil || opts.Stderr == nil {
		return Result{}, fmt.Errorf("stdout and stderr are required")
	}

	idx, err := index.Load(filepath.Join(repoRoot, indexFilePath))
	if err != nil {
		return Result{}, fmt.Errorf("load task index: %w", err)
	}
	planning, err := newPlanningTask(repoRoot)
	if err != nil {
		return Result{}, fmt.Errorf("load planning spec: %w", err)
	}
	complete, err := planningComplete(idx, planning)
	if err != nil {
		return Result{}, fmt.Errorf("planning index: %w", err)
	}
	if !complete {
		return Result{}, fmt.Errorf("planning incomplete: run `governator start` before executing")
	}

	return Run(repoRoot, opts)
}
