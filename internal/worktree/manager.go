// Package worktree manages task-scoped git worktrees for worker execution.
package worktree

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// localStateDirName is the relative path for transient governator state.
	localStateDirName = "_governator/_local-state"
	// metadataDirName holds metadata for workstreams.
	metadataDirName = "meta"
	// localStateDirMode defines permissions for the local state directory.
	localStateDirMode = 0o755
	// taskDirPrefix prefixes per-task worktree directories.
	taskDirPrefix = "task-"
)

// Manager coordinates creation and reuse of task worktrees.
type Manager struct {
	repoRoot      string
	localStateDir string
}

// Spec defines the inputs needed to locate or create a task worktree.
type Spec struct {
	WorkstreamID string
	Branch       string
	BaseBranch   string
}

// Result captures the resolved worktree location and whether it was reused.
type Result struct {
	Path         string
	RelativePath string
	Reused       bool
}

// NewManager constructs a Manager rooted at the provided repository root.
func NewManager(repoRoot string) (Manager, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return Manager{}, errors.New("repo root is required")
	}
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return Manager{}, fmt.Errorf("resolve absolute repo root %s: %w", repoRoot, err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return Manager{}, fmt.Errorf("stat repo root %s: %w", absRoot, err)
	}
	if !info.IsDir() {
		return Manager{}, fmt.Errorf("repo root %s is not a directory", absRoot)
	}
	localStateDir := filepath.Join(absRoot, localStateDirName)
	return Manager{repoRoot: absRoot, localStateDir: localStateDir}, nil
}

// WorktreePath returns the deterministic worktree path for a task attempt.
func (manager Manager) WorktreePath(workstreamID string) (string, error) {
	if strings.TrimSpace(manager.localStateDir) == "" {
		return "", errors.New("worktree manager is not initialized")
	}
	if err := validateWorkstreamID(workstreamID); err != nil {
		return "", err
	}
	dirName := taskDirName(workstreamID)
	return filepath.Join(manager.localStateDir, dirName), nil
}

// EnsureWorktree returns a task worktree path, creating it when needed.
// This method now integrates with branch lifecycle management to ensure
// task branches are created before worktrees.
func (manager Manager) EnsureWorktree(spec Spec) (Result, error) {
	if strings.TrimSpace(manager.repoRoot) == "" || strings.TrimSpace(manager.localStateDir) == "" {
		return Result{}, errors.New("worktree manager is not initialized")
	}
	if err := validateWorkstreamID(spec.WorkstreamID); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(spec.Branch) == "" {
		return Result{}, errors.New("branch is required")
	}

	if err := os.MkdirAll(manager.localStateDir, localStateDirMode); err != nil {
		return Result{}, fmt.Errorf("create worktree directory %s: %w", manager.localStateDir, err)
	}

	path, reused, err := manager.locateExistingWorktree(spec)
	if err != nil {
		return Result{}, err
	}

	if reused {
		if err := manager.ensureMetadata(spec.WorkstreamID, path, spec.Branch); err != nil {
			return Result{}, err
		}
		relative, err := repoRelativePath(manager.repoRoot, path)
		if err != nil {
			return Result{}, err
		}
		return Result{Path: path, RelativePath: relative, Reused: true}, nil
	}

	target, err := manager.WorktreePath(spec.WorkstreamID)
	if err != nil {
		return Result{}, err
	}

	if err := manager.addWorktree(target, spec); err != nil {
		return Result{}, err
	}

	if err := manager.ensureMetadata(spec.WorkstreamID, target, spec.Branch); err != nil {
		return Result{}, err
	}

	relative, err := repoRelativePath(manager.repoRoot, target)
	if err != nil {
		return Result{}, err
	}
	return Result{Path: target, RelativePath: relative, Reused: false}, nil
}

// addWorktree creates the git worktree for the given spec.
func (manager Manager) addWorktree(path string, spec Spec) error {
	branchExists, err := manager.branchExists(spec.Branch)
	if err != nil {
		return err
	}
	if branchExists {
		if _, err := manager.runGit("worktree", "add", path, spec.Branch); err != nil {
			return err
		}
		return nil
	}
	if strings.TrimSpace(spec.BaseBranch) == "" {
		return fmt.Errorf("branch %q does not exist; base branch is required", spec.Branch)
	}
	baseExists, err := manager.branchExists(spec.BaseBranch)
	if err != nil {
		return err
	}
	if !baseExists {
		return fmt.Errorf("base branch %q does not exist", spec.BaseBranch)
	}
	if _, err := manager.runGit("worktree", "add", "-b", spec.Branch, path, spec.BaseBranch); err != nil {
		return err
	}
	return nil
}

func (manager Manager) locateExistingWorktree(spec Spec) (string, bool, error) {
	if meta, ok, err := manager.readMetadata(spec.WorkstreamID); err != nil {
		return "", false, err
	} else if ok && meta.WorktreeRelPath != "" {
		path := filepath.Join(manager.repoRoot, filepath.FromSlash(meta.WorktreeRelPath))
		if exists, err := pathExists(path); err != nil {
			return "", false, err
		} else if exists {
			if strings.TrimSpace(spec.Branch) != "" {
				if err := ensureIsWorktree(path, spec.Branch); err != nil {
					return "", false, err
				}
			}
			return path, true, nil
		}
	}

	canonical, err := manager.WorktreePath(spec.WorkstreamID)
	if err != nil {
		return "", false, err
	}
	if exists, err := pathExists(canonical); err != nil {
		return "", false, err
	} else if exists {
		if strings.TrimSpace(spec.Branch) != "" {
			if err := ensureIsWorktree(canonical, spec.Branch); err != nil {
				return "", false, err
			}
		}
		return canonical, true, nil
	}
	return "", false, nil
}

