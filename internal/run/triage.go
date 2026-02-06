// Package run provides execution backlog triage helpers.
package run

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/digests"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/roles"
	"github.com/cmtonkinson/governator/internal/templates"
	"github.com/cmtonkinson/governator/internal/worker"
)

const (
	triageTaskID         = "triage-dag"
	triageDirName        = "triage"
	triageStateFileName  = "state.json"
	triageOutputFileName = "dag.json"
	triageTaskFileName   = "dag-order-task.md"
	triageMaxAttempts    = 2
)

// TriageState tracks the execution backlog triage lifecycle.
type TriageState struct {
	Attempt        int       `json:"attempt"`
	RunningPID     int       `json:"running_pid,omitempty"`
	WorkerStateDir string    `json:"worker_state_dir,omitempty"`
	LastError      string    `json:"last_error,omitempty"`
	LastAttemptAt  time.Time `json:"last_attempt_at,omitempty"`
}

// TriageCycleResult reports the outcome of a triage loop iteration.
type TriageCycleResult struct {
	Running        bool
	Completed      bool
	WorkerPID      int
	WorkerStateDir string
}

// RunBacklogTriage handles dispatching and collecting the DAG triage agent.
func RunBacklogTriage(repoRoot string, idx *index.Index, cfg config.Config, opts Options) (TriageCycleResult, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return TriageCycleResult{}, errors.New("repo root is required")
	}
	if idx == nil {
		return TriageCycleResult{}, errors.New("task index is required")
	}

	state, ok, err := LoadTriageState(repoRoot)
	if err != nil {
		return TriageCycleResult{}, err
	}
	if ok && strings.TrimSpace(state.WorkerStateDir) != "" {
		if _, finished, err := worker.ReadExitStatus(state.WorkerStateDir, triageTaskID, roles.StageWork); err != nil {
			return failTriageAttempt(repoRoot, state, err, opts)
		} else if finished {
			return finalizeTriageAttempt(repoRoot, idx, cfg, opts, state)
		}
	}

	if ok && state.RunningPID > 0 {
		if alive, err := processAlive(state.RunningPID); err != nil {
			return TriageCycleResult{}, err
		} else if alive {
			return TriageCycleResult{
				Running:        true,
				WorkerPID:      state.RunningPID,
				WorkerStateDir: state.WorkerStateDir,
			}, nil
		}
		if strings.TrimSpace(state.WorkerStateDir) != "" {
			return failTriageAttempt(repoRoot, state, errors.New("triage agent exited without exit status"), opts)
		}
	}

	if ok && state.Attempt >= triageMaxAttempts {
		return TriageCycleResult{}, fmt.Errorf("triage failed after %d attempts: %s", state.Attempt, state.LastError)
	}

	return dispatchTriageAttempt(repoRoot, idx, cfg, opts, state)
}

// LoadTriageState reads the triage state file when present.
func LoadTriageState(repoRoot string) (TriageState, bool, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return TriageState{}, false, errors.New("repo root is required")
	}
	path := triageStatePath(repoRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return TriageState{}, false, nil
		}
		return TriageState{}, false, fmt.Errorf("read triage state %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return TriageState{}, false, nil
	}
	var state TriageState
	if err := json.Unmarshal(data, &state); err != nil {
		return TriageState{}, false, fmt.Errorf("decode triage state %s: %w", path, err)
	}
	return state, true, nil
}

// SaveTriageState persists the triage state file.
func SaveTriageState(repoRoot string, state TriageState) error {
	if strings.TrimSpace(repoRoot) == "" {
		return errors.New("repo root is required")
	}
	path := triageStatePath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create triage directory %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode triage state: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write triage state %s: %w", path, err)
	}
	return nil
}

// ClearTriageState removes persisted triage state.
func ClearTriageState(repoRoot string) error {
	if strings.TrimSpace(repoRoot) == "" {
		return errors.New("repo root is required")
	}
	path := triageStatePath(repoRoot)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove triage state %s: %w", path, err)
	}
	return nil
}

