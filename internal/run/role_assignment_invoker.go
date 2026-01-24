package run

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
	"github.com/cmtonkinson/governator/internal/roles"
	"github.com/cmtonkinson/governator/internal/worker"
)

const (
	roleAssignmentStateDir   = "_governator/_local-state/role-assignment"
	roleAssignmentPromptMode = 0o644
)

// workerCommandInvoker executes the configured worker command against the role assignment prompt.
type workerCommandInvoker struct {
	cfg      config.Config
	repoRoot string
	timeout  int
	warn     func(string)
}

// newWorkerCommandInvoker constructs an invoker that reuses the worker command template.
func newWorkerCommandInvoker(cfg config.Config, repoRoot string, warn func(string)) (roles.LLMInvoker, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return nil, errors.New("repo root is required")
	}
	timeout := cfg.Timeouts.WorkerSeconds
	if timeout <= 0 {
		timeout = config.Defaults().Timeouts.WorkerSeconds
	}
	return &workerCommandInvoker{
		cfg:      cfg,
		repoRoot: repoRoot,
		timeout:  timeout,
		warn:     warn,
	}, nil
}

// Invoke runs the worker command with the role assignment prompt.
func (inv *workerCommandInvoker) Invoke(ctx context.Context, prompt string) (string, error) {
	trimmed := strings.TrimSpace(prompt)
	if trimmed == "" {
		return "", errors.New("role assignment prompt is required")
	}

	promptPath, err := inv.writePrompt(trimmed)
	if err != nil {
		return "", err
	}

	relativePath, err := repoRelativePath(inv.repoRoot, promptPath)
	if err != nil {
		relativePath = filepath.ToSlash(promptPath)
	}

	command, err := worker.ResolveCommand(inv.cfg, "", relativePath, inv.repoRoot)
	if err != nil {
		return "", fmt.Errorf("resolve role assignment command: %w", err)
	}

	execCtx := ctx
	cancel := func() {}
	if inv.timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, time.Duration(inv.timeout)*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(execCtx, command[0], command[1:]...)
	cmd.Dir = inv.repoRoot

	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	stderrText := strings.TrimSpace(stderr.String())
	if stderrText != "" {
		inv.warnf("role assignment command stderr: %s", stderrText)
	}
	if err != nil {
		return "", fmt.Errorf("role assignment command failed: %w", err)
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return "", fmt.Errorf("role assignment command returned empty output")
	}
	return output, nil
}

func (inv *workerCommandInvoker) writePrompt(content string) (string, error) {
	dir := filepath.Join(inv.repoRoot, roleAssignmentStateDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create role assignment state dir %s: %w", dir, err)
	}
	filename := fmt.Sprintf("prompt-%d.md", time.Now().UnixNano())
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content+"\n"), roleAssignmentPromptMode); err != nil {
		return "", fmt.Errorf("write role assignment prompt %s: %w", path, err)
	}
	return path, nil
}

func repoRelativePath(root string, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func (inv *workerCommandInvoker) warnf(format string, args ...interface{}) {
	if inv.warn == nil {
		return
	}
	inv.warn(fmt.Sprintf(format, args...))
}
