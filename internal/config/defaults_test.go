// Package config tests default configuration behavior.
package config

import (
	"strings"
	"testing"
)

// TestDefaultsDocumentedValues verifies the published defaults are stable.
func TestDefaultsDocumentedValues(t *testing.T) {
	t.Parallel()

	cfg := Defaults()

	if got, want := cfg.Concurrency.Global, defaultConcurrencyGlobal; got != want {
		t.Fatalf("concurrency.global = %d, want %d", got, want)
	}
	if got, want := cfg.Concurrency.DefaultRole, defaultConcurrencyDefaultRole; got != want {
		t.Fatalf("concurrency.default_role = %d, want %d", got, want)
	}
	if got, want := cfg.Timeouts.WorkerSeconds, defaultWorkerTimeoutSeconds; got != want {
		t.Fatalf("timeouts.worker_seconds = %d, want %d", got, want)
	}
	if got, want := cfg.Retries.MaxAttempts, defaultRetriesMaxAttempts; got != want {
		t.Fatalf("retries.max_attempts = %d, want %d", got, want)
	}
	if got, want := cfg.Workers.CLI.Default, defaultWorkerCLI; got != want {
		t.Fatalf("workers.cli.default = %q, want %q", got, want)
	}
	if cfg.Workers.CLI.Roles == nil || len(cfg.Workers.CLI.Roles) != 0 {
		t.Fatal("workers.cli.roles should default to empty map")
	}
	if cfg.Workers.Commands.Default != nil {
		t.Fatal("workers.commands.default should be nil (uses CLI built-in)")
	}
	if cfg.Workers.Commands.Roles == nil || len(cfg.Workers.Commands.Roles) != 0 {
		t.Fatal("workers.commands.roles should default to empty map")
	}
	if cfg.Branches.Base != defaultBranchBase {
		t.Fatalf("branches.base = %q, want %q", cfg.Branches.Base, defaultBranchBase)
	}
}

// TestApplyDefaultsMissingConfig verifies defaults apply to an empty config.
func TestApplyDefaultsMissingConfig(t *testing.T) {
	t.Parallel()

	cfg := ApplyDefaults(Config{}, nil)
	expected := Defaults()

	if !configsEqual(cfg, expected) {
		t.Fatal("ApplyDefaults should match Defaults for empty config")
	}
}

// TestApplyDefaultsInvalidValues verifies invalid values fall back to defaults with warnings.
func TestApplyDefaultsInvalidValues(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Workers: WorkersConfig{
			CLI: WorkerCLI{
				Default: "invalid-cli",
				Roles: map[string]string{
					"planner": "also-invalid",
					"worker":  "claude",
				},
			},
			Commands: WorkerCommands{
				Default: []string{"echo", "no-task-path"},
				Roles: map[string][]string{
					"architect": {"echo", "{repo_root}"},
					"devops":    {"bash", "-lc", "cat {task_path}"},
				},
			},
		},
		Concurrency: ConcurrencyConfig{
			Global:      -1,
			DefaultRole: 0,
			Roles: map[string]int{
				"planner":  -2,
				"executor": 2,
			},
		},
		Timeouts: TimeoutsConfig{
			WorkerSeconds: 0,
		},
		Retries: RetriesConfig{
			MaxAttempts: -1,
		},
		Branches: BranchConfig{
			Base: "",
		},
	}

	var warnings []string
	warn := func(message string) {
		warnings = append(warnings, message)
	}

	normalized := ApplyDefaults(cfg, warn)

	// Invalid CLI should fall back to default
	if normalized.Workers.CLI.Default != defaultWorkerCLI {
		t.Fatal("workers.cli.default should fall back to default")
	}
	// Invalid role CLI should be removed
	if _, ok := normalized.Workers.CLI.Roles["planner"]; ok {
		t.Fatal("invalid workers.cli.roles.planner should be removed")
	}
	// Valid role CLI should be preserved
	if cli, ok := normalized.Workers.CLI.Roles["worker"]; !ok || cli != "claude" {
		t.Fatal("valid workers.cli.roles.worker should be preserved")
	}
	// Invalid command override should be cleared
	if normalized.Workers.Commands.Default != nil {
		t.Fatal("invalid workers.commands.default should be cleared")
	}
	// Invalid role command should be removed
	if _, ok := normalized.Workers.Commands.Roles["architect"]; ok {
		t.Fatal("invalid workers.commands.roles.architect should be removed")
	}
	// Valid role command should be preserved
	if _, ok := normalized.Workers.Commands.Roles["devops"]; !ok {
		t.Fatal("valid workers.commands.roles.devops should be preserved")
	}
	if normalized.Concurrency.Global != defaultConcurrencyGlobal {
		t.Fatal("concurrency.global should fall back to default")
	}
	if normalized.Concurrency.DefaultRole != defaultConcurrencyDefaultRole {
		t.Fatal("concurrency.default_role should fall back to default")
	}
	if _, ok := normalized.Concurrency.Roles["planner"]; ok {
		t.Fatal("invalid concurrency.roles.planner should be removed")
	}
	if _, ok := normalized.Concurrency.Roles["executor"]; !ok {
		t.Fatal("valid concurrency.roles.executor should be preserved")
	}
	if normalized.Timeouts.WorkerSeconds != defaultWorkerTimeoutSeconds {
		t.Fatal("timeouts.worker_seconds should fall back to default")
	}
	if normalized.Retries.MaxAttempts != defaultRetriesMaxAttempts {
		t.Fatal("retries.max_attempts should fall back to default")
	}
	if len(warnings) == 0 {
		t.Fatal("expected warnings for invalid values")
	}
	if !warningsContain(warnings, "workers.cli.default") {
		t.Fatal("expected warning for workers.cli.default")
	}
	if !warningsContain(warnings, "workers.cli.roles.planner") {
		t.Fatal("expected warning for workers.cli.roles.planner")
	}
	if !warningsContain(warnings, "workers.commands.default") {
		t.Fatal("expected warning for workers.commands.default")
	}
	if !warningsContain(warnings, "workers.commands.roles.architect") {
		t.Fatal("expected warning for workers.commands.roles.architect")
	}
	if !warningsContain(warnings, "concurrency.global") {
		t.Fatal("expected warning for concurrency.global")
	}
	if !warningsContain(warnings, "branches.base") {
		t.Fatal("expected warning for branches.base")
	}
}