// dispatchTriageAttempt starts the triage agent and records state.
func dispatchTriageAttempt(repoRoot string, idx *index.Index, cfg config.Config, opts Options, state TriageState) (TriageCycleResult, error) {
	attempt := state.Attempt + 1
	attemptState := state
	attemptState.Attempt = attempt
	if err := prepareTriageTask(repoRoot, *idx); err != nil {
		return failTriageAttempt(repoRoot, attemptState, err, opts)
	}
	role := index.Role("default")
	task := index.Task{
		ID:   triageTaskID,
		Path: triageTaskRelativePath(),
		Role: role,
	}
	stageInput := newWorkerStageInput(repoRoot, repoRoot, task, roles.StageWork, role, attempt, cfg, func(msg string) {
		fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
	})
	stageInput.WorkerStateDir = triageWorkerStateDir(repoRoot, attempt, role)

	stageResult, err := worker.StageEnvAndPrompts(stageInput)
	if err != nil {
		return failTriageAttempt(repoRoot, attemptState, fmt.Errorf("stage triage agent: %w", err), opts)
	}

	dispatchResult, err := worker.DispatchWorkerFromConfig(cfg, task, stageResult, repoRoot, roles.StageWork, func(msg string) {
		fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
	})
	if err != nil {
		return failTriageAttempt(repoRoot, attemptState, fmt.Errorf("dispatch triage agent: %w", err), opts)
	}

	if err := SaveTriageState(repoRoot, TriageState{
		Attempt:        attempt,
		RunningPID:     dispatchResult.PID,
		WorkerStateDir: dispatchResult.WorkerStateDir,
		LastAttemptAt:  dispatchResult.StartedAt.UTC(),
	}); err != nil {
		return TriageCycleResult{}, err
	}

	fmt.Fprintf(opts.Stdout, "triage dispatched (pid %d)\n", dispatchResult.PID)
	return TriageCycleResult{
		Running:        true,
		WorkerPID:      dispatchResult.PID,
		WorkerStateDir: dispatchResult.WorkerStateDir,
	}, nil
}

// finalizeTriageAttempt collects triage results, applies DAG ordering, and clears state.
func finalizeTriageAttempt(repoRoot string, idx *index.Index, cfg config.Config, opts Options, state TriageState) (TriageCycleResult, error) {
	exitStatus, finished, err := worker.ReadExitStatus(state.WorkerStateDir, triageTaskID, roles.StageWork)
	if err != nil {
		return failTriageAttempt(repoRoot, state, fmt.Errorf("read triage exit status: %w", err), opts)
	}
	if !finished {
		return TriageCycleResult{Running: true, WorkerPID: state.RunningPID, WorkerStateDir: state.WorkerStateDir}, nil
	}
	if exitStatus.ExitCode != 0 {
		return failTriageAttempt(repoRoot, state, fmt.Errorf("triage agent exited with code %d", exitStatus.ExitCode), opts)
	}

	mapping, err := readDagMapping(triageOutputPath(repoRoot))
	if err != nil {
		return failTriageAttempt(repoRoot, state, err, opts)
	}

	warnings := applyDagMapping(idx, mapping)
	for _, warning := range warnings {
		fmt.Fprintf(opts.Stderr, "Warning: %s\n", warning)
	}

	digestsMap, err := digests.Compute(repoRoot)
	if err != nil {
		return failTriageAttempt(repoRoot, state, fmt.Errorf("compute digests: %w", err), opts)
	}
	idx.Digests = digestsMap

	indexPath := filepath.Join(repoRoot, indexFilePath)
	if err := index.Save(indexPath, *idx); err != nil {
		return failTriageAttempt(repoRoot, state, fmt.Errorf("save task index: %w", err), opts)
	}

	if err := ClearTriageState(repoRoot); err != nil {
		return TriageCycleResult{}, err
	}

	fmt.Fprintln(opts.Stdout, "triage complete")
	return TriageCycleResult{Completed: true}, nil
}

// failTriageAttempt records failure metadata and enforces retry limits.
func failTriageAttempt(repoRoot string, state TriageState, err error, opts Options) (TriageCycleResult, error) {
	state.RunningPID = 0
	state.WorkerStateDir = ""
	state.LastError = err.Error()
	state.LastAttemptAt = time.Now().UTC()
	if saveErr := SaveTriageState(repoRoot, state); saveErr != nil {
		return TriageCycleResult{}, fmt.Errorf("%w; triage state save failed: %v", err, saveErr)
	}
	if state.Attempt >= triageMaxAttempts {
		return TriageCycleResult{}, fmt.Errorf("triage failed after %d attempts: %w", state.Attempt, err)
	}
	if opts.Stderr != nil {
		fmt.Fprintf(opts.Stderr, "Warning: triage attempt %d failed: %v\n", state.Attempt, err)
	}
	return TriageCycleResult{}, nil
}

// applyDagMapping overwrites dependencies and triages backlog tasks.
func applyDagMapping(idx *index.Index, mapping map[string][]string) []string {
	eligible := map[string]struct{}{}
	for _, task := range idx.Tasks {
		if isTriageEligible(task) {
			eligible[task.ID] = struct{}{}
		}
	}

	var warnings []string
	for i := range idx.Tasks {
		task := &idx.Tasks[i]
		if !isTriageEligible(*task) {
			continue
		}
		deps, ok := mapping[task.ID]
		if !ok {
			deps = nil
		}
		filtered := make([]string, 0, len(deps))
		for _, dep := range deps {
			dep = strings.TrimSpace(dep)
			if dep == "" || dep == task.ID {
				continue
			}
			if _, ok := eligible[dep]; !ok {
				warnings = append(warnings, fmt.Sprintf("triage dependency %q for task %q ignored (not eligible)", dep, task.ID))
				continue
			}
			filtered = append(filtered, dep)
		}
		task.Dependencies = filtered
		task.State = index.TaskStateTriaged
	}
	return warnings
}

