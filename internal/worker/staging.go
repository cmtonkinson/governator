// Package worker provides worker command resolution helpers.
package worker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/roles"
)

const (
	// localStateDirName is the relative path for transient governator state.
	localStateDirName = "_governator/_local-state"
	// workerStateDirName holds worker staging artifacts inside local state.
	workerStateDirName = "worker"
	workerContractPath = "_governator/worker-contract.md"
	reasoningDirName  = "_governator/reasoning"
)

// StageInput defines the inputs required to stage worker prompts and environment.
type StageInput struct {
	RepoRoot     string
	WorktreeRoot string
	Task         index.Task
	Stage        roles.Stage
	Role         index.Role
	ReasoningEffort string
	Warn         func(string)
}

// StageResult captures staged prompt and environment artifacts.
type StageResult struct {
	PromptPath     string
	PromptFiles    []string
	PromptListPath string
	EnvPath        string
	Env            map[string]string
}

// StageEnvAndPrompts prepares worker prompt and environment staging artifacts.
func StageEnvAndPrompts(input StageInput) (StageResult, error) {
	repoRoot := strings.TrimSpace(input.RepoRoot)
	if repoRoot == "" {
		return StageResult{}, errors.New("repo root is required")
	}
	worktreeRoot := strings.TrimSpace(input.WorktreeRoot)
	if worktreeRoot == "" {
		return StageResult{}, errors.New("worktree root is required")
	}
	if !input.Stage.Valid() {
		return StageResult{}, fmt.Errorf("unsupported stage %q", input.Stage)
	}
	taskID := strings.TrimSpace(input.Task.ID)
	if taskID == "" {
		return StageResult{}, errors.New("task id is required")
	}
	taskPath := strings.TrimSpace(input.Task.Path)
	if taskPath == "" {
		return StageResult{}, errors.New("task path is required")
	}
	role := input.Role
	if role == "" {
		role = input.Task.Role
	}
	if role == "" {
		return StageResult{}, errors.New("role is required")
	}

	absWorktree, err := filepath.Abs(worktreeRoot)
	if err != nil {
		return StageResult{}, fmt.Errorf("resolve worktree root %s: %w", worktreeRoot, err)
	}

	registry, err := roles.LoadRegistry(absWorktree, input.Warn)
	if err != nil {
		return StageResult{}, fmt.Errorf("load role registry: %w", err)
	}
	reasoningLevel := strings.TrimSpace(input.ReasoningEffort)
	promptFiles, err := orderedPromptFiles(absWorktree, registry, role, reasoningLevel, taskPath)
	if err != nil {
		return StageResult{}, err
	}

	stageDir := filepath.Join(absWorktree, localStateDirName, workerStateDirName)
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return StageResult{}, fmt.Errorf("create worker stage dir %s: %w", stageDir, err)
	}

	promptListPath := filepath.Join(stageDir, promptListFileName(input.Stage))
	if err := writePromptList(promptListPath, promptFiles); err != nil {
		return StageResult{}, err
	}

	markerPath := repoRelativePath(absWorktree, filepath.Join(absWorktree, localStateDirName, markerFileName(input.Stage)))
	promptPath := filepath.Join(stageDir, promptFileName(input.Stage))
	if err := writePromptFile(promptPath, promptFiles, markerPath, input.Stage); err != nil {
		return StageResult{}, err
	}

	envPath := filepath.Join(stageDir, envFileName(input.Stage))
	env := buildEnvMap(absWorktree, taskID, taskPath, role, input.Stage, promptPath, promptListPath)
	if err := writeEnvFile(envPath, env); err != nil {
		return StageResult{}, err
	}

	return StageResult{
		PromptPath:     promptPath,
		PromptFiles:    promptFiles,
		PromptListPath: promptListPath,
		EnvPath:        envPath,
		Env:            env,
	}, nil
}

// orderedPromptFiles returns the stable prompt order for worker execution.
// Prompt order:
//  1. _governator/reasoning/<level>.md (when configured)
//  2. _governator/worker-contract.md
//  3. _governator/roles/<role>.md
//  4. _governator/custom-prompts/_global.md (optional)
//  5. _governator/custom-prompts/<role>.md (optional)
//  6. <task path>
func orderedPromptFiles(worktreeRoot string, registry roles.Registry, role index.Role, reasoningLevel string, taskPath string) ([]string, error) {
	rolePrompt, ok := registry.RolePromptPath(role)
	if !ok {
		return nil, fmt.Errorf("missing role prompt for %q", role)
	}
	rolePrompts := registry.PromptFiles(role)
	if len(rolePrompts) == 0 || rolePrompts[0] != rolePrompt {
		return nil, fmt.Errorf("role prompt %s missing from prompt order", rolePrompt)
	}

	promptFiles := make([]string, 0, len(rolePrompts)+3)
	if reasoningLevel != "" {
		promptFiles = append(promptFiles, reasoningPromptPath(reasoningLevel))
	}
	promptFiles = append(promptFiles, workerContractPath)
	promptFiles = append(promptFiles, rolePrompts...)

	taskPath = filepath.ToSlash(taskPath)
	promptFiles = append(promptFiles, taskPath)
	for _, prompt := range promptFiles {
		abs := filepath.Join(worktreeRoot, filepath.FromSlash(prompt))
		if err := ensurePromptFile(abs); err != nil {
			return nil, err
		}
	}
	return promptFiles, nil
}

