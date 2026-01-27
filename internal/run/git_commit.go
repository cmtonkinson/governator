// Package run provides git helpers for deterministic Governator commits.
package run

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/roles"
	"github.com/cmtonkinson/governator/internal/worker"
)

const (
	gitChangesFileName    = "git-changes.txt"
	commitMessageFileName = "commit-message.txt"
	stdoutLogFileName     = "stdout.log"
	commitLogCharLimit    = 8000
)

// finalizeStageSuccess captures git status and creates a Governator-owned commit.
func finalizeStageSuccess(worktreePath string, workerStateDir string, task index.Task, stage roles.Stage) (worker.IngestResult, error) {
	if strings.TrimSpace(worktreePath) == "" {
		return worker.IngestResult{}, errors.New("worktree path is required")
	}
	if strings.TrimSpace(workerStateDir) == "" {
		return worker.IngestResult{}, errors.New("worker state dir is required")
	}
	if strings.TrimSpace(task.ID) == "" {
		return worker.IngestResult{}, errors.New("task id is required")
	}
	if !stage.Valid() {
		return worker.IngestResult{}, fmt.Errorf("invalid stage %q", stage)
	}
	if err := os.MkdirAll(workerStateDir, 0o755); err != nil {
		return worker.IngestResult{}, fmt.Errorf("create worker state dir %s: %w", workerStateDir, err)
	}

	statusOutput, err := runGitOutput(worktreePath, "status", "--untracked-files=all")
	if err != nil {
		return worker.IngestResult{}, err
	}
	if err := os.WriteFile(filepath.Join(workerStateDir, gitChangesFileName), []byte(statusOutput), 0o644); err != nil {
		return worker.IngestResult{}, fmt.Errorf("write git status: %w", err)
	}

	hasChanges, err := worktreeHasChanges(worktreePath)
	if err != nil {
		return worker.IngestResult{}, err
	}
	if hasChanges {
		if err := runGit(worktreePath, "add", "-A"); err != nil {
			return worker.IngestResult{}, err
		}
		if err := commitWorktree(worktreePath, workerStateDir, task, stage); err != nil {
			return worker.IngestResult{}, err
		}
	}

	hasCommit, err := worktreeHasCommit(worktreePath)
	if err != nil {
		return worker.IngestResult{}, err
	}
	return worker.IngestResult{
		Success:   true,
		NewState:  stageToSuccessState(stage),
		HasCommit: hasCommit,
	}, nil
}

// stageToSuccessState maps a stage to its corresponding task success state.
func stageToSuccessState(stage roles.Stage) index.TaskState {
	switch stage {
	case roles.StageWork:
		return index.TaskStateImplemented
	case roles.StageTest:
		return index.TaskStateTested
	case roles.StageReview:
		return index.TaskStateReviewed
	case roles.StageResolve:
		return index.TaskStateResolved
	default:
		return index.TaskStateBlocked
	}
}

// worktreeHasChanges returns true when there are staged, modified, or untracked files.
func worktreeHasChanges(worktreePath string) (bool, error) {
	output, err := runGitOutput(worktreePath, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) != "", nil
}

// worktreeHasCommit verifies that HEAD resolves to a commit in the worktree.
func worktreeHasCommit(worktreePath string) (bool, error) {
	output, err := runGitOutput(worktreePath, "rev-parse", "--verify", "HEAD")
	if err != nil {
		// A missing HEAD is treated as no commit, not a hard failure.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return false, nil
		}
		return false, err
	}
	return strings.TrimSpace(output) != "", nil
}

// commitWorktree builds the Governator commit message and performs the commit.
func commitWorktree(worktreePath string, workerStateDir string, task index.Task, stage roles.Stage) error {
	message, err := buildCommitMessage(workerStateDir, task, stage)
	if err != nil {
		return err
	}
	messagePath := filepath.Join(workerStateDir, commitMessageFileName)
	if err := os.WriteFile(messagePath, []byte(message), 0o644); err != nil {
		return fmt.Errorf("write commit message: %w", err)
	}
	return runGitWithEnv(worktreePath, []string{
		"GIT_AUTHOR_NAME=Governator CLI",
		"GIT_AUTHOR_EMAIL=governator@localhost",
		"GIT_COMMITTER_NAME=Governator CLI",
		"GIT_COMMITTER_EMAIL=governator@localhost",
	}, "commit", "-F", messagePath)
}

// buildCommitMessage creates a message of the form "[state] Title\n\n<stdout.log>".
func buildCommitMessage(workerStateDir string, task index.Task, stage roles.Stage) (string, error) {
	statusLabel := strings.ToLower(string(stageToSuccessState(stage)))
	title := strings.TrimSpace(task.Title)
	if title == "" {
		title = task.ID
	}
	subject := fmt.Sprintf("[%s] %s", statusLabel, title)

	stdoutPath := filepath.Join(workerStateDir, stdoutLogFileName)
	body, err := readLogWithLimit(stdoutPath, commitLogCharLimit)
	if err != nil {
		// Missing logs should not prevent commits; record the failure explicitly.
		body = fmt.Sprintf("stdout log unavailable: %v\n", err)
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return subject + "\n", nil
	}
	return subject + "\n\n" + body + "\n", nil
}

// readLogWithLimit reads up to the provided character limit from a log file.
func readLogWithLimit(path string, limit int) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("log path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if limit > 0 && len(data) > limit {
		data = data[:limit]
	}
	return string(data), nil
}

// runGit executes a git command and returns a formatted error on failure.
func runGit(worktreePath string, args ...string) error {
	_, err := runGitBytes(worktreePath, nil, args...)
	return err
}

// runGitOutput executes git and returns stdout on success.
func runGitOutput(worktreePath string, args ...string) (string, error) {
	output, err := runGitBytes(worktreePath, nil, args...)
	return string(output), err
}

// runGitWithEnv executes git with additional environment variables.
func runGitWithEnv(worktreePath string, env []string, args ...string) error {
	_, err := runGitBytes(worktreePath, env, args...)
	return err
}

// runGitBytes runs git and returns stdout bytes, including stderr in the error message.
func runGitBytes(worktreePath string, env []string, args ...string) ([]byte, error) {
	if strings.TrimSpace(worktreePath) == "" {
		return nil, errors.New("worktree path is required")
	}
	if len(args) == 0 {
		return nil, errors.New("git arguments are required")
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = worktreePath
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf(
			"git %s failed: %w: %s",
			strings.Join(args, " "),
			err,
			strings.TrimSpace(stderr.String()),
		)
	}
	return stdout.Bytes(), nil
}
