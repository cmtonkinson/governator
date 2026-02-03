// Package audit provides append-only audit logging for Governator v2 runs.
package audit

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// localStateDirName is the relative path for transient governator state.
	localStateDirName = "_governator/_local-state"
	// auditLogFileName is the filename used for audit logging.
	auditLogFileName = "audit.log"
	// auditLogFileMode defines the permissions for the audit log file.
	auditLogFileMode = 0o644
	// auditLogDirMode defines the permissions for the audit log directory.
	auditLogDirMode = 0o755
)

const (
	// EventWorktreeCreate records worktree creation.
	EventWorktreeCreate = "worktree.create"
	// EventWorktreeDelete records worktree deletion.
	EventWorktreeDelete = "worktree.delete"
	// EventBranchCreate records branch creation.
	EventBranchCreate = "branch.create"
	// EventBranchDelete records branch deletion.
	EventBranchDelete = "branch.delete"
	// EventTaskTransition records task lifecycle transitions.
	EventTaskTransition = "task.transition"
	// EventAgentInvoke records agent invocation.
	EventAgentInvoke = "agent.invoke"
	// EventAgentOutcome records agent completion.
	EventAgentOutcome = "agent.outcome"
	// EventWorkerTimeout records worker process timeout.
	EventWorkerTimeout = "worker.timeout"
)

// Logger appends audit entries to a log file.
type Logger struct {
	path     string
	warnings io.Writer
	now      func() time.Time
	mu       sync.Mutex
}

// Field represents a logfmt key/value pair.
type Field struct {
	Key   string
	Value string
}

// Entry captures the required audit log fields and any optional fields.
type Entry struct {
	TaskID string
	Role   string
	Event  string
	Fields []Field
}

// NewLogger builds an audit logger rooted at the provided repo.
func NewLogger(repoRoot string, warnings io.Writer) (*Logger, error) {
	if repoRoot == "" {
		return nil, errors.New("repo root is required")
	}
	if warnings == nil {
		warnings = os.Stderr
	}
	return &Logger{
		path:     filepath.Join(repoRoot, localStateDirName, auditLogFileName),
		warnings: warnings,
		now:      time.Now,
	}, nil
}

// Log writes a generic audit entry to the log file.
func (logger *Logger) Log(entry Entry) error {
	if logger == nil {
		return errors.New("audit logger is nil")
	}
	logger.mu.Lock()
	defer logger.mu.Unlock()

	line, err := logger.formatEntry(entry)
	if err != nil {
		logger.warnf("audit log entry rejected: %v", err)
		return err
	}

	exists, err := fileExists(logger.path)
	if err != nil {
		logger.warnf("audit log check failed for %s: %v", logger.path, err)
		return err
	}
	if !exists {
		logger.warnf("audit log missing at %s; creating new file", logger.path)
	}

	if err := logger.appendLine(line); err != nil {
		logger.warnf("audit log write failed for %s: %v", logger.path, err)
		return err
	}
	return nil
}

// LogWorktreeCreate records a worktree creation event.
func (logger *Logger) LogWorktreeCreate(taskID string, role string, path string, branch string) error {
	return logger.Log(Entry{
		TaskID: taskID,
		Role:   role,
		Event:  EventWorktreeCreate,
		Fields: []Field{
			{Key: "path", Value: path},
			{Key: "branch", Value: branch},
		},
	})
}

// LogWorktreeDelete records a worktree deletion event.
func (logger *Logger) LogWorktreeDelete(taskID string, role string, path string, branch string) error {
	return logger.Log(Entry{
		TaskID: taskID,
		Role:   role,
		Event:  EventWorktreeDelete,
		Fields: []Field{
			{Key: "path", Value: path},
			{Key: "branch", Value: branch},
		},
	})
}

// LogBranchCreate records a branch creation event.
func (logger *Logger) LogBranchCreate(taskID string, role string, branch string, base string) error {
	return logger.Log(Entry{
		TaskID: taskID,
		Role:   role,
		Event:  EventBranchCreate,
		Fields: []Field{
			{Key: "branch", Value: branch},
			{Key: "base", Value: base},
		},
	})
}

// LogBranchDelete records a branch deletion event.
func (logger *Logger) LogBranchDelete(taskID string, role string, branch string) error {
	return logger.Log(Entry{
		TaskID: taskID,
		Role:   role,
		Event:  EventBranchDelete,
		Fields: []Field{
			{Key: "branch", Value: branch},
		},
	})
}

