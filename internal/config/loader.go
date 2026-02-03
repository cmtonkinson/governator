// Package config provides configuration loading helpers.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

const (
	userConfigDirName       = ".config"
	userConfigFileName      = "config.json"
	repoConfigDirName       = "_governator/_durable-state"
	repoLegacyConfigDirName = "_governator/config"
)

// Load resolves configuration from user defaults, repo overrides, and CLI overrides.
func Load(repoRoot string, cliOverrides map[string]any, warn func(string)) (Config, error) {
	userConfigPath, err := userConfigPath()
	if err != nil {
		return Config{}, err
	}

	merged := map[string]any{}
	merged, err = mergeConfigLayer(merged, userConfigPath, "user defaults")
	if err != nil {
		return Config{}, err
	}

	if repoRoot != "" {
		repoConfigPath := filepath.Join(repoRoot, repoConfigDirName, userConfigFileName)
		merged, err = mergeConfigLayer(merged, repoConfigPath, "repo overrides")
		if err != nil {
			return Config{}, err
		}

		legacyConfigPath := filepath.Join(repoRoot, repoLegacyConfigDirName, userConfigFileName)
		merged, err = mergeConfigLayer(merged, legacyConfigPath, "repo legacy overrides")
		if err != nil {
			return Config{}, err
		}
	}

	if cliOverrides != nil {
		merged = mergeConfigMaps(merged, cliOverrides)
	}

	cfg := decodeConfig(merged)
	return ApplyDefaults(cfg, warn), nil
}

// userConfigPath resolves the user defaults path for config.json.
func userConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(homeDir, userConfigDirName, "governator", userConfigFileName), nil
}

// mergeConfigLayer reads a config file and merges it into the base map.
func mergeConfigLayer(base map[string]any, path string, label string) (map[string]any, error) {
	layer, err := readConfigFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return base, nil
		}
		return nil, fmt.Errorf("load %s config %s: %w", label, path, err)
	}
	return mergeConfigMaps(base, layer), nil
}

// readConfigFile parses a config JSON object from the given path.
func readConfigFile(path string) (map[string]any, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.UseNumber()

	var data map[string]any
	if err := decoder.Decode(&data); err != nil {
		return nil, err
	}
	if err := ensureEOF(decoder); err != nil {
		return nil, err
	}
	if data == nil {
		return map[string]any{}, nil
	}
	return data, nil
}

// ensureEOF verifies the decoder consumed the entire input.
func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return errors.New("invalid trailing content after JSON object")
}

// mergeConfigMaps overlays override onto base and returns a merged map.
func mergeConfigMaps(base map[string]any, override map[string]any) map[string]any {
	if base == nil {
		base = map[string]any{}
	}
	merged := cloneConfigMap(base)
	for key, value := range override {
		overrideMap, ok := value.(map[string]any)
		if !ok {
			merged[key] = value
			continue
		}
		if baseMap, ok := merged[key].(map[string]any); ok {
			merged[key] = mergeConfigMaps(baseMap, overrideMap)
			continue
		}
		merged[key] = cloneConfigMap(overrideMap)
	}
	return merged
}

// cloneConfigMap copies a map recursively to prevent aliasing.
func cloneConfigMap(values map[string]any) map[string]any {
	clone := make(map[string]any, len(values))
	for key, value := range values {
		if nested, ok := value.(map[string]any); ok {
			clone[key] = cloneConfigMap(nested)
			continue
		}
		clone[key] = value
	}
	return clone
}

// decodeConfig best-effort decodes a config map into the Config struct.
func decodeConfig(raw map[string]any) Config {
	var cfg Config

	workers := toConfigMap(raw["workers"])
	commands := toConfigMap(workers["commands"])
	cfg.Workers.Commands.Default = parseStringSlice(commands["default"])
	cfg.Workers.Commands.Roles = parseStringSliceMap(commands["roles"])

	concurrency := toConfigMap(raw["concurrency"])
	cfg.Concurrency.Global = parseInt(concurrency["global"])
	cfg.Concurrency.DefaultRole = parseInt(concurrency["default_role"])
	cfg.Concurrency.Roles = parseIntMap(concurrency["roles"])

	timeouts := toConfigMap(raw["timeouts"])
	cfg.Timeouts.WorkerSeconds = parseInt(timeouts["worker_seconds"])

	retries := toConfigMap(raw["retries"])
	cfg.Retries.MaxAttempts = parseInt(retries["max_attempts"])

	branches := toConfigMap(raw["branches"])
	cfg.Branches.Base = parseString(branches["base"])

	reasoningEffort := toConfigMap(raw["reasoning_effort"])
	cfg.ReasoningEffort.Default = parseString(reasoningEffort["default"])
	cfg.ReasoningEffort.Roles = parseStringMap(reasoningEffort["roles"])

	return cfg
}

// toConfigMap asserts a value as map[string]any.
func toConfigMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	typed, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return typed
}

// parseStringSlice reads a JSON array of strings.
func parseStringSlice(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	if len(raw) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok {
			return nil
		}
		result = append(result, text)
	}
	return result
}

// parseStringSliceMap reads a map of string arrays.
func parseStringSliceMap(value any) map[string][]string {
	raw, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	result := make(map[string][]string, len(raw))
	for key, item := range raw {
		parsed := parseStringSlice(item)
		if parsed == nil {
			continue
		}
		result[key] = parsed
	}
	return result
}

// parseInt reads an integer from a JSON number.
func parseInt(value any) int {
	parsed, ok := parseIntValue(value)
	if !ok {
		return 0
	}
	return parsed
}

// parseIntMap reads a map of integers.
func parseIntMap(value any) map[string]int {
	raw, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	result := make(map[string]int, len(raw))
	for key, item := range raw {
		parsed, ok := parseIntValue(item)
		if !ok {
			continue
		}
		result[key] = parsed
	}
	return result
}

// parseIntValue converts supported numeric types into an int.
func parseIntValue(value any) (int, bool) {
	switch typed := value.(type) {
	case json.Number:
		if integer, err := typed.Int64(); err == nil {
			return int(integer), true
		}
		if floatValue, err := typed.Float64(); err == nil {
			return floatToInt(floatValue)
		}
	case float64:
		return floatToInt(typed)
	case float32:
		return floatToInt(float64(typed))
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case int32:
		return int(typed), true
	case uint:
		return int(typed), true
	case uint64:
		return int(typed), true
	}
	return 0, false
}

// floatToInt converts a float64 to int when it represents an integer.
func floatToInt(value float64) (int, bool) {
	if math.Trunc(value) != value {
		return 0, false
	}
	return int(value), true
}

// parseBool reads a JSON boolean.
func parseBool(value any) bool {
	typed, ok := value.(bool)
	if !ok {
		return false
	}
	return typed
}

// parseString returns trimmed string values from config maps.
func parseString(value any) string {
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(typed)
}

// parseStringMap reads a map of trimmed strings.
func parseStringMap(value any) map[string]string {
	raw, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	result := make(map[string]string, len(raw))
	for key, item := range raw {
		if str, ok := item.(string); ok {
			result[key] = strings.TrimSpace(str)
		}
	}
	return result
}
