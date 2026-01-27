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
	Command        []string
	WorkDir        string
	TaskID         string
	Stage          roles.Stage
	EnvVars        map[string]string
	Warn           func(string)
	WorkerStateDir string
}

// DispatchResult captures the worker dispatch metadata.
type DispatchResult struct {
	PID            int
	StartedAt      time.Time
	StdoutPath     string
	StderrPath     string
	ExitPath       string
	WorkerStateDir string
}

// dispatchMetadata captures dispatch-time details for observability and debugging.
type dispatchMetadata struct {
	TaskID      string    `json:"task_id"`
	Stage       string    `json:"stage"`
	WorkDir     string    `json:"work_dir"`
	WrapperPath string    `json:"wrapper_path"`
	WrapperPID  int       `json:"wrapper_pid"`
	StartedAt   time.Time `json:"started_at"`
	Command     []string  `json:"command"`
	AgentName   string    `json:"agent_name,omitempty"`
	PIDFiles    []string  `json:"pid_files"`
	StartError  string    `json:"start_error,omitempty"`
}

// ExitStatus records the terminal status of a worker process.
type ExitStatus struct {
	ExitCode   int       `json:"exit_code"`
	FinishedAt time.Time `json:"finished_at"`
	PID        int       `json:"pid,omitempty"`
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
	if strings.TrimSpace(input.WorkerStateDir) == "" {
		return DispatchResult{}, errors.New("worker state dir is required")
	}
	if err := os.MkdirAll(input.WorkerStateDir, 0o755); err != nil {
		return DispatchResult{}, fmt.Errorf("create worker state dir %s: %w", input.WorkerStateDir, err)
	}

	logFiles, err := createWorkerLogFiles(input.WorkDir, input.WorkerStateDir, input.TaskID)
	if err != nil {
		return DispatchResult{}, err
	}
	defer logFiles.stdoutFile.Close()
	defer logFiles.stderrFile.Close()

	exitPath, err := exitStatusPath(input.WorkerStateDir, input.TaskID, input.Stage)
	if err != nil {
		return DispatchResult{}, err
	}
	agentName := detectAgentName(input.Command)
	pidPaths := agentPIDPaths(input.WorkerStateDir, agentName)
	wrapperPath, err := writeDispatchWrapper(input.WorkerStateDir, input.TaskID, input.Stage, input.Command, exitPath, pidPaths)
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
	meta := dispatchMetadata{
		TaskID:      input.TaskID,
		Stage:       string(input.Stage),
		WorkDir:     input.WorkDir,
		WrapperPath: wrapperPath,
		WrapperPID:  0,
		StartedAt:   startedAt,
		Command:     cloneStrings(input.Command),
		AgentName:   agentName,
		PIDFiles:    pidPaths,
	}
	writeDispatchMetadata(input.WorkerStateDir, meta, input.Warn)
	if err := cmd.Start(); err != nil {
		meta.StartError = err.Error()
		writeDispatchMetadata(input.WorkerStateDir, meta, input.Warn)
		return DispatchResult{}, fmt.Errorf("start worker process: %w", err)
	}
	pid := cmd.Process.Pid
	meta.WrapperPID = pid
	meta.StartError = ""
	writeDispatchMetadata(input.WorkerStateDir, meta, input.Warn)
	if err := cmd.Process.Release(); err != nil {
		emitWarning(input.Warn, fmt.Sprintf("failed to detach worker process: %v", err))
	}

	return DispatchResult{
		PID:            pid,
		StartedAt:      startedAt,
		StdoutPath:     repoRelativePath(input.WorkDir, logFiles.stdoutPath),
		StderrPath:     repoRelativePath(input.WorkDir, logFiles.stderrPath),
		ExitPath:       repoRelativePath(input.WorkDir, exitPath),
		WorkerStateDir: input.WorkerStateDir,
	}, nil
}

// DispatchWorkerFromConfig resolves the worker command and dispatches asynchronously.
func DispatchWorkerFromConfig(cfg config.Config, task index.Task, stageResult StageResult, workDir string, stage roles.Stage, warn func(string)) (DispatchResult, error) {
	command, err := ResolveCommand(cfg, task.Role, task.Path, workDir, stageResult.PromptPath)
	if err != nil {
		return DispatchResult{}, fmt.Errorf("resolve worker command: %w", err)
	}
	command = applyCodexReasoningFlag(command, stageResult.ReasoningEffort)

	input := DispatchInput{
		Command:        command,
		WorkDir:        workDir,
		TaskID:         task.ID,
		Stage:          stage,
		EnvVars:        stageResult.Env,
		Warn:           warn,
		WorkerStateDir: stageResult.WorkerStateDir,
	}

	return DispatchWorker(input)
}

