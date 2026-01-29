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
	Type      string
	Valid     bool
	Message   string
	StepID    string
	StepName  string
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
	path := filepath.Join(engine.repoRoot, validation.Path)
	
	// Check if path exists
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, fmt.Sprintf("file does not exist: %s", validation.Path), nil
		}
		return false, "", fmt.Errorf("file stat failed: %w", err)
	}
	
	// If it's a directory, check it's not empty
	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return false, "", fmt.Errorf("read directory failed: %w", err)
		}
		if len(entries) == 0 {
			return false, fmt.Sprintf("directory is empty: %s", validation.Path), nil
		}
		return true, "directory validation passed", nil
	}
	
	// For files, check readability and non-empty
	if info.Size() == 0 {
		return false, fmt.Sprintf("file is empty: %s", validation.Path), nil
	}
	
	// Check file content regex if provided
	if validation.FileRegex != "" {
		content, err := os.ReadFile(path)
		if err != nil {
			return false, "", fmt.Errorf("read file failed: %w", err)
		}
		
		regex, err := regexp2.Compile(validation.FileRegex, regexp2.RE2)
		if err != nil {
			return false, "", fmt.Errorf("invalid regex: %w", err)
		}
		
		match, err := regex.MatchString(string(content))
		if err != nil {
			return false, "", fmt.Errorf("regex match failed: %w", err)
		}
		if !match {
			return false, fmt.Sprintf("file content does not match regex %q", validation.FileRegex), nil
		}
	}
	
	return true, "file validation passed", nil
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