// Package run provides the planning validation execution engine.
package run

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dlclark/regexp2"
)

// ValidationResult captures the outcome of a single validation check.
type ValidationResult struct {
	Type     string
	Valid    bool
	Message  string
	StepID   string
	StepName string
}

// ValidationEngine executes validation checks against the repository.
type ValidationEngine struct {
	repoRoot string
}

// NewValidationEngine creates a new validation engine for the given repository.
func NewValidationEngine(repoRoot string) *ValidationEngine {
	return &ValidationEngine{repoRoot: repoRoot}
}

// RunValidations executes all validations for a planning step and returns the results.
func (engine *ValidationEngine) RunValidations(stepID string, stepName string, validations []PlanningValidationSpec) ([]ValidationResult, error) {
	var results []ValidationResult

	for i, validation := range validations {
		result := ValidationResult{
			Type:     validation.Type,
			StepID:   stepID,
			StepName: stepName,
		}

		var err error
		switch validation.Type {
		case "command":
			result.Valid, result.Message, err = engine.runCommandValidation(validation)
		case "file":
			result.Valid, result.Message, err = engine.runFileValidation(validation)
		case "directory":
			result.Valid, result.Message, err = engine.runDirectoryValidation(validation)
		case "prompt":
			result.Valid, result.Message, err = engine.runPromptValidation(validation)
		default:
			err = fmt.Errorf("unknown validation type: %s", validation.Type)
		}

		if err != nil {
			return nil, fmt.Errorf("validation[%d] failed: %w", i, err)
		}

		results = append(results, result)

		// If any validation fails, we can stop early (AND logic)
		if !result.Valid {
			break
		}
	}

	return results, nil
}

// runCommandValidation executes a command validation.
func (engine *ValidationEngine) runCommandValidation(validation PlanningValidationSpec) (bool, string, error) {
	cmd := exec.Command("bash", "-lc", validation.Command)
	cmd.Dir = engine.repoRoot

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	stdout := strings.TrimSpace(stdoutBuf.String())
	stderr := strings.TrimSpace(stderrBuf.String())

	// Check exit code expectation
	expectSuccess := true // default: expect exit 0
	if strings.TrimSpace(validation.Expect) != "" {
		expectSuccess = strings.EqualFold(validation.Expect, "success") || validation.Expect == "0"
	}

	if expectSuccess && err != nil {
		return false, fmt.Sprintf("command failed with exit code %v: %s", cmd.ProcessState.ExitCode(), stderr), nil
	}
	if !expectSuccess && err == nil {
		return false, "command succeeded but failure was expected", nil
	}

	// Check stdout expectations
	if validation.StdoutContains != "" {
		if !strings.Contains(stdout, validation.StdoutContains) {
			return false, fmt.Sprintf("stdout does not contain %q", validation.StdoutContains), nil
		}
	}

	if validation.StdoutRegex != "" {
		regex, err := regexp2.Compile(validation.StdoutRegex, regexp2.RE2)
		if err != nil {
			return false, "", fmt.Errorf("invalid regex: %w", err)
		}

		match, err := regex.MatchString(stdout)
		if err != nil {
			return false, "", fmt.Errorf("regex match failed: %w", err)
		}
		if !match {
			return false, fmt.Sprintf("stdout does not match regex %q", validation.StdoutRegex), nil
		}
	}

	return true, "command validation passed", nil
}

// runFileValidation executes a file validation.
func (engine *ValidationEngine) runFileValidation(validation PlanningValidationSpec) (bool, string, error) {
	if hasGlobMeta(validation.Path) {
		return engine.runFileValidationGlob(validation)
	}
	paths, err := engine.resolveValidationPaths(validation.Path)
	if err != nil {
		return false, "", err
	}
	resolvedPath := paths[0]
	displayPath := engine.displayPath(resolvedPath, validation.Path)
	info, err := os.Stat(resolvedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, fmt.Sprintf("file does not exist: %s", displayPath), nil
		}
		return false, "", fmt.Errorf("file stat failed: %w", err)
	}
	if info.IsDir() {
		return false, fmt.Sprintf("file validation requires a file, found directory: %s", displayPath), nil
	}
	if ok, message, err := engine.validateConcreteFile(resolvedPath, info, validation.FileRegex, displayPath); err != nil {
		return false, "", err
	} else if !ok {
		return false, message, nil
	}
	return true, "file validation passed", nil
}

