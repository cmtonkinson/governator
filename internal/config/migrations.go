// Package config provides repository migration helpers for durable layout updates.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/inflight"
	"github.com/cmtonkinson/governator/internal/templates"
)

const (
	legacyGovernatorDirName            = "_governator"
	legacyDurableStateDir              = "_governator/_durable-state"
	legacyLocalStateRelPath            = "_governator/_local-state"
	layoutMigrationID                  = "20260216_migrate_workspace_layout"
	conflictResolutionPromptName       = "conflict-resolution.md"
	conflictResolutionTemplatePath     = "planning/conflict-resolution.md"
	conflictResolutionMigrationID      = "20260209_add_conflict_resolution_prompt"
	conflictResolutionNormalizedPrompt = "conflictresolution"
	resetOpenTasksMigrationID          = "20260214_reset_open_tasks_strip_role_suffix"
)

type repoMigration struct {
	id          string
	destructive bool
	apply       func(repoRoot string, opts InitOptions) error
}

var repoMigrations = []repoMigration{
	{
		id:          layoutMigrationID,
		destructive: false,
		apply:       migrateWorkspaceLayout,
	},
	{
		id:          conflictResolutionMigrationID,
		destructive: false,
		apply:       migrateConflictResolutionPrompt,
	},
	{
		id:          resetOpenTasksMigrationID,
		destructive: true,
		apply:       migrateResetOpenTasksStripRoleSuffix,
	},
}

// RepoMigrationInfo describes a pending repo migration and whether it is destructive.
type RepoMigrationInfo struct {
	ID          string
	Destructive bool
}

// PendingRepoMigrationInfo returns pending migration metadata for confirmation flows.
func PendingRepoMigrationInfo(repoRoot string) ([]RepoMigrationInfo, error) {
	pending, err := pendingRepoMigrations(repoRoot)
	if err != nil {
		return nil, err
	}
	info := make([]RepoMigrationInfo, 0, len(pending))
	for _, migration := range pending {
		info = append(info, RepoMigrationInfo{
			ID:          migration.id,
			Destructive: migration.destructive,
		})
	}
	return info, nil
}

// PendingRepoMigrations returns repo migration IDs that have not yet been marked complete.
func PendingRepoMigrations(repoRoot string) ([]string, error) {
	pending, err := pendingRepoMigrations(repoRoot)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(pending))
	for _, migration := range pending {
		ids = append(ids, migration.id)
	}
	return ids, nil
}

func pendingRepoMigrations(repoRoot string) ([]repoMigration, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return nil, fmt.Errorf("repo root cannot be empty")
	}

	migrationsDir := filepath.Join(repoRoot, repoDurableStateDir, "migrations")
	pending := make([]repoMigration, 0, len(repoMigrations))
	for _, migration := range repoMigrations {
		if strings.TrimSpace(migration.id) == "" {
			continue
		}
		exists, err := migrationMarkerExists(repoRoot, migrationsDir, migration.id)
		if err != nil {
			return nil, err
		}
		if !exists {
			pending = append(pending, migration)
		}
	}
	return pending, nil
}

// StampExistingMigrations marks all currently-known migrations as complete without
// running them. Call this during init so that migrations predating a fresh project
// are never surfaced as pending.
func StampExistingMigrations(repoRoot string, opts InitOptions) error {
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root cannot be empty")
	}

	migrationsDir := filepath.Join(repoRoot, repoDurableStateDir, "migrations")
	if err := ensureDir(migrationsDir, opts); err != nil {
		return fmt.Errorf("create migrations directory %s: %w", migrationsDir, err)
	}

	for _, migration := range repoMigrations {
		if strings.TrimSpace(migration.id) == "" {
			continue
		}
		markerPath := filepath.Join(migrationsDir, migration.id+".done")
		exists, err := migrationMarkerExists(repoRoot, migrationsDir, migration.id)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if err := os.WriteFile(markerPath, []byte("ok\n"), 0o644); err != nil {
			return fmt.Errorf("write migration marker %s: %w", markerPath, err)
		}
	}
	return nil
}