// configsEqual compares configs by value without relying on reflect.DeepEqual.
func configsEqual(left Config, right Config) bool {
	if left.Concurrency.Global != right.Concurrency.Global ||
		left.Concurrency.DefaultRole != right.Concurrency.DefaultRole ||
		left.Timeouts.WorkerSeconds != right.Timeouts.WorkerSeconds ||
		left.Retries.MaxAttempts != right.Retries.MaxAttempts {
		return false
	}
	if left.Branches.Base != right.Branches.Base {
		return false
	}

	// Compare CLI settings
	if left.Workers.CLI.Default != right.Workers.CLI.Default {
		return false
	}
	if len(left.Workers.CLI.Roles) != len(right.Workers.CLI.Roles) {
		return false
	}
	for role, cli := range left.Workers.CLI.Roles {
		other, ok := right.Workers.CLI.Roles[role]
		if !ok || cli != other {
			return false
		}
	}

	// Compare command overrides
	if !stringSlicesEqual(left.Workers.Commands.Default, right.Workers.Commands.Default) {
		return false
	}
	if len(left.Workers.Commands.Roles) != len(right.Workers.Commands.Roles) {
		return false
	}
	for role, command := range left.Workers.Commands.Roles {
		other, ok := right.Workers.Commands.Roles[role]
		if !ok || !stringSlicesEqual(command, other) {
			return false
		}
	}
	if len(left.Concurrency.Roles) != len(right.Concurrency.Roles) {
		return false
	}
	for role, cap := range left.Concurrency.Roles {
		other, ok := right.Concurrency.Roles[role]
		if !ok || cap != other {
			return false
		}
	}
	return true
}

// stringSlicesEqual compares string slices in order.
func stringSlicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

// warningsContain reports whether any warning contains the substring.
func warningsContain(warnings []string, substr string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, substr) {
			return true
		}
	}
	return false
}

func containsTaskOrPromptToken(command []string) bool {
	for _, token := range command {
		if strings.Contains(token, "{task_path}") || strings.Contains(token, "{prompt_path}") {
			return true
		}
	}
	return false
}

// TestBuiltInCommand verifies all built-in CLI commands are defined correctly.
func TestBuiltInCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cli       string
		wantValid bool
		wantLen   int
	}{
		{
			name:      "codex",
			cli:       "codex",
			wantValid: true,
			wantLen:   5, // ["codex", "exec", "--ask-for-approval=never", "--sandbox=workspace-write", "{prompt_path}"]
		},
		{
			name:      "claude",
			cli:       "claude",
			wantValid: true,
			wantLen:   3, // ["claude", "--print", "{prompt_path}"]
		},
		{
			name:      "gemini",
			cli:       "gemini",
			wantValid: true,
			wantLen:   2, // ["gemini", "{prompt_path}"]
		},
		{
			name:      "invalid",
			cli:       "invalid",
			wantValid: false,
			wantLen:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, ok := BuiltInCommand(tt.cli)
			if ok != tt.wantValid {
				t.Fatalf("BuiltInCommand(%q) validity = %v, want %v", tt.cli, ok, tt.wantValid)
			}
			if ok {
				if len(cmd) != tt.wantLen {
					t.Fatalf("BuiltInCommand(%q) length = %d, want %d", tt.cli, len(cmd), tt.wantLen)
				}
				if !containsTaskOrPromptToken(cmd) {
					t.Fatalf("BuiltInCommand(%q) should contain {task_path} or {prompt_path}", tt.cli)
				}
			}
		})
	}
}

// TestIsValidCLI verifies CLI name validation.
func TestIsValidCLI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cli  string
		want bool
	}{
		{name: "codex", cli: "codex", want: true},
		{name: "claude", cli: "claude", want: true},
		{name: "gemini", cli: "gemini", want: true},
		{name: "invalid", cli: "invalid", want: false},
		{name: "empty", cli: "", want: false},
		{name: "spaces", cli: "  ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidCLI(tt.cli)
			if got != tt.want {
				t.Fatalf("IsValidCLI(%q) = %v, want %v", tt.cli, got, tt.want)
			}
		})
	}
}

// TestApplyDefaultsWithCLI verifies CLI-based configuration works correctly.
func TestApplyDefaultsWithCLI(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Workers: WorkersConfig{
			CLI: WorkerCLI{
				Default: "claude",
				Roles: map[string]string{
					"architect": "gemini",
				},
			},
		},
	}

	normalized := ApplyDefaults(cfg, nil)

	if normalized.Workers.CLI.Default != "claude" {
		t.Fatalf("workers.cli.default = %q, want %q", normalized.Workers.CLI.Default, "claude")
	}
	if cli, ok := normalized.Workers.CLI.Roles["architect"]; !ok || cli != "gemini" {
		t.Fatalf("workers.cli.roles.architect = %q, want %q", cli, "gemini")
	}
}
