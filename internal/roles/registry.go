// Package roles provides role prompt loading and stage-based role selection helpers.
package roles

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cmtonkinson/governator/internal/index"
)

// Registry captures available roles and prompt paths loaded from disk.
type Registry struct {
	root         string
	roles        map[index.Role]RoleDefinition
	roleOrder    []index.Role
	customGlobal string
	customRoles  map[index.Role]string
	warn         func(string)
}

// RoleDefinition describes a role and its primary prompt path.
type RoleDefinition struct {
	Name       index.Role
	PromptPath string
}

// Stage labels the worker lifecycle stage used for runtime role selection.
type Stage string

const (
	// StageWork selects the role for implementation work.
	StageWork Stage = "work"
	// StageTest selects the role for test execution.
	StageTest Stage = "test"
	// StageReview selects the role for review execution.
	StageReview Stage = "review"
	// StageResolve selects the role for conflict resolution.
	StageResolve Stage = "resolve"
)

// StageRoleSelector maps lifecycle stages to roles, falling back to a default role.
type StageRoleSelector struct {
	Default   index.Role
	Overrides map[Stage]index.Role
}

// RoleForStage selects the role to use for the supplied stage.
func (selector StageRoleSelector) RoleForStage(stage Stage) (index.Role, error) {
	if selector.Overrides != nil {
		if role, ok := selector.Overrides[stage]; ok && role != "" {
			return role, nil
		}
	}
	if selector.Default != "" {
		return selector.Default, nil
	}
	return "", fmt.Errorf("no role available for stage %q", stage)
}

// LoadRegistry loads role prompts from the repository root.
func LoadRegistry(root string, warn func(string)) (Registry, error) {
	if strings.TrimSpace(root) == "" {
		return Registry{}, errors.New("root is required")
	}
	rolesDir := filepath.Join(root, "_governator", "roles")
	roleEntries, err := os.ReadDir(rolesDir)
	if err != nil {
		return Registry{}, fmt.Errorf("read roles dir: %w", err)
	}
	roles := make(map[index.Role]RoleDefinition, len(roleEntries))
	roleOrder := make([]index.Role, 0, len(roleEntries))
	for _, entry := range roleEntries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".md" {
			continue
		}
		roleName := strings.TrimSuffix(name, ".md")
		if roleName == "" {
			continue
		}
		role := index.Role(roleName)
		roles[role] = RoleDefinition{
			Name:       role,
			PromptPath: repoRelativePath(root, filepath.Join(rolesDir, name)),
		}
		roleOrder = append(roleOrder, role)
	}
	sort.Slice(roleOrder, func(i, j int) bool {
		return roleOrder[i] < roleOrder[j]
	})

	customDir := filepath.Join(root, "_governator", "custom-prompts")
	customGlobal := ""
	customRoles := map[index.Role]string{}
	customEntries, err := os.ReadDir(customDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Registry{}, fmt.Errorf("read custom prompts dir: %w", err)
		}
	} else {
		for _, entry := range customEntries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if filepath.Ext(name) != ".md" {
				continue
			}
			roleName := strings.TrimSuffix(name, ".md")
			if roleName == "" {
				continue
			}
			path := repoRelativePath(root, filepath.Join(customDir, name))
			if roleName == "_global" {
				customGlobal = path
				continue
			}
			customRoles[index.Role(roleName)] = path
		}
	}

	return Registry{
		root:         root,
		roles:        roles,
		roleOrder:    roleOrder,
		customGlobal: customGlobal,
		customRoles:  customRoles,
		warn:         warn,
	}, nil
}

// Roles returns the available role names in stable order.
func (registry Registry) Roles() []index.Role {
	if len(registry.roleOrder) == 0 {
		return nil
	}
	roles := make([]index.Role, len(registry.roleOrder))
	copy(roles, registry.roleOrder)
	return roles
}

// RolePromptPath returns the repo-relative prompt path for a role, if present.
func (registry Registry) RolePromptPath(role index.Role) (string, bool) {
	if role == "" {
		return "", false
	}
	def, ok := registry.roles[role]
	if !ok || def.PromptPath == "" {
		return "", false
	}
	return def.PromptPath, true
}

// PromptFiles returns the ordered prompt files for the supplied role.
//
// Prompt order:
//  1. _governator/roles/<role>.md
//  2. _governator/custom-prompts/_global.md
//  3. _governator/custom-prompts/<role>.md
func (registry Registry) PromptFiles(role index.Role) []string {
	prompts := []string{}
	if def, ok := registry.roles[role]; ok {
		prompts = append(prompts, def.PromptPath)
	} else if role != "" {
		emitWarning(registry.warn, fmt.Sprintf("missing role prompt for %s", role))
	}
	if registry.customGlobal != "" {
		prompts = append(prompts, registry.customGlobal)
	}
	if custom, ok := registry.customRoles[role]; ok && custom != "" {
		prompts = append(prompts, custom)
	}
	return prompts
}

// CustomGlobalPromptPath returns the global prompt path when configured.
func (registry Registry) CustomGlobalPromptPath() (string, bool) {
	if registry.customGlobal == "" {
		return "", false
	}
	return registry.customGlobal, true
}

// CustomRolePromptPath returns the custom prompt path for a role when available.
func (registry Registry) CustomRolePromptPath(role index.Role) (string, bool) {
	path, ok := registry.customRoles[role]
	return path, ok
}

// repoRelativePath returns a repository-relative path using forward slashes.
func repoRelativePath(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// emitWarning sends a warning to the configured sink.
func emitWarning(warn func(string), message string) {
	if warn == nil {
		return
	}
	warn(message)
}
