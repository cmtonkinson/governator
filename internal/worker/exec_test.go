// Package worker provides tests for worker process execution.
package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
)

// auditCall captures the parameters passed to LogWorkerTimeout.
type auditCall struct {
	taskID       string
	role         string
	timeoutSecs  int
	worktreePath string
}

// mockAuditLogger implements AuditLogger for testing.
type mockAuditLogger struct {
	calls *[]auditCall
}

// LogWorkerTimeout records the call parameters for testing.
func (m *mockAuditLogger) LogWorkerTimeout(taskID string, role string, timeoutSecs int, worktreePath string) error {
	*m.calls = append(*m.calls, auditCall{
		taskID:       taskID,
		role:         role,
		timeoutSecs:  timeoutSecs,
		worktreePath: worktreePath,
	})
	return nil
}

// TestExecuteWorkerHappyPath ensures successful worker execution produces logs.
func TestExecuteWorkerHappyPath(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	input := ExecInput{
		Command:     []string{"echo", "hello world"},
		WorkDir:     workDir,
		TaskID:      "T-001",
		TimeoutSecs: 5,
		EnvVars: map[string]string{
			"TEST_VAR": "test_value",
		},
	}

	result, err := ExecuteWorker(input)
	if err != nil {
		t.Fatalf("ExecuteWorker failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (error: %v)", result.ExitCode, result.Error)
	}
	if result.Error != nil {
		t.Fatalf("unexpected exec error: %v", result.Error)
	}
	if result.TimedOut {
		t.Fatal("process should not have timed out")
	}
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.Duration <= 0 {
		t.Fatal("duration should be positive")
	}

	// Check that log files were created
	stdoutPath := filepath.Join(workDir, result.StdoutPath)
	stderrPath := filepath.Join(workDir, result.StderrPath)

	if _, err := os.Stat(stdoutPath); err != nil {
		t.Fatalf("stdout log file missing: %v", err)
	}
	if _, err := os.Stat(stderrPath); err != nil {
		t.Fatalf("stderr log file missing: %v", err)
	}

	// Check stdout content
	stdoutContent, err := os.ReadFile(stdoutPath)
	if err != nil {
		t.Fatalf("read stdout log: %v", err)
	}
	if !strings.Contains(string(stdoutContent), "hello world") {
		t.Fatalf("stdout log missing expected content: %s", string(stdoutContent))
	}

	// Check log file paths are repo-relative
	if !strings.HasPrefix(result.StdoutPath, "_governator/_local_state/logs/") {
		t.Fatalf("stdout path = %q, want _governator/_local_state/logs/ prefix", result.StdoutPath)
	}
	if !strings.HasPrefix(result.StderrPath, "_governator/_local_state/logs/") {
		t.Fatalf("stderr path = %q, want _governator/_local_state/logs/ prefix", result.StderrPath)
	}
}

// TestExecuteWorkerTimeout ensures timeout handling works correctly.
func TestExecuteWorkerTimeout(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	var warnings []string
	warn := func(msg string) {
		warnings = append(warnings, msg)
	}

	input := ExecInput{
		Command:     []string{"sleep", "10"}, // Sleep longer than timeout
		WorkDir:     workDir,
		TaskID:      "T-002",
		TimeoutSecs: 1, // Short timeout
		Warn:        warn,
	}

	result, err := ExecuteWorker(input)
	if err != nil {
		t.Fatalf("ExecuteWorker failed: %v", err)
	}

	if !result.TimedOut {
		t.Fatal("process should have timed out")
	}
	if result.ExitCode != -1 {
		t.Fatalf("exit code = %d, want -1 for timeout", result.ExitCode)
	}
	if result.Error == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(result.Error.Error(), "timed out") {
		t.Fatalf("error = %q, want timeout message", result.Error.Error())
	}

	// Check that warning was emitted
	if len(warnings) == 0 {
		t.Fatal("expected timeout warning")
	}
	if !strings.Contains(warnings[0], "timed out") {
		t.Fatalf("warning = %q, want timeout message", warnings[0])
	}

	// Check that log files were still created
	stdoutPath := filepath.Join(workDir, result.StdoutPath)
	stderrPath := filepath.Join(workDir, result.StderrPath)

	if _, err := os.Stat(stdoutPath); err != nil {
		t.Fatalf("stdout log file missing: %v", err)
	}
	if _, err := os.Stat(stderrPath); err != nil {
		t.Fatalf("stderr log file missing: %v", err)
	}
}

