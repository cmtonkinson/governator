// Package run contains tests for prompt validation behavior.
package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidationEnginePromptValidation(t *testing.T) {
	repoRoot := t.TempDir()
	engine := NewValidationEngine(repoRoot)

	tests := []struct {
		name       string
		validation PlanningValidationSpec
		setupFiles func(t *testing.T) // Optional setup for file-based tests
		wantValid  bool
		wantMsg    string
		wantErr    bool
	}{
		{
			name: "inline_prompt_succeeds",
			validation: PlanningValidationSpec{
				Type:   "prompt",
				Inline: "Analyze the codebase and report findings",
			},
			wantValid: true,
			wantMsg:   "prompt validation passed",
		},
		{
			name: "inline_prompt_with_stdout_contains_matches",
			validation: PlanningValidationSpec{
				Type:           "prompt",
				Inline:         "Check the system status",
				StdoutContains: "SIMULATED",
			},
			wantValid: true,
			wantMsg:   "prompt validation passed",
		},
		{
			name: "inline_prompt_with_stdout_contains_no_match_fails",
			validation: PlanningValidationSpec{
				Type:           "prompt",
				Inline:         "Check the system status",
				StdoutContains: "NONEXISTENT",
			},
			wantValid: false,
			wantMsg:   "does not contain",
		},
		{
			name: "inline_prompt_with_stdout_regex_matches",
			validation: PlanningValidationSpec{
				Type:        "prompt",
				Inline:      "Generate report",
				StdoutRegex: "SIMULATED.*OUTPUT",
			},
			wantValid: true,
			wantMsg:   "prompt validation passed",
		},
		{
			name: "inline_prompt_with_stdout_regex_no_match_fails",
			validation: PlanningValidationSpec{
				Type:        "prompt",
				Inline:      "Generate report",
				StdoutRegex: "\\d{10}",
			},
			wantValid: false,
			wantMsg:   "does not match regex",
		},
		{
			name: "file_based_prompt_succeeds",
			validation: PlanningValidationSpec{
				Type:       "prompt",
				PromptPath: "prompts/test-prompt.md",
			},
			setupFiles: func(t *testing.T) {
				promptPath := filepath.Join(repoRoot, "prompts", "test-prompt.md")
				writeTestFile(t, promptPath, "# Test Prompt\n\nAnalyze the architecture.")
			},
			wantValid: true,
			wantMsg:   "prompt validation passed",
		},
		{
			name: "file_based_prompt_with_stdout_validation",
			validation: PlanningValidationSpec{
				Type:           "prompt",
				PromptPath:     "prompts/analysis.md",
				StdoutContains: "SIMULATED",
			},
			setupFiles: func(t *testing.T) {
				promptPath := filepath.Join(repoRoot, "prompts", "analysis.md")
				writeTestFile(t, promptPath, "Perform analysis")
			},
			wantValid: true,
			wantMsg:   "prompt validation passed",
		},
		{
			name: "file_based_prompt_missing_file_errors",
			validation: PlanningValidationSpec{
				Type:       "prompt",
				PromptPath: "prompts/nonexistent.md",
			},
			wantValid: false,
			wantErr:   true,
		},
		{
			name: "neither_inline_nor_prompt_path_errors",
			validation: PlanningValidationSpec{
				Type: "prompt",
			},
			wantValid: false,
			wantErr:   true,
		},
		{
			name: "invalid_stdout_regex_errors",
			validation: PlanningValidationSpec{
				Type:        "prompt",
				Inline:      "Test prompt",
				StdoutRegex: "[invalid(regex",
			},
			wantValid: false,
			wantErr:   true,
		},
		{
			name: "empty_inline_prompt_errors",
			validation: PlanningValidationSpec{
				Type:   "prompt",
				Inline: "",
			},
			wantValid: false,
			wantErr:   true,
		},
		{
			name: "both_stdout_contains_and_regex_must_match",
			validation: PlanningValidationSpec{
				Type:           "prompt",
				Inline:         "Test",
				StdoutContains: "SIMULATED",
				StdoutRegex:    "OUTPUT",
			},
			wantValid: true,
			wantMsg:   "prompt validation passed",
		},
		{
			name: "both_checks_first_fails",
			validation: PlanningValidationSpec{
				Type:           "prompt",
				Inline:         "Test",
				StdoutContains: "MISSING",
				StdoutRegex:    "OUTPUT",
			},
			wantValid: false,
			wantMsg:   "does not contain",
		},
		{
			name: "both_checks_second_fails",
			validation: PlanningValidationSpec{
				Type:           "prompt",
				Inline:         "Test",
				StdoutContains: "SIMULATED",
				StdoutRegex:    "\\d{10}",
			},
			wantValid: false,
			wantMsg:   "does not match regex",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupFiles != nil {
				tt.setupFiles(t)
			}

			ok, msg, err := engine.runPromptValidation(tt.validation)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got none, valid=%v message=%q", ok, msg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if ok != tt.wantValid {
				t.Fatalf("valid = %v, want %v (message: %q)", ok, tt.wantValid, msg)
			}

			if tt.wantMsg != "" && !strings.Contains(msg, tt.wantMsg) {
				t.Fatalf("message = %q, want substring %q", msg, tt.wantMsg)
			}
		})
	}
}

