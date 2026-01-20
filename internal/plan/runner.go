// Package plan provides the explicit planning command implementation.
package plan

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cmtonkinson/governator/internal/bootstrap"
	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/planner"
)

const (
	localStateDirName   = "_governator/_local_state"
	plannerStateDirName = "planner"
	plannerPromptName   = "plan-request.md"
	localStateDirMode   = 0o755
	plannerPromptMode   = 0o644
)

// Options configures the plan command behavior.
type Options struct {
	RepoState       planner.RepoState
	ConfigOverrides map[string]any
	PlannerCommand  []string
	MaxAttempts     *int
	Stdout          io.Writer
	Stderr          io.Writer
	Warn            func(string)
}

// Result captures the plan command outputs.
type Result struct {
	BootstrapRan    bool
	BootstrapResult bootstrap.Result
	PlanResult      planner.PlanResult
	TaskCount       int
	PromptPath      string
}

// Run executes the planning pipeline, emitting tasks and a task index.
func Run(repoRoot string, options Options) (Result, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return Result{}, errors.New("repo root is required")
	}
	stdout := resolveWriter(options.Stdout)
	stderr := resolveWriter(options.Stderr)
	warn := options.Warn
	if warn == nil {
		warn = func(message string) {
			if message == "" {
				return
			}
			fmt.Fprintf(stderr, "warning: %s\n", message)
		}
	}

	cfg, err := config.Load(repoRoot, options.ConfigOverrides, warn)
	if err != nil {
		return Result{}, err
	}

	maxAttempts, err := resolveMaxAttempts(cfg, options.MaxAttempts)
	if err != nil {
		return Result{}, err
	}

	ranBootstrap, bootstrapResult, err := ensureBootstrap(repoRoot)
	if err != nil {
		return Result{}, err
	}
	if ranBootstrap {
		fmt.Fprintln(stdout, "bootstrap ok")
	}

	prompt, err := planner.AssemblePrompt(repoRoot, cfg, options.RepoState, warn)
	if err != nil {
		return Result{}, err
	}

	promptPath, err := writePrompt(repoRoot, prompt)
	if err != nil {
		return Result{}, err
	}

	command, err := resolvePlannerCommand(cfg, options.PlannerCommand)
	if err != nil {
		return Result{}, err
	}

	output, err := runPlannerCommand(command, promptPath, repoRoot)
	if err != nil {
		return Result{}, err
	}

	parsed, err := planner.ParsePlannerOutput(output)
	if err != nil {
		return Result{}, err
	}

	planResult, err := planner.EmitPlan(repoRoot, parsed, planner.PlanOptions{MaxAttempts: maxAttempts})
	if err != nil {
		return Result{}, err
	}

	taskCount := len(parsed.Tasks.Tasks)
	fmt.Fprintf(stdout, "plan ok tasks=%d\n", taskCount)

	return Result{
		BootstrapRan:    ranBootstrap,
		BootstrapResult: bootstrapResult,
		PlanResult:      planResult,
		TaskCount:       taskCount,
		PromptPath:      repoRelativePath(repoRoot, promptPath),
	}, nil
}

// ensureBootstrap creates required Power Six artifacts when they are missing.
func ensureBootstrap(repoRoot string) (bool, bootstrap.Result, error) {
	missing, err := requiredBootstrapMissing(repoRoot)
	if err != nil {
		return false, bootstrap.Result{}, err
	}
	if !missing {
		return false, bootstrap.Result{}, nil
	}
	result, err := bootstrap.Run(repoRoot, bootstrap.Options{Force: false})
	if err != nil {
		return true, result, fmt.Errorf("bootstrap failed: %w", err)
	}
	return true, result, nil
}

// requiredBootstrapMissing reports whether required Power Six artifacts are absent.
func requiredBootstrapMissing(repoRoot string) (bool, error) {
	docsDir := filepath.Join(repoRoot, "_governator", "docs")
	for _, artifact := range bootstrap.Artifacts() {
		if !artifact.Required {
			continue
		}
		path := filepath.Join(docsDir, artifact.Name)
		_, err := os.Stat(path)
		if err == nil {
			continue
		}
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, fmt.Errorf("stat bootstrap artifact %s: %w", path, err)
	}
	return false, nil
}