// TestExecuteWorkerTimeoutWithAuditLogging ensures timeout audit logging works correctly.
func TestExecuteWorkerTimeoutWithAuditLogging(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Mock audit logger
	var auditCalls []auditCall
	mockAuditLogger := &mockAuditLogger{calls: &auditCalls}

	var warnings []string
	warn := func(msg string) {
		warnings = append(warnings, msg)
	}

	input := ExecInput{
		Command:      []string{"sleep", "10"}, // Sleep longer than timeout
		WorkDir:      workDir,
		TaskID:       "T-003",
		TimeoutSecs:  1, // Short timeout
		Warn:         warn,
		AuditLogger:  mockAuditLogger,
		Role:         "worker",
		WorktreePath: "_governator/_local_state/worktrees/T-003",
	}

	result, err := ExecuteWorker(input)
	if err != nil {
		t.Fatalf("ExecuteWorker failed: %v", err)
	}
	if !result.TimedOut {
		t.Fatal("expected timeout")
	}

	// Check that audit logger was called
	if len(auditCalls) != 1 {
		t.Fatalf("expected 1 audit call, got %d", len(auditCalls))
	}
	call := auditCalls[0]
	if call.taskID != "T-003" {
		t.Fatalf("audit call task ID = %q, want %q", call.taskID, "T-003")
	}
	if call.role != "worker" {
		t.Fatalf("audit call role = %q, want %q", call.role, "worker")
	}
	if call.timeoutSecs != 1 {
		t.Fatalf("audit call timeout seconds = %d, want %d", call.timeoutSecs, 1)
	}
	if call.worktreePath != "_governator/_local_state/worktrees/T-003" {
		t.Fatalf("audit call worktree path = %q, want %q", call.worktreePath, "_governator/_local_state/worktrees/T-003")
	}
}

// TestExecuteWorkerTimeoutWithoutAuditLogger ensures timeout works without audit logger.
func TestExecuteWorkerTimeoutWithoutAuditLogger(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	var warnings []string
	warn := func(msg string) {
		warnings = append(warnings, msg)
	}

	input := ExecInput{
		Command:      []string{"sleep", "10"}, // Sleep longer than timeout
		WorkDir:      workDir,
		TaskID:       "T-004",
		TimeoutSecs:  1, // Short timeout
		Warn:         warn,
		AuditLogger:  nil, // No audit logger
		Role:         "worker",
		WorktreePath: "_governator/_local_state/worktrees/T-004",
	}

	result, err := ExecuteWorker(input)
	if err != nil {
		t.Fatalf("ExecuteWorker failed: %v", err)
	}
	if !result.TimedOut {
		t.Fatal("expected timeout")
	}

	// Should still work without audit logger
	if len(warnings) == 0 {
		t.Fatal("expected timeout warning")
	}
	if !strings.Contains(warnings[0], "timed out") {
		t.Fatalf("warning = %q, want timeout message", warnings[0])
	}
}

