// Test helpers for async dispatch behavior.
package run

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cmtonkinson/governator/internal/roles"
)

// waitForExitStatus polls for the worker exit status marker emitted by async dispatch.
func waitForExitStatus(t *testing.T, worktreePath string, taskID string, stage roles.Stage) {
	t.Helper()
	exitPath, err := findExitStatusPath(worktreePath, taskID, stage)
	if err != nil {
		t.Fatalf("failed to locate exit status path: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		exitPath, err := findExitStatusPath(worktreePath, taskID, stage)
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if _, err := os.Stat(exitPath); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for exit status %s", exitPath)
}

func findExitStatusPath(worktreePath, taskID string, stage roles.Stage) (string, error) {
	localState := filepath.Join(worktreePath, "_governator", "_local-state")
	fileName := fmt.Sprintf("exit-%s-%s.json", stage, taskID)
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
		target := filepath.Join(localState, entry.Name(), fileName)
		if _, err := os.Stat(target); err == nil {
			return target, nil
		}
	}
	// Fallback to legacy worker directory
	return filepath.Join(localState, "worker", fileName), nil
}