// LogTaskTransition records a task lifecycle state transition.
func (logger *Logger) LogTaskTransition(taskID string, role string, from string, to string) error {
	if from == "" || to == "" {
		return fmt.Errorf("task transition requires from and to states")
	}
	return logger.Log(Entry{
		TaskID: taskID,
		Role:   role,
		Event:  EventTaskTransition,
		Fields: []Field{
			{Key: "from", Value: from},
			{Key: "to", Value: to},
		},
	})
}

// LogAgentInvoke records an agent invocation event.
func (logger *Logger) LogAgentInvoke(taskID string, role string, agent string, attempt int) error {
	return logger.Log(Entry{
		TaskID: taskID,
		Role:   role,
		Event:  EventAgentInvoke,
		Fields: []Field{
			{Key: "agent", Value: agent},
			{Key: "attempt", Value: strconv.Itoa(attempt)},
		},
	})
}

// LogAgentOutcome records an agent outcome event.
func (logger *Logger) LogAgentOutcome(taskID string, role string, agent string, status string, exitCode int) error {
	return logger.Log(Entry{
		TaskID: taskID,
		Role:   role,
		Event:  EventAgentOutcome,
		Fields: []Field{
			{Key: "agent", Value: agent},
			{Key: "status", Value: status},
			{Key: "exit_code", Value: strconv.Itoa(exitCode)},
		},
	})
}

// LogWorkerTimeout records a worker timeout event.
func (logger *Logger) LogWorkerTimeout(taskID string, role string, timeoutSecs int, worktreePath string) error {
	return logger.Log(Entry{
		TaskID: taskID,
		Role:   role,
		Event:  EventWorkerTimeout,
		Fields: []Field{
			{Key: "timeout_seconds", Value: strconv.Itoa(timeoutSecs)},
			{Key: "worktree_path", Value: worktreePath},
		},
	})
}

// formatEntry renders an audit entry in logfmt-style order.
func (logger *Logger) formatEntry(entry Entry) (string, error) {
	if entry.TaskID == "" {
		return "", errors.New("task id is required")
	}
	if entry.Role == "" {
		return "", errors.New("role is required")
	}
	if entry.Event == "" {
		return "", errors.New("event is required")
	}
	now := logger.now
	if now == nil {
		now = time.Now
	}

	ts := now().UTC().Format(time.RFC3339)
	fields := []string{
		formatField("ts", ts),
		formatField("task_id", entry.TaskID),
		formatField("role", entry.Role),
		formatField("event", entry.Event),
	}

	for _, field := range entry.Fields {
		if field.Value == "" {
			continue
		}
		if field.Key == "" {
			return "", errors.New("field key is required")
		}
		fields = append(fields, formatField(field.Key, field.Value))
	}
	return strings.Join(fields, " "), nil
}

// formatField encodes a logfmt key/value pair.
func formatField(key string, value string) string {
	encoded := sanitizeValue(value)
	if needsQuoting(encoded) {
		return fmt.Sprintf(`%s="%s"`, key, escapeLogfmt(encoded))
	}
	return fmt.Sprintf("%s=%s", key, encoded)
}

// sanitizeValue ensures values stay single-line.
func sanitizeValue(value string) string {
	value = strings.ReplaceAll(value, "\n", `\n`)
	return strings.ReplaceAll(value, "\r", `\r`)
}

// needsQuoting reports whether the value needs logfmt quoting.
func needsQuoting(value string) bool {
	if value == "" {
		return true
	}
	for _, r := range value {
		if r == ' ' || r == '\t' || r == '\n' || r == '=' || r == '"' {
			return true
		}
	}
	return false
}

// escapeLogfmt escapes characters that must be quoted in logfmt values.
func escapeLogfmt(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

// appendLine writes the log entry to the audit log file.
func (logger *Logger) appendLine(line string) error {
	if logger.path == "" {
		return errors.New("audit log path is required")
	}
	if err := os.MkdirAll(filepath.Dir(logger.path), auditLogDirMode); err != nil {
		return fmt.Errorf("create audit log directory %s: %w", filepath.Dir(logger.path), err)
	}
	file, err := os.OpenFile(logger.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, auditLogFileMode)
	if err != nil {
		return fmt.Errorf("open audit log %s: %w", logger.path, err)
	}
	if _, err := file.WriteString(line + "\n"); err != nil {
		_ = file.Close()
		return fmt.Errorf("write audit log %s: %w", logger.path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close audit log %s: %w", logger.path, err)
	}
	return nil
}

// warnf writes a warning message to the configured warnings writer.
func (logger *Logger) warnf(format string, args ...any) {
	if logger == nil || logger.warnings == nil {
		return
	}
	_, _ = fmt.Fprintf(logger.warnings, format+"\n", args...)
}

// fileExists reports whether the file exists at the path.
func fileExists(path string) (bool, error) {
	if path == "" {
		return false, errors.New("path is required")
	}
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
