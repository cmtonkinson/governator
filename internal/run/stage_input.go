package run

import (
	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/roles"
	"github.com/cmtonkinson/governator/internal/worker"
)

func newWorkerStageInput(repoRoot, worktreeRoot string, task index.Task, stage roles.Stage, role index.Role, cfg config.Config, warn func(string)) worker.StageInput {
	return worker.StageInput{
		RepoRoot:        repoRoot,
		WorktreeRoot:    worktreeRoot,
		Task:            task,
		Stage:           stage,
		Role:            role,
		ReasoningEffort: cfg.ReasoningEffort.LevelForRole(string(role)),
		Warn:            warn,
	}
}
