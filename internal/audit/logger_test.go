// Tests for the audit logger.
package audit

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLoggerWritesEntries ensures audit entries are written in order.
func TestLoggerWritesEntries(t *testing.T) {
	repoRoot := t.TempDir()
	logPath := filepath.Join(repoRoot, localStateDirName, auditLogFileName)
	if err := os.MkdirAll(filepath.Dir(logPath), auditLogDirMode); err != nil {
		t.Fatalf("create audit log dir: %v", err)
	}
	if err := os.WriteFile(logPath, []byte(""), auditLogFileMode); err != nil {
		t.Fatalf("create audit log file: %v", err)
	}

	var warnings bytes.Buffer
	logger, err := NewLogger(repoRoot, &warnings)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	fixedTime := time.Date(2025, 1, 14, 19, 2, 11, 0, time.UTC)
	logger.now = func() time.Time {
		return fixedTime
	}

	if err := logger.LogWorktreeCreate("T-014", "worker", "_governator/_local-state/task-T-014", "task-T-014"); err != nil {
		t.Fatalf("log worktree create: %v", err)
	}
	if err := logger.LogTaskTransition("T-014", "worker", "open", "worked"); err != nil {
		t.Fatalf("log transition: %v", err)
	}

	if warnings.Len() != 0 {
		t.Fatalf("expected no warnings, got %q", warnings.String())
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 audit log lines, got %d", len(lines))
	}
	expectedFirst := "ts=2025-01-14T19:02:11Z task_id=T-014 role=worker event=worktree.create path=_governator/_local-state/task-T-014 branch=task-T-014"
	if lines[0] != expectedFirst {
		t.Fatalf("expected first audit line %q, got %q", expectedFirst, lines[0])
	}
	expectedSecond := "ts=2025-01-14T19:02:11Z task_id=T-014 role=worker event=task.transition from=open to=worked"
	if lines[1] != expectedSecond {
		t.Fatalf("expected second audit line %q, got %q", expectedSecond, lines[1])
	}
}

// TestLoggerMissingFileCreatesAndWarns ensures missing audit logs are recreated with a warning.
func TestLoggerMissingFileCreatesAndWarns(t *testing.T) {
	repoRoot := t.TempDir()
	logPath := filepath.Join(repoRoot, localStateDirName, auditLogFileName)

	var warnings bytes.Buffer
	logger, err := NewLogger(repoRoot, &warnings)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	logger.now = func() time.Time {
		return time.Date(2025, 1, 14, 20, 8, 11, 0, time.UTC)
	}

	if err := logger.LogBranchCreate("T-014", "worker", "gov/T-014", "main"); err != nil {
		t.Fatalf("log branch create: %v", err)
	}

	if warnings.Len() == 0 {
		t.Fatal("expected warning when audit log was missing")
	}
	if !strings.Contains(warnings.String(), "audit log missing") {
		t.Fatalf("expected missing log warning, got %q", warnings.String())
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("expected audit log file to exist, got %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if !strings.Contains(string(data), "event=branch.create") {
		t.Fatalf("expected branch create entry, got %q", string(data))
	}
}

// TestLoggerRejectsMissingFields ensures invalid entries are rejected.
func TestLoggerRejectsMissingFields(t *testing.T) {
	repoRoot := t.TempDir()
	var warnings bytes.Buffer
	logger, err := NewLogger(repoRoot, &warnings)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	if err := logger.Log(Entry{}); err == nil {
		t.Fatal("expected error for missing entry fields")
	}
	if warnings.Len() == 0 {
		t.Fatal("expected warning for rejected entry")
	}
}

// TestLogWorkerTimeout ensures worker timeout events are logged correctly.
func TestLogWorkerTimeout(t *testing.T) {
	repoRoot := t.TempDir()
	logPath := filepath.Join(repoRoot, localStateDirName, auditLogFileName)
	if err := os.MkdirAll(filepath.Dir(logPath), auditLogDirMode); err != nil {
		t.Fatalf("create audit log dir: %v", err)
	}
	if err := os.WriteFile(logPath, []byte(""), auditLogFileMode); err != nil {
		t.Fatalf("create audit log file: %v", err)
	}

	var warnings bytes.Buffer
	logger, err := NewLogger(repoRoot, &warnings)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	fixedTime := time.Date(2025, 1, 14, 20, 15, 30, 0, time.UTC)
	logger.now = func() time.Time {
		return fixedTime
	}

	if err := logger.LogWorkerTimeout("T-042", "worker", 300, "_governator/_local-state/task-T-042"); err != nil {
		t.Fatalf("log worker timeout: %v", err)
	}

	if warnings.Len() != 0 {
		t.Fatalf("expected no warnings, got %q", warnings.String())
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 audit log line, got %d", len(lines))
	}
	expected := "ts=2025-01-14T20:15:30Z task_id=T-042 role=worker event=worker.timeout timeout_seconds=300 worktree_path=_governator/_local-state/task-T-042"
	if lines[0] != expected {
		t.Fatalf("expected audit line %q, got %q", expected, lines[0])
	}
}