// resolvePlannerCommand selects the planner command template.
func resolvePlannerCommand(cfg config.Config, override []string) ([]string, error) {
	if len(override) > 0 {
		if !containsTaskPathToken(override) {
			return nil, errors.New("planner command missing {task_path} token")
		}
		return cloneStrings(override), nil
	}
	if roleCommand, ok := cfg.Workers.Commands.Roles["planner"]; ok && len(roleCommand) > 0 {
		return cloneStrings(roleCommand), nil
	}
	if len(cfg.Workers.Commands.Default) == 0 {
		return nil, errors.New("planner command is required")
	}
	if !containsTaskPathToken(cfg.Workers.Commands.Default) {
		return nil, errors.New("default worker command missing {task_path} token")
	}
	return cloneStrings(cfg.Workers.Commands.Default), nil
}

// runPlannerCommand executes the planner command with the prompt path.
func runPlannerCommand(command []string, promptPath string, repoRoot string) ([]byte, error) {
	if len(command) == 0 {
		return nil, errors.New("planner command is required")
	}
	if strings.TrimSpace(promptPath) == "" {
		return nil, errors.New("prompt path is required")
	}

	resolved, err := applyTaskPath(command, promptPath)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(resolved[0], resolved[1:]...)
	cmd.Dir = repoRoot

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return nil, fmt.Errorf("planner command failed: %w: %s", err, message)
		}
		return nil, fmt.Errorf("planner command failed: %w", err)
	}

	output := bytes.TrimSpace(stdout.Bytes())
	if len(output) == 0 {
		return nil, errors.New("planner command returned empty output")
	}
	return output, nil
}

// writePrompt writes the planner prompt to the local state directory.
func writePrompt(repoRoot string, prompt string) (string, error) {
	content := strings.TrimSpace(prompt)
	if content == "" {
		return "", errors.New("planner prompt is required")
	}
	dir := filepath.Join(repoRoot, localStateDirName, plannerStateDirName)
	if err := os.MkdirAll(dir, localStateDirMode); err != nil {
		return "", fmt.Errorf("create planner state dir %s: %w", dir, err)
	}
	path := filepath.Join(dir, plannerPromptName)
	data := append([]byte(content), '\n')
	if err := os.WriteFile(path, data, plannerPromptMode); err != nil {
		return "", fmt.Errorf("write planner prompt %s: %w", path, err)
	}
	return path, nil
}

// resolveMaxAttempts picks the max attempts setting.
func resolveMaxAttempts(cfg config.Config, override *int) (int, error) {
	if override == nil {
		return cfg.Retries.MaxAttempts, nil
	}
	if *override < 0 {
		return 0, errors.New("max attempts must be zero or positive")
	}
	return *override, nil
}

// applyTaskPath replaces the task path token with the prompt path.
func applyTaskPath(command []string, taskPath string) ([]string, error) {
	updated := make([]string, len(command))
	replaced := 0
	for i, token := range command {
		if strings.Contains(token, "{task_path}") {
			replaced++
		}
		updated[i] = strings.ReplaceAll(token, "{task_path}", taskPath)
	}
	if replaced == 0 {
		return nil, errors.New("planner command missing {task_path} token")
	}
	return updated, nil
}

// containsTaskPathToken reports whether the template includes {task_path}.
func containsTaskPathToken(command []string) bool {
	for _, token := range command {
		if strings.Contains(token, "{task_path}") {
			return true
		}
	}
	return false
}

// cloneStrings copies a string slice to avoid shared references.
func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	clone := make([]string, len(values))
	copy(clone, values)
	return clone
}

// resolveWriter returns io.Discard when the writer is nil.
func resolveWriter(writer io.Writer) io.Writer {
	if writer == nil {
		return io.Discard
	}
	return writer
}

// repoRelativePath returns a repository-relative path using forward slashes.
func repoRelativePath(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
