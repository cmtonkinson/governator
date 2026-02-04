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
			CLI: config.WorkerCLI{
				Default: "codex",
				Roles:   map[string]string{},
			},
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
			CLI: config.WorkerCLI{
				Default: "",
				Roles:   map[string]string{},
			},
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
			CLI: config.WorkerCLI{
				Default: "",
				Roles:   map[string]string{},
			},
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
	if !strings.Contains(message, "worker command") {
		t.Fatalf("error message %q should mention worker command", message)
	}
}

// TestResolveCommandMissingTaskPathToken verifies missing task_path tokens error.
func TestResolveCommandMissingTaskPathToken(t *testing.T) {
	cfg := config.Config{
		Workers: config.WorkersConfig{
			CLI: config.WorkerCLI{
				Default: "",
				Roles:   map[string]string{},
			},
			Commands: config.WorkerCommands{
				Default: []string{"runner", "--repo", "{repo_root}"},
			},
		},
	}

	_, err := ResolveCommand(cfg, index.Role("worker"), "/task.md", "/repo", "/tmp/prompt.md")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "{task_path}") && !strings.Contains(err.Error(), "{prompt_path}") {
		t.Fatalf("error message %q should mention {task_path} or {prompt_path}", err.Error())
	}
}

// TestResolveCommandWithCLI verifies CLI-based command resolution.
func TestResolveCommandWithCLI(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.Config
		role    index.Role
		want    []string
	}{
		{
			name: "codex-default",
			cfg: config.Config{
				Workers: config.WorkersConfig{
					CLI: config.WorkerCLI{
						Default: "codex",
						Roles:   map[string]string{},
					},
					Commands: config.WorkerCommands{},
				},
			},
			role: "worker",
			want: []string{"codex", "exec", "--sandbox=workspace-write", "/tmp/prompt.md"},
		},
		{
			name: "claude-default",
			cfg: config.Config{
				Workers: config.WorkersConfig{
					CLI: config.WorkerCLI{
						Default: "claude",
						Roles:   map[string]string{},
					},
					Commands: config.WorkerCommands{},
				},
			},
			role: "worker",
			want: []string{"claude", "--print", "/tmp/prompt.md"},
		},
		{
			name: "gemini-default",
			cfg: config.Config{
				Workers: config.WorkersConfig{
					CLI: config.WorkerCLI{
						Default: "gemini",
						Roles:   map[string]string{},
					},
					Commands: config.WorkerCommands{},
				},
			},
			role: "worker",
			want: []string{"gemini", "/tmp/prompt.md"},
		},
		{
			name: "role-specific-cli",
			cfg: config.Config{
				Workers: config.WorkersConfig{
					CLI: config.WorkerCLI{
						Default: "codex",
						Roles: map[string]string{
							"architect": "claude",
						},
					},
					Commands: config.WorkerCommands{},
				},
			},
			role: "architect",
			want: []string{"claude", "--print", "/tmp/prompt.md"},
		},
		{
			name: "command-overrides-cli",
			cfg: config.Config{
				Workers: config.WorkersConfig{
					CLI: config.WorkerCLI{
						Default: "codex",
						Roles:   map[string]string{},
					},
					Commands: config.WorkerCommands{
						Default: []string{"custom", "{prompt_path}"},
					},
				},
			},
			role: "worker",
			want: []string{"custom", "/tmp/prompt.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveCommand(tt.cfg, tt.role, "/tmp/task.md", "/repo", "/tmp/prompt.md")
			if err != nil {
				t.Fatalf("ResolveCommand returned error: %v", err)
			}
			if !stringSlicesEqual(got, tt.want) {
				t.Fatalf("ResolveCommand = %v, want %v", got, tt.want)
			}
		})
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
			CLI: config.WorkerCLI{
				Default: "codex",
				Roles:   map[string]string{},
			},
			Commands: config.WorkerCommands{},
		},
	}
	got, err := IsCodexCommand(cfg, index.Role("worker"))
	if err != nil {
		t.Fatalf("IsCodexCommand returned error: %v", err)
	}
	if !got {
		t.Fatal("expected codex to be detected")
	}

	cfg.Workers.CLI.Default = "claude"
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
	command := []string{"codex", "exec", "--sandbox=workspace-write", "{prompt_path}"}
	got := applyCodexReasoningFlag(command, "high")
	want := []string{"codex", "--config", "model_reasoning_effort=\"high\"", "exec", "--sandbox=workspace-write", "{prompt_path}"}
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

