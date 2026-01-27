package worker

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
)

const (
	reasoningLevelHigh = "high"
	reasoningLevelLow  = "low"
)

// normalizeReasoningLevel trims whitespace and lowercases the level value.
func normalizeReasoningLevel(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// needsReasoningSupport reports whether the level has dedicated reasoning prompts or flags.
func needsReasoningSupport(level string) bool {
	return level == reasoningLevelHigh || level == reasoningLevelLow
}

// shouldIncludeReasoningPrompt returns true when the reasoning prompt needs to be prepended.
func shouldIncludeReasoningPrompt(level string, agentUsesCodex bool) bool {
	if level == "" || agentUsesCodex {
		return false
	}
	return needsReasoningSupport(level)
}

// applyCodexReasoningFlag inserts the model reasoning config option when codex is used.
func applyCodexReasoningFlag(command []string, level string) []string {
	normalized := normalizeReasoningLevel(level)
	if len(command) == 0 || !needsReasoningSupport(normalized) || !isCodexExecutablePath(command[0]) {
		return command
	}
	flagValue := fmt.Sprintf("model_reasoning_effort=%q", normalized)
	augmented := make([]string, 0, len(command)+2)
	augmented = append(augmented, command[0])
	augmented = append(augmented, "--config", flagValue)
	augmented = append(augmented, command[1:]...)
	return augmented
}

// isCodexExecutablePath detects codex by inspecting the executable's basename.
func isCodexExecutablePath(executable string) bool {
	if executable == "" {
		return false
	}
	base := filepath.Base(executable)
	if base == "" {
		return false
	}
	ext := filepath.Ext(base)
	base = strings.TrimSuffix(base, ext)
	return strings.EqualFold(base, "codex")
}

// IsCodexCommand reports whether the configured template runs the Codex CLI.
func IsCodexCommand(cfg config.Config, role index.Role) (bool, error) {
	template, err := selectCommandTemplate(cfg, role)
	if err != nil {
		return false, err
	}
	if len(template) == 0 {
		return false, nil
	}
	return isCodexExecutablePath(template[0]), nil
}