// ApplyRepoMigrations applies idempotent durable migrations for existing repositories.
func ApplyRepoMigrations(repoRoot string, opts InitOptions) error {
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root cannot be empty")
	}

	migrationsDir := filepath.Join(repoRoot, repoDurableStateDir, "migrations")
	if err := ensureDir(migrationsDir, opts); err != nil {
		return fmt.Errorf("create migrations directory %s: %w", migrationsDir, err)
	}

	for _, migration := range repoMigrations {
		if strings.TrimSpace(migration.id) == "" {
			continue
		}
		markerPath := filepath.Join(migrationsDir, migration.id+".done")
		exists, err := pathExists(markerPath)
		if err != nil {
			return fmt.Errorf("check migration marker %s: %w", markerPath, err)
		}
		if exists {
			continue
		}
		if err := migration.apply(repoRoot, opts); err != nil {
			return fmt.Errorf("run migration %s: %w", migration.id, err)
		}
		if err := os.WriteFile(markerPath, []byte("ok\n"), 0o644); err != nil {
			return fmt.Errorf("write migration marker %s: %w", markerPath, err)
		}
	}

	return nil
}

// migrationMarkerExists checks both current and legacy marker locations for migration completion.
func migrationMarkerExists(repoRoot, migrationsDir, migrationID string) (bool, error) {
	current := filepath.Join(migrationsDir, migrationID+".done")
	exists, err := pathExists(current)
	if err != nil {
		return false, fmt.Errorf("check migration marker %s: %w", current, err)
	}
	if exists {
		return true, nil
	}

	legacy := filepath.Join(repoRoot, legacyDurableStateDir, "migrations", migrationID+".done")
	exists, err = pathExists(legacy)
	if err != nil {
		return false, fmt.Errorf("check legacy migration marker %s: %w", legacy, err)
	}
	return exists, nil
}

