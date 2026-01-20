// Package scheduler provides deterministic routing helpers for task dispatch.
package scheduler

import (
	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
)

// RoleCaps captures the global, default-role, and per-role concurrency limits.
type RoleCaps struct {
	Global      int
	DefaultRole int
	Roles       map[index.Role]int
}

// RoleCapsFromConfig builds role caps from the supplied config, applying defaults as needed.
func RoleCapsFromConfig(cfg config.Config) RoleCaps {
	defaults := config.Defaults()
	global := cfg.Concurrency.Global
	if global <= 0 {
		global = defaults.Concurrency.Global
	}
	defaultRole := cfg.Concurrency.DefaultRole
	if defaultRole <= 0 {
		defaultRole = defaults.Concurrency.DefaultRole
	}
	roles := make(map[index.Role]int, len(cfg.Concurrency.Roles))
	for role, cap := range cfg.Concurrency.Roles {
		if cap <= 0 {
			continue
		}
		roles[index.Role(role)] = cap
	}
	return RoleCaps{
		Global:      global,
		DefaultRole: defaultRole,
		Roles:       roles,
	}
}

// ApplyRoleCaps selects tasks in order, enforcing global and per-role caps.
func ApplyRoleCaps(ordered []index.Task, caps RoleCaps) []index.Task {
	if caps.Global <= 0 {
		return nil
	}
	selected := make([]index.Task, 0, min(caps.Global, len(ordered)))
	if len(ordered) == 0 {
		return selected
	}
	usage := map[index.Role]int{}
	for _, task := range ordered {
		if len(selected) >= caps.Global {
			break
		}
		roleCap := capForRole(task.Role, caps)
		if roleCap <= 0 {
			continue
		}
		if usage[task.Role] >= roleCap {
			continue
		}
		selected = append(selected, task)
		usage[task.Role]++
	}
	return selected
}

// capForRole returns the concurrency cap that applies to the provided role.
func capForRole(role index.Role, caps RoleCaps) int {
	if caps.Roles != nil {
		if cap, ok := caps.Roles[role]; ok {
			return cap
		}
	}
	return caps.DefaultRole
}

// min returns the smaller of two ints.
func min(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
