// Tests for execution backlog triage helpers.
package run

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/digests"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/worker"
)

// TestReadTriageMappingWithCodeFence ensures JSON can be extracted from fenced output.
func TestReadTriageMappingWithCodeFence(t *testing.T) {
	dir := t.TempDir()
	repoRoot := dir
	path := filepath.Join(dir, "dag.json")
	if err := os.WriteFile(path, []byte("```json\n{\"task-01\": [\"task-00\"]}\n```\n"), 0o644); err != nil {
		t.Fatalf("write mapping: %v", err)
	}

	mapping, err := readTriageMapping(repoRoot, path)
	if err != nil {
		t.Fatalf("read mapping: %v", err)
	}

	expected := map[string]TriageTaskInfo{"task-01": {Dependencies: []string{"task-00"}}}
	if !reflect.DeepEqual(mapping, expected) {
		t.Fatalf("unexpected mapping: %#v", mapping)
	}
}

// TestReadTriageMappingNewFormat ensures new format with roles is parsed correctly.
func TestReadTriageMappingNewFormat(t *testing.T) {
	dir := t.TempDir()
	repoRoot := dir
	path := filepath.Join(dir, "dag.json")
	content := `{
		"task-01": {
			"dependencies": ["task-00"],
			"role": "backend"
		},
		"task-02": {
			"dependencies": [],
			"role": "frontend"
		},
		"task-03": {
			"dependencies": ["task-01", "task-02"]
		}
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write mapping: %v", err)
	}

	mapping, err := readTriageMapping(repoRoot, path)
	if err != nil {
		t.Fatalf("read mapping: %v", err)
	}

	// Verify task-01
	if len(mapping) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(mapping))
	}
	if task01, ok := mapping["task-01"]; !ok {
		t.Fatal("task-01 not found")
	} else {
		if !reflect.DeepEqual(task01.Dependencies, []string{"task-00"}) {
			t.Fatalf("task-01 deps: got %#v, want [task-00]", task01.Dependencies)
		}
		if task01.Role != "backend" {
			t.Fatalf("task-01 role: got %s, want backend", task01.Role)
		}
	}

	// Verify task-02
	if task02, ok := mapping["task-02"]; !ok {
		t.Fatal("task-02 not found")
	} else {
		if len(task02.Dependencies) != 0 {
			t.Fatalf("task-02 deps: got %#v, want empty", task02.Dependencies)
		}
		if task02.Role != "frontend" {
			t.Fatalf("task-02 role: got %s, want frontend", task02.Role)
		}
	}

	// Verify task-03
	if task03, ok := mapping["task-03"]; !ok {
		t.Fatal("task-03 not found")
	} else {
		if !reflect.DeepEqual(task03.Dependencies, []string{"task-01", "task-02"}) {
			t.Fatalf("task-03 deps: got %#v, want [task-01 task-02]", task03.Dependencies)
		}
		if task03.Role != "" {
			t.Fatalf("task-03 role: got %s, want empty", task03.Role)
		}
	}
}

// TestApplyTriageMappingWithRoles ensures roles are applied from triage output.
func TestApplyTriageMappingWithRoles(t *testing.T) {
	repoRoot := t.TempDir()

	// Create a default role file
	rolesDir := filepath.Join(repoRoot, "_governator", "roles")
	if err := os.MkdirAll(rolesDir, 0o755); err != nil {
		t.Fatalf("create roles dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rolesDir, "default.md"), []byte("# Default Role\n"), 0o644); err != nil {
		t.Fatalf("write default role: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rolesDir, "backend.md"), []byte("# Backend Role\n"), 0o644); err != nil {
		t.Fatalf("write backend role: %v", err)
	}

	idx := index.Index{
		Tasks: []index.Task{
			{ID: "task-01", Kind: index.TaskKindExecution, State: index.TaskStateBacklog, Role: ""},
			{ID: "task-02", Kind: index.TaskKindExecution, State: index.TaskStateBacklog, Role: ""},
		},
	}
	mapping := map[string]TriageTaskInfo{
		"task-01": {Dependencies: []string{}, Role: "backend"},
		"task-02": {Dependencies: []string{"task-01"}}, // No role specified
	}

	warnings := applyTriageMapping(&idx, mapping, repoRoot)
	if len(warnings) > 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	task01 := idx.Tasks[0]
	if task01.Role != index.Role("backend") {
		t.Fatalf("task-01 role: got %s, want backend", task01.Role)
	}
	if task01.State != index.TaskStateTriaged {
		t.Fatalf("task-01 state: got %s, want triaged", task01.State)
	}

	task02 := idx.Tasks[1]
	if task02.Role != index.Role("default") {
		t.Fatalf("task-02 role: got %s, want default (empty role defaults to default)", task02.Role)
	}
	if !reflect.DeepEqual(task02.Dependencies, []string{"task-01"}) {
		t.Fatalf("task-02 deps: %#v", task02.Dependencies)
	}
}

// TestApplyTriageMappingInvalidRole ensures invalid roles are coerced to default with warning.
func TestApplyTriageMappingInvalidRole(t *testing.T) {
	repoRoot := t.TempDir()

	// Create only default role file
	rolesDir := filepath.Join(repoRoot, "_governator", "roles")
	if err := os.MkdirAll(rolesDir, 0o755); err != nil {
		t.Fatalf("create roles dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rolesDir, "default.md"), []byte("# Default Role\n"), 0o644); err != nil {
		t.Fatalf("write default role: %v", err)
	}

	idx := index.Index{
		Tasks: []index.Task{
			{ID: "task-01", Kind: index.TaskKindExecution, State: index.TaskStateBacklog, Role: ""},
		},
	}
	mapping := map[string]TriageTaskInfo{
		"task-01": {Dependencies: []string{}, Role: "nonexistent"},
	}

	warnings := applyTriageMapping(&idx, mapping, repoRoot)
	if len(warnings) == 0 {
		t.Fatalf("expected warning for invalid role")
	}

	foundWarning := false
	for _, w := range warnings {
		if strings.Contains(w, "nonexistent") && strings.Contains(w, "invalid") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Fatalf("expected warning about invalid role 'nonexistent', got: %v", warnings)
	}

	task01 := idx.Tasks[0]
	if task01.Role != index.Role("default") {
		t.Fatalf("task-01 role: got %s, want default (invalid role coerced)", task01.Role)
	}
}

// TestRunBacklogTriageFinalizesMapping ensures triage applies dependencies and clears state.
func TestRunBacklogTriageFinalizesMapping(t *testing.T) {
	repoRoot := t.TempDir()

	idx := index.Index{
		Tasks: []index.Task{
			{ID: "task-01", Kind: index.TaskKindExecution, State: index.TaskStateBacklog},
			{ID: "task-02", Kind: index.TaskKindExecution, State: index.TaskStateBacklog},
		},
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "_governator"), 0o755); err != nil {
		t.Fatalf("create governator dir: %v", err)
	}
	if err := index.Save(filepath.Join(repoRoot, indexFilePath), idx); err != nil {
		t.Fatalf("save index: %v", err)
	}

	outputPath := triageOutputPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		t.Fatalf("create output dir: %v", err)
	}
	if err := os.WriteFile(outputPath, []byte("{\"task-02\": [\"task-01\"], \"task-01\": []}\n"), 0o644); err != nil {
		t.Fatalf("write dag output: %v", err)
	}

	workerStateDir := filepath.Join(repoRoot, "_governator/_local-state/triage/test-worker")
	if err := os.MkdirAll(workerStateDir, 0o755); err != nil {
		t.Fatalf("create worker state dir: %v", err)
	}
	exitStatus := worker.ExitStatus{
		ExitCode:   0,
		FinishedAt: time.Now().UTC(),
		PID:        1234,
	}
	exitData, err := json.Marshal(exitStatus)
	if err != nil {
		t.Fatalf("marshal exit status: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerStateDir, "exit.json"), exitData, 0o644); err != nil {
		t.Fatalf("write exit status: %v", err)
	}

	state := TriageState{
		Attempt:        1,
		RunningPID:     999999,
		WorkerStateDir: workerStateDir,
		LastAttemptAt:  time.Now().UTC(),
	}
	if err := SaveTriageState(repoRoot, state); err != nil {
		t.Fatalf("save triage state: %v", err)
	}

	var stderr bytes.Buffer
	result, err := RunBacklogTriage(repoRoot, &idx, config.Defaults(), Options{Stdout: ioDiscard{}, Stderr: &stderr})
	if err != nil {
		t.Fatalf("run triage: %v", err)
	}
	if !result.Completed {
		t.Fatalf("expected completed triage result")
	}

	if _, err := os.Stat(triageStatePath(repoRoot)); !os.IsNotExist(err) {
		t.Fatalf("expected triage state cleared, stat err=%v", err)
	}

	if idx.Tasks[0].State != index.TaskStateTriaged || idx.Tasks[1].State != index.TaskStateTriaged {
		t.Fatalf("expected tasks triaged, got %s/%s", idx.Tasks[0].State, idx.Tasks[1].State)
	}
	if !reflect.DeepEqual(idx.Tasks[1].Dependencies, []string{"task-01"}) {
		t.Fatalf("unexpected deps for task-02: %#v", idx.Tasks[1].Dependencies)
	}
}

// TestRunBacklogTriageRefreshesDigests ensures triage updates planning digests after applying changes.
func TestRunBacklogTriageRefreshesDigests(t *testing.T) {
	repoRoot := t.TempDir()

	if err := os.WriteFile(filepath.Join(repoRoot, "GOVERNATOR.md"), []byte("governance v1"), 0o644); err != nil {
		t.Fatalf("write GOVERNATOR.md: %v", err)
	}
	docsDir := filepath.Join(repoRoot, "_governator", "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("create docs dir: %v", err)
	}
	docPath := filepath.Join(docsDir, "arch-asr.md")
	if err := os.WriteFile(docPath, []byte("planning v1"), 0o644); err != nil {
		t.Fatalf("write planning doc: %v", err)
	}

	initialDigests, err := digests.Compute(repoRoot)
	if err != nil {
		t.Fatalf("compute initial digests: %v", err)
	}

	idx := index.Index{
		Digests: initialDigests,
		Tasks: []index.Task{
			{ID: "task-01", Kind: index.TaskKindExecution, State: index.TaskStateBacklog},
		},
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "_governator"), 0o755); err != nil {
		t.Fatalf("create governator dir: %v", err)
	}
	if err := index.Save(filepath.Join(repoRoot, indexFilePath), idx); err != nil {
		t.Fatalf("save index: %v", err)
	}

	if err := os.WriteFile(docPath, []byte("planning v2"), 0o644); err != nil {
		t.Fatalf("update planning doc: %v", err)
	}

	outputPath := triageOutputPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		t.Fatalf("create output dir: %v", err)
	}
	if err := os.WriteFile(outputPath, []byte("{\"task-01\": []}\n"), 0o644); err != nil {
		t.Fatalf("write dag output: %v", err)
	}

	workerStateDir := filepath.Join(repoRoot, "_governator/_local-state/triage/test-worker")
	if err := os.MkdirAll(workerStateDir, 0o755); err != nil {
		t.Fatalf("create worker state dir: %v", err)
	}
	exitStatus := worker.ExitStatus{
		ExitCode:   0,
		FinishedAt: time.Now().UTC(),
		PID:        4321,
	}
	exitData, err := json.Marshal(exitStatus)
	if err != nil {
		t.Fatalf("marshal exit status: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerStateDir, "exit.json"), exitData, 0o644); err != nil {
		t.Fatalf("write exit status: %v", err)
	}

	state := TriageState{
		Attempt:        1,
		RunningPID:     999999,
		WorkerStateDir: workerStateDir,
		LastAttemptAt:  time.Now().UTC(),
	}
	if err := SaveTriageState(repoRoot, state); err != nil {
		t.Fatalf("save triage state: %v", err)
	}

	result, err := RunBacklogTriage(repoRoot, &idx, config.Defaults(), Options{Stdout: ioDiscard{}, Stderr: ioDiscard{}})
	if err != nil {
		t.Fatalf("run triage: %v", err)
	}
	if !result.Completed {
		t.Fatalf("expected completed triage result")
	}

	currentDigests, err := digests.Compute(repoRoot)
	if err != nil {
		t.Fatalf("compute current digests: %v", err)
	}
	if !reflect.DeepEqual(idx.Digests, currentDigests) {
		t.Fatalf("expected digests refreshed, got %#v want %#v", idx.Digests, currentDigests)
	}
}

// TestFailTriageAttemptRecordsState ensures failures persist state and warnings.
func TestFailTriageAttemptRecordsState(t *testing.T) {
	repoRoot := t.TempDir()
	state := TriageState{Attempt: 1, RunningPID: 123, WorkerStateDir: "ignored"}
	var stderr bytes.Buffer

	if _, err := failTriageAttempt(repoRoot, state, errors.New("boom"), Options{Stderr: &stderr}); err != nil {
		t.Fatalf("fail triage attempt: %v", err)
	}

	loaded, ok, err := LoadTriageState(repoRoot)
	if err != nil {
		t.Fatalf("load triage state: %v", err)
	}
	if !ok {
		t.Fatalf("expected triage state to be saved")
	}
	if loaded.LastError != "boom" {
		t.Fatalf("expected last error boom, got %q", loaded.LastError)
	}
	if loaded.RunningPID != 0 || loaded.WorkerStateDir != "" {
		t.Fatalf("expected cleared running info, got pid=%d dir=%q", loaded.RunningPID, loaded.WorkerStateDir)
	}
	if !strings.Contains(stderr.String(), "triage attempt 1 failed") {
		t.Fatalf("expected warning in stderr, got %q", stderr.String())
	}
}

// TestFailTriageAttemptLimitsRetries ensures retry cap is enforced.
func TestFailTriageAttemptLimitsRetries(t *testing.T) {
	repoRoot := t.TempDir()
	state := TriageState{Attempt: triageMaxAttempts}
	if _, err := failTriageAttempt(repoRoot, state, errors.New("boom"), Options{Stderr: ioDiscard{}}); err == nil {
		t.Fatalf("expected retry limit error")
	}
}

// TestApplyTriageMappingTriagesEligibleTasks ensures backlog/triaged tasks are updated.
func TestApplyTriageMappingTriagesEligibleTasks(t *testing.T) {
	repoRoot := t.TempDir()
	idx := index.Index{
		Tasks: []index.Task{
			{ID: "task-01", Kind: index.TaskKindExecution, State: index.TaskStateBacklog},
			{ID: "task-02", Kind: index.TaskKindExecution, State: index.TaskStateTriaged},
			{ID: "task-03", Kind: index.TaskKindExecution, State: index.TaskStateImplemented},
			{ID: "task-04", Kind: index.TaskKindPlanning, State: index.TaskStateBacklog},
		},
	}
	mapping := map[string]TriageTaskInfo{
		"task-01": {Dependencies: []string{"task-02", "task-03"}},
	}

	warnings := applyTriageMapping(&idx, mapping, repoRoot)
	if len(warnings) == 0 {
		t.Fatalf("expected warning for non-eligible dependency")
	}

	task01 := idx.Tasks[0]
	if task01.State != index.TaskStateTriaged {
		t.Fatalf("task-01 expected triaged, got %s", task01.State)
	}
	if !reflect.DeepEqual(task01.Dependencies, []string{"task-02"}) {
		t.Fatalf("task-01 deps: %#v", task01.Dependencies)
	}

	task02 := idx.Tasks[1]
	if task02.State != index.TaskStateTriaged {
		t.Fatalf("task-02 expected triaged, got %s", task02.State)
	}
	if task02.Dependencies != nil && len(task02.Dependencies) != 0 {
		t.Fatalf("task-02 expected independent deps, got %#v", task02.Dependencies)
	}

	task03 := idx.Tasks[2]
	if task03.State != index.TaskStateImplemented {
		t.Fatalf("task-03 state changed unexpectedly: %s", task03.State)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