// migrateWorkspaceLayout upgrades legacy _governator layout to the dot-prefixed workspace layout.
func migrateWorkspaceLayout(repoRoot string, opts InitOptions) error {
	legacyRoot := filepath.Join(repoRoot, legacyGovernatorDirName)
	legacyExists, err := pathExists(legacyRoot)
	if err != nil {
		return fmt.Errorf("check legacy governator dir %s: %w", legacyRoot, err)
	}
	if !legacyExists {
		if err := ensureDotLayoutScaffold(repoRoot, opts); err != nil {
			return err
		}
		return ensureWorkspaceGitignore(repoRoot)
	}

	targetRoot := filepath.Join(repoRoot, ".governator")
	if err := ensureDir(targetRoot, opts); err != nil {
		return fmt.Errorf("create .governator directory %s: %w", targetRoot, err)
	}

	entries, err := os.ReadDir(legacyRoot)
	if err != nil {
		return fmt.Errorf("read legacy workspace %s: %w", legacyRoot, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		sourcePath := filepath.Join(legacyRoot, name)
		targetName := name
		switch name {
		case "_durable-state":
			targetName = "state"
		case "_local-state":
			targetName = ".local-state"
		}
		targetPath := filepath.Join(targetRoot, targetName)
		if err := mergePath(sourcePath, targetPath); err != nil {
			return err
		}
	}

	if err := migrateLegacyWorktreeDirs(repoRoot); err != nil {
		return err
	}
	if err := rewriteLegacyMetadataWorktreePaths(repoRoot); err != nil {
		return err
	}
	if err := ensureDotLayoutScaffold(repoRoot, opts); err != nil {
		return err
	}
	if err := ensureWorkspaceGitignore(repoRoot); err != nil {
		return err
	}

	legacyEntries, err := os.ReadDir(legacyRoot)
	if err != nil {
		return fmt.Errorf("read legacy workspace %s after migration: %w", legacyRoot, err)
	}
	if len(legacyEntries) == 0 {
		if err := os.Remove(legacyRoot); err != nil {
			return fmt.Errorf("remove legacy workspace %s: %w", legacyRoot, err)
		}
	}
	return nil
}

// ensureDotLayoutScaffold creates required directories/files for the new layout.
func ensureDotLayoutScaffold(repoRoot string, opts InitOptions) error {
	worktreesDir := filepath.Join(repoRoot, ".governator", "worktrees")
	if err := ensureDir(worktreesDir, opts); err != nil {
		return fmt.Errorf("create worktrees directory %s: %w", worktreesDir, err)
	}
	if err := ensureKeepFile(filepath.Join(worktreesDir, ".keep"), opts); err != nil {
		return fmt.Errorf("ensure worktrees .keep: %w", err)
	}
	localStateDir := filepath.Join(repoRoot, ".governator", ".local-state")
	if err := ensureDir(localStateDir, opts); err != nil {
		return fmt.Errorf("create local-state directory %s: %w", localStateDir, err)
	}
	return nil
}

// ensureWorkspaceGitignore ensures .governator/.gitignore contains required runtime ignore rules.
func ensureWorkspaceGitignore(repoRoot string) error {
	gitignorePath := filepath.Join(repoRoot, ".governator", ".gitignore")
	requiredRules := []string{
		".local-state/*",
		"!.local-state/.keep",
		"worktrees/*",
		"!worktrees/.keep",
	}

	exists, err := pathExists(gitignorePath)
	if err != nil {
		return fmt.Errorf("check workspace gitignore %s: %w", gitignorePath, err)
	}
	if !exists {
		content := strings.Join(requiredRules, "\n") + "\n"
		if err := os.WriteFile(gitignorePath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write workspace gitignore %s: %w", gitignorePath, err)
		}
		return nil
	}

	current, err := os.ReadFile(gitignorePath)
	if err != nil {
		return fmt.Errorf("read workspace gitignore %s: %w", gitignorePath, err)
	}
	lines := strings.Split(strings.ReplaceAll(string(current), "\r\n", "\n"), "\n")
	seen := map[string]struct{}{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		seen[trimmed] = struct{}{}
	}
	changed := false
	for _, rule := range requiredRules {
		if _, ok := seen[rule]; ok {
			continue
		}
		lines = append(lines, rule)
		changed = true
	}
	if !changed {
		return nil
	}
	updated := strings.Join(lines, "\n")
	updated = strings.TrimRight(updated, "\n") + "\n"
	if err := os.WriteFile(gitignorePath, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("update workspace gitignore %s: %w", gitignorePath, err)
	}
	return nil
}

// mergePath moves source into destination, merging directories and rejecting conflicting files.
func mergePath(sourcePath, targetPath string) error {
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("stat source path %s: %w", sourcePath, err)
	}

	targetInfo, err := os.Stat(targetPath)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(sourcePath, targetPath); err != nil {
			return fmt.Errorf("move %s to %s: %w", sourcePath, targetPath, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat target path %s: %w", targetPath, err)
	}
	if sourceInfo.IsDir() != targetInfo.IsDir() {
		return fmt.Errorf("layout migration conflict: %s and %s have different types", sourcePath, targetPath)
	}
	if !sourceInfo.IsDir() {
		sourceBytes, readErr := os.ReadFile(sourcePath)
		if readErr != nil {
			return fmt.Errorf("read source file %s: %w", sourcePath, readErr)
		}
		targetBytes, readErr := os.ReadFile(targetPath)
		if readErr != nil {
			return fmt.Errorf("read target file %s: %w", targetPath, readErr)
		}
		if string(sourceBytes) != string(targetBytes) {
			return fmt.Errorf("layout migration conflict: destination exists with different content: %s", targetPath)
		}
		if err := os.Remove(sourcePath); err != nil {
			return fmt.Errorf("remove duplicate source file %s: %w", sourcePath, err)
		}
		return nil
	}

	entries, err := os.ReadDir(sourcePath)
	if err != nil {
		return fmt.Errorf("read source directory %s: %w", sourcePath, err)
	}
	for _, entry := range entries {
		childSource := filepath.Join(sourcePath, entry.Name())
		childTarget := filepath.Join(targetPath, entry.Name())
		if err := mergePath(childSource, childTarget); err != nil {
			return err
		}
	}
	if err := os.Remove(sourcePath); err != nil {
		return fmt.Errorf("remove merged source directory %s: %w", sourcePath, err)
	}
	return nil
}

// migrateLegacyWorktreeDirs moves legacy task and merge worktree directories into .governator/worktrees.
func migrateLegacyWorktreeDirs(repoRoot string) error {
	localStateDir := filepath.Join(repoRoot, ".governator", ".local-state")
	worktreesDir := filepath.Join(repoRoot, ".governator", "worktrees")
	entries, err := os.ReadDir(localStateDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read local state dir %s: %w", localStateDir, err)
	}
	if err := os.MkdirAll(worktreesDir, 0o755); err != nil {
		return fmt.Errorf("create worktrees dir %s: %w", worktreesDir, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name != "merge-worktrees" && !strings.HasPrefix(name, "task-") {
			continue
		}
		sourcePath := filepath.Join(localStateDir, name)
		targetPath := filepath.Join(worktreesDir, name)
		if err := mergePath(sourcePath, targetPath); err != nil {
			return err
		}
	}
	return nil
}

// rewriteLegacyMetadataWorktreePaths rewrites metadata worktree_rel_path values to .governator/worktrees/*.
func rewriteLegacyMetadataWorktreePaths(repoRoot string) error {
	metaDir := filepath.Join(repoRoot, ".governator", ".local-state", "meta")
	entries, err := os.ReadDir(metaDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read metadata dir %s: %w", metaDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(metaDir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read metadata file %s: %w", path, err)
		}

		var decoded map[string]any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return fmt.Errorf("decode metadata file %s: %w", path, err)
		}
		value, ok := decoded["worktree_rel_path"].(string)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}

		normalized := filepath.ToSlash(filepath.Clean(value))
		var replacement string
		switch {
		case strings.HasPrefix(normalized, legacyLocalStateRelPath+"/task-"):
			replacement = strings.Replace(normalized, legacyLocalStateRelPath+"/", ".governator/worktrees/", 1)
		case strings.HasPrefix(normalized, ".governator/.local-state/task-"):
			replacement = strings.Replace(normalized, ".governator/.local-state/", ".governator/worktrees/", 1)
		}
		if replacement == "" || replacement == value {
			continue
		}
		decoded["worktree_rel_path"] = replacement
		updated, err := json.MarshalIndent(decoded, "", "  ")
		if err != nil {
			return fmt.Errorf("encode metadata file %s: %w", path, err)
		}
		updated = append(updated, '\n')
		if err := os.WriteFile(path, updated, 0o644); err != nil {
			return fmt.Errorf("write metadata file %s: %w", path, err)
		}
	}
	return nil
}

