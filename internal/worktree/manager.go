// Package worktree manages task-scoped git worktrees for worker execution.
package worktree

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// localStateDirName is the relative path for transient governator state.
	localStateDirName = "_governator/_local_state"
	// worktreesDirName is the directory name that holds task worktrees.
	worktreesDirName = "worktrees"
	// worktreesDirMode defines permissions for the worktrees directory.
	worktreesDirMode = 0o755
)

// Manager coordinates creation and reuse of task worktrees.
type Manager struct {
	repoRoot    string
	worktreeDir string
}

// Spec defines the inputs needed to locate or create a task worktree.
type Spec struct {
	TaskID     string
	Attempt    int
	Branch     string
	BaseBranch string
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
	worktreeDir := filepath.Join(absRoot, localStateDirName, worktreesDirName)
	return Manager{repoRoot: absRoot, worktreeDir: worktreeDir}, nil
}

// WorktreePath returns the deterministic worktree path for a task attempt.
func (manager Manager) WorktreePath(taskID string, attempt int) (string, error) {
	if strings.TrimSpace(manager.worktreeDir) == "" {
		return "", errors.New("worktree manager is not initialized")
	}
	if err := validateTaskID(taskID); err != nil {
		return "", err
	}
	if attempt < 1 {
		return "", errors.New("attempt must be positive")
	}
	dirName := worktreeDirName(taskID, attempt)
	return filepath.Join(manager.worktreeDir, dirName), nil
}

// EnsureWorktree returns a task worktree path, creating it when needed.
// This method now integrates with branch lifecycle management to ensure
// task branches are created before worktrees.
func (manager Manager) EnsureWorktree(spec Spec) (Result, error) {
	if strings.TrimSpace(manager.repoRoot) == "" || strings.TrimSpace(manager.worktreeDir) == "" {
		return Result{}, errors.New("worktree manager is not initialized")
	}
	if err := validateTaskID(spec.TaskID); err != nil {
		return Result{}, err
	}
	if spec.Attempt < 1 {
		return Result{}, errors.New("attempt must be positive")
	}
	if strings.TrimSpace(spec.Branch) == "" {
		return Result{}, errors.New("branch is required")
	}

	path, err := manager.WorktreePath(spec.TaskID, spec.Attempt)
	if err != nil {
		return Result{}, err
	}

	exists, err := pathExists(path)
	if err != nil {
		return Result{}, err
	}
	if exists {
		if err := ensureIsWorktree(path, spec.Branch); err != nil {
			return Result{}, err
		}
		relative, err := repoRelativePath(manager.repoRoot, path)
		if err != nil {
			return Result{}, err
		}
		return Result{Path: path, RelativePath: relative, Reused: true}, nil
	}

	if err := os.MkdirAll(manager.worktreeDir, worktreesDirMode); err != nil {
		return Result{}, fmt.Errorf("create worktree directory %s: %w", manager.worktreeDir, err)
	}

	if err := manager.addWorktree(path, spec); err != nil {
		return Result{}, err
	}

	relative, err := repoRelativePath(manager.repoRoot, path)
	if err != nil {
		return Result{}, err
	}
	return Result{Path: path, RelativePath: relative, Reused: false}, nil
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

// worktreeDirName builds the worktree directory name for a task attempt.
func worktreeDirName(taskID string, attempt int) string {
	if attempt <= 1 {
		return taskID
	}
	return fmt.Sprintf("%s-attempt-%d", taskID, attempt)
}

// validateTaskID ensures the task id is safe for filesystem use.
func validateTaskID(taskID string) error {
	if strings.TrimSpace(taskID) == "" {
		return errors.New("task id is required")
	}
	if strings.Contains(taskID, "/") || strings.Contains(taskID, "\\") {
		return fmt.Errorf("task id %q must not contain path separators", taskID)
	}
	if strings.Contains(taskID, "..") {
		return fmt.Errorf("task id %q must not contain '..'", taskID)
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
