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
	Commands WorkerCommands `json:"commands"`
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