func migrateConflictResolutionPrompt(repoRoot string, opts InitOptions) error {
	promptsDir := filepath.Join(repoRoot, ".governator", "prompts")
	if err := ensureDir(promptsDir, opts); err != nil {
		return fmt.Errorf("create prompts directory %s: %w", promptsDir, err)
	}

	targetPath := filepath.Join(promptsDir, conflictResolutionPromptName)
	exists, err := pathExists(targetPath)
	if err != nil {
		return fmt.Errorf("check conflict prompt %s: %w", targetPath, err)
	}
	if exists {
		return nil
	}
	if similarPromptExists(promptsDir, conflictResolutionPromptName) {
		return nil
	}

	data, err := templates.Read(conflictResolutionTemplatePath)
	if err != nil {
		return fmt.Errorf("read embedded template %s: %w", conflictResolutionTemplatePath, err)
	}
	if err := os.WriteFile(targetPath, data, 0o644); err != nil {
		return fmt.Errorf("write conflict prompt %s: %w", targetPath, err)
	}
	opts.logf("created planning prompt %s", repoRelativePath(repoRoot, targetPath))
	return nil
}

func similarPromptExists(promptsDir string, targetName string) bool {
	entries, err := os.ReadDir(promptsDir)
	if err != nil {
		return false
	}
	targetNormalized := normalizePromptStem(targetName)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		if strings.EqualFold(name, targetName) {
			return true
		}
		nameLower := strings.ToLower(name)
		if strings.Contains(nameLower, "conflict") && strings.Contains(nameLower, "resolution") {
			return true
		}
		if normalizePromptStem(name) == targetNormalized {
			return true
		}
	}
	return false
}

