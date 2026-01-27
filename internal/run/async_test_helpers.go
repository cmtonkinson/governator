// Test helpers for async dispatch behavior.
package run

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cmtonkinson/governator/internal/inflight"
	"github.com/cmtonkinson/governator/internal/roles"
	"github.com/cmtonkinson/governator/internal/worker"
)

// waitForExitStatus polls for the worker exit status marker emitted by async dispatch.
func waitForExitStatus(t *testing.T, worktreePath string, taskID string, stage roles.Stage) {
	t.Helper()
	exitPath, err := findExitStatusPath(worktreePath, taskID, stage)
	if err != nil {
		t.Fatalf("failed to locate exit status path: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	lastErr := ""
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(worktreePath)))
	for time.Now().Before(deadline) {
		if present, presentErr := inflightExitStatusPresent(repoRoot, taskID, stage); presentErr == nil && present {
			return
		} else if presentErr != nil {
			lastErr = presentErr.Error()
		}
		present, presentErr := exitStatusPresent(worktreePath, stage)
		if presentErr != nil {
			lastErr = presentErr.Error()
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if present {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	localState := filepath.Join(worktreePath, "_governator", "_local-state")
	var entries []string
	var diagnostics []string
	if dirEntries, err := os.ReadDir(localState); err == nil {
		for _, entry := range dirEntries {
			entries = append(entries, entry.Name())
			if !entry.IsDir() {
				continue
			}
			if !dirStageMatches(entry.Name(), stage) {
				continue
			}
			stageDir := filepath.Join(localState, entry.Name())
			stdoutTail := tailFile(filepath.Join(stageDir, "stdout.log"))
			stderrTail := tailFile(filepath.Join(stageDir, "stderr.log"))
			dispatchTail := tailFile(filepath.Join(stageDir, "dispatch.json"))
			if stdoutTail != "" {
				diagnostics = append(diagnostics, fmt.Sprintf("%s stdout: %s", entry.Name(), stdoutTail))
			}
			if stderrTail != "" {
				diagnostics = append(diagnostics, fmt.Sprintf("%s stderr: %s", entry.Name(), stderrTail))
			}
			if dispatchTail != "" {
				diagnostics = append(diagnostics, fmt.Sprintf("%s dispatch: %s", entry.Name(), dispatchTail))
			}
		}
	}
	metaPath := filepath.Join(repoRoot, "_governator", "_local-state", "meta", fmt.Sprintf("%s.json", taskID))
	if metaTail := tailFile(metaPath); metaTail != "" {
		diagnostics = append(diagnostics, fmt.Sprintf("meta: %s", metaTail))
	}
	t.Fatalf(
		"timed out waiting for exit status %s (last error: %s; local-state entries: %s; diagnostics: %s)",
		exitPath,
		lastErr,
		strings.Join(entries, ","),
		strings.Join(diagnostics, " | "),
	)
}

func findExitStatusPath(worktreePath, taskID string, stage roles.Stage) (string, error) {
	localState := filepath.Join(worktreePath, "_governator", "_local-state")
	const fileName = "exit.json"
	entries, err := os.ReadDir(localState)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("local state missing %s: %w", localState, err)
		}
		return "", fmt.Errorf("read local state %s: %w", localState, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !dirStageMatches(entry.Name(), stage) {
			continue
		}
		target := filepath.Join(localState, entry.Name(), fileName)
		if _, err := os.Stat(target); err == nil {
			return target, nil
		}
	}
	// Fallback to legacy worker directory
	return filepath.Join(localState, "worker", fileName), nil
}

// exitStatusPresent checks whether any stage-scoped worker directory contains an exit marker.
func exitStatusPresent(worktreePath string, stage roles.Stage) (bool, error) {
	localState := filepath.Join(worktreePath, "_governator", "_local-state")
	entries, err := os.ReadDir(localState)
	if err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("local state missing %s: %w", localState, err)
		}
		return false, fmt.Errorf("read local state %s: %w", localState, err)
	}
	latestDir, _, stageFound := latestStageDir(localState, entries, stage)
	if stageFound {
		exitPath := filepath.Join(latestDir, "exit.json")
		if _, err := os.Stat(exitPath); err == nil {
			return true, nil
		} else if err != nil && !os.IsNotExist(err) {
			return false, err
		}
		return false, nil
	}
	legacy := filepath.Join(localState, "worker", "exit.json")
	if _, err := os.Stat(legacy); err == nil {
		return true, nil
	}
	return false, nil
}

// dirStageMatches parses a worker directory name and reports whether it matches the requested stage.
func dirStageMatches(name string, stage roles.Stage) bool {
	_, dirStage, ok := parseWorkerDir(name)
	if !ok {
		return false
	}
	return dirStage == string(stage)
}

// inflightExitStatusPresent checks the in-flight record for a task and reads its exit marker when available.
func inflightExitStatusPresent(repoRoot, taskID string, stage roles.Stage) (bool, error) {
	store, err := inflight.NewStore(repoRoot)
	if err != nil {
		return false, err
	}
	set, err := store.Load()
	if err != nil {
		return false, err
	}
	entry, ok := set.Entry(taskID)
	if !ok || strings.TrimSpace(entry.WorkerStateDir) == "" {
		return false, nil
	}
	if strings.TrimSpace(entry.Stage) != string(stage) {
		return false, nil
	}
	_, found, err := worker.ReadExitStatus(entry.WorkerStateDir, taskID, stage)
	if err != nil {
		return false, err
	}
	return found, nil
}

// latestStageDir selects the highest-attempt worker directory for the requested stage.
func latestStageDir(localState string, entries []os.DirEntry, stage roles.Stage) (string, int, bool) {
	targetStage := string(stage)
	bestAttempt := -1
	bestDir := ""
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		attempt, dirStage, ok := parseWorkerDir(entry.Name())
		if !ok || dirStage != targetStage {
			continue
		}
		if attempt > bestAttempt {
			bestAttempt = attempt
			bestDir = filepath.Join(localState, entry.Name())
		}
	}
	if bestAttempt < 0 || bestDir == "" {
		return "", 0, false
	}
	return bestDir, bestAttempt, true
}

// parseWorkerDir parses a worker directory name into its attempt and stage components.
func parseWorkerDir(name string) (int, string, bool) {
	parts := strings.Split(name, "-")
	if len(parts) < 4 || parts[0] != "worker" {
		return 0, "", false
	}
	attempt, err := strconv.Atoi(parts[1])
	if err != nil || attempt <= 0 {
		return 0, "", false
	}
	return attempt, parts[2], true
}

// tailFile returns a short trailing snippet from a log file when present.
func tailFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return ""
	}
	const max = 200
	if len(data) > max {
		data = data[len(data)-max:]
	}
	return strings.TrimSpace(string(data))
}
