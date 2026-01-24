// Package worker provides worker process execution with timeout and logging.
package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
)

const (
	// logsDirName is the relative path for worker execution logs.
	logsDirName = "_governator/_local-state/logs"
	// logFileMode is the file mode for log files.
	logFileMode = 0o644
	// logDirMode is the directory mode for log directories.
	logDirMode = 0o755
)

// AuditLogger defines the interface for audit logging.
type AuditLogger interface {
	LogWorkerTimeout(taskID string, role string, timeoutSecs int, worktreePath string) error
}

// ExecInput defines the inputs required for worker process execution.
type ExecInput struct {
	Command      []string
	WorkDir      string
	TaskID       string
	TimeoutSecs  int
	EnvVars      map[string]string
	Warn         func(string)
	AuditLogger  AuditLogger
	Role         string
	WorktreePath string
}

// ExecResult captures the worker process execution results.
type ExecResult struct {
	ExitCode   int
	TimedOut   bool
	StdoutPath string
	StderrPath string
	Duration   time.Duration
	Error      error
}

// workerLogFiles groups log paths and file handles for a worker execution.
type workerLogFiles struct {
	stdoutPath string
	stderrPath string
	stdoutFile *os.File
	stderrFile *os.File
}

// ExecuteWorker runs a worker process with timeout and log capture.
func ExecuteWorker(input ExecInput) (ExecResult, error) {
	if len(input.Command) == 0 {
		return ExecResult{}, errors.New("command is required")
	}
	if strings.TrimSpace(input.WorkDir) == "" {
		return ExecResult{}, errors.New("work directory is required")
	}
	if strings.TrimSpace(input.TaskID) == "" {
		return ExecResult{}, errors.New("task id is required")
	}
	if input.TimeoutSecs <= 0 {
		return ExecResult{}, errors.New("timeout seconds must be positive")
	}

	logFiles, err := createWorkerLogFiles(input.WorkDir, input.TaskID)
	if err != nil {
		return ExecResult{}, err
	}
	defer logFiles.stdoutFile.Close()
	defer logFiles.stderrFile.Close()

	// Set up command with timeout context
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(input.TimeoutSecs)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, input.Command[0], input.Command[1:]...)
	cmd.Dir = input.WorkDir
	cmd.Stdout = logFiles.stdoutFile
	cmd.Stderr = logFiles.stderrFile

	// Set environment variables
	if len(input.EnvVars) > 0 {
		env := os.Environ()
		for key, value := range input.EnvVars {
			env = append(env, fmt.Sprintf("%s=%s", key, value))
		}
		cmd.Env = env
	}

	// Execute the command
	start := time.Now()
	err = cmd.Run()
	duration := time.Since(start)

	// Determine result
	result := ExecResult{
		StdoutPath: repoRelativePath(input.WorkDir, logFiles.stdoutPath),
		StderrPath: repoRelativePath(input.WorkDir, logFiles.stderrPath),
		Duration:   duration,
	}

	if err != nil {
		// Check if it was a timeout
		if ctx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
			result.ExitCode = -1
			result.Error = fmt.Errorf("worker process timed out after %d seconds", input.TimeoutSecs)

			// Log timeout to audit log if logger is provided
			if input.AuditLogger != nil && input.Role != "" && input.WorktreePath != "" {
				if auditErr := input.AuditLogger.LogWorkerTimeout(input.TaskID, input.Role, input.TimeoutSecs, input.WorktreePath); auditErr != nil {
					emitWarning(input.Warn, fmt.Sprintf("failed to log timeout to audit: %v", auditErr))
				}
			}

			emitWarning(input.Warn, fmt.Sprintf("task %s timed out after %d seconds", input.TaskID, input.TimeoutSecs))
		} else if exitError, ok := err.(*exec.ExitError); ok {
			// Process exited with non-zero code
			result.ExitCode = exitError.ExitCode()
			result.Error = fmt.Errorf("worker process exited with code %d", result.ExitCode)
		} else {
			// Other execution error
			result.ExitCode = -1
			result.Error = fmt.Errorf("worker process execution failed: %w", err)
		}
	} else {
		result.ExitCode = 0
	}

	return result, nil
}

// ExecuteWorkerFromConfig executes a worker using configuration and staging results.
func ExecuteWorkerFromConfig(cfg config.Config, task index.Task, stageResult StageResult, workDir string, warn func(string)) (ExecResult, error) {
	return ExecuteWorkerFromConfigWithAudit(cfg, task, stageResult, workDir, warn, nil, "")
}

// ExecuteWorkerFromConfigWithAudit executes a worker using configuration and staging results with audit logging.
func ExecuteWorkerFromConfigWithAudit(cfg config.Config, task index.Task, stageResult StageResult, workDir string, warn func(string), auditLogger AuditLogger, worktreePath string) (ExecResult, error) {
	command, err := ResolveCommand(cfg, task.Role, task.Path, workDir)
	if err != nil {
		return ExecResult{}, fmt.Errorf("resolve worker command: %w", err)
	}

	input := ExecInput{
		Command:      command,
		WorkDir:      workDir,
		TaskID:       task.ID,
		TimeoutSecs:  cfg.Timeouts.WorkerSeconds,
		EnvVars:      stageResult.Env,
		Warn:         warn,
		AuditLogger:  auditLogger,
		Role:         string(task.Role),
		WorktreePath: worktreePath,
	}

	return ExecuteWorker(input)
}

// emitWarning sends a warning to the configured sink.
func emitWarning(warn func(string), message string) {
	if warn == nil {
		return
	}
	warn(message)
}

// createWorkerLogFiles creates stdout/stderr log files for worker execution.
func createWorkerLogFiles(workDir string, taskID string) (workerLogFiles, error) {
	logsDir := filepath.Join(workDir, logsDirName)
	if err := os.MkdirAll(logsDir, logDirMode); err != nil {
		return workerLogFiles{}, fmt.Errorf("create logs directory %s: %w", logsDir, err)
	}

	timestamp := time.Now().Format("20060102-150405")
	stdoutPath := filepath.Join(logsDir, fmt.Sprintf("%s-%s-stdout.log", taskID, timestamp))
	stderrPath := filepath.Join(logsDir, fmt.Sprintf("%s-%s-stderr.log", taskID, timestamp))

	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return workerLogFiles{}, fmt.Errorf("create stdout log %s: %w", stdoutPath, err)
	}

	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		stdoutFile.Close()
		return workerLogFiles{}, fmt.Errorf("create stderr log %s: %w", stderrPath, err)
	}

	return workerLogFiles{
		stdoutPath: stdoutPath,
		stderrPath: stderrPath,
		stdoutFile: stdoutFile,
		stderrFile: stderrFile,
	}, nil
}