// TestIsCodexCommandMultipleCLIs verifies claude and gemini are detected as non-codex.
func TestIsCodexCommandMultipleCLIs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		cli       string
		wantCodex bool
	}{
		{
			name:      "codex",
			cli:       "codex",
			wantCodex: true,
		},
		{
			name:      "claude",
			cli:       "claude",
			wantCodex: false,
		},
		{
			name:      "gemini",
			cli:       "gemini",
			wantCodex: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{
				Workers: config.WorkersConfig{
					CLI: config.WorkerCLI{
						Default: tt.cli,
						Roles:   map[string]string{},
					},
					Commands: config.WorkerCommands{},
				},
			}
			got, err := IsCodexCommand(cfg, index.Role("worker"))
			if err != nil {
				t.Fatalf("IsCodexCommand returned error: %v", err)
			}
			if got != tt.wantCodex {
				t.Fatalf("IsCodexCommand = %v, want %v", got, tt.wantCodex)
			}
		})
	}
}

// TestIsCodexCommandWithCommandOverride verifies command overrides are detected correctly.
func TestIsCodexCommandWithCommandOverride(t *testing.T) {
	t.Parallel()
	// Custom command override should be detected
	cfg := config.Config{
		Workers: config.WorkersConfig{
			CLI: config.WorkerCLI{
				Default: "claude",
				Roles:   map[string]string{},
			},
			Commands: config.WorkerCommands{
				Default: []string{"python", "runner.py", "{prompt_path}"},
			},
		},
	}
	got, err := IsCodexCommand(cfg, index.Role("worker"))
	if err != nil {
		t.Fatalf("IsCodexCommand returned error: %v", err)
	}
	if got {
		t.Fatal("expected non-codex command override")
	}
}

// TestApplyCodexReasoningFlagMultipleCLIs verifies reasoning flags are only added to codex.
func TestApplyCodexReasoningFlagMultipleCLIs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		command []string
		level   string
		want    []string
	}{
		{
			name:    "codex-high",
			command: []string{"codex", "exec", "--sandbox=workspace-write", "{prompt_path}"},
			level:   "high",
			want:    []string{"codex", "--config", "model_reasoning_effort=\"high\"", "exec", "--sandbox=workspace-write", "{prompt_path}"},
		},
		{
			name:    "codex-low",
			command: []string{"codex", "exec", "--sandbox=workspace-write", "{prompt_path}"},
			level:   "low",
			want:    []string{"codex", "--config", "model_reasoning_effort=\"low\"", "exec", "--sandbox=workspace-write", "{prompt_path}"},
		},
		{
			name:    "codex-medium",
			command: []string{"codex", "exec", "--sandbox=workspace-write", "{prompt_path}"},
			level:   "medium",
			want:    []string{"codex", "exec", "--sandbox=workspace-write", "{prompt_path}"},
		},
		{
			name:    "claude-high",
			command: []string{"claude", "--print", "{prompt_path}"},
			level:   "high",
			want:    []string{"claude", "--print", "{prompt_path}"},
		},
		{
			name:    "gemini-high",
			command: []string{"gemini", "{prompt_path}"},
			level:   "high",
			want:    []string{"gemini", "{prompt_path}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyCodexReasoningFlag(tt.command, tt.level)
			if !stringSlicesEqual(got, tt.want) {
				t.Fatalf("applyCodexReasoningFlag = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestShouldIncludeReasoningPrompt verifies reasoning prompt inclusion logic.
func TestShouldIncludeReasoningPrompt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		level          string
		agentUsesCodex bool
		want           bool
	}{
		{
			name:           "codex-high",
			level:          "high",
			agentUsesCodex: true,
			want:           false, // codex uses flags, not prompts
		},
		{
			name:           "codex-low",
			level:          "low",
			agentUsesCodex: true,
			want:           false, // codex uses flags, not prompts
		},
		{
			name:           "claude-high",
			level:          "high",
			agentUsesCodex: false,
			want:           true, // claude uses prompts
		},
		{
			name:           "claude-low",
			level:          "low",
			agentUsesCodex: false,
			want:           true, // claude uses prompts
		},
		{
			name:           "claude-medium",
			level:          "medium",
			agentUsesCodex: false,
			want:           false, // medium has no special reasoning prompt
		},
		{
			name:           "gemini-high",
			level:          "high",
			agentUsesCodex: false,
			want:           true, // gemini uses prompts
		},
		{
			name:           "empty-level",
			level:          "",
			agentUsesCodex: false,
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldIncludeReasoningPrompt(tt.level, tt.agentUsesCodex)
			if got != tt.want {
				t.Fatalf("shouldIncludeReasoningPrompt = %v, want %v", got, tt.want)
			}
		})
	}
}
