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
	if got, want := cfg.AutoRerun.CooldownSeconds, defaultAutoRerunCooldown; got != want {
		t.Fatalf("auto_rerun.cooldown_seconds = %d, want %d", got, want)
	}
	if cfg.AutoRerun.Enabled != defaultAutoRerunEnabled {
		t.Fatalf("auto_rerun.enabled = %v, want %v", cfg.AutoRerun.Enabled, defaultAutoRerunEnabled)
	}
	if len(cfg.Workers.Commands.Default) == 0 {
		t.Fatal("workers.commands.default should not be empty")
	}
	if !containsTaskOrPromptToken(cfg.Workers.Commands.Default) {
		t.Fatal("workers.commands.default should include {task_path} or {prompt_path}")
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
			Commands: WorkerCommands{
				Default: []string{"echo", "no-task-path"},
				Roles: map[string][]string{
					"planner": {"echo", "{repo_root}"},
					"worker":  {"bash", "-lc", "cat {task_path}"},
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
		AutoRerun: AutoRerunConfig{
			Enabled:         true,
			CooldownSeconds: -5,
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

	if !containsTaskOrPromptToken(normalized.Workers.Commands.Default) {
		t.Fatal("workers.commands.default should fall back to default command")
	}
	if _, ok := normalized.Workers.Commands.Roles["planner"]; ok {
		t.Fatal("invalid workers.commands.roles.planner should be removed")
	}
	if _, ok := normalized.Workers.Commands.Roles["worker"]; !ok {
		t.Fatal("valid workers.commands.roles.worker should be preserved")
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
	if normalized.AutoRerun.CooldownSeconds != defaultAutoRerunCooldown {
		t.Fatal("auto_rerun.cooldown_seconds should fall back to default")
	}
	if len(warnings) == 0 {
		t.Fatal("expected warnings for invalid values")
	}
	if !warningsContain(warnings, "workers.commands.default") {
		t.Fatal("expected warning for workers.commands.default")
	}
	if !warningsContain(warnings, "workers.commands.roles.planner") {
		t.Fatal("expected warning for workers.commands.roles.planner")
	}
	if !warningsContain(warnings, "concurrency.global") {
		t.Fatal("expected warning for concurrency.global")
	}
	if !warningsContain(warnings, "auto_rerun.cooldown_seconds") {
		t.Fatal("expected warning for auto_rerun.cooldown_seconds")
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
		left.Retries.MaxAttempts != right.Retries.MaxAttempts ||
		left.AutoRerun.Enabled != right.AutoRerun.Enabled ||
		left.AutoRerun.CooldownSeconds != right.AutoRerun.CooldownSeconds {
		return false
	}
	if left.Branches.Base != right.Branches.Base {
		return false
	}

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
