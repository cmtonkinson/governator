// Package run contains tests for command validation behavior.
package run

import (
	"strings"
	"testing"
)

func TestValidationEngineCommandValidation(t *testing.T) {
	tests := []struct {
		name       string
		validation PlanningValidationSpec
		wantValid  bool
		wantMsg    string // substring check
		wantErr    bool
	}{
		{
			name: "command_exit_0_expect_success",
			validation: PlanningValidationSpec{
				Type:    "command",
				Command: "echo hello",
				Expect:  "success",
			},
			wantValid: true,
			wantMsg:   "command validation passed",
		},
		{
			name: "command_exit_0_no_expect_defaults_to_success",
			validation: PlanningValidationSpec{
				Type:    "command",
				Command: "echo hello",
			},
			wantValid: true,
			wantMsg:   "command validation passed",
		},
		{
			name: "command_with_stdout_contains_matches",
			validation: PlanningValidationSpec{
				Type:           "command",
				Command:        "echo hello world",
				StdoutContains: "hello",
			},
			wantValid: true,
			wantMsg:   "command validation passed",
		},
		{
			name: "command_with_stdout_regex_matches",
			validation: PlanningValidationSpec{
				Type:        "command",
				Command:     "echo build-2024-01-15",
				StdoutRegex: "build-\\d{4}",
			},
			wantValid: true,
			wantMsg:   "command validation passed",
		},
		{
			name: "command_with_both_stdout_contains_and_regex_matches",
			validation: PlanningValidationSpec{
				Type:           "command",
				Command:        "echo build-2024-01-15",
				StdoutContains: "build",
				StdoutRegex:    "\\d{4}",
			},
			wantValid: true,
			wantMsg:   "command validation passed",
		},
		{
			name: "command_exit_1_expect_success_fails",
			validation: PlanningValidationSpec{
				Type:    "command",
				Command: "exit 1",
				Expect:  "success",
			},
			wantValid: false,
			wantMsg:   "exit code 1",
		},
		{
			name: "command_exit_0_expect_failure_fails",
			validation: PlanningValidationSpec{
				Type:    "command",
				Command: "echo ok",
				Expect:  "failure",
			},
			wantValid: false,
			wantMsg:   "succeeded but failure was expected",
		},
		{
			name: "command_stdout_contains_no_match_fails",
			validation: PlanningValidationSpec{
				Type:           "command",
				Command:        "echo foo",
				StdoutContains: "bar",
			},
			wantValid: false,
			wantMsg:   "does not contain",
		},
		{
			name: "command_stdout_regex_no_match_fails",
			validation: PlanningValidationSpec{
				Type:        "command",
				Command:     "echo hello",
				StdoutRegex: "\\d+",
			},
			wantValid: false,
			wantMsg:   "does not match regex",
		},
		{
			name: "command_invalid_regex_returns_error",
			validation: PlanningValidationSpec{
				Type:        "command",
				Command:     "echo test",
				StdoutRegex: "[invalid(regex",
			},
			wantValid: false,
			wantErr:   true,
		},
		{
			name: "command_nonexistent_binary_returns_error",
			validation: PlanningValidationSpec{
				Type:    "command",
				Command: "this_binary_does_not_exist_12345",
			},
			wantValid: false,
			wantErr:   false, // Command failure is not an error, just validation failure
			wantMsg:   "exit code",
		},
		{
			name: "command_exit_0_expect_0_succeeds",
			validation: PlanningValidationSpec{
				Type:    "command",
				Command: "echo test",
				Expect:  "0",
			},
			wantValid: true,
			wantMsg:   "command validation passed",
		},
		{
			name: "command_multiline_stdout",
			validation: PlanningValidationSpec{
				Type:           "command",
				Command:        "printf 'line1\\nline2\\nline3'",
				StdoutContains: "line2",
			},
			wantValid: true,
			wantMsg:   "command validation passed",
		},
		{
			name: "command_stderr_ignored_for_validation",
			validation: PlanningValidationSpec{
				Type:    "command",
				Command: "echo error >&2 && echo ok",
			},
			wantValid: true,
			wantMsg:   "command validation passed",
		},
		{
			name: "command_empty_output_no_contains_check",
			validation: PlanningValidationSpec{
				Type:    "command",
				Command: "true", // No output
			},
			wantValid: true,
			wantMsg:   "command validation passed",
		},
		{
			name: "command_empty_output_with_contains_check_fails",
			validation: PlanningValidationSpec{
				Type:           "command",
				Command:        "true",
				StdoutContains: "something",
			},
			wantValid: false,
			wantMsg:   "does not contain",
		},
	}

	repoRoot := t.TempDir()
	engine := NewValidationEngine(repoRoot)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, msg, err := engine.runCommandValidation(tt.validation)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got none, message: %q", msg)
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

// TestCommandValidationWithRepoContext verifies command runs in repo directory
func TestCommandValidationWithRepoContext(t *testing.T) {
	repoRoot := t.TempDir()
	engine := NewValidationEngine(repoRoot)

	// Create a test file in the repo
	testFile := "test-marker.txt"
	writeTestFile(t, repoRoot+"/"+testFile, "marker content")

	t.Run("command_executes_in_repo_directory", func(t *testing.T) {
		ok, msg, err := engine.runCommandValidation(PlanningValidationSpec{
			Type:           "command",
			Command:        "cat test-marker.txt",
			StdoutContains: "marker content",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected validation to pass, got message: %q", msg)
		}
	})

	t.Run("command_can_access_repo_files", func(t *testing.T) {
		ok, msg, err := engine.runCommandValidation(PlanningValidationSpec{
			Type:        "command",
			Command:     "ls test-marker.txt",
			StdoutRegex: "test-marker\\.txt",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected validation to pass, got message: %q", msg)
		}
	})
}

// TestCommandValidationExpectVariations tests different expect values
func TestCommandValidationExpectVariations(t *testing.T) {
	repoRoot := t.TempDir()
	engine := NewValidationEngine(repoRoot)

	tests := []struct {
		name      string
		command   string
		expect    string
		wantValid bool
	}{
		{"expect_success_lowercase", "true", "success", true},
		{"expect_success_uppercase", "true", "SUCCESS", true},
		{"expect_success_mixedcase", "true", "Success", true},
		{"expect_0_string", "true", "0", true},
		{"expect_failure_on_exit_0", "true", "failure", false},
		{"expect_failure_on_exit_1", "false", "failure", true},
		{"empty_expect_defaults_success", "true", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, _, err := engine.runCommandValidation(PlanningValidationSpec{
				Type:    "command",
				Command: tt.command,
				Expect:  tt.expect,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok != tt.wantValid {
				t.Fatalf("valid = %v, want %v", ok, tt.wantValid)
			}
		})
	}
}