// exitStatusPath returns the absolute path for the worker exit status file.
func exitStatusPath(workerStateDir string, taskID string, stage roles.Stage) (string, error) {
	if strings.TrimSpace(taskID) == "" {
		return "", errors.New("task id is required")
	}
	if !stage.Valid() {
		return "", fmt.Errorf("invalid stage %q", stage)
	}
	if strings.TrimSpace(workerStateDir) == "" {
		return "", errors.New("worker state dir is required")
	}
	return filepath.Join(workerStateDir, "exit.json"), nil
}

// ReadExitStatus reads the exit status file if present.
func ReadExitStatus(workerStateDir string, taskID string, stage roles.Stage) (ExitStatus, bool, error) {
	if strings.TrimSpace(workerStateDir) == "" {
		return ExitStatus{}, false, errors.New("worker state dir is required")
	}
	path, err := exitStatusPath(workerStateDir, taskID, stage)
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

// writeDispatchWrapper writes a wrapper script that captures exit status and persists the agent pid.
func writeDispatchWrapper(workerStateDir string, taskID string, stage roles.Stage, command []string, exitPath string, pidPaths []string) (string, error) {
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
	if len(pidPaths) == 0 {
		return "", errors.New("pid paths are required")
	}

	if strings.TrimSpace(workerStateDir) == "" {
		return "", errors.New("worker state dir is required")
	}
	wrapperPath := filepath.Join(workerStateDir, "dispatch.sh")
	commandLine := shellCommandLine(command)
	exitPathEscaped := shellEscapeArg(exitPath)
	pidWriteLines := buildPIDWriteLines(pidPaths)
	content := strings.Join([]string{
		"#!/bin/sh",
		"set +e",
		commandLine + " &",
		"pid=$!",
		pidWriteLines,
		"wait $pid",
		"code=$?",
		"finished_at=$(date -u +\"%Y-%m-%dT%H:%M:%SZ\")",
		"printf '{\"exit_code\":%d,\"finished_at\":\"%s\",\"pid\":%d}\\n' \"$code\" \"$finished_at\" \"$pid\" > " + exitPathEscaped,
		"exit $code",
		"",
	}, "\n")
	if err := os.WriteFile(wrapperPath, []byte(content), wrapperFileMode); err != nil {
		return "", fmt.Errorf("write dispatch wrapper %s: %w", wrapperPath, err)
	}
	return wrapperPath, nil
}

// detectAgentName derives a friendly agent label from the command executable when possible.
func detectAgentName(command []string) string {
	if len(command) == 0 {
		return ""
	}
	name := strings.ToLower(strings.TrimSpace(filepath.Base(command[0])))
	switch {
	case strings.Contains(name, "codex"):
		return "codex"
	case strings.Contains(name, "claude"):
		return "claude"
	case strings.Contains(name, "gemini"):
		return "gemini"
	default:
		return ""
	}
}

// agentPIDPaths returns pid files written by the dispatch wrapper.
func agentPIDPaths(workerStateDir string, agentName string) []string {
	return []string{filepath.Join(workerStateDir, agentPIDFileName)}
}

// buildPIDWriteLines emits shell lines that persist the launched agent pid for observability.
func buildPIDWriteLines(pidPaths []string) string {
	lines := make([]string, 0, len(pidPaths))
	for _, path := range pidPaths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		lines = append(lines, "printf '%s\\n' \"$pid\" > "+shellEscapeArg(path))
	}
	return strings.Join(lines, "\n")
}

// writeDispatchMetadata persists dispatch-time metadata without failing the dispatch on error.
func writeDispatchMetadata(workerStateDir string, meta dispatchMetadata, warn func(string)) {
	path := filepath.Join(workerStateDir, "dispatch.json")
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		emitWarning(warn, fmt.Sprintf("failed to encode dispatch metadata: %v", err))
		return
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		emitWarning(warn, fmt.Sprintf("failed to write dispatch metadata: %v", err))
	}
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