func normalizePromptStem(name string) string {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	stem := strings.TrimSuffix(trimmed, strings.ToLower(filepath.Ext(trimmed)))
	builder := strings.Builder{}
	for _, r := range stem {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func migrateResetOpenTasksStripRoleSuffix(repoRoot string, opts InitOptions) error {
	indexPath := filepath.Join(repoRoot, ".governator", ".local-state", "index.json")
	indexExists, err := pathExists(indexPath)
	if err != nil {
		return fmt.Errorf("check task index %s: %w", indexPath, err)
	}
	if !indexExists {
		return nil
	}

	idx, err := index.Load(indexPath)
	if err != nil {
		return fmt.Errorf("load task index %s: %w", indexPath, err)
	}

	roleSet, err := roleSuffixCandidates(repoRoot, idx)
	if err != nil {
		return fmt.Errorf("load role suffix candidates: %w", err)
	}
	idRenames := map[string]string{}
	pathRenames := map[string]string{}
	openTaskIDs := map[string]struct{}{}

	for _, task := range idx.Tasks {
		if task.Kind != index.TaskKindExecution {
			continue
		}
		if task.State != index.TaskStateBacklog && task.State != index.TaskStateMerged {
			openTaskIDs[task.ID] = struct{}{}
		}
		newID := stripRoleSuffix(task.ID, roleSet)
		if newID != "" && newID != task.ID {
			idRenames[task.ID] = newID
		}
		newPath := stripRoleSuffixFromPath(task.Path, roleSet)
		if newPath != "" && newPath != task.Path {
			pathRenames[task.Path] = newPath
		}
	}

	if err := validateMigrationRenames(idRenames, pathRenames); err != nil {
		return err
	}
	if err := applyTaskFileRenames(repoRoot, pathRenames); err != nil {
		return err
	}

	for i := range idx.Tasks {
		task := &idx.Tasks[i]
		if task.Kind != index.TaskKindExecution {
			continue
		}
		originalID := task.ID

		if renamedID, ok := idRenames[task.ID]; ok {
			task.ID = renamedID
		}
		if renamedPath, ok := pathRenames[task.Path]; ok {
			task.Path = renamedPath
		}
		for d := range task.Dependencies {
			if renamedDep, ok := idRenames[task.Dependencies[d]]; ok {
				task.Dependencies[d] = renamedDep
			}
		}

		if _, shouldReset := openTaskIDs[originalID]; shouldReset {
			task.State = index.TaskStateBacklog
			task.Role = ""
			task.AssignedRole = ""
			task.BlockedReason = ""
			task.MergeConflict = false
			task.PID = 0
			task.Attempts = index.AttemptCounters{}
		}
	}

	if err := index.Save(indexPath, idx); err != nil {
		return fmt.Errorf("save migrated task index %s: %w", indexPath, err)
	}

	if err := resetInFlightState(repoRoot); err != nil {
		return err
	}
	if err := purgeTaskWorktreesAndBranches(repoRoot, openTaskIDs); err != nil {
		return err
	}
	opts.logf("reset %d non-merged task(s) to backlog", len(openTaskIDs))
	return nil
}

func roleSuffixCandidates(repoRoot string, idx index.Index) (map[string]struct{}, error) {
	candidates := map[string]struct{}{
		"default": {},
	}
	for _, task := range idx.Tasks {
		role := strings.ToLower(strings.TrimSpace(string(task.Role)))
		if role != "" {
			candidates[role] = struct{}{}
		}
		assigned := strings.ToLower(strings.TrimSpace(task.AssignedRole))
		if assigned != "" {
			candidates[assigned] = struct{}{}
		}
	}

	rolesDir := filepath.Join(repoRoot, ".governator", "roles")
	entries, err := os.ReadDir(rolesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return candidates, nil
		}
		return nil, fmt.Errorf("read roles dir %s: %w", rolesDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		candidates[name] = struct{}{}
	}
	return candidates, nil
}

func stripRoleSuffixFromPath(path string, roleSet map[string]struct{}) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	ext := filepath.Ext(trimmed)
	stem := strings.TrimSuffix(filepath.Base(trimmed), ext)
	renamed := stripRoleSuffix(stem, roleSet)
	if renamed == "" || renamed == stem {
		return trimmed
	}
	return filepath.ToSlash(filepath.Join(filepath.Dir(trimmed), renamed+ext))
}

func stripRoleSuffix(value string, roleSet map[string]struct{}) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len(roleSet) == 0 {
		return trimmed
	}
	parts := strings.Split(trimmed, "-")
	if len(parts) < 2 {
		return trimmed
	}
	suffix := strings.ToLower(strings.TrimSpace(parts[len(parts)-1]))
	if suffix == "" {
		return trimmed
	}
	if _, ok := roleSet[suffix]; !ok {
		return trimmed
	}
	without := strings.Join(parts[:len(parts)-1], "-")
	if strings.TrimSpace(without) == "" {
		return trimmed
	}
	return without
}

