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
	exitPath := filepath.Join(worktreePath, "_governator", "_local_state", "worker", fmt.Sprintf("exit-%s-%s.json", stage, taskID))
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(exitPath); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for exit status %s", exitPath)
}
