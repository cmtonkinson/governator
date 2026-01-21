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
	logsDirName = "_governator/_local_state/logs"
	// logFileMode is the file mode for log files.
	logFileMode = 0o644
	// logDirMode is the directory mode for log directories.
	logDirMode = 0o755
)

// ExecInput defines the inputs required for worker process execution.
type ExecInput struct {
	Command      []string
	WorkDir      string
	TaskID       string
	TimeoutSecs  int
	EnvVars      map[string]string
	Warn         func(string)
}

// ExecResult captures the worker process execution results.
type ExecResult struct {
	ExitCode    int
	TimedOut    bool
	StdoutPath  string
	StderrPath  string
	Duration    time.Duration
	Error       error
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

	// Create logs directory
	logsDir := filepath.Join(input.WorkDir, logsDirName)
	if err := os.MkdirAll(logsDir, logDirMode); err != nil {
		return ExecResult{}, fmt.Errorf("create logs directory %s: %w", logsDir, err)
	}

	// Create log files
	timestamp := time.Now().Format("20060102-150405")
	stdoutPath := filepath.Join(logsDir, fmt.Sprintf("%s-%s-stdout.log", input.TaskID, timestamp))
	stderrPath := filepath.Join(logsDir, fmt.Sprintf("%s-%s-stderr.log", input.TaskID, timestamp))

	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return ExecResult{}, fmt.Errorf("create stdout log %s: %w", stdoutPath, err)
	}
	defer stdoutFile.Close()

	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		return ExecResult{}, fmt.Errorf("create stderr log %s: %w", stderrPath, err)
	}
	defer stderrFile.Close()

	// Set up command with timeout context
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(input.TimeoutSecs)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, input.Command[0], input.Command[1:]...)
	cmd.Dir = input.WorkDir
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile

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
		StdoutPath: repoRelativePath(input.WorkDir, stdoutPath),
		StderrPath: repoRelativePath(input.WorkDir, stderrPath),
		Duration:   duration,
	}

	if err != nil {
		// Check if it was a timeout
		if ctx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
			result.ExitCode = -1
			result.Error = fmt.Errorf("worker process timed out after %d seconds", input.TimeoutSecs)
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
	command, err := ResolveCommand(cfg, task.Role, task.Path, workDir)
	if err != nil {
		return ExecResult{}, fmt.Errorf("resolve worker command: %w", err)
	}

	input := ExecInput{
		Command:     command,
		WorkDir:     workDir,
		TaskID:      task.ID,
		TimeoutSecs: cfg.Timeouts.WorkerSeconds,
		EnvVars:     stageResult.Env,
		Warn:        warn,
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