func validateMigrationRenames(idRenames map[string]string, pathRenames map[string]string) error {
	targetIDs := map[string]string{}
	for src, dst := range idRenames {
		if previous, exists := targetIDs[dst]; exists && previous != src {
			return fmt.Errorf("migration id rename collision: %q and %q both map to %q", previous, src, dst)
		}
		targetIDs[dst] = src
	}
	targetPaths := map[string]string{}
	for src, dst := range pathRenames {
		if previous, exists := targetPaths[dst]; exists && previous != src {
			return fmt.Errorf("migration path rename collision: %q and %q both map to %q", previous, src, dst)
		}
		targetPaths[dst] = src
	}
	return nil
}

func applyTaskFileRenames(repoRoot string, renames map[string]string) error {
	if len(renames) == 0 {
		return nil
	}

	keys := make([]string, 0, len(renames))
	for src := range renames {
		keys = append(keys, src)
	}
	sort.Strings(keys)

	for _, src := range keys {
		dst := renames[src]
		srcAbs := filepath.Join(repoRoot, filepath.FromSlash(src))
		dstAbs := filepath.Join(repoRoot, filepath.FromSlash(dst))
		srcExists, err := pathExists(srcAbs)
		if err != nil {
			return fmt.Errorf("check source task file %s: %w", srcAbs, err)
		}
		if !srcExists {
			continue
		}
		dstExists, err := pathExists(dstAbs)
		if err != nil {
			return fmt.Errorf("check destination task file %s: %w", dstAbs, err)
		}
		if dstExists {
			return fmt.Errorf("cannot rename task file %s to %s: destination exists", src, dst)
		}
		if err := os.MkdirAll(filepath.Dir(dstAbs), 0o755); err != nil {
			return fmt.Errorf("create destination dir for %s: %w", dstAbs, err)
		}
		if err := os.Rename(srcAbs, dstAbs); err != nil {
			return fmt.Errorf("rename task file %s to %s: %w", src, dst, err)
		}
	}
	return nil
}

func resetInFlightState(repoRoot string) error {
	store, err := inflight.NewStore(repoRoot)
	if err != nil {
		return fmt.Errorf("create in-flight store: %w", err)
	}
	if err := store.Save(inflight.Set{}); err != nil {
		return fmt.Errorf("clear in-flight store: %w", err)
	}
	return nil
}

func purgeTaskWorktreesAndBranches(repoRoot string, openTaskIDs map[string]struct{}) error {
	if len(openTaskIDs) == 0 {
		return nil
	}

	for taskID := range openTaskIDs {
		_ = removeTaskWorktreeDir(repoRoot, taskID)
		_ = removeTaskMetaFile(repoRoot, taskID)
		_ = deleteBranchIfExists(repoRoot, taskID)
		legacyBranch := "task-" + taskID
		if legacyBranch != taskID {
			_ = deleteBranchIfExists(repoRoot, legacyBranch)
		}
	}
	if _, err := runGit(repoRoot, "worktree", "prune"); err != nil {
		return fmt.Errorf("git worktree prune: %w", err)
	}
	return nil
}

func removeTaskWorktreeDir(repoRoot string, taskID string) error {
	abs := filepath.Join(repoRoot, ".governator", "worktrees", "task-"+taskID)
	exists, err := pathExists(abs)
	if err != nil || !exists {
		return err
	}
	// Try git-assisted removal first to keep metadata clean.
	_, _ = runGit(repoRoot, "worktree", "remove", "--force", abs)
	if err := os.RemoveAll(abs); err != nil {
		return fmt.Errorf("remove worktree dir %s: %w", abs, err)
	}
	return nil
}

func removeTaskMetaFile(repoRoot string, taskID string) error {
	metaPath := filepath.Join(repoRoot, ".governator", ".local-state", "meta", taskID+".json")
	if err := os.Remove(metaPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove worktree metadata %s: %w", metaPath, err)
	}
	return nil
}

func deleteBranchIfExists(repoRoot string, branch string) error {
	trimmed := strings.TrimSpace(branch)
	if trimmed == "" {
		return nil
	}
	if _, err := runGit(repoRoot, "show-ref", "--verify", "--quiet", "refs/heads/"+trimmed); err != nil {
		return nil
	}
	if _, err := runGit(repoRoot, "branch", "-D", trimmed); err != nil {
		return fmt.Errorf("delete branch %s: %w", trimmed, err)
	}
	return nil
}

func runGit(repoRoot string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}
