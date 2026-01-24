package run

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/roles"
	"github.com/cmtonkinson/governator/internal/worker"
)

const localStateDirName = "_governator/_local-state"

func newWorkerStageInput(repoRoot, worktreeRoot string, task index.Task, stage roles.Stage, role index.Role, attempt int, cfg config.Config, warn func(string)) worker.StageInput {
	return worker.StageInput{
		RepoRoot:        repoRoot,
		WorktreeRoot:    worktreeRoot,
		Task:            task,
		Stage:           stage,
		Role:            role,
		ReasoningEffort: cfg.ReasoningEffort.LevelForRole(string(role)),
		Warn:            warn,
		WorkerStateDir:  workerStateDirPath(worktreeRoot, attempt, stage, role),
	}
}

func workerStateDirPath(worktreeRoot string, attempt int, stage roles.Stage, role index.Role) string {
	dirName := workerStateDirName(attempt, stage, role)
	return filepath.Join(worktreeRoot, localStateDirName, dirName)
}

func workerStateDirName(attempt int, stage roles.Stage, role index.Role) string {
	if attempt < 1 {
		attempt = 1
	}
	stageName := sanitizeComponent(string(stage))
	if stageName == "" {
		stageName = "stage"
	}
	roleName := sanitizeComponent(string(role))
	if roleName == "" {
		roleName = "role"
	}
	return strings.Join([]string{
		"worker",
		strconv.Itoa(attempt),
		stageName,
		roleName,
	}, "-")
}

func sanitizeComponent(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, " ", "-")
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.Trim(value, "-")
	return value
}