func reasoningPromptPath(level string) string {
	return filepath.ToSlash(filepath.Join(reasoningDirName, fmt.Sprintf("%s.md", level)))
}

// ensurePromptFile validates that the prompt file exists and is a regular file.
func ensurePromptFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("missing prompt file %s", path)
		}
		return fmt.Errorf("stat prompt file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("prompt file %s is not a regular file", path)
	}
	return nil
}

// promptListFileName returns the stage-scoped prompt list file name.
func promptListFileName(stage roles.Stage) string {
	return fmt.Sprintf("prompt-files-%s.txt", stage)
}

// promptFileName returns the stage-scoped prompt file name.
func promptFileName(stage roles.Stage) string {
	return fmt.Sprintf("prompt-%s.md", stage)
}

// envFileName returns the stage-scoped env file name.
func envFileName(stage roles.Stage) string {
	return fmt.Sprintf("env-%s", stage)
}

// markerFileName maps a stage to its required marker filename.
func markerFileName(stage roles.Stage) string {
	switch stage {
	case roles.StageWork:
		return "worked.md"
	case roles.StageTest:
		return "tested.md"
	case roles.StageReview:
		return "reviewed.md"
	case roles.StageResolve:
		return "resolved.md"
	default:
		return "blocked.md"
	}
}

// writePromptList writes the prompt list in deterministic order.
func writePromptList(path string, prompts []string) error {
	content := strings.TrimSpace(strings.Join(prompts, "\n"))
	if content == "" {
		return errors.New("prompt list is required")
	}
	if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
		return fmt.Errorf("write prompt list %s: %w", path, err)
	}
	return nil
}

// writePromptFile writes the worker prompt that references the ordered prompt inputs.
func writePromptFile(path string, prompts []string, markerPath string, stage roles.Stage) error {
	if len(prompts) == 0 {
		return errors.New("prompt files are required")
	}
	content := buildPromptContent(prompts, markerPath, stage)
	if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
		return fmt.Errorf("write worker prompt %s: %w", path, err)
	}
	return nil
}

// buildPromptContent builds the worker prompt text with stage requirements.
func buildPromptContent(prompts []string, markerPath string, stage roles.Stage) string {
	builder := &strings.Builder{}
	builder.WriteString("Read and follow the instructions in the following files, in this order:\n")
	for _, prompt := range prompts {
		builder.WriteString("- ")
		builder.WriteString(prompt)
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
	builder.WriteString("Completion requirements for stage \"")
	builder.WriteString(string(stage))
	builder.WriteString("\":\n")
	builder.WriteString("- Create ")
	builder.WriteString(markerPath)
	builder.WriteString(" with a short summary.\n")
	builder.WriteString("- Create a commit on the task branch.\n")
	return strings.TrimSpace(builder.String())
}

// buildEnvMap assembles the environment variables for worker execution.
func buildEnvMap(worktreeRoot string, taskID string, taskPath string, role index.Role, stage roles.Stage, promptPath string, promptListPath string) map[string]string {
	return map[string]string{
		"GOVERNATOR_PROMPT_LIST":  repoRelativePath(worktreeRoot, promptListPath),
		"GOVERNATOR_PROMPT_PATH":  repoRelativePath(worktreeRoot, promptPath),
		"GOVERNATOR_ROLE":         string(role),
		"GOVERNATOR_STAGE":        string(stage),
		"GOVERNATOR_TASK_ID":      taskID,
		"GOVERNATOR_TASK_PATH":    filepath.ToSlash(taskPath),
		"GOVERNATOR_WORKTREE_DIR": worktreeRoot,
	}
}

// writeEnvFile writes a dotenv-style file for worker execution context.
func writeEnvFile(path string, values map[string]string) error {
	if len(values) == 0 {
		return errors.New("env values are required")
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	builder := &strings.Builder{}
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteString("=")
		builder.WriteString(values[key])
		builder.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		return fmt.Errorf("write env file %s: %w", path, err)
	}
	return nil
}

// repoRelativePath returns a repository-relative path using forward slashes.
func repoRelativePath(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