// TestExecuteWorkerTimeoutMissingFields ensures audit logging is skipped when fields are missing.
func TestExecuteWorkerTimeoutMissingFields(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Mock audit logger
	var auditCalls []auditCall
	mockAuditLogger := &mockAuditLogger{calls: &auditCalls}

	var warnings []string
	warn := func(msg string) {
		warnings = append(warnings, msg)
	}

	input := ExecInput{
		Command:      []string{"sleep", "10"}, // Sleep longer than timeout
		WorkDir:      workDir,
		TaskID:       "T-005",
		TimeoutSecs:  1, // Short timeout
		Warn:         warn,
		AuditLogger:  mockAuditLogger,
		Role:         "", // Missing role
		WorktreePath: "", // Missing worktree path
	}

	result, err := ExecuteWorker(input)
	if err != nil {
		t.Fatalf("ExecuteWorker failed: %v", err)
	}
	if !result.TimedOut {
		t.Fatal("expected timeout")
	}

	// Audit logger should not be called when fields are missing
	if len(auditCalls) != 0 {
		t.Fatalf("expected 0 audit calls, got %d", len(auditCalls))
	}
}

// TestExecuteWorkerNonTimeoutFailureNoTimeoutMessage ensures non-timeout failures don't emit timeout messages.
func TestExecuteWorkerNonTimeoutFailureNoTimeoutMessage(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Mock audit logger
	var auditCalls []auditCall
	mockAuditLogger := &mockAuditLogger{calls: &auditCalls}

	var warnings []string
	warn := func(msg string) {
		warnings = append(warnings, msg)
	}

	input := ExecInput{
		Command:      []string{"false"}, // Command that fails but doesn't timeout
		WorkDir:      workDir,
		TaskID:       "T-006",
		TimeoutSecs:  10, // Long timeout
		Warn:         warn,
		AuditLogger:  mockAuditLogger,
		Role:         "worker",
		WorktreePath: "_governator/_local_state/worktrees/T-006",
	}

	result, err := ExecuteWorker(input)
	if err != nil {
		t.Fatalf("ExecuteWorker failed: %v", err)
	}
	if result.TimedOut {
		t.Fatal("process should not have timed out")
	}
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}

	// Audit logger should not be called for non-timeout failures
	if len(auditCalls) != 0 {
		t.Fatalf("expected 0 audit calls for non-timeout failure, got %d", len(auditCalls))
	}

	// Warnings should not contain timeout messages
	for _, warning := range warnings {
		if strings.Contains(warning, "timed out") {
			t.Fatalf("warning should not contain timeout message for non-timeout failure: %q", warning)
		}
	}
}

// TestExecuteWorkerNonZeroExit ensures non-zero exit codes are handled correctly.
func TestExecuteWorkerNonZeroExit(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	input := ExecInput{
		Command:     []string{"sh", "-c", "echo 'error message' >&2; exit 42"},
		WorkDir:     workDir,
		TaskID:      "T-003",
		TimeoutSecs: 5,
	}

	result, err := ExecuteWorker(input)
	if err != nil {
		t.Fatalf("ExecuteWorker failed: %v", err)
	}

	if result.ExitCode != 42 {
		t.Fatalf("exit code = %d, want 42", result.ExitCode)
	}
	if result.TimedOut {
		t.Fatal("process should not have timed out")
	}
	if result.Error == nil {
		t.Fatal("expected error for non-zero exit")
	}
	if !strings.Contains(result.Error.Error(), "exited with code 42") {
		t.Fatalf("error = %q, want exit code message", result.Error.Error())
	}

	// Check stderr content
	stderrPath := filepath.Join(workDir, result.StderrPath)
	stderrContent, err := os.ReadFile(stderrPath)
	if err != nil {
		t.Fatalf("read stderr log: %v", err)
	}
	if !strings.Contains(string(stderrContent), "error message") {
		t.Fatalf("stderr log missing expected content: %s", string(stderrContent))
	}
}