// ExistingWorktreePath returns the actual worktree path for a workstream when present.
func (manager Manager) ExistingWorktreePath(workstreamID string) (string, bool, error) {
	if err := validateWorkstreamID(workstreamID); err != nil {
		return "", false, err
	}
	return manager.locateExistingWorktree(Spec{WorkstreamID: workstreamID})
}

func (manager Manager) ensureMetadata(workstreamID, path, branch string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("worktree path is required")
	}
	meta := metadata{
		Branch: branch,
	}
	relative, err := repoRelativePath(manager.repoRoot, path)
	if err != nil {
		return err
	}
	meta.WorktreeRelPath = relative
	return manager.writeMetadata(workstreamID, meta)
}

func (manager Manager) metadataDir() string {
	return filepath.Join(manager.localStateDir, metadataDirName)
}

func (manager Manager) metadataFilePath(workstreamID string) string {
	return filepath.Join(manager.metadataDir(), fmt.Sprintf("%s.json", workstreamID))
}

type metadata struct {
	WorktreeRelPath string `json:"worktree_rel_path"`
	Branch          string `json:"branch,omitempty"`
}

func (manager Manager) readMetadata(workstreamID string) (metadata, bool, error) {
	metaPath := manager.metadataFilePath(workstreamID)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return metadata{}, false, nil
		}
		return metadata{}, false, fmt.Errorf("read metadata %s: %w", metaPath, err)
	}
	var meta metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return metadata{}, false, fmt.Errorf("decode metadata %s: %w", metaPath, err)
	}
	return meta, true, nil
}

func (manager Manager) writeMetadata(workstreamID string, meta metadata) error {
	dir := manager.metadataDir()
	if err := os.MkdirAll(dir, localStateDirMode); err != nil {
		return fmt.Errorf("create metadata directory %s: %w", dir, err)
	}
	metaPath := manager.metadataFilePath(workstreamID)
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encode metadata %s: %w", metaPath, err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	if err := os.WriteFile(metaPath, data, 0o644); err != nil {
		return fmt.Errorf("write metadata %s: %w", metaPath, err)
	}
	return nil
}

// branchExists reports whether a local branch exists in the repository.
func (manager Manager) branchExists(branch string) (bool, error) {
	if strings.TrimSpace(branch) == "" {
		return false, errors.New("branch is required")
	}
	_, err := manager.runGit("show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if err == nil {
		return true, nil
	}
	if isExitStatus(err, 1) {
		return false, nil
	}
	return false, err
}

// ensureIsWorktree validates the path is a git worktree on the expected branch.
func ensureIsWorktree(path string, branch string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat worktree path %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("worktree path %s is not a directory", path)
	}
	if err := verifyInsideWorktree(path); err != nil {
		return err
	}
	currentBranch, err := currentBranch(path)
	if err != nil {
		return err
	}
	if currentBranch != branch {
		return fmt.Errorf("worktree at %s is on branch %q, expected %q", path, currentBranch, branch)
	}
	return nil
}

// verifyInsideWorktree confirms the path is a git worktree.
func verifyInsideWorktree(path string) error {
	output, err := runGitWithDir(path, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return fmt.Errorf("verify worktree %s: %w", path, err)
	}
	if strings.TrimSpace(output) != "true" {
		return fmt.Errorf("path %s is not a git worktree", path)
	}
	return nil
}

// currentBranch resolves the active branch in the worktree.
func currentBranch(path string) (string, error) {
	output, err := runGitWithDir(path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("resolve worktree branch %s: %w", path, err)
	}
	return strings.TrimSpace(output), nil
}

// pathExists reports whether the path exists on disk.
func pathExists(path string) (bool, error) {
	if strings.TrimSpace(path) == "" {
		return false, errors.New("path is required")
	}
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("stat path %s: %w", path, err)
}

// taskDirName builds the worktree directory name for a workstream.
func taskDirName(workstreamID string) string {
	return fmt.Sprintf("%s%s", taskDirPrefix, workstreamID)
}

// validateWorkstreamID ensures the workstream id is safe for filesystem use.
func validateWorkstreamID(workstreamID string) error {
	if strings.TrimSpace(workstreamID) == "" {
		return errors.New("workstream id is required")
	}
	if strings.Contains(workstreamID, "/") || strings.Contains(workstreamID, "\\") {
		return fmt.Errorf("workstream id %q must not contain path separators", workstreamID)
	}
	if strings.Contains(workstreamID, "..") {
		return fmt.Errorf("workstream id %q must not contain '..'", workstreamID)
	}
	return nil
}

// repoRelativePath returns a repo-relative path using forward slashes.
func repoRelativePath(root string, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", fmt.Errorf("resolve relative path for %s: %w", path, err)
	}
	return filepath.ToSlash(rel), nil
}

// runGit executes a git command in the repo root.
func (manager Manager) runGit(args ...string) (string, error) {
	return runGitWithDir(manager.repoRoot, args...)
}

// runGitWithDir runs a git command in the provided directory.
func runGitWithDir(dir string, args ...string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", errors.New("git directory is required")
	}
	if len(args) == 0 {
		return "", errors.New("git arguments are required")
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// isExitStatus reports whether the error is an exec.ExitError with the given status.
func isExitStatus(err error, status int) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	return exitErr.ExitCode() == status
}
