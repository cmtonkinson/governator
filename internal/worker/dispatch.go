// Package worker provides asynchronous worker dispatch helpers.
package worker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/roles"
)

const (
	wrapperFileMode = 0o755
)

// DispatchInput defines the inputs required for asynchronous worker dispatch.
type DispatchInput struct {
	Command []string
	WorkDir string
	TaskID  string
	Stage   roles.Stage
	EnvVars map[string]string
	Warn    func(string)
}

// DispatchResult captures the worker dispatch metadata.
type DispatchResult struct {
	PID        int
	StartedAt  time.Time
	StdoutPath string
	StderrPath string
	ExitPath   string
}

// ExitStatus records the terminal status of a worker process.
type ExitStatus struct {
	ExitCode   int       `json:"exit_code"`
	FinishedAt time.Time `json:"finished_at"`
}

// DispatchWorker launches a worker process in the background using nohup.
func DispatchWorker(input DispatchInput) (DispatchResult, error) {
	if len(input.Command) == 0 {
		return DispatchResult{}, errors.New("command is required")
	}
	if strings.TrimSpace(input.WorkDir) == "" {
		return DispatchResult{}, errors.New("work directory is required")
	}
	if strings.TrimSpace(input.TaskID) == "" {
		return DispatchResult{}, errors.New("task id is required")
	}
	if !input.Stage.Valid() {
		return DispatchResult{}, fmt.Errorf("invalid stage %q", input.Stage)
	}

	logFiles, err := createWorkerLogFiles(input.WorkDir, input.TaskID)
	if err != nil {
		return DispatchResult{}, err
	}
	defer logFiles.stdoutFile.Close()
	defer logFiles.stderrFile.Close()

	exitPath, err := exitStatusPath(input.WorkDir, input.TaskID, input.Stage)
	if err != nil {
		return DispatchResult{}, err
	}
	wrapperPath, err := writeDispatchWrapper(input.WorkDir, input.TaskID, input.Stage, input.Command, exitPath)
	if err != nil {
		return DispatchResult{}, err
	}

	cmd := exec.Command("nohup", wrapperPath)
	cmd.Dir = input.WorkDir
	cmd.Stdout = logFiles.stdoutFile
	cmd.Stderr = logFiles.stderrFile
	if len(input.EnvVars) > 0 {
		env := os.Environ()
		for key, value := range input.EnvVars {
			env = append(env, fmt.Sprintf("%s=%s", key, value))
		}
		cmd.Env = env
	}

	startedAt := time.Now().UTC()
	if err := cmd.Start(); err != nil {
		return DispatchResult{}, fmt.Errorf("start worker process: %w", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		emitWarning(input.Warn, fmt.Sprintf("failed to detach worker process: %v", err))
	}

	return DispatchResult{
		PID:        pid,
		StartedAt:  startedAt,
		StdoutPath: repoRelativePath(input.WorkDir, logFiles.stdoutPath),
		StderrPath: repoRelativePath(input.WorkDir, logFiles.stderrPath),
		ExitPath:   repoRelativePath(input.WorkDir, exitPath),
	}, nil
}

// DispatchWorkerFromConfig resolves the worker command and dispatches asynchronously.
func DispatchWorkerFromConfig(cfg config.Config, task index.Task, stageResult StageResult, workDir string, stage roles.Stage, warn func(string)) (DispatchResult, error) {
	command, err := ResolveCommand(cfg, task.Role, task.Path, workDir, stageResult.PromptPath)
	if err != nil {
		return DispatchResult{}, fmt.Errorf("resolve worker command: %w", err)
	}

	input := DispatchInput{
		Command: command,
		WorkDir: workDir,
		TaskID:  task.ID,
		Stage:   stage,
		EnvVars: stageResult.Env,
		Warn:    warn,
	}

	return DispatchWorker(input)
}

// exitStatusPath returns the absolute path for the worker exit status file.
func exitStatusPath(workDir string, taskID string, stage roles.Stage) (string, error) {
	if strings.TrimSpace(workDir) == "" {
		return "", errors.New("work directory is required")
	}
	if strings.TrimSpace(taskID) == "" {
		return "", errors.New("task id is required")
	}
	if !stage.Valid() {
		return "", fmt.Errorf("invalid stage %q", stage)
	}
	dir := filepath.Join(workDir, localStateDirName, workerStateDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create worker state dir %s: %w", dir, err)
	}
	name := fmt.Sprintf("exit-%s-%s.json", stage, taskID)
	return filepath.Join(dir, name), nil
}

// ReadExitStatus reads the exit status file if present.
func ReadExitStatus(workDir string, taskID string, stage roles.Stage) (ExitStatus, bool, error) {
	path, err := exitStatusPath(workDir, taskID, stage)
	if err != nil {
		return ExitStatus{}, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ExitStatus{}, false, nil
		}
		return ExitStatus{}, false, fmt.Errorf("read exit status %s: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return ExitStatus{}, false, nil
	}
	var status ExitStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return ExitStatus{}, false, fmt.Errorf("decode exit status %s: %w", path, err)
	}
	return status, true, nil
}

// writeDispatchWrapper writes a wrapper script that captures exit status after execution.
func writeDispatchWrapper(workDir string, taskID string, stage roles.Stage, command []string, exitPath string) (string, error) {
	if strings.TrimSpace(workDir) == "" {
		return "", errors.New("work directory is required")
	}
	if strings.TrimSpace(taskID) == "" {
		return "", errors.New("task id is required")
	}
	if !stage.Valid() {
		return "", fmt.Errorf("invalid stage %q", stage)
	}
	if len(command) == 0 {
		return "", errors.New("command is required")
	}
	if strings.TrimSpace(exitPath) == "" {
		return "", errors.New("exit path is required")
	}

	wrapperDir := filepath.Join(workDir, localStateDirName, workerStateDirName)
	if err := os.MkdirAll(wrapperDir, 0o755); err != nil {
		return "", fmt.Errorf("create wrapper dir %s: %w", wrapperDir, err)
	}
	wrapperPath := filepath.Join(wrapperDir, fmt.Sprintf("dispatch-%s-%s.sh", stage, taskID))
	commandLine := shellCommandLine(command)
	exitPathEscaped := shellEscapeArg(exitPath)
	content := strings.Join([]string{
		"#!/bin/sh",
		"set +e",
		commandLine,
		"code=$?",
		"finished_at=$(date -u +\"%Y-%m-%dT%H:%M:%SZ\")",
		"printf '{\"exit_code\":%d,\"finished_at\":\"%s\"}\\n' \"$code\" \"$finished_at\" > " + exitPathEscaped,
		"exit $code",
		"",
	}, "\n")
	if err := os.WriteFile(wrapperPath, []byte(content), wrapperFileMode); err != nil {
		return "", fmt.Errorf("write dispatch wrapper %s: %w", wrapperPath, err)
	}
	return wrapperPath, nil
}

// shellCommandLine builds a shell-safe command string from arguments.
func shellCommandLine(args []string) string {
	escaped := make([]string, 0, len(args))
	for _, arg := range args {
		escaped = append(escaped, shellEscapeArg(arg))
	}
	return strings.Join(escaped, " ")
}

// shellEscapeArg escapes a string for safe use in /bin/sh.
func shellEscapeArg(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\n'\"\\$`") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