// TestExecuteWorkerValidation ensures input validation works correctly.
func TestExecuteWorkerValidation(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	tests := []struct {
		name    string
		input   ExecInput
		wantErr string
	}{
		{
			name: "empty command",
			input: ExecInput{
				Command:     []string{},
				WorkDir:     workDir,
				TaskID:      "T-001",
				TimeoutSecs: 5,
			},
			wantErr: "command is required",
		},
		{
			name: "empty work directory",
			input: ExecInput{
				Command:     []string{"echo", "test"},
				WorkDir:     "",
				TaskID:      "T-001",
				TimeoutSecs: 5,
			},
			wantErr: "work directory is required",
		},
		{
			name: "empty task id",
			input: ExecInput{
				Command:     []string{"echo", "test"},
				WorkDir:     workDir,
				TaskID:      "",
				TimeoutSecs: 5,
			},
			wantErr: "task id is required",
		},
		{
			name: "zero timeout",
			input: ExecInput{
				Command:     []string{"echo", "test"},
				WorkDir:     workDir,
				TaskID:      "T-001",
				TimeoutSecs: 0,
			},
			wantErr: "timeout seconds must be positive",
		},
		{
			name: "negative timeout",
			input: ExecInput{
				Command:     []string{"echo", "test"},
				WorkDir:     workDir,
				TaskID:      "T-001",
				TimeoutSecs: -1,
			},
			wantErr: "timeout seconds must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ExecuteWorker(tt.input)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestExecuteWorkerFromConfig ensures config-based execution works correctly.
func TestExecuteWorkerFromConfig(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	cfg := config.Config{
		Workers: config.WorkersConfig{
			Commands: config.WorkerCommands{
				Default: []string{"echo", "task: {task_path}"},
			},
		},
		Timeouts: config.TimeoutsConfig{
			WorkerSeconds: 10,
		},
	}

	task := index.Task{
		ID:   "T-004",
		Path: "tasks/example.md",
		Role: "worker",
	}

	stageResult := StageResult{
		Env: map[string]string{
			"GOVERNATOR_TASK_ID": "T-004",
			"GOVERNATOR_ROLE":    "worker",
		},
	}

	result, err := ExecuteWorkerFromConfig(cfg, task, stageResult, workDir, nil)
	if err != nil {
		t.Fatalf("ExecuteWorkerFromConfig failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (error: %v)", result.ExitCode, result.Error)
	}
	if result.Error != nil {
		t.Fatalf("unexpected exec error: %v", result.Error)
	}
	if result.TimedOut {
		t.Fatal("process should not have timed out")
	}

	// Check stdout content contains resolved task path
	stdoutPath := filepath.Join(workDir, result.StdoutPath)
	stdoutContent, err := os.ReadFile(stdoutPath)
	if err != nil {
		t.Fatalf("read stdout log: %v", err)
	}
	if !strings.Contains(string(stdoutContent), "task: tasks/example.md") {
		t.Fatalf("stdout log missing expected content: %s", string(stdoutContent))
	}
}

// TestExecuteWorkerEnvironmentVariables ensures environment variables are passed correctly.
func TestExecuteWorkerEnvironmentVariables(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	input := ExecInput{
		Command:     []string{"sh", "-c", "echo $CUSTOM_VAR"},
		WorkDir:     workDir,
		TaskID:      "T-005",
		TimeoutSecs: 5,
		EnvVars: map[string]string{
			"CUSTOM_VAR": "custom_value",
		},
	}

	result, err := ExecuteWorker(input)
	if err != nil {
		t.Fatalf("ExecuteWorker failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (error: %v)", result.ExitCode, result.Error)
	}
	if result.Error != nil {
		t.Fatalf("unexpected exec error: %v", result.Error)
	}

	// Check stdout content contains environment variable value
	stdoutPath := filepath.Join(workDir, result.StdoutPath)
	stdoutContent, err := os.ReadFile(stdoutPath)
	if err != nil {
		t.Fatalf("read stdout log: %v", err)
	}
	if !strings.Contains(string(stdoutContent), "custom_value") {
		t.Fatalf("stdout log missing environment variable: %s", string(stdoutContent))
	}
}

// TestExecuteWorkerLogFileNaming ensures log files are named correctly.
func TestExecuteWorkerLogFileNaming(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	input := ExecInput{
		Command:     []string{"echo", "test"},
		WorkDir:     workDir,
		TaskID:      "T-006",
		TimeoutSecs: 5,
	}

	result, err := ExecuteWorker(input)
	if err != nil {
		t.Fatalf("ExecuteWorker failed: %v", err)
	}

	// Check log file naming pattern
	if !strings.Contains(result.StdoutPath, "T-006") {
		t.Fatalf("stdout path = %q, want task ID in filename", result.StdoutPath)
	}
	if !strings.Contains(result.StderrPath, "T-006") {
		t.Fatalf("stderr path = %q, want task ID in filename", result.StderrPath)
	}
	if !strings.HasSuffix(result.StdoutPath, "-stdout.log") {
		t.Fatalf("stdout path = %q, want -stdout.log suffix", result.StdoutPath)
	}
	if !strings.HasSuffix(result.StderrPath, "-stderr.log") {
		t.Fatalf("stderr path = %q, want -stderr.log suffix", result.StderrPath)
	}
}

// TestExecuteWorkerWithStubCommandSuccess confirms a stub worker completes and logs output.
func TestExecuteWorkerWithStubCommandSuccess(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()
	stub := stubRunnerPath(t)

	input := ExecInput{
		Command:     []string{stub, "success"},
		WorkDir:     workDir,
		TaskID:      "T-007",
		TimeoutSecs: 5,
	}

	result, err := ExecuteWorker(input)
	if err != nil {
		t.Fatalf("ExecuteWorker failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (error: %v)", result.ExitCode, result.Error)
	}
	if result.Error != nil {
		t.Fatalf("unexpected exec error: %v", result.Error)
	}
	if result.TimedOut {
		t.Fatal("process should not have timed out")
	}

	stdoutPath := filepath.Join(workDir, result.StdoutPath)
	stdoutContent, err := os.ReadFile(stdoutPath)
	if err != nil {
		t.Fatalf("read stdout log: %v", err)
	}
	if !strings.Contains(string(stdoutContent), "stub worker runner success") {
		t.Fatalf("stdout log missing stub success message: %s", string(stdoutContent))
	}
}

// TestExecuteWorkerWithStubCommandTimeout ensures a stub worker that sleeps past the timeout is terminated.
func TestExecuteWorkerWithStubCommandTimeout(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()
	stub := stubRunnerPath(t)

	var warnings []string
	warn := func(msg string) {
		warnings = append(warnings, msg)
	}

	input := ExecInput{
		Command:     []string{stub, "sleep", "10"},
		WorkDir:     workDir,
		TaskID:      "T-008",
		TimeoutSecs: 1,
		Warn:        warn,
	}

	result, err := ExecuteWorker(input)
	if err != nil {
		t.Fatalf("ExecuteWorker failed: %v", err)
	}

	if !result.TimedOut {
		t.Fatal("process should have timed out")
	}
	if result.ExitCode != -1 {
		t.Fatalf("exit code = %d, want -1 for timeout", result.ExitCode)
	}
	if result.Error == nil {
		t.Fatal("expected timeout error")
	}

	if len(warnings) == 0 {
		t.Fatal("expected timeout warning")
	}
	if !strings.Contains(warnings[0], "timed out") {
		t.Fatalf("warning = %q, want timeout notice", warnings[0])
	}

	stdoutPath := filepath.Join(workDir, result.StdoutPath)
	stdoutContent, err := os.ReadFile(stdoutPath)
	if err != nil {
		t.Fatalf("read stdout log: %v", err)
	}
	if !strings.Contains(string(stdoutContent), "stub worker runner sleeping for") {
		t.Fatalf("stdout log missing stub sleep message: %s", string(stdoutContent))
	}
}

func stubRunnerPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join("testdata", "stub-worker-runner.sh")
	absPath, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve stub runner path: %v", err)
	}
	return absPath
}
