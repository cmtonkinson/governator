package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cmtonkinson/governator/internal/roles"
)

func TestReadExitStatusIncludesPID(t *testing.T) {
	workerStateDir := t.TempDir()
	taskID := "T-123"

	exitPath, err := exitStatusPath(workerStateDir, taskID, roles.StageWork)
	if err != nil {
		t.Fatalf("exitStatusPath failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(exitPath), 0o755); err != nil {
		t.Fatalf("mkdir exit dir: %v", err)
	}

	payload := `{"exit_code":42,"finished_at":"2025-01-01T00:00:00Z","pid":314159}`
	if err := os.WriteFile(exitPath, []byte(payload), 0o644); err != nil {
		t.Fatalf("write exit status: %v", err)
	}

	status, found, err := ReadExitStatus(workerStateDir, taskID, roles.StageWork)
	if err != nil {
		t.Fatalf("ReadExitStatus failed: %v", err)
	}
	if !found {
		t.Fatal("exit status not found")
	}
	if status.ExitCode != 42 {
		t.Fatalf("exit code = %d, want 42", status.ExitCode)
	}
	if status.PID != 314159 {
		t.Fatalf("pid = %d, want %d", status.PID, 314159)
	}
	if status.FinishedAt.IsZero() {
		t.Fatalf("finished at should be set")
	}
}