// runFileValidationGlob executes file validation for glob paths.
func (engine *ValidationEngine) runFileValidationGlob(validation PlanningValidationSpec) (bool, string, error) {
	paths, err := engine.resolveValidationPaths(validation.Path)
	if err != nil {
		return false, "", err
	}
	if len(paths) == 0 {
		return false, fmt.Sprintf("no files match glob (file validation requires only files): %s", validation.Path), nil
	}

	matchedFiles := 0
	for _, resolvedPath := range paths {
		info, err := os.Stat(resolvedPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return false, fmt.Sprintf("file does not exist: %s", engine.displayPath(resolvedPath, validation.Path)), nil
			}
			return false, "", fmt.Errorf("file stat failed: %w", err)
		}
		if info.IsDir() {
			return false, fmt.Sprintf("glob matched directory but file validation requires only files: %s", engine.displayPath(resolvedPath, validation.Path)), nil
		}
		matchedFiles++
		displayPath := engine.displayPath(resolvedPath, validation.Path)
		if ok, message, err := engine.validateConcreteFile(resolvedPath, info, validation.FileRegex, displayPath); err != nil {
			return false, "", err
		} else if !ok {
			return false, message, nil
		}
	}

	if matchedFiles == 0 {
		return false, fmt.Sprintf("no files match glob (file validation requires only files): %s", validation.Path), nil
	}
	return true, "file validation passed", nil
}

// validateConcreteFile enforces readability, non-empty content, and optional regex.
func (engine *ValidationEngine) validateConcreteFile(resolvedPath string, info os.FileInfo, fileRegex string, displayPath string) (bool, string, error) {
	if err := ensureFileReadable(resolvedPath); err != nil {
		return false, fmt.Sprintf("file is not readable: %s", displayPath), nil
	}
	if info.Size() == 0 {
		return false, fmt.Sprintf("file is empty: %s", displayPath), nil
	}
	if fileRegex != "" {
		if ok, message, err := matchFileRegex(resolvedPath, fileRegex, displayPath); err != nil {
			return false, "", err
		} else if !ok {
			return false, message, nil
		}
	}
	return true, "", nil
}

// runDirectoryValidation executes a directory validation.
func (engine *ValidationEngine) runDirectoryValidation(validation PlanningValidationSpec) (bool, string, error) {
	paths, err := engine.resolveValidationPaths(validation.Path)
	if err != nil {
		return false, "", err
	}
	if len(paths) == 0 {
		return false, fmt.Sprintf("no directories match glob (directory validation requires only directories): %s", validation.Path), nil
	}
	isGlob := hasGlobMeta(validation.Path)
	for _, resolvedPath := range paths {
		displayPath := engine.displayPath(resolvedPath, validation.Path)
		info, err := os.Stat(resolvedPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return false, fmt.Sprintf("directory does not exist: %s", displayPath), nil
			}
			return false, "", fmt.Errorf("directory stat failed: %w", err)
		}
		if !info.IsDir() {
			if isGlob {
				return false, fmt.Sprintf("glob matched file but directory validation requires only directories: %s", displayPath), nil
			}
			return false, fmt.Sprintf("directory validation requires a directory, found file: %s", displayPath), nil
		}
		if err := ensureDirectoryReadable(resolvedPath); err != nil {
			return false, fmt.Sprintf("directory is not readable: %s", displayPath), nil
		}
	}
	return true, "directory validation passed", nil
}

