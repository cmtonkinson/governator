// Package worker provides worker command resolution helpers.
package worker

import (
	"errors"
	"log"
	"strings"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
)

// ResolveCommand resolves a worker command template for the given role and fills tokens.
func ResolveCommand(cfg config.Config, role index.Role, taskPath string, repoRoot string, promptPath string) ([]string, error) {
	if strings.TrimSpace(taskPath) == "" {
		return nil, errors.New("task path is required")
	}
	if strings.TrimSpace(repoRoot) == "" {
		return nil, errors.New("repo root is required")
	}
	template, err := selectCommandTemplate(cfg, role)
	if err != nil {
		return nil, err
	}
	resolved, err := applyTemplate(template, taskPath, repoRoot, role, promptPath)
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

// selectCommandTemplate chooses the worker command template.
// Role-based CLI selection is deprecated; only default CLI is used.
func selectCommandTemplate(cfg config.Config, role index.Role) ([]string, error) {
	// Emit deprecation warnings if role-based config is present
	warnDeprecatedRoleConfig(cfg)

	// Priority 1: Default command override
	if len(cfg.Workers.Commands.Default) > 0 {
		return cloneStrings(cfg.Workers.Commands.Default), nil
	}

	// Priority 2: Default CLI selection
	if cfg.Workers.CLI.Default != "" {
		if template, ok := config.BuiltInCommand(cfg.Workers.CLI.Default); ok {
			return cloneStrings(template), nil
		}
	}

	// No command found
	return nil, errors.New("worker command is required (set workers.cli.default or workers.commands.default)")
}

// warnDeprecatedRoleConfig logs a deprecation warning if role-based CLI/command config is present.
func warnDeprecatedRoleConfig(cfg config.Config) {
	if len(cfg.Workers.Commands.Roles) > 0 {
		log.Printf("WARNING: workers.commands.roles is deprecated and will be ignored. Use workers.commands.default instead.")
	}
	if len(cfg.Workers.CLI.Roles) > 0 {
		log.Printf("WARNING: workers.cli.roles is deprecated and will be ignored. Use workers.cli.default instead.")
	}
}

// applyTemplate substitutes supported tokens in the command template.
func applyTemplate(template []string, taskPath string, repoRoot string, role index.Role, promptPath string) ([]string, error) {
	updated := make([]string, len(template))
	replaced := 0
	replacedPrompt := false
	roleValue := string(role)
	for i, token := range template {
		if strings.Contains(token, "{task_path}") {
			replaced++
		}
		if strings.Contains(token, "{prompt_path}") {
			replacedPrompt = true
		}
		token = strings.ReplaceAll(token, "{task_path}", taskPath)
		if promptPath != "" {
			token = strings.ReplaceAll(token, "{prompt_path}", promptPath)
		}
		token = strings.ReplaceAll(token, "{repo_root}", repoRoot)
		token = strings.ReplaceAll(token, "{role}", roleValue)
		updated[i] = token
	}
	if replaced == 0 && !replacedPrompt {
		return nil, errors.New("worker command must include {task_path} or {prompt_path}")
	}
	if replacedPrompt && strings.TrimSpace(promptPath) == "" {
		return nil, errors.New("worker command uses {prompt_path} but prompt data is missing")
	}
	return updated, nil
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
