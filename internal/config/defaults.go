// Package config provides default configuration handling.
package config

import "strings"

const (
	defaultConcurrencyGlobal      = 1
	defaultConcurrencyDefaultRole = 1
	defaultWorkerTimeoutSeconds   = 900
	defaultRetriesMaxAttempts     = 2
	defaultAutoRerunEnabled       = false
	defaultAutoRerunCooldown      = 60
)

var defaultWorkerCommand = []string{
	"codex",
	"exec",
	"--sandbox=danger-full-access",
	"{task_path}",
}

// Defaults returns the documented configuration defaults.
//
// Defaults:
// - workers.commands.default: ["codex", "exec", "--sandbox=danger-full-access", "{task_path}"]
// - workers.commands.roles: {}
// - concurrency.global: 1
// - concurrency.default_role: 1
// - concurrency.roles: {}
// - timeouts.worker_seconds: 900
// - retries.max_attempts: 2
// - auto_rerun.enabled: false
// - auto_rerun.cooldown_seconds: 60
func Defaults() Config {
	return Config{
		Workers: WorkersConfig{
			Commands: WorkerCommands{
				Default: cloneStrings(defaultWorkerCommand),
				Roles:   map[string][]string{},
			},
		},
		Concurrency: ConcurrencyConfig{
			Global:      defaultConcurrencyGlobal,
			DefaultRole: defaultConcurrencyDefaultRole,
			Roles:       map[string]int{},
		},
		Timeouts: TimeoutsConfig{
			WorkerSeconds: defaultWorkerTimeoutSeconds,
		},
		Retries: RetriesConfig{
			MaxAttempts: defaultRetriesMaxAttempts,
		},
		AutoRerun: AutoRerunConfig{
			Enabled:         defaultAutoRerunEnabled,
			CooldownSeconds: defaultAutoRerunCooldown,
		},
	}
}

// ApplyDefaults fills missing or invalid values with documented defaults.
func ApplyDefaults(cfg Config, warn func(string)) Config {
	defaults := Defaults()
	cfg.Workers.Commands.Default = normalizeCommand(
		cfg.Workers.Commands.Default,
		defaults.Workers.Commands.Default,
		"workers.commands.default",
		warn,
	)
	cfg.Workers.Commands.Roles = normalizeRoleCommands(
		cfg.Workers.Commands.Roles,
		"workers.commands.roles",
		warn,
	)

	cfg.Concurrency.Global = normalizePositiveInt(
		cfg.Concurrency.Global,
		defaults.Concurrency.Global,
		"concurrency.global",
		warn,
	)
	cfg.Concurrency.DefaultRole = normalizePositiveInt(
		cfg.Concurrency.DefaultRole,
		defaults.Concurrency.DefaultRole,
		"concurrency.default_role",
		warn,
	)
	cfg.Concurrency.Roles = normalizeRoleCaps(
		cfg.Concurrency.Roles,
		"concurrency.roles",
		warn,
	)

	cfg.Timeouts.WorkerSeconds = normalizePositiveInt(
		cfg.Timeouts.WorkerSeconds,
		defaults.Timeouts.WorkerSeconds,
		"timeouts.worker_seconds",
		warn,
	)
	cfg.Retries.MaxAttempts = normalizePositiveInt(
		cfg.Retries.MaxAttempts,
		defaults.Retries.MaxAttempts,
		"retries.max_attempts",
		warn,
	)
	cfg.AutoRerun.CooldownSeconds = normalizePositiveInt(
		cfg.AutoRerun.CooldownSeconds,
		defaults.AutoRerun.CooldownSeconds,
		"auto_rerun.cooldown_seconds",
		warn,
	)
	if cfg.Workers.Commands.Roles == nil {
		cfg.Workers.Commands.Roles = map[string][]string{}
	}
	if cfg.Concurrency.Roles == nil {
		cfg.Concurrency.Roles = map[string]int{}
	}
	return cfg
}

// normalizeCommand ensures the command includes the task path token.
func normalizeCommand(value []string, fallback []string, key string, warn func(string)) []string {
	if len(value) == 0 || !containsTaskPathToken(value) {
		emitWarning(warn, "invalid "+key+"; using default command")
		return cloneStrings(fallback)
	}
	return cloneStrings(value)
}

// normalizeRoleCommands filters invalid role commands while preserving valid ones.
func normalizeRoleCommands(values map[string][]string, keyPrefix string, warn func(string)) map[string][]string {
	if values == nil {
		return map[string][]string{}
	}
	normalized := make(map[string][]string, len(values))
	for role, command := range values {
		if len(command) == 0 || !containsTaskPathToken(command) {
			emitWarning(warn, "invalid "+keyPrefix+"."+role+"; falling back to default command")
			continue
		}
		normalized[role] = cloneStrings(command)
	}
	return normalized
}

// normalizeRoleCaps filters invalid role caps while preserving valid ones.
func normalizeRoleCaps(values map[string]int, keyPrefix string, warn func(string)) map[string]int {
	if values == nil {
		return map[string]int{}
	}
	normalized := make(map[string]int, len(values))
	for role, cap := range values {
		if cap <= 0 {
			emitWarning(warn, "invalid "+keyPrefix+"."+role+"; using default role cap")
			continue
		}
		normalized[role] = cap
	}
	return normalized
}

// normalizePositiveInt defaults invalid values.
func normalizePositiveInt(value int, fallback int, key string, warn func(string)) int {
	if value <= 0 {
		emitWarning(warn, "invalid "+key+"; using default")
		return fallback
	}
	return value
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

// emitWarning forwards warnings to the provided sink.
func emitWarning(warn func(string), message string) {
	if warn == nil {
		return
	}
	warn(message)
}
