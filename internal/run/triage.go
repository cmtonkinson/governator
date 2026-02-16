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

// TriageTaskInfo captures triage agent output for a single task.
type TriageTaskInfo struct {
	Dependencies []string `json:"dependencies,omitempty"`
	Role         string   `json:"role,omitempty"`
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

	mapping, mappingSource, err := loadTriageMapping(repoRoot, triageOutputPath(repoRoot), state.WorkerStateDir)
	if err != nil {
		return failTriageAttempt(repoRoot, state, err, opts)
	}
	if mappingSource != triageOutputPath(repoRoot) && opts.Stderr != nil {
		fmt.Fprintf(opts.Stderr, "Warning: triage mapping loaded from fallback path %s\n", mappingSource)
	}

	warnings := applyTriageMapping(idx, mapping, repoRoot)
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

// loadTriageMapping loads triage output from the canonical path and retries/falls back when needed.
func loadTriageMapping(repoRoot string, primaryPath string, workerStateDir string) (map[string]TriageTaskInfo, string, error) {
	mapping, err := readTriageMapping(repoRoot, primaryPath)
	if err == nil {
		return mapping, primaryPath, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, "", err
	}

	// On some filesystems (notably mounted volumes), file visibility can lag worker
	// process completion slightly. Retry before treating this as a hard failure.
	lastErr := err
	for range 5 {
		time.Sleep(150 * time.Millisecond)
		mapping, err = readTriageMapping(repoRoot, primaryPath)
		if err == nil {
			return mapping, primaryPath, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, "", err
		}
		lastErr = err
	}

	for _, fallbackPath := range triageFallbackOutputPaths(repoRoot, primaryPath, workerStateDir) {
		mapping, err = readTriageMapping(repoRoot, fallbackPath)
		if err == nil {
			return mapping, fallbackPath, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, "", err
		}
		lastErr = err
	}
	return nil, "", fmt.Errorf("%w; searched fallback triage output paths", lastErr)
}

// triageFallbackOutputPaths returns non-canonical paths where a triage agent may emit dag output.
func triageFallbackOutputPaths(repoRoot string, primaryPath string, workerStateDir string) []string {
	seen := map[string]struct{}{filepath.Clean(primaryPath): {}}
	candidates := []string{
		filepath.Join(repoRoot, localStateDirName, triageDirName, triageOutputFileName),
		filepath.Join(workerStateDir, triageOutputFileName),
		filepath.Join(workerStateDir, localStateDirName, triageOutputFileName),
	}

	paths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		cleaned := filepath.Clean(candidate)
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		paths = append(paths, cleaned)
	}
	return paths
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

// applyTriageMapping overwrites dependencies, roles, and triages backlog tasks.
func applyTriageMapping(idx *index.Index, mapping map[string]TriageTaskInfo, repoRoot string) []string {
	eligible := map[string]struct{}{}
	for _, task := range idx.Tasks {
		if isTriageEligible(task) {
			eligible[task.ID] = struct{}{}
		}
	}

	var warnings []string

	// Load role registry for validation
	registry, err := roles.LoadRegistry(repoRoot, func(msg string) {
		warnings = append(warnings, msg)
	})
	if err != nil {
		// If registry fails to load, we'll just use default for all roles
		registry = roles.Registry{}
	}
	for i := range idx.Tasks {
		task := &idx.Tasks[i]
		if !isTriageEligible(*task) {
			continue
		}

		info, ok := mapping[task.ID]
		if !ok {
			info = TriageTaskInfo{} // Empty info for tasks not in mapping
		}

		// Apply dependencies
		filtered := make([]string, 0, len(info.Dependencies))
		for _, dep := range info.Dependencies {
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

		// Apply role (validate and coerce to default if invalid)
		assignedRole := index.Role(strings.TrimSpace(info.Role))
		if assignedRole == "" {
			assignedRole = index.Role("default")
		} else {
			// Validate role exists in registry
			if _, exists := registry.RolePromptPath(assignedRole); !exists {
				warnings = append(warnings, fmt.Sprintf("triage role %q for task %q is invalid, using default", assignedRole, task.ID))
				assignedRole = index.Role("default")
			}
		}
		task.Role = assignedRole

		task.State = index.TaskStateTriaged
	}
	return warnings
}

// readTriageMapping parses the triage DAG mapping from disk, supporting both new and legacy formats.
// New format: {"task-id": {"dependencies": ["dep1"], "role": "backend"}}
// Legacy format: {"task-id": ["dep1", "dep2"]}
func readTriageMapping(repoRoot string, path string) (map[string]TriageTaskInfo, error) {
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

	result := make(map[string]TriageTaskInfo, len(raw))
	for key, value := range raw {
		switch typed := value.(type) {
		case nil:
			// null value - empty info
			result[key] = TriageTaskInfo{}

		case string:
			// Legacy: single dependency as string
			result[key] = TriageTaskInfo{Dependencies: []string{typed}}

		case []interface{}:
			// Legacy: array of dependencies
			var deps []string
			for _, item := range typed {
				switch dep := item.(type) {
				case string:
					deps = append(deps, dep)
				default:
					deps = append(deps, fmt.Sprintf("%v", dep))
				}
			}
			result[key] = TriageTaskInfo{Dependencies: deps}

		case map[string]interface{}:
			// New format: object with dependencies and role
			info := TriageTaskInfo{}

			// Extract dependencies
			if depsVal, ok := typed["dependencies"]; ok {
				switch depsTyped := depsVal.(type) {
				case []interface{}:
					for _, item := range depsTyped {
						if dep, ok := item.(string); ok {
							info.Dependencies = append(info.Dependencies, dep)
						}
					}
				case nil:
					info.Dependencies = nil
				}
			}

			// Extract role
			if roleVal, ok := typed["role"]; ok {
				if roleStr, ok := roleVal.(string); ok {
					info.Role = roleStr
				}
			}

			result[key] = info

		default:
			return nil, fmt.Errorf("triage mapping for %q must be array or object, got %T", key, value)
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
	b.WriteString("\n\nOutput the JSON mapping to `.governator/.local-state/dag.json`.\n")
	b.WriteString("Schema: {\"task-id\": {\"dependencies\": [\"dep1\"], \"role\": \"backend\"}}\n")
	b.WriteString("  - dependencies: array of task IDs (or [] for independent tasks)\n")
	b.WriteString("  - role: string role name (optional, defaults to \"default\")\n")
	b.WriteString("Legacy format {\"task-id\": [\"deps\"]} is supported but deprecated.\n")
	b.WriteString("Include only backlog and triaged tasks. Prefer listing every task with [] for independent work; omitted tasks are treated as independent.\n")
	b.WriteString("\n**CRITICAL: Maximize parallelism. Only add dependencies for TRUE blockers, not for cosmetic ordering.**\n")
	b.WriteString("\nTRUE dependency = Task X needs Y's output/code/feature to proceed\n")
	b.WriteString("FALSE dependency = Tasks touch same file but don't share code, or are just conceptually related\n")
	b.WriteString("\nWhen in doubt, prefer NO dependency (parallel execution) over serial ordering.\n")
	b.WriteString("\nCurrent backlog + triaged tasks:\n")
	for _, task := range idx.Tasks {
		if !isTriageEligible(task) {
			continue
		}
		roleStr := string(task.Role)
		if roleStr == "" {
			roleStr = "(unassigned)"
		}
		b.WriteString(fmt.Sprintf("- id: %s\n  title: %s\n  state: %s\n  role: %s\n  deps: %s\n  path: %s\n",
			task.ID,
			normalizeToken(task.Title),
			task.State,
			roleStr,
			strings.Join(task.Dependencies, ", "),
			task.Path,
		))
	}
	b.WriteString("\nExisting dependencies and roles are hints only - reevaluate based on TRUE dependency criteria and task requirements.\n")
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