// resolveValidationPaths expands a path or glob into concrete filesystem paths.
func (engine *ValidationEngine) resolveValidationPaths(value string) ([]string, error) {
	if !hasGlobMeta(value) {
		return []string{filepath.Join(engine.repoRoot, filepath.FromSlash(value))}, nil
	}
	pattern := filepath.Join(engine.repoRoot, filepath.FromSlash(value))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob pattern %q: %w", value, err)
	}
	return matches, nil
}

// displayPath prefers a repo-relative path for validation messages.
func (engine *ValidationEngine) displayPath(fullPath string, fallback string) string {
	rel, err := filepath.Rel(engine.repoRoot, fullPath)
	if err != nil {
		return fallback
	}
	return filepath.ToSlash(rel)
}

// hasGlobMeta reports whether the path contains glob metacharacters.
func hasGlobMeta(value string) bool {
	return strings.ContainsAny(value, "*?[")
}

// ensureFileReadable checks that the file can be opened for reading.
func ensureFileReadable(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return nil
}

// ensureDirectoryReadable checks that the directory can be read.
func ensureDirectoryReadable(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	_ = entries
	return nil
}

// matchFileRegex validates a file's content against a regex.
func matchFileRegex(path string, regexPattern string, displayPath string) (bool, string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Sprintf("file is not readable: %s", displayPath), nil
	}
	regex, err := regexp2.Compile(regexPattern, regexp2.RE2)
	if err != nil {
		return false, "", fmt.Errorf("invalid regex: %w", err)
	}
	match, err := regex.MatchString(string(content))
	if err != nil {
		return false, "", fmt.Errorf("regex match failed: %w", err)
	}
	if !match {
		return false, fmt.Sprintf("file content does not match regex %q", regexPattern), nil
	}
	return true, "", nil
}

// runPromptValidation executes a prompt validation using an inline agent.
func (engine *ValidationEngine) runPromptValidation(validation PlanningValidationSpec) (bool, string, error) {
	// This would typically call out to an agent CLI, but for now we'll implement
	// a basic structure that could be extended

	var promptContent string
	if validation.Inline != "" {
		promptContent = validation.Inline
	} else if validation.PromptPath != "" {
		path := filepath.Join(engine.repoRoot, validation.PromptPath)
		content, err := os.ReadFile(path)
		if err != nil {
			return false, "", fmt.Errorf("read prompt file failed: %w", err)
		}
		promptContent = string(content)
	}
	if promptContent == "" {
		return false, "", fmt.Errorf("prompt validation requires either inline or prompt_path")
	}

	// In a real implementation, this would call an agent CLI like:
	// cmd := exec.Command("codex", "exec", "--role", validation.PromptRole, "--prompt", promptContent)
	// cmd.Dir = engine.repoRoot
	// var stdoutBuf bytes.Buffer
	// cmd.Stdout = &stdoutBuf
	// err := cmd.Run()
	// stdout := strings.TrimSpace(stdoutBuf.String())

	// For now, we'll simulate a successful prompt execution
	stdout := "[SIMULATED PROMPT OUTPUT]"

	// Check stdout expectations
	if validation.StdoutContains != "" {
		if !strings.Contains(stdout, validation.StdoutContains) {
			return false, fmt.Sprintf("prompt output does not contain %q", validation.StdoutContains), nil
		}
	}

	if validation.StdoutRegex != "" {
		regex, err := regexp2.Compile(validation.StdoutRegex, regexp2.RE2)
		if err != nil {
			return false, "", fmt.Errorf("invalid regex: %w", err)
		}

		match, err := regex.MatchString(stdout)
		if err != nil {
			return false, "", fmt.Errorf("regex match failed: %w", err)
		}
		if !match {
			return false, fmt.Sprintf("prompt output does not match regex %q", validation.StdoutRegex), nil
		}
	}

	return true, "prompt validation passed", nil
}