// TestPromptValidationFileContent verifies file content is read correctly
func TestPromptValidationFileContent(t *testing.T) {
	repoRoot := t.TempDir()
	engine := NewValidationEngine(repoRoot)

	t.Run("prompt_file_with_complex_content", func(t *testing.T) {
		promptPath := filepath.Join(repoRoot, "prompts", "complex.md")
		complexContent := `# Complex Prompt

## Instructions

1. Analyze the architecture
2. Identify gaps
3. Propose solutions

## Context

The system uses microservices architecture with event-driven communication.
`
		writeTestFile(t, promptPath, complexContent)

		ok, msg, err := engine.runPromptValidation(PlanningValidationSpec{
			Type:       "prompt",
			PromptPath: "prompts/complex.md",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected validation to pass, got message: %q", msg)
		}
	})

	t.Run("prompt_file_with_special_characters", func(t *testing.T) {
		promptPath := filepath.Join(repoRoot, "prompts", "special.md")
		specialContent := "Prompt with special chars: $VAR, @user, #tag, 100% complete"
		writeTestFile(t, promptPath, specialContent)

		ok, msg, err := engine.runPromptValidation(PlanningValidationSpec{
			Type:       "prompt",
			PromptPath: "prompts/special.md",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected validation to pass, got message: %q", msg)
		}
	})

	t.Run("prompt_file_empty_errors", func(t *testing.T) {
		promptPath := filepath.Join(repoRoot, "prompts", "empty.md")
		writeTestFile(t, promptPath, "")

		_, _, err := engine.runPromptValidation(PlanningValidationSpec{
			Type:       "prompt",
			PromptPath: "prompts/empty.md",
		})
		// Empty file content results in an error because the prompt content is empty
		if err == nil {
			t.Fatalf("expected error for empty prompt file")
		}
		if !strings.Contains(err.Error(), "prompt validation requires") {
			t.Fatalf("expected 'prompt validation requires' error, got: %v", err)
		}
	})
}

// TestPromptValidationPathResolution tests path handling
func TestPromptValidationPathResolution(t *testing.T) {
	repoRoot := t.TempDir()
	engine := NewValidationEngine(repoRoot)

	t.Run("nested_directory_structure", func(t *testing.T) {
		promptPath := filepath.Join(repoRoot, "deep", "nested", "path", "prompt.md")
		writeTestFile(t, promptPath, "Deep prompt")

		ok, msg, err := engine.runPromptValidation(PlanningValidationSpec{
			Type:       "prompt",
			PromptPath: "deep/nested/path/prompt.md",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected validation to pass, got message: %q", msg)
		}
	})

	t.Run("prompt_path_with_spaces", func(t *testing.T) {
		promptPath := filepath.Join(repoRoot, "prompts with spaces", "my prompt.md")
		writeTestFile(t, promptPath, "Prompt content")

		ok, msg, err := engine.runPromptValidation(PlanningValidationSpec{
			Type:       "prompt",
			PromptPath: "prompts with spaces/my prompt.md",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected validation to pass, got message: %q", msg)
		}
	})

	t.Run("unreadable_prompt_file", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("skipping permission test when running as root")
		}

		promptPath := filepath.Join(repoRoot, "prompts", "unreadable.md")
		writeTestFile(t, promptPath, "content")

		// Make file unreadable
		if err := os.Chmod(promptPath, 0000); err != nil {
			t.Fatalf("chmod failed: %v", err)
		}
		defer os.Chmod(promptPath, 0644) // Restore for cleanup

		_, _, err := engine.runPromptValidation(PlanningValidationSpec{
			Type:       "prompt",
			PromptPath: "prompts/unreadable.md",
		})
		if err == nil {
			t.Fatalf("expected error for unreadable file")
		}
	})
}

// TestPromptValidationSimulatedOutput validates the simulated behavior
func TestPromptValidationSimulatedOutput(t *testing.T) {
	repoRoot := t.TempDir()
	engine := NewValidationEngine(repoRoot)

	t.Run("simulated_output_contains_expected_string", func(t *testing.T) {
		// The current implementation returns "[SIMULATED PROMPT OUTPUT]"
		ok, _, err := engine.runPromptValidation(PlanningValidationSpec{
			Type:           "prompt",
			Inline:         "test",
			StdoutContains: "[SIMULATED PROMPT OUTPUT]",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected simulated output to contain marker string")
		}
	})

	t.Run("simulated_output_matches_regex", func(t *testing.T) {
		ok, _, err := engine.runPromptValidation(PlanningValidationSpec{
			Type:        "prompt",
			Inline:      "test",
			StdoutRegex: "\\[SIMULATED.*OUTPUT\\]",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected simulated output to match regex")
		}
	})
}
