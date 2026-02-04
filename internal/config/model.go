// Package config defines the configuration model for Governator v2.
package config

import "strings"

// Config defines the full configuration surface for Governator v2.
type Config struct {
	Workers        WorkersConfig         `json:"workers"`
	Concurrency    ConcurrencyConfig     `json:"concurrency"`
	Timeouts       TimeoutsConfig        `json:"timeouts"`
	Retries        RetriesConfig         `json:"retries"`
	Branches       BranchConfig          `json:"branches"`
	ReasoningEffort ReasoningEffortConfig `json:"reasoning_effort"`
}

// WorkersConfig captures worker execution settings.
type WorkersConfig struct {
	CLI      WorkerCLI      `json:"cli"`
	Commands WorkerCommands `json:"commands"`
}

// WorkerCLI defines which built-in CLI tool to use for workers.
type WorkerCLI struct {
	Default string            `json:"default"` // "codex", "claude", or "gemini"
	Roles   map[string]string `json:"roles"`   // per-role CLI overrides
}

// WorkerCommands defines command templates for worker execution.
type WorkerCommands struct {
	Default []string            `json:"default"`
	Roles   map[string][]string `json:"roles"`
}

// ConcurrencyConfig defines limits on worker concurrency.
type ConcurrencyConfig struct {
	Global      int            `json:"global"`
	DefaultRole int            `json:"default_role"`
	Roles       map[string]int `json:"roles"`
}

// TimeoutsConfig defines timeout settings in seconds.
type TimeoutsConfig struct {
	WorkerSeconds int `json:"worker_seconds"`
}

// RetriesConfig defines retry limits.
type RetriesConfig struct {
	MaxAttempts int `json:"max_attempts"`
}

// BranchConfig describes how branches should be created for tasks.
type BranchConfig struct {
	Base string `json:"base"`
}

// ReasoningEffortConfig captures the default reasoning effort and role overrides.
type ReasoningEffortConfig struct {
	Default string            `json:"default"`
	Roles   map[string]string `json:"roles"`
}

const DefaultReasoningEffort = "medium"

// Built-in CLI names
const (
	CLICodex  = "codex"
	CLIClaude = "claude"
	CLIGemini = "gemini"
)

// BuiltInCommand returns the command template for a built-in CLI.
func BuiltInCommand(cli string) ([]string, bool) {
	switch cli {
	case CLICodex:
		return []string{"codex", "exec", "--sandbox=workspace-write", "{prompt_path}"}, true
	case CLIClaude:
		return []string{"claude", "--print", "{prompt_path}"}, true
	case CLIGemini:
		return []string{"gemini", "{prompt_path}"}, true
	default:
		return nil, false
	}
}

// IsValidCLI returns true if the CLI name is a known built-in.
func IsValidCLI(cli string) bool {
	_, ok := BuiltInCommand(cli)
	return ok
}

// LevelForRole returns the reasoning effort for the supplied role.
func (cfg ReasoningEffortConfig) LevelForRole(role string) string {
	if cfg.Roles != nil {
		if level, ok := cfg.Roles[role]; ok {
			level = strings.TrimSpace(level)
			if level != "" {
				return level
			}
		}
	}
	if trimmed := strings.TrimSpace(cfg.Default); trimmed != "" {
		return trimmed
	}
	return DefaultReasoningEffort
}
