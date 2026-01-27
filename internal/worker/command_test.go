// Package worker tests command resolution behavior.
package worker

import (
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
)

// TestResolveCommandRoleOverride verifies role-specific commands resolve with substitutions.
func TestResolveCommandRoleOverride(t *testing.T) {
	cfg := config.Config{
		Workers: config.WorkersConfig{
			Commands: config.WorkerCommands{
				Default: []string{"default", "{task_path}"},
				Roles: map[string][]string{
					"review": {"runner", "--task", "{task_path}", "--repo", "{repo_root}", "--role", "{role}"},
				},
			},
		},
	}

	got, err := ResolveCommand(cfg, index.Role("review"), "/tmp/task.md", "/repo", "/tmp/prompt.md")
	if err != nil {
		t.Fatalf("ResolveCommand returned error: %v", err)
	}

	want := []string{"runner", "--task", "/tmp/task.md", "--repo", "/repo", "--role", "review"}
	if !stringSlicesEqual(got, want) {
		t.Fatalf("ResolveCommand = %v, want %v", got, want)
	}
}

// TestResolveCommandFallbackToDefault verifies fallback behavior when role command is missing.
func TestResolveCommandFallbackToDefault(t *testing.T) {
	cfg := config.Config{
		Workers: config.WorkersConfig{
			Commands: config.WorkerCommands{
				Default: []string{"runner", "--role", "{role}", "{task_path}", "--repo", "{repo_root}"},
			},
		},
	}

	got, err := ResolveCommand(cfg, index.Role("worker"), "/task.md", "/repo", "/tmp/prompt.md")
	if err != nil {
		t.Fatalf("ResolveCommand returned error: %v", err)
	}

	want := []string{"runner", "--role", "worker", "/task.md", "--repo", "/repo"}
	if !stringSlicesEqual(got, want) {
		t.Fatalf("ResolveCommand = %v, want %v", got, want)
	}
}

// TestResolveCommandMissingDefault verifies missing defaults yield clear errors.
func TestResolveCommandMissingDefault(t *testing.T) {
	cfg := config.Config{
		Workers: config.WorkersConfig{
			Commands: config.WorkerCommands{
				Default: nil,
				Roles:   map[string][]string{},
			},
		},
	}

	_, err := ResolveCommand(cfg, index.Role("worker"), "/task.md", "/repo", "/tmp/prompt.md")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	message := err.Error()
	if !strings.Contains(message, "role") || !strings.Contains(message, "default") {
		t.Fatalf("error message %q should mention role and default", message)
	}
}

// TestResolveCommandMissingTaskPathToken verifies missing task_path tokens error.
func TestResolveCommandMissingTaskPathToken(t *testing.T) {
	cfg := config.Config{
		Workers: config.WorkersConfig{
			Commands: config.WorkerCommands{
				Default: []string{"runner", "--repo", "{repo_root}"},
			},
		},
	}

	_, err := ResolveCommand(cfg, index.Role("worker"), "/task.md", "/repo", "/tmp/prompt.md")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "{task_path}") {
		t.Fatalf("error message %q should mention {task_path}", err.Error())
	}
}

// stringSlicesEqual compares string slices for exact match.
func stringSlicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i, value := range left {
		if value != right[i] {
			return false
		}
	}
	return true
}

// TestIsCodexCommand ensures codex detection works against templates.
func TestIsCodexCommand(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		Workers: config.WorkersConfig{
			Commands: config.WorkerCommands{
				Default: []string{"codex", "exec", "{prompt_path}"},
			},
		},
	}
	got, err := IsCodexCommand(cfg, index.Role("worker"))
	if err != nil {
		t.Fatalf("IsCodexCommand returned error: %v", err)
	}
	if !got {
		t.Fatal("expected codex to be detected")
	}

	cfg.Workers.Commands.Default = []string{"python", "runner.py", "{prompt_path}"}
	got, err = IsCodexCommand(cfg, index.Role("worker"))
	if err != nil {
		t.Fatalf("IsCodexCommand returned error: %v", err)
	}
	if got {
		t.Fatal("expected non-codex to be ignored")
	}
}

// TestApplyCodexReasoningFlag verifies the reasoning config flag is added only for codex/high|low.
func TestApplyCodexReasoningFlag(t *testing.T) {
	t.Parallel()
	command := []string{"codex", "exec", "--sandbox=danger-full-access", "{prompt_path}"}
	got := applyCodexReasoningFlag(command, "high")
	want := []string{"codex", "--config", "model_reasoning_effort=\"high\"", "exec", "--sandbox=danger-full-access", "{prompt_path}"}
	if !stringSlicesEqual(got, want) {
		t.Fatalf("applyCodexReasoningFlag high = %v, want %v", got, want)
	}

	got = applyCodexReasoningFlag(command, "medium")
	if !stringSlicesEqual(got, command) {
		t.Fatalf("applyCodexReasoningFlag medium = %v, want original", got)
	}

	got = applyCodexReasoningFlag([]string{"python", "run"}, "high")
	if !stringSlicesEqual(got, []string{"python", "run"}) {
		t.Fatalf("applyCodexReasoningFlag non-codex = %v, want original", got)
	}
}
