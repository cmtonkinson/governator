// Package worker provides worker command resolution helpers.
package worker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
)

// ResolveCommand resolves a worker command template for the given role and fills tokens.
func ResolveCommand(cfg config.Config, role index.Role, taskPath string, repoRoot string) ([]string, error) {
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
	resolved, err := applyTemplate(template, taskPath, repoRoot, role)
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

// selectCommandTemplate chooses the worker command template for the supplied role.
func selectCommandTemplate(cfg config.Config, role index.Role) ([]string, error) {
	if role != "" {
		if command, ok := cfg.Workers.Commands.Roles[string(role)]; ok && len(command) > 0 {
			return cloneStrings(command), nil
		}
	}
	if len(cfg.Workers.Commands.Default) == 0 {
		if role != "" {
			return nil, fmt.Errorf("worker command missing for role %q and no default command configured", role)
		}
		return nil, errors.New("worker default command is required")
	}
	return cloneStrings(cfg.Workers.Commands.Default), nil
}

// applyTemplate substitutes supported tokens in the command template.
func applyTemplate(template []string, taskPath string, repoRoot string, role index.Role) ([]string, error) {
	updated := make([]string, len(template))
	replaced := 0
	roleValue := string(role)
	for i, token := range template {
		if strings.Contains(token, "{task_path}") {
			replaced++
		}
		token = strings.ReplaceAll(token, "{task_path}", taskPath)
		token = strings.ReplaceAll(token, "{repo_root}", repoRoot)
		token = strings.ReplaceAll(token, "{role}", roleValue)
		updated[i] = token
	}
	if replaced == 0 {
		return nil, errors.New("worker command missing {task_path} token")
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