// readDagMapping parses the triage DAG mapping from disk, tolerating extra text.
func readDagMapping(path string) (map[string][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read triage mapping %s: %w", path, err)
	}
	blob := extractJSONObject(string(data))
	if strings.TrimSpace(blob) == "" {
		return nil, fmt.Errorf("triage mapping missing JSON object")
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(blob), &raw); err != nil {
		return nil, fmt.Errorf("parse triage mapping: %w", err)
	}

	result := make(map[string][]string, len(raw))
	for key, value := range raw {
		switch typed := value.(type) {
		case nil:
			result[key] = nil
		case string:
			result[key] = []string{typed}
		case []interface{}:
			var deps []string
			for _, item := range typed {
				switch dep := item.(type) {
				case string:
					deps = append(deps, dep)
				default:
					deps = append(deps, fmt.Sprintf("%v", dep))
				}
			}
			result[key] = deps
		default:
			return nil, fmt.Errorf("triage mapping for %q must be array, got %T", key, value)
		}
	}
	return result, nil
}

// extractJSONObject pulls the first JSON object from a blob of text.
func extractJSONObject(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return text[start : end+1]
}

// prepareTriageTask builds the triage prompt task file and clears old output.
func prepareTriageTask(repoRoot string, idx index.Index) error {
	template, err := templates.Read("planning/dag-order-tasks.md")
	if err != nil {
		return fmt.Errorf("read triage template: %w", err)
	}
	taskPath := triageTaskPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(taskPath), 0o755); err != nil {
		return fmt.Errorf("create triage directory: %w", err)
	}
	content := buildTriageTaskContent(string(template), idx)
	if err := os.WriteFile(taskPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write triage task: %w", err)
	}
	outputPath := triageOutputPath(repoRoot)
	if err := os.Remove(outputPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clear triage output: %w", err)
	}
	return nil
}

// buildTriageTaskContent renders the prompt used by the DAG ordering agent.
func buildTriageTaskContent(template string, idx index.Index) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(template))
	b.WriteString("\n\nOutput the JSON mapping to `_governator/_local-state/dag.json`.\n")
	b.WriteString("Schema example: {\"task-07\": [\"task-03\", \"task-04\"], \"task-08\": [\"task-03\", \"task-06\", \"task-07\"]}\n")
	b.WriteString("Include only backlog and triaged tasks. Prefer listing every task with [] for independent work; omitted tasks are treated as independent.\n")
	b.WriteString("Ensure the mapping captures the complete ordering among backlog/triaged tasks.\n")
	b.WriteString("\nCurrent backlog + triaged tasks:\n")
	for _, task := range idx.Tasks {
		if !isTriageEligible(task) {
			continue
		}
		b.WriteString(fmt.Sprintf("- id: %s\n  title: %s\n  state: %s\n  deps: %s\n  path: %s\n",
			task.ID,
			normalizeToken(task.Title),
			task.State,
			strings.Join(task.Dependencies, ", "),
			task.Path,
		))
	}
	b.WriteString("\nExisting dependencies should be treated as hints, not constraints.\n")
	return b.String()
}

// isTriageEligible reports whether a task is in backlog/triaged and not started.
func isTriageEligible(task index.Task) bool {
	if task.Kind != index.TaskKindExecution {
		return false
	}
	return task.State == index.TaskStateBacklog || task.State == index.TaskStateTriaged
}

// triageStatePath returns the location of the triage state file.
func triageStatePath(repoRoot string) string {
	return filepath.Join(repoRoot, localStateDirName, triageDirName, triageStateFileName)
}

// triageOutputPath returns the path to the DAG mapping output.
func triageOutputPath(repoRoot string) string {
	return filepath.Join(repoRoot, localStateDirName, triageOutputFileName)
}

// triageTaskPath returns the absolute path to the triage task prompt file.
func triageTaskPath(repoRoot string) string {
	return filepath.Join(repoRoot, triageTaskRelativePath())
}

// triageTaskRelativePath returns the repo-relative path to the triage task prompt file.
func triageTaskRelativePath() string {
	return filepath.ToSlash(filepath.Join(localStateDirName, triageDirName, triageTaskFileName))
}

// triageWorkerStateDir returns the worker state directory for triage attempts.
func triageWorkerStateDir(repoRoot string, attempt int, role index.Role) string {
	dirName := workerStateDirName(attempt, roles.StageWork, role)
	return filepath.Join(repoRoot, localStateDirName, triageDirName, dirName)
}

// processAlive checks if a PID is currently alive.
func processAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	if errors.Is(err, syscall.EPERM) {
		return true, nil
	}
	return false, err
}